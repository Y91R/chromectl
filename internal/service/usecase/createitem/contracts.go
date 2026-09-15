package createitem

import (
	"context"

	"github.com/your-org/chrome_skill/internal/service/entity"
)

// Интерфейсы объявлены потребителем и ровно той ширины, которая нужна операции:
// от репозитория здесь требуется только запись, и подменять в тесте приходится
// один метод, а не весь ports.ItemRepository.

type ItemWriter interface {
	Create(ctx context.Context, item *entity.Item) error
}

type Transaction interface {
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
