package netlog_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/Y91R/chromectl/internal/netlog"
)

func TestRedact_MasksSecretsCaseInsensitive(t *testing.T) {
	t.Parallel()

	got := netlog.Redact(map[string]string{
		"Authorization":       "Bearer secret",
		"cookie":              "a=1",
		"Set-Cookie":          "s=2",
		"proxy-authorization": "Basic x",
		"X-Test":              "1",
	})

	assert.Equal(t, map[string]string{
		"Authorization":       "<redacted>",
		"cookie":              "<redacted>",
		"Set-Cookie":          "<redacted>",
		"proxy-authorization": "<redacted>",
		"X-Test":              "1",
	}, got)
}

func TestStoredBody(t *testing.T) {
	t.Parallel()

	body, note := netlog.StoredBody([]byte(`{"ok":true}`))
	assert.Equal(t, `{"ok":true}`, body)
	assert.Empty(t, note)

	body, note = netlog.StoredBody([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0xff})
	assert.Empty(t, body)
	assert.Equal(t, "<бинарные данные, 8 байт>", note)

	big := strings.Repeat("я", 600*1024)
	body, note = netlog.StoredBody([]byte(big))
	assert.LessOrEqual(t, len(body), 1024*1024)
	assert.Greater(t, len(body), 1024*1024-4)
	assert.True(t, utf8.ValidString(body), "обрезка не рвёт символ")
	assert.Equal(t, "<обрезано до 1048576 из 1228800 байт>", note)

	body, note = netlog.StoredBody(nil)
	assert.Empty(t, body)
	assert.Equal(t, "<пустое тело>", note)
}

func TestConcise(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "reqid=1 GET http://x/api [200]", netlog.Concise(netlog.Request{ReqID: 1, Method: "GET", URL: "http://x/api", Status: 200}))
	assert.Equal(t, "reqid=2 POST http://x/echo [net::ERR_FAILED]", netlog.Concise(netlog.Request{ReqID: 2, Method: "POST", URL: "http://x/echo", Failure: "net::ERR_FAILED"}))
	assert.Equal(t, "reqid=3 GET http://x/slow [pending]", netlog.Concise(netlog.Request{ReqID: 3, Method: "GET", URL: "http://x/slow"}))

	long := "http://x/" + strings.Repeat("a", 300)
	assert.Equal(t, "reqid=4 GET "+long[:255]+"... <обрезано> [200]", netlog.Concise(netlog.Request{ReqID: 4, Method: "GET", URL: long, Status: 200}))
}

func TestDetail(t *testing.T) {
	t.Parallel()

	got := netlog.Detail(netlog.Request{
		ReqID:           1,
		Method:          "POST",
		URL:             "http://x/api",
		Status:          200,
		RequestHeaders:  map[string]string{"X-Test": "1", "Authorization": "<redacted>"},
		RequestBody:     `{"a":1}`,
		ResponseHeaders: map[string]string{"Content-Type": "application/json"},
		ResponseBody:    `{"ok":true}`,
	})

	assert.Equal(t, `## Запрос POST http://x/api
Статус: 200
### Заголовки запроса
Authorization: <redacted>
X-Test: 1
### Тело запроса
{"a":1}
### Заголовки ответа
Content-Type: application/json
### Тело ответа
{"ok":true}`, got)
}

func TestDetail_NotesFailureAndInlineLimit(t *testing.T) {
	t.Parallel()

	got := netlog.Detail(netlog.Request{
		Method:           "GET",
		URL:              "http://x/big",
		Status:           200,
		ResponseBody:     strings.Repeat("ж", 10005),
		ResponseBodyNote: "<обрезано до 1048576 из 5242880 байт>",
	})
	assert.Equal(t, "## Запрос GET http://x/big\nСтатус: 200\n### Тело ответа\n"+strings.Repeat("ж", 10000)+"... <обрезано>\n<обрезано до 1048576 из 5242880 байт>", got)

	got = netlog.Detail(netlog.Request{Method: "GET", URL: "http://x/img", Status: 200, ResponseBodyNote: "<бинарные данные, 8 байт>"})
	assert.Equal(t, "## Запрос GET http://x/img\nСтатус: 200\n### Тело ответа\n<бинарные данные, 8 байт>", got)

	got = netlog.Detail(netlog.Request{Method: "GET", URL: "http://x/down", Failure: "net::ERR_CONNECTION_REFUSED"})
	assert.Equal(t, "## Запрос GET http://x/down\nСтатус: net::ERR_CONNECTION_REFUSED\n### Ошибка\nnet::ERR_CONNECTION_REFUSED", got)
}

func TestConciseTimingAndType(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "- fetch http://x/data.json [200]", netlog.ConciseTiming(netlog.Timing{URL: "http://x/data.json", InitiatorType: "fetch", Status: 200}))
	assert.Equal(t, "- image http://x/a.png [?]", netlog.ConciseTiming(netlog.Timing{URL: "http://x/a.png", InitiatorType: "img"}))

	for initiator, want := range map[string]string{
		"navigation":     "document",
		"fetch":          "fetch",
		"xmlhttprequest": "xhr",
		"img":            "image",
		"script":         "script",
		"css":            "stylesheet",
		"link":           "other",
		"beacon":         "ping",
		"video":          "media",
		"":               "other",
	} {
		assert.Equal(t, want, netlog.TypeFromInitiator(initiator), initiator)
	}
}
