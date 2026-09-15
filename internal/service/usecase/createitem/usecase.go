// Package createitem — операция создания элемента. Одна операция = один пакет:
// зависимости видны в contracts.go, и разрастание не прячется в общем сервисе.
//
// ADR: docs/adr/0005-domennyy-sloy-vmesto-dto.md — сценарий работает с сущностями,
// зависимости объявлены в contracts.go.
package createitem

import (
	"context"
	"fmt"
	"time"

	"github.com/your-org/chrome_skill/internal/service/entity"
)

type UseCase interface {
	Execute(ctx context.Context, name string) (*entity.Item, error)
}

type useCase struct {
	items ItemWriter
	tx    Transaction
	now   func() time.Time
}

// New возвращает интерфейс, а реализацию не экспортирует: снаружи от операции
// нужен только вызов, а подменяют её в тестах через тот же интерфейс.
func New(items ItemWriter, tx Transaction, now func() time.Time) UseCase {
	if now == nil {
		now = time.Now
	}
	return &useCase{items: items, tx: tx, now: now}
}

func (u *useCase) Execute(ctx context.Context, name string) (*entity.Item, error) {
	item, err := entity.NewItem(name, u.now())
	if err != nil {
		return nil, err
	}

	// Границу транзакции объявляет операция, а не репозиторий: шаги внутри
	// WithTransaction атомарны, и второй шаг (запись в смежную таблицу,
	// публикация события) добавляется сюда, не меняя адаптеры.
	err = u.tx.WithTransaction(ctx, func(ctx context.Context) error {
		if err := u.items.Create(ctx, item); err != nil {
			return fmt.Errorf("сохранение элемента: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return item, nil
}
