package console_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Y91R/chromectl/internal/console"
)

func TestFromAPICalled(t *testing.T) {
	t.Parallel()
	params := json.RawMessage(`{"type":"log","timestamp":1.5,
	  "args":[{"type":"string","value":"обычное"},{"type":"number","value":42,"description":"42"},{"type":"object","className":"Object","description":"Object"},{"type":"undefined"}],
	  "stackTrace":{"callFrames":[{"functionName":"","url":"http://x/","lineNumber":2,"columnNumber":8},{"functionName":"boot","url":"http://x/app.js","lineNumber":0,"columnNumber":0}]}}`)

	got, err := console.FromAPICalled(params)

	require.NoError(t, err)
	assert.Equal(t, console.Message{
		Type:      "log",
		Text:      "обычное 42 Object undefined",
		Args:      []string{"обычное", "42", "Object", "undefined"},
		Stack:     []string{"at <anonymous> (http://x/:3:9)", "at boot (http://x/app.js:1:1)"},
		Timestamp: 1.5,
	}, got)
}

// Регрессия: CDP присылает console.warn с типом "warning", а CLI показывает и
// фильтрует его как "warn".
func TestFromAPICalled_WarningIsWarn(t *testing.T) {
	t.Parallel()

	got, err := console.FromAPICalled(json.RawMessage(`{"type":"warning","timestamp":1,"args":[{"type":"string","value":"повтор"}]}`))

	require.NoError(t, err)
	assert.Equal(t, "warn", got.Type)
}

func TestFromException(t *testing.T) {
	t.Parallel()
	params := json.RawMessage(`{"timestamp":2,"exceptionDetails":{"text":"Uncaught",
	  "exception":{"type":"object","subtype":"error","description":"Error: бум\n    at tick (http://x/:5:13)"},
	  "stackTrace":{"callFrames":[{"functionName":"tick","url":"http://x/","lineNumber":4,"columnNumber":12}]}}}`)

	got, err := console.FromException(params)

	require.NoError(t, err)
	assert.Equal(t, console.Message{
		Type:      "error",
		Text:      "Error: бум",
		Stack:     []string{"at tick (http://x/:5:13)"},
		Timestamp: 2,
	}, got)
}

func TestFromException_WithoutExceptionObjectUsesText(t *testing.T) {
	t.Parallel()

	got, err := console.FromException(json.RawMessage(`{"timestamp":3,"exceptionDetails":{"text":"Uncaught SyntaxError: Unexpected token"}}`))

	require.NoError(t, err)
	assert.Equal(t, "Uncaught SyntaxError: Unexpected token", got.Text)
	assert.Equal(t, "error", got.Type)
}

func TestNumber_OrdersByTimestamp(t *testing.T) {
	t.Parallel()

	got := console.Number([]console.Message{
		{Type: "error", Text: "позже", Timestamp: 5},
		{Type: "log", Text: "раньше", Timestamp: 1},
	})

	assert.Equal(t, []console.Message{
		{ID: 1, Type: "log", Text: "раньше", Timestamp: 1},
		{ID: 2, Type: "error", Text: "позже", Timestamp: 5},
	}, got)
}

func messages() []console.Message {
	return []console.Message{
		{ID: 1, Type: "log", Text: "a", Args: []string{"a"}},
		{ID: 2, Type: "error", Text: "b", Args: []string{"b"}},
		{ID: 3, Type: "error", Text: "b", Args: []string{"b"}},
		{ID: 4, Type: "warn", Text: "c", Args: []string{"c"}},
		{ID: 5, Type: "log", Text: "d", Args: []string{"d"}},
	}
}

func TestList_GroupsConsecutiveDuplicates(t *testing.T) {
	t.Parallel()

	got, err := console.List(messages(), console.ListOptions{})

	require.NoError(t, err)
	assert.Equal(t, "msgid=1 [log] a (1 args)\nmsgid=2 [error] b (1 args) [2 times]\nmsgid=4 [warn] c (1 args)\nmsgid=5 [log] d (1 args)", got)
}

func TestList_FiltersByType(t *testing.T) {
	t.Parallel()

	got, err := console.List(messages(), console.ListOptions{Types: []string{"error", "warn"}})

	require.NoError(t, err)
	assert.Equal(t, "msgid=2 [error] b (1 args) [2 times]\nmsgid=4 [warn] c (1 args)", got)
}

func TestList_PaginatesBeforeGrouping(t *testing.T) {
	t.Parallel()

	got, err := console.List(messages(), console.ListOptions{PageSize: 2, PageIdx: 1})

	require.NoError(t, err)
	assert.Equal(t, "msgid=3 [error] b (1 args)\nmsgid=4 [warn] c (1 args)", got)
}

func TestList_EmptyAndInvalidOptions(t *testing.T) {
	t.Parallel()

	got, err := console.List(nil, console.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, "сообщений нет", got)

	_, err = console.List(messages(), console.ListOptions{Types: []string{"шум"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `неизвестный тип сообщения "шум"`)

	_, err = console.List(messages(), console.ListOptions{PageIdx: -1})
	require.Error(t, err)
}

func TestDetail(t *testing.T) {
	t.Parallel()

	got := console.Detail(console.Message{
		ID:    3,
		Type:  "log",
		Text:  "обычное 42",
		Args:  []string{"обычное", "42"},
		Stack: []string{"at <anonymous> (http://x/:3:9)"},
	})

	assert.Equal(t, "ID: 3\nСообщение: log> обычное 42\n### Аргументы\nArg #0: обычное\nArg #1: 42\n### Стек\nat <anonymous> (http://x/:3:9)", got)
	assert.Equal(t, "ID: 7\nСообщение: error> Error: бум", console.Detail(console.Message{ID: 7, Type: "error", Text: "Error: бум"}))
}
