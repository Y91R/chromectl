//go:build e2e

// e2e Э4, сеть: список без захвата — из Performance API, детали — из --capture.
// Ожидания — из docs/plans/etap-4-konsol-i-set.md, раздел «Тесты».
package e2e_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const networkPage = `<!doctype html><meta charset="utf-8"><title>Сеть</title>
<button onclick="fetch('/echo', {method: 'POST', body: '{&quot;a&quot;:1}', headers: {'Content-Type': 'application/json'}}).then(() => out.textContent = 'готово')">Отправить</button>
<div id="out"></div>
<script>
// Токен собирается из частей: исходник страницы тоже захватывается как тело
// ответа, и проверка «секрет не на диске» должна видеть только заголовок.
fetch('/api', {headers: {Authorization: 'Bearer ' + 'sec' + 'ret'}});
fetch('/big');
new Image().src = '/img.png';
</script>`

func startNetworkFixture(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, networkPage)
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprint(w, strings.Repeat("a", 5*1024*1024))
	})
	// Настоящий PNG: тело картинки, которую Chrome не смог декодировать, getResponseBody не отдаёт.
	mux.HandleFunc("/img.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pixelPNG(t))
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func pixelPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func reqID(t *testing.T, list, method, path string) string {
	t.Helper()
	m := regexp.MustCompile(`reqid=(\d+) ` + method + ` \S*` + regexp.QuoteMeta(path) + ` \[`).FindStringSubmatch(list)
	require.NotNil(t, m, "нет %s %s в списке:\n%s", method, path, list)
	return m[1]
}

func TestNetwork_WithoutCaptureListsPerformanceEntries(t *testing.T) {
	t.Parallel()
	base := startNetworkFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", base+"/page")
	e.mustRun("eval", "() => new Promise(r => setTimeout(() => r(1), 500))")

	list := e.mustRun("network", "list").stdout
	assert.Contains(t, list, "- fetch "+base+"/api [200]")
	assert.Contains(t, list, "повторите действие с --capture")

	r := e.run("network", "get", "1")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "нет захваченных запросов: выполните navigate или действие с --capture")
}

func TestNavigateCapture_HeadersBodiesAndLimits(t *testing.T) {
	t.Parallel()
	base := startNetworkFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", "about:blank")

	e.mustRun("navigate", base+"/page?first", "--capture")
	list := e.mustRun("network", "list").stdout
	assert.Contains(t, list, "GET "+base+"/page?first [200]")

	api := e.mustRun("network", "get", reqID(t, list, "GET", "/api")).stdout
	assert.Contains(t, api, "Authorization: <redacted>")
	assert.Contains(t, api, `{"ok":true}`)
	assert.NotContains(t, api, "Bearer secret")

	assert.Contains(t, e.mustRun("network", "get", reqID(t, list, "GET", "/big")).stdout, "<обрезано до 1048576 из 5242880 байт>")
	assert.Contains(t, e.mustRun("network", "get", reqID(t, list, "GET", "/img.png")).stdout,
		fmt.Sprintf("<бинарные данные, %d байт>", len(pixelPNG(t))))

	capture := filepath.Join(e.stateDir, strconv.Itoa(e.port), "network", e.selected().ID+".jsonl")
	info, err := os.Stat(capture)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	data, err := os.ReadFile(capture)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "Bearer secret", "секрет не попадает на диск")

	e.mustRun("navigate", base+"/page?second", "--capture", "--unredacted")
	list = e.mustRun("network", "list").stdout
	assert.NotContains(t, list, "?first", "новый захват перезаписывает прежний")
	assert.Contains(t, e.mustRun("network", "get", reqID(t, list, "GET", "/api")).stdout, "Authorization: Bearer secret")
}

func TestClickCapture_PostBodyAndResponseFile(t *testing.T) {
	t.Parallel()
	base := startNetworkFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", base+"/page")
	snap := e.mustRun("snapshot").stdout

	e.mustRun("click", uidOf(t, snap, "button", "Отправить"), "--capture")
	list := e.mustRun("network", "list").stdout
	id := reqID(t, list, "POST", "/echo")

	path := filepath.Join(t.TempDir(), "echo.json")
	detail := e.mustRun("network", "get", id, "--response-file", path).stdout
	assert.Contains(t, detail, "### Тело запроса\n{\"a\":1}")
	assert.Contains(t, detail, path)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, `{"a":1}`, string(body))
}
