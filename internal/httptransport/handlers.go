// Package httptransport реализует сгенерированный контракт. Здесь только
// перевод запроса в вызов операции и результата в ответ — никакой бизнес-логики.
package httptransport

import (
	"context"

	api "github.com/your-org/chrome_skill/gen/api"
	"github.com/your-org/chrome_skill/internal/httptransport/schemas"
	"github.com/your-org/chrome_skill/internal/service/entity"
	"github.com/your-org/chrome_skill/internal/service/usecase/createitem"
	"github.com/your-org/chrome_skill/internal/service/usecase/getitem"
)

// Ready — проба готовности зависимостей, объявленная потребителем: транспорту
// нужен один вызов, а не весь граф приложения.
type Ready func(ctx context.Context) error

type Handlers struct {
	create createitem.UseCase
	get    getitem.UseCase
	ready  Ready
}

var _ api.StrictServerInterface = (*Handlers)(nil)

func New(create createitem.UseCase, get getitem.UseCase, ready Ready) *Handlers {
	return &Handlers{create: create, get: get, ready: ready}
}

func (h *Handlers) GetHealth(ctx context.Context, _ api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	if err := h.ready(ctx); err != nil {
		return api.GetHealth503JSONResponse{Status: "db down"}, nil
	}
	return api.GetHealth200JSONResponse{Status: "ok"}, nil
}

func (h *Handlers) ListItems(ctx context.Context, request api.ListItemsRequestObject) (api.ListItemsResponseObject, error) {
	var limit int
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}

	items, err := h.get.List(ctx, limit)
	if err != nil {
		return schemas.ListItemsError(err)
	}
	return api.ListItems200JSONResponse(schemas.ToItems(items)), nil
}

func (h *Handlers) CreateItem(ctx context.Context, request api.CreateItemRequestObject) (api.CreateItemResponseObject, error) {
	item, err := h.create.Execute(ctx, request.Body.Name)
	if err != nil {
		return schemas.CreateItemError(err)
	}
	return api.CreateItem201JSONResponse(schemas.ToItem(item)), nil
}

func (h *Handlers) GetItem(ctx context.Context, request api.GetItemRequestObject) (api.GetItemResponseObject, error) {
	item, err := h.get.Get(ctx, entity.ItemID(request.Id))
	if err != nil {
		return schemas.GetItemError(err)
	}
	return api.GetItem200JSONResponse(schemas.ToItem(item)), nil
}
