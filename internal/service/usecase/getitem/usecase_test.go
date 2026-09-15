// ADR: docs/adr/0008-testy-kak-specifikaciya.md — исходы операции задаются тестом
// до реализации, ожидания берутся из контракта.
package getitem_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	mocks "github.com/your-org/chrome_skill/gen/mocks/service/usecase/getitem"
	"github.com/your-org/chrome_skill/internal/service/entity"
	domainerrors "github.com/your-org/chrome_skill/internal/service/errors"
	"github.com/your-org/chrome_skill/internal/service/usecase/getitem"
)

func TestGet_ReturnsItem(t *testing.T) {
	t.Parallel()

	id := entity.NewItemID()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	items := mocks.NewItemReader(t)
	items.EXPECT().Get(mock.Anything, id).Return(entity.RestoreItem(id, "виджет", now), nil).Once()

	item, err := getitem.New(items).Get(context.Background(), id)

	require.NoError(t, err)
	require.Equal(t, id, item.ID())
	require.Equal(t, "виджет", item.Name())
}

// Отсутствие элемента — ожидаемый исход, а не сбой: ошибка доходит до транспорта
// как есть, иначе он не сможет отличить 404 от 500.
func TestGet_NotFoundPropagates(t *testing.T) {
	t.Parallel()

	id := entity.NewItemID()
	items := mocks.NewItemReader(t)
	items.EXPECT().Get(mock.Anything, id).Return(nil, domainerrors.ErrNotFound).Once()

	_, err := getitem.New(items).Get(context.Background(), id)

	require.ErrorIs(t, err, domainerrors.ErrNotFound)
}

// Ожидание взято из контракта (`docs/api/openapi.yaml`, listItems → limit.default),
// а не из константы реализации: расхождение кода со спекой должно валить тест.
func TestList_AppliesLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		limit    int
		wantSent int
	}{
		{name: "ноль — по умолчанию из спеки", limit: 0, wantSent: 20},
		{name: "отрицательный — по умолчанию из спеки", limit: -5, wantSent: 20},
		{name: "заданный передаётся как есть", limit: 5, wantSent: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			items := mocks.NewItemReader(t)
			items.EXPECT().List(mock.Anything, tt.wantSent).Return(nil, nil).Once()

			_, err := getitem.New(items).List(context.Background(), tt.limit)

			require.NoError(t, err)
		})
	}
}
