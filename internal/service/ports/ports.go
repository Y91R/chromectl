// Package ports — интерфейсы адаптеров внешнего мира глазами домена.
// Реализации живут в internal/repository, но форму им задаёт потребитель,
// а не наоборот.
package ports

import (
	"context"

	"github.com/your-org/chrome_skill/internal/service/entity"
)

type ItemRepository interface {
	Get(ctx context.Context, id entity.ItemID) (*entity.Item, error)
	List(ctx context.Context, limit int) ([]*entity.Item, error)
	Create(ctx context.Context, item *entity.Item) error
}

type Transaction interface {
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
