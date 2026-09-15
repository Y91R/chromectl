// Package app — единственное место, где собирается граф зависимостей.
// DI вручную: контейнер здесь не окупается, а компилятор проверяет граф лучше,
// чем любой резолвер в рантайме.
//
// ADR: docs/adr/0007-ruchnoy-di-i-lifo-ostanovka.md — почему без контейнера.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/your-org/chrome_skill/internal/config"
	"github.com/your-org/chrome_skill/internal/observability"
	itemrepo "github.com/your-org/chrome_skill/internal/repository/item"
	"github.com/your-org/chrome_skill/internal/repository/txmanager"
	"github.com/your-org/chrome_skill/internal/service/usecase/createitem"
	"github.com/your-org/chrome_skill/internal/service/usecase/getitem"
)

type App struct {
	Config *config.Config
	Log    *slog.Logger

	// Узел ленивый: команде, которой БД не нужна, незачем падать из-за неё
	// на старте.
	pool      func() (*pgxpool.Pool, error)
	shutdowns []func(context.Context) error
	stopped   bool
	mu        sync.Mutex
}

func New(cfg *config.Config) *App {
	a := &App{
		Config: cfg,
		Log:    observability.NewLogger(cfg.LogLevel, cfg.ServiceName),
	}
	a.pool = sync.OnceValues(a.newPool)
	return a
}

func (a *App) newPool() (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(a.Config.Database.URL)
	if err != nil {
		return nil, fmt.Errorf("разбор DATABASE_URL: %w", err)
	}
	poolCfg.MaxConns = a.Config.Database.MaxConns
	poolCfg.MaxConnLifetime = a.Config.Database.ConnMaxLifetime

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("подключение к БД: %w", err)
	}
	a.onShutdown(func(context.Context) error {
		pool.Close()
		return nil
	})
	return pool, nil
}

func (a *App) Pool() (*pgxpool.Pool, error) { return a.pool() }

func (a *App) CreateItem() (createitem.UseCase, error) {
	pool, err := a.Pool()
	if err != nil {
		return nil, err
	}
	return createitem.New(itemrepo.New(pool), txmanager.New(pool, a.Log), time.Now), nil
}

func (a *App) GetItem() (getitem.UseCase, error) {
	pool, err := a.Pool()
	if err != nil {
		return nil, err
	}
	return getitem.New(itemrepo.New(pool)), nil
}

// Ready — проба готовности: сервис принимает нагрузку, только если доступна БД.
func (a *App) Ready(ctx context.Context) error {
	pool, err := a.Pool()
	if err != nil {
		return err
	}
	return pool.Ping(ctx)
}

// onShutdown регистрирует остановку узла. Если Shutdown уже прошёл, узел
// создан «после закрытия» — гасим его сразу, иначе он утечёт: список остановок
// больше никто не обойдёт.
func (a *App) onShutdown(fn func(context.Context) error) {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		if err := fn(context.Background()); err != nil {
			a.Log.Error("ошибка при остановке запоздавшего узла", slog.Any("error", err))
		}
		return
	}
	a.shutdowns = append(a.shutdowns, fn)
	a.mu.Unlock()
}

// Shutdown закрывает ресурсы в обратном порядке создания: зависимый узел
// гасится раньше того, от чего он зависит.
func (a *App) Shutdown(ctx context.Context) {
	a.mu.Lock()
	fns := a.shutdowns
	a.shutdowns = nil
	a.stopped = true
	a.mu.Unlock()

	for i := len(fns) - 1; i >= 0; i-- {
		if err := fns[i](ctx); err != nil {
			a.Log.Error("ошибка при остановке", slog.Any("error", err))
		}
	}
}
