// Package txmanager задаёт границы транзакций. Use case говорит «эти шаги атомарны»,
// а репозитории сами достают из контекста транзакцию, если она открыта.
package txmanager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/your-org/chrome_skill/gen/db"
)

type ctxKey struct{}

type Manager struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func New(pool *pgxpool.Pool, log *slog.Logger) *Manager {
	return &Manager{pool: pool, log: log}
}

// WithTransaction открывает транзакцию, если её ещё нет. Вложенный вызов
// переиспользует существующую: два независимых коммита внутри одной операции
// нарушили бы её атомарность.
func (m *Manager) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(ctxKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}

	// Паника внутри fn не должна оставлять транзакцию открытой: соединение
	// не вернулось бы в пул, а взятые блокировки держались бы до его закрытия.
	// На штатных путях транзакция уже закрыта, и Rollback вернёт ErrTxClosed.
	settled := false
	defer func() {
		if settled {
			return
		}
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			m.log.Error("откат транзакции после паники не удался", slog.Any("error", rbErr))
		}
	}()

	if err := fn(context.WithValue(ctx, ctxKey{}, tx)); err != nil {
		settled = true
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			return fmt.Errorf("%w (откат: %v)", err, rbErr)
		}
		return err
	}
	settled = true
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("фиксация транзакции: %w", err)
	}
	return nil
}

// FromContext отдаёт транзакцию, если операция выполняется внутри неё, иначе пул.
// Благодаря этому репозиторий один и тот же и внутри транзакции, и вне её.
func FromContext(ctx context.Context, pool *pgxpool.Pool) db.DBTX {
	if tx, ok := ctx.Value(ctxKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}
