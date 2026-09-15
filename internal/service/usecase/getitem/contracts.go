package getitem

import (
	"context"

	"github.com/your-org/chrome_skill/internal/service/entity"
)

type ItemReader interface {
	Get(ctx context.Context, id entity.ItemID) (*entity.Item, error)
	List(ctx context.Context, limit int) ([]*entity.Item, error)
}
