// Package schemas переводит доменные сущности в модели контракта и доменные
// ошибки — в коды ответов. Отдельный пакет, чтобы это соответствие лежало
// в одном месте, а не расползалось по хендлерам.
//
// ADR: docs/adr/0005-domennyy-sloy-vmesto-dto.md — маппинг сущность ↔ модель
// контракта живёт здесь, а не в хендлере.
package schemas

import (
	"github.com/google/uuid"

	api "github.com/your-org/chrome_skill/gen/api"
	"github.com/your-org/chrome_skill/internal/service/entity"
	domainerrors "github.com/your-org/chrome_skill/internal/service/errors"
)

func ToItem(item *entity.Item) api.Item {
	return api.Item{
		Id:   uuid.UUID(item.ID()),
		Name: item.Name(),
		// Время наружу всегда в UTC: иначе клиенты получают разные ответы
		// в зависимости от таймзоны машины, где запущен сервис.
		CreatedAt: item.CreatedAt().UTC(),
	}
}

func ToItems(items []*entity.Item) []api.Item {
	out := make([]api.Item, 0, len(items))
	for _, item := range items {
		out = append(out, ToItem(item))
	}
	return out
}

func errorBody(code, message string) api.Error {
	return api.Error{Code: code, Message: message}
}

func ListItemsError(err error) (api.ListItemsResponseObject, error) {
	if v, ok := domainerrors.AsValidation(err); ok {
		return api.ListItems400JSONResponse{
			BadRequestJSONResponse: api.BadRequestJSONResponse(errorBody("validation", v.Error())),
		}, nil
	}
	return nil, err
}

func CreateItemError(err error) (api.CreateItemResponseObject, error) {
	switch {
	case isValidation(err):
		v, _ := domainerrors.AsValidation(err)
		return api.CreateItem400JSONResponse{
			BadRequestJSONResponse: api.BadRequestJSONResponse(errorBody("validation", v.Error())),
		}, nil
	case domainerrors.IsConflict(err):
		// Наружу уходит текст доменной ошибки, а не собранная по слоям цепочка:
		// клиенту незачем знать, что внутри называлось «сохранение элемента».
		return api.CreateItem409JSONResponse{
			ConflictJSONResponse: api.ConflictJSONResponse(errorBody("conflict", domainerrors.ErrConflict.Error())),
		}, nil
	default:
		// Неизвестная ошибка уходит наверх: превратить её в 400 значило бы
		// сказать клиенту «ты виноват» при сбое базы. Логирует её тот, кто
		// обработал (обработчик ошибок echo), — иначе одна ошибка попадёт в лог дважды.
		return nil, err
	}
}

func GetItemError(err error) (api.GetItemResponseObject, error) {
	if domainerrors.IsNotFound(err) {
		return api.GetItem404JSONResponse{
			NotFoundJSONResponse: api.NotFoundJSONResponse(errorBody("not_found", domainerrors.ErrNotFound.Error())),
		}, nil
	}
	return nil, err
}

func isValidation(err error) bool {
	_, ok := domainerrors.AsValidation(err)
	return ok
}
