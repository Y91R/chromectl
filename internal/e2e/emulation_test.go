//go:build e2e

// e2e Э3: эмуляция и размер окна. Ожидания — из
// docs/plans/etap-3-emulyaciya.md, раздел «Тесты».
package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type echoFixture struct {
	url    string
	mu     sync.Mutex
	header string
}

func (f *echoFixture) lastHeader() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.header
}

func startEchoFixture(t *testing.T) *echoFixture {
	t.Helper()
	f := &echoFixture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.header = r.Header.Get("X-Test")
		f.mu.Unlock()
		_, _ = fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>echo</title><body>эмуляция</body>`)
	}))
	t.Cleanup(srv.Close)
	f.url = srv.URL + "/"
	return f
}

func (e *env) evalValue(args ...string) string {
	e.t.Helper()
	return strings.TrimSpace(e.mustRun(append([]string{"eval"}, args...)...).stdout)
}

func TestEmulate_ViewportAndColorSchemePersistAcrossCommands(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", fx.url+"/mobile")
	const dark = "() => matchMedia('(prefers-color-scheme: dark)').matches"

	e.mustRun("emulate", "--viewport", "390x844x3,mobile", "--color-scheme", "dark")
	assert.Equal(t, "390", e.evalValue("() => innerWidth"))
	assert.Equal(t, "3", e.evalValue("() => devicePixelRatio"))
	assert.Equal(t, "true", e.evalValue(dark))

	e.mustRun("emulate", "--color-scheme", "auto")
	assert.Equal(t, "false", e.evalValue(dark))
	assert.Equal(t, "390", e.evalValue("() => innerWidth"), "сброс темы не трогает viewport")

	e.mustRun("emulate", "--viewport", "")
	assert.NotEqual(t, "390", e.evalValue("() => innerWidth"))
}

func TestEmulate_ShowPrintsCurrentSettings(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", fx.url+"/a")

	e.mustRun("emulate", "--viewport", "390x844x3,mobile", "--color-scheme", "dark")
	r := e.mustRun("emulate", "--show", "--json")

	assert.JSONEq(t, `{"viewport":{"width":390,"height":844,"deviceScaleFactor":3,"mobile":true},"colorScheme":"dark"}`, r.stdout)
	assert.Contains(t, e.mustRun("emulate", "--show").stdout, "viewport 390x844x3,mobile; тема dark")
}

func TestEmulate_InvalidValuesRejectedBeforeBrowser(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	for _, tc := range []struct {
		args    []string
		message string
	}{
		{[]string{"--cpu", "21"}, "--cpu должен быть от 1 до 20, получено 21"},
		{[]string{"--viewport", "390x"}, `неверная высота viewport ""`},
		{[]string{"--network", "5G"}, `неизвестный профиль сети "5G"`},
		{[]string{"--geolocation", "91,0"}, `неверная широта "91"`},
		{[]string{"--color-scheme", "blue"}, "--color-scheme: dark, light или auto"},
	} {
		r := e.run(append([]string{"emulate"}, tc.args...)...)
		assert.NotEqual(t, 0, r.code, "%v", tc.args)
		assert.Contains(t, r.stderr, tc.message, "%v", tc.args)
		assert.NotContains(t, r.stderr, "браузер не запущен", "неверное значение отклоняется до подключения")
	}
}

func TestEmulate_HeadersAndOfflineNetwork(t *testing.T) {
	t.Parallel()
	echo := startEchoFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", echo.url)

	e.mustRun("emulate", "--headers", `{"X-Test":"1"}`)
	e.mustRun("navigate", echo.url+"?headers")
	assert.Equal(t, "1", echo.lastHeader())

	e.mustRun("emulate", "--headers", "")
	e.mustRun("navigate", echo.url+"?no-headers")
	assert.Equal(t, "", echo.lastHeader())

	e.mustRun("emulate", "--network", "Offline")
	r := e.run("navigate", echo.url+"?offline")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "net::ERR_INTERNET_DISCONNECTED")

	e.mustRun("emulate", "--network", "none")
	e.mustRun("navigate", echo.url+"?online")
}

func TestEmulate_OnlyAffectsItsTab(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	e.startBrowser()
	first := e.pages()[0].ID
	e.mustRun("pages", "new", fx.url+"/a")

	e.mustRun("emulate", "--viewport", "390x844")

	assert.Equal(t, "390", e.evalValue("() => innerWidth"))
	assert.NotEqual(t, "390", strings.TrimSpace(e.mustRun("--page", first, "eval", "() => innerWidth").stdout))
}

// Часть override Chrome не снимает при отключении сессии (спайк Э3), поэтому
// сброс настройки обязан снять её явно, а не полагаться на отключение.
func TestEmulate_ResetClearsTouchAndUserAgent(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", fx.url+"/mobile")
	originalAgent := e.evalValue("() => navigator.userAgent")

	e.mustRun("emulate", "--viewport", "390x844x2,mobile,touch", "--user-agent", "chromectl-bot")
	assert.Equal(t, "1", e.evalValue("() => navigator.maxTouchPoints"))
	assert.Equal(t, `"chromectl-bot"`, e.evalValue("() => navigator.userAgent"))

	e.mustRun("emulate", "--viewport", "", "--user-agent", "")
	assert.Equal(t, "0", e.evalValue("() => navigator.maxTouchPoints"))
	assert.Equal(t, originalAgent, e.evalValue("() => navigator.userAgent"))
}

func TestResize_ChangesPageSize(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", fx.url+"/a")

	e.mustRun("resize", "800", "600")

	assert.Equal(t, "800", e.evalValue("() => innerWidth"))
	assert.Equal(t, "600", e.evalValue("() => innerHeight"))
}
