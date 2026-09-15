// Кейс на каждый код ответа из docs/api/openapi.yaml. Ожидания взяты из спеки, а не
// из хендлера, и каждый кейс был красным до реализации.
//
// ADR: docs/adr/0008-testy-kak-specifikaciya.md — тест как точка спецификации.
package httptransport_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	api "github.com/your-org/chrome_skill/gen/api"
	createmocks "github.com/your-org/chrome_skill/gen/mocks/service/usecase/createitem"
	getmocks "github.com/your-org/chrome_skill/gen/mocks/service/usecase/getitem"
	"github.com/your-org/chrome_skill/internal/httptransport"
	"github.com/your-org/chrome_skill/internal/service/entity"
	domainerrors "github.com/your-org/chrome_skill/internal/service/errors"
)

func ready(err error) httptransport.Ready {
	return func(context.Context) error { return err }
}

func TestGetHealth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		readyFn httptransport.Ready
		want    api.GetHealthResponseObject
	}{
		{name: "БД доступна", readyFn: ready(nil), want: api.GetHealth200JSONResponse{Status: "ok"}},
		{name: "БД недоступна", readyFn: ready(errors.New("connection refused")), want: api.GetHealth503JSONResponse{Status: "db down"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := httptransport.New(createmocks.NewUseCase(t), getmocks.NewUseCase(t), tt.readyFn)

			got, err := h.GetHealth(context.Background(), api.GetHealthRequestObject{})
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCreateItem_201(t *testing.T) {
	t.Parallel()

	// Время сущности намеренно в непустом офсете: наружу оно обязано уйти в UTC,
	// иначе клиенты получают разные ответы в зависимости от машины.
	now := time.Date(2026, 1, 2, 6, 4, 5, 0, time.FixedZone("MSK", 3*60*60))
	item := entity.RestoreItem(entity.NewItemID(), "виджет", now)

	create := createmocks.NewUseCase(t)
	create.EXPECT().Execute(mock.Anything, "виджет").Return(item, nil).Once()

	h := httptransport.New(create, getmocks.NewUseCase(t), ready(nil))

	got, err := h.CreateItem(context.Background(), api.CreateItemRequestObject{
		Body: &api.CreateItemJSONRequestBody{Name: "виджет"},
	})
	require.NoError(t, err)
	require.Equal(t, api.CreateItem201JSONResponse{
		Id:        item.ID().UUID(),
		Name:      "виджет",
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}, got)
}

// Доменная ошибка обязана превращаться в код из спеки, а не в 500. Ошибки приходят
// такими же обёрнутыми, как в проде (репозиторий → операция), а в тело ответа
// внутренние формулировки слоёв попадать не должны.
func TestCreateItem_ErrorCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want api.CreateItemResponseObject
	}{
		{
			name: "нарушение инварианта → 400",
			err:  fmt.Errorf("создание элемента: %w", domainerrors.Validation("name", "не может быть пустым")),
			want: api.CreateItem400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
				Code: "validation", Message: "name: не может быть пустым",
			}},
		},
		{
			name: "конфликт уникальности → 409",
			err: fmt.Errorf("сохранение элемента: %w",
				fmt.Errorf("элемент с именем %q уже есть: %w", "виджет", domainerrors.ErrConflict)),
			want: api.CreateItem409JSONResponse{ConflictJSONResponse: api.ConflictJSONResponse{
				Code: "conflict", Message: "конфликт состояния",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			create := createmocks.NewUseCase(t)
			create.EXPECT().Execute(mock.Anything, "виджет").Return(nil, tt.err).Once()

			h := httptransport.New(create, getmocks.NewUseCase(t), ready(nil))

			got, err := h.CreateItem(context.Background(), api.CreateItemRequestObject{
				Body: &api.CreateItemJSONRequestBody{Name: "виджет"},
			})
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// Неизвестная ошибка уходит наверх: 500 отдаёт echo, а не транспорт.
func TestCreateItem_UnknownErrorPropagates(t *testing.T) {
	t.Parallel()

	create := createmocks.NewUseCase(t)
	create.EXPECT().Execute(mock.Anything, "виджет").Return(nil, errors.New("база недоступна")).Once()

	h := httptransport.New(create, getmocks.NewUseCase(t), ready(nil))

	_, err := h.CreateItem(context.Background(), api.CreateItemRequestObject{
		Body: &api.CreateItemJSONRequestBody{Name: "виджет"},
	})
	require.Error(t, err)
}

func TestGetItem(t *testing.T) {
	t.Parallel()

	id := entity.NewItemID()
	now := time.Date(2026, 1, 2, 6, 4, 5, 0, time.FixedZone("MSK", 3*60*60))
	wantCreatedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("найден → 200", func(t *testing.T) {
		t.Parallel()

		get := getmocks.NewUseCase(t)
		get.EXPECT().Get(mock.Anything, id).Return(entity.RestoreItem(id, "виджет", now), nil).Once()

		h := httptransport.New(createmocks.NewUseCase(t), get, ready(nil))

		got, err := h.GetItem(context.Background(), api.GetItemRequestObject{Id: id.UUID()})
		require.NoError(t, err)
		require.Equal(t, api.GetItem200JSONResponse{Id: id.UUID(), Name: "виджет", CreatedAt: wantCreatedAt}, got)
	})

	t.Run("не найден → 404", func(t *testing.T) {
		t.Parallel()

		missing := uuid.New()
		get := getmocks.NewUseCase(t)
		get.EXPECT().Get(mock.Anything, entity.ItemID(missing)).
			Return(nil, fmt.Errorf("элемент %s: %w", missing, domainerrors.ErrNotFound)).Once()

		h := httptransport.New(createmocks.NewUseCase(t), get, ready(nil))

		got, err := h.GetItem(context.Background(), api.GetItemRequestObject{Id: missing})
		require.NoError(t, err)
		require.Equal(t, api.GetItem404JSONResponse{NotFoundJSONResponse: api.NotFoundJSONResponse{
			Code: "not_found", Message: "не найдено",
		}}, got)
	})
}

func TestListItems(t *testing.T) {
	t.Parallel()

	id := entity.NewItemID()
	now := time.Date(2026, 1, 2, 6, 4, 5, 0, time.FixedZone("MSK", 3*60*60))
	wantCreatedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	limit := 5

	t.Run("список → 200", func(t *testing.T) {
		t.Parallel()

		get := getmocks.NewUseCase(t)
		get.EXPECT().List(mock.Anything, limit).
			Return([]*entity.Item{entity.RestoreItem(id, "виджет", now)}, nil).Once()

		h := httptransport.New(createmocks.NewUseCase(t), get, ready(nil))

		got, err := h.ListItems(context.Background(), api.ListItemsRequestObject{
			Params: api.ListItemsParams{Limit: &limit},
		})
		require.NoError(t, err)
		require.Equal(t, api.ListItems200JSONResponse{{Id: id.UUID(), Name: "виджет", CreatedAt: wantCreatedAt}}, got)
	})

	t.Run("нарушение инварианта → 400", func(t *testing.T) {
		t.Parallel()

		get := getmocks.NewUseCase(t)
		get.EXPECT().List(mock.Anything, 0).
			Return(nil, domainerrors.Validation("limit", "должен быть положительным")).Once()

		h := httptransport.New(createmocks.NewUseCase(t), get, ready(nil))

		got, err := h.ListItems(context.Background(), api.ListItemsRequestObject{})
		require.NoError(t, err)
		require.Equal(t, api.ListItems400JSONResponse{BadRequestJSONResponse: api.BadRequestJSONResponse{
			Code: "validation", Message: "limit: должен быть положительным",
		}}, got)
	})
}
