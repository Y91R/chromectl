package entity_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/your-org/chrome_skill/internal/service/entity"
	domainerrors "github.com/your-org/chrome_skill/internal/service/errors"
)

func TestNewItem(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name      string
		input     string
		wantName  string
		wantField string
	}{
		{name: "обычное имя", input: "виджет", wantName: "виджет"},
		{name: "пробелы по краям обрезаются", input: "  виджет  ", wantName: "виджет"},
		{name: "пустое имя отвергается", input: "", wantField: "name"},
		{name: "имя из пробелов отвергается", input: "   ", wantField: "name"},
		// Граница взята из спеки (docs/api/openapi.yaml, CreateItemRequest.name.maxLength):
		// 255 — это символы, а не байты, поэтому кейс кириллический.
		{name: "ровно 255 символов принимается", input: strings.Repeat("я", 255), wantName: strings.Repeat("я", 255)},
		{name: "длиннее 255 символов отвергается", input: strings.Repeat("я", 256), wantField: "name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			item, err := entity.NewItem(tt.input, now)
			if tt.wantField != "" {
				v, ok := domainerrors.AsValidation(err)
				require.True(t, ok, "ожидалась ошибка валидации, получено: %v", err)
				require.Equal(t, tt.wantField, v.Field)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantName, item.Name())
			require.Equal(t, now, item.CreatedAt())
			require.NotEqual(t, entity.ItemID{}, item.ID())
		})
	}
}

// Rename обязан держать тот же инвариант, что и NewItem: иначе некорректное имя
// попадает в сущность через легальный метод, минуя конструктор.
func TestRename(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name      string
		input     string
		wantName  string
		wantField string
	}{
		{name: "новое имя", input: "  колесо  ", wantName: "колесо"},
		{name: "пустое имя отвергается", input: "   ", wantField: "name"},
		{name: "ровно 255 символов принимается", input: strings.Repeat("я", 255), wantName: strings.Repeat("я", 255)},
		{name: "длиннее 255 символов отвергается", input: strings.Repeat("я", 256), wantField: "name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			item, err := entity.NewItem("виджет", now)
			require.NoError(t, err)

			err = item.Rename(tt.input)
			if tt.wantField != "" {
				v, ok := domainerrors.AsValidation(err)
				require.True(t, ok, "ожидалась ошибка валидации, получено: %v", err)
				require.Equal(t, tt.wantField, v.Field)
				require.Equal(t, "виджет", item.Name(), "отвергнутое имя не должно попадать в сущность")
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantName, item.Name())
		})
	}
}

func TestParseItemID(t *testing.T) {
	t.Parallel()

	id := entity.NewItemID()
	parsed, err := entity.ParseItemID(id.String())
	require.NoError(t, err)
	require.Equal(t, id, parsed)

	_, err = entity.ParseItemID("не-uuid")
	_, ok := domainerrors.AsValidation(err)
	require.True(t, ok, "ожидалась ошибка валидации, получено: %v", err)
}
