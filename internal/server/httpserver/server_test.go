// Тест внутри пакета: проверяется связка middleware и обработчика ошибок,
// а не публичный API сервера.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	api "github.com/your-org/chrome_skill/gen/api"
)

var errBoom = errors.New("пароль от базы hunter2")

// stubHandlers нужен только чтобы собрать роутер: до хендлеров ни один кейс
// этого теста не доходит — все ответы формируют middleware.
type stubHandlers struct{}

func (stubHandlers) GetHealth(context.Context, api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	return api.GetHealth200JSONResponse{Status: "ok"}, nil
}

func (stubHandlers) ListItems(context.Context, api.ListItemsRequestObject) (api.ListItemsResponseObject, error) {
	return api.ListItems200JSONResponse{}, nil
}

func (stubHandlers) CreateItem(context.Context, api.CreateItemRequestObject) (api.CreateItemResponseObject, error) {
	return api.CreateItem201JSONResponse{}, nil
}

func (stubHandlers) GetItem(context.Context, api.GetItemRequestObject) (api.GetItemResponseObject, error) {
	return api.GetItem200JSONResponse{}, nil
}

// failingHandlers имитирует сбой внутри операции: хендлер возвращает ошибку,
// её обрабатывает уже сервер.
type failingHandlers struct{ stubHandlers }

func (failingHandlers) GetHealth(context.Context, api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	return nil, errBoom
}

func newTestServer(t *testing.T, handlers api.StrictServerInterface) *Server {
	t.Helper()

	srv, err := New(
		Config{Addr: "127.0.0.1:0", StaticDir: t.TempDir()},
		handlers,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	require.NoError(t, err)
	return srv
}

// Ошибку запроса отдаёт middleware валидации, а не хендлер, поэтому её формат
// не покрыт тестами транспорта. Спека обещает на 4xx схему Error{code,message} —
// клиент разбирает ответ по ней независимо от того, какой слой его сформировал.
func TestAPIErrorsMatchSpecSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   string
		target   string
		body     string
		wantCode int
		wantSlug string
	}{
		{
			name:     "параметр вне границ спеки → 400",
			method:   http.MethodGet,
			target:   "/api/v1/items?limit=0",
			wantCode: http.StatusBadRequest,
			wantSlug: "validation",
		},
		{
			name:     "тело не по схеме → 400",
			method:   http.MethodPost,
			target:   "/api/v1/items",
			body:     `{"name":""}`, // minLength: 1 в спеке
			wantCode: http.StatusBadRequest,
			wantSlug: "validation",
		},
		{
			name:     "операции нет в спеке → 404",
			method:   http.MethodGet,
			target:   "/api/v1/nope",
			wantCode: http.StatusNotFound,
			wantSlug: "not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()

			newTestServer(t, stubHandlers{}).echo.ServeHTTP(rec, req)

			require.Equal(t, tt.wantCode, rec.Code)

			var body api.Error
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body),
				"тело ответа обязано разбираться по схеме Error из спеки, получено: %s", rec.Body.String())
			require.Equal(t, tt.wantSlug, body.Code)
			require.NotEmpty(t, body.Message)
		})
	}
}

// Внутренняя ошибка не должна выносить наружу свой текст: клиенту достаётся код
// и нейтральное сообщение, подробности остаются в логе.
func TestInternalErrorHidesDetails(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	newTestServer(t, failingHandlers{}).echo.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "hunter2")

	var body api.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "internal", body.Code)
}
