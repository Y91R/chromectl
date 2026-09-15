// Package item — адаптер хранилища для элементов: переводит строки БД в доменные
// сущности и обратно. Наружу пакет отдаёт только доменные типы и доменные ошибки.
//
// ADR: docs/adr/0005-domennyy-sloy-vmesto-dto.md — репозиторий отдаёт сущности,
// а не строки БД, и переводит ошибки драйвера в доменные.
package item

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/your-org/chrome_skill/gen/db"
	"github.com/your-org/chrome_skill/internal/repository/txmanager"
	"github.com/your-org/chrome_skill/internal/service/entity"
	domainerrors "github.com/your-org/chrome_skill/internal/service/errors"
)

const uniqueViolation = "23505"

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) queries(ctx context.Context) *db.Queries {
	return db.New(txmanager.FromContext(ctx, r.pool))
}

func (r *Repository) Get(ctx context.Context, id entity.ItemID) (*entity.Item, error) {
	row, err := r.queries(ctx).GetItem(ctx, id.UUID())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("элемент %s: %w", id, domainerrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("чтение элемента: %w", err)
	}
	return toEntity(row), nil
}

func (r *Repository) List(ctx context.Context, limit int) ([]*entity.Item, error) {
	rows, err := r.queries(ctx).ListItems(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("чтение списка элементов: %w", err)
	}

	out := make([]*entity.Item, 0, len(rows))
	for _, row := range rows {
		out = append(out, toEntity(row))
	}
	return out, nil
}

func (r *Repository) Create(ctx context.Context, item *entity.Item) error {
	_, err := r.queries(ctx).CreateItem(ctx, db.CreateItemParams{
		ID:        item.ID().UUID(),
		Name:      item.Name(),
		CreatedAt: toTimestamptz(item.CreatedAt()),
	})

	// Нарушение уникальности — не сбой инфраструктуры, а конфликт состояния:
	// без перевода в доменную ошибку транспорт отдал бы 500 вместо 409.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return fmt.Errorf("элемент с именем %q уже есть: %w", item.Name(), domainerrors.ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("создание элемента: %w", err)
	}
	return nil
}
