// Package getitem — чтение элемента и списка.
package getitem

import (
	"context"

	"github.com/your-org/chrome_skill/internal/service/entity"
)

const defaultLimit = 20

type UseCase interface {
	Get(ctx context.Context, id entity.ItemID) (*entity.Item, error)
	List(ctx context.Context, limit int) ([]*entity.Item, error)
}

type useCase struct {
	items ItemReader
}

func New(items ItemReader) UseCase {
	return &useCase{items: items}
}

func (u *useCase) Get(ctx context.Context, id entity.ItemID) (*entity.Item, error) {
	return u.items.Get(ctx, id)
}

func (u *useCase) List(ctx context.Context, limit int) ([]*entity.Item, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	return u.items.List(ctx, limit)
}
