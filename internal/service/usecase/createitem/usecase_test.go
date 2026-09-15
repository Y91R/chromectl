// ADR: docs/adr/0008-testy-kak-specifikaciya.md — исходы операции задаются тестом
// до реализации, ожидания берутся из требования.
package createitem_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	mocks "github.com/your-org/chrome_skill/gen/mocks/service/usecase/createitem"
	domainerrors "github.com/your-org/chrome_skill/internal/service/errors"
	"github.com/your-org/chrome_skill/internal/service/usecase/createitem"
)

// transactionPassthrough выполняет функцию как есть: границу транзакции проверяют
// тесты репозитория, а здесь важна логика операции.
func transactionPassthrough(t *testing.T) *mocks.Transaction {
	t.Helper()

	tx := mocks.NewTransaction(t)
	tx.EXPECT().
		WithTransaction(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).
		Maybe()
	return tx
}

func TestExecute_CreatesItem(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	items := mocks.NewItemWriter(t)
	items.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()

	uc := createitem.New(items, transactionPassthrough(t), func() time.Time { return now })

	item, err := uc.Execute(context.Background(), "виджет")
	require.NoError(t, err)
	require.Equal(t, "виджет", item.Name())
	require.Equal(t, now, item.CreatedAt())
}

// Инвариант проверяется до похода в хранилище: пустое имя не должно доходить до БД.
func TestExecute_RejectsInvalidName(t *testing.T) {
	t.Parallel()

	items := mocks.NewItemWriter(t)
	uc := createitem.New(items, transactionPassthrough(t), time.Now)

	_, err := uc.Execute(context.Background(), "  ")

	_, ok := domainerrors.AsValidation(err)
	require.True(t, ok, "ожидалась ошибка валидации, получено: %v", err)
	items.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// Сбой записи обязан отменять транзакцию, а не фиксировать её.
func TestExecute_WriteFailureAbortsTransaction(t *testing.T) {
	t.Parallel()

	items := mocks.NewItemWriter(t)
	items.EXPECT().Create(mock.Anything, mock.Anything).
		Return(errors.New("база недоступна")).Once()

	var committed bool
	tx := mocks.NewTransaction(t)
	tx.EXPECT().WithTransaction(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			err := fn(ctx)
			committed = err == nil
			return err
		}).Once()

	uc := createitem.New(items, tx, time.Now)

	_, err := uc.Execute(context.Background(), "виджет")
	require.Error(t, err)
	require.False(t, committed, "транзакция не должна быть зафиксирована при сбое записи")
}

// Конфликт уникальности обязан дойти до транспорта конфликтом, а не 500.
func TestExecute_RepositoryConflict(t *testing.T) {
	t.Parallel()

	items := mocks.NewItemWriter(t)
	items.EXPECT().Create(mock.Anything, mock.Anything).
		Return(domainerrors.ErrConflict).Once()

	uc := createitem.New(items, transactionPassthrough(t), time.Now)

	_, err := uc.Execute(context.Background(), "виджет")
	require.True(t, domainerrors.IsConflict(err), "ошибка должна оставаться конфликтом: %v", err)
}
