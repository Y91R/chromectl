// Команда server поднимает публичный HTTP (API + статика SPA) и служебный порт
// с пробами в одном процессе.
//
// ADR: docs/adr/0007-ruchnoy-di-i-lifo-ostanovka.md — граф собирается вручную,
// остановка идёт в обратном порядке создания.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/your-org/chrome_skill/internal/app"
	"github.com/your-org/chrome_skill/internal/config"
	"github.com/your-org/chrome_skill/internal/httptransport"
	"github.com/your-org/chrome_skill/internal/server/debugserver"
	"github.com/your-org/chrome_skill/internal/server/httpserver"
)

func main() {
	if err := run(); err != nil {
		slog.Default().Error("сервис остановлен с ошибкой", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	application := app.New(cfg)
	log := application.Log

	create, err := application.CreateItem()
	if err != nil {
		return err
	}
	get, err := application.GetItem()
	if err != nil {
		return err
	}

	httpSrv, err := httpserver.New(httpserver.Config{
		Addr:        cfg.HTTPAddr(),
		CORSOrigins: cfg.CORSOrigins,
		StaticDir:   cfg.StaticDir,
	}, httptransport.New(create, get, application.Ready), log)
	if err != nil {
		return err
	}
	debugSrv := debugserver.New(cfg.DebugAddr(), application.Ready, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 2)
	go func() { errs <- httpSrv.Run() }()
	go func() { errs <- debugSrv.Run() }()

	// Ошибка старта возвращается наружу, а не только логируется: с нулевым кодом
	// выхода оркестратор считает занятый порт штатным завершением и не перезапускает.
	var runErr error
	select {
	case <-ctx.Done():
		log.Info("получен сигнал остановки")
		// Перехват снимается сразу: повторный сигнал должен убивать процесс,
		// а не ждать конца graceful shutdown.
		stop()
	case err := <-errs:
		if err != nil {
			log.Error("сервер завершился с ошибкой", slog.Any("error", err))
			runErr = err
		}
	}

	// Отдельный контекст на каждый узел: контекст сигнала уже отменён (на нём
	// остановка завершилась бы мгновенно), а один общий таймаут на всех съедали бы
	// зависшие запросы к API — служебному серверу и пулу доставался бы отменённый.
	stopNode := func(name string, fn func(context.Context) error) {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := fn(shutdownCtx); err != nil {
			log.Error("остановка узла не удалась", slog.String("node", name), slog.Any("error", err))
		}
	}

	stopNode("http", httpSrv.Shutdown)
	stopNode("debug", debugSrv.Shutdown)
	stopNode("app", func(ctx context.Context) error {
		application.Shutdown(ctx)
		return nil
	})

	if runErr != nil {
		return runErr
	}
	log.Info("сервис остановлен")
	return nil
}
