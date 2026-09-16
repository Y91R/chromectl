//go:build e2e

// e2e Э1: каждая команда — отдельный процесс собранного бинаря, как у агента.
// Ожидания взяты из docs/plans/etap-1-brauzer-i-vkladki.md, раздел «Тесты».
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Y91R/chromectl/internal/browser"
)

var binPath string

func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	// Пустой зелёный прогон без Chrome хуже падения: он подтвердил бы то, чего никто не проверял.
	if _, err := browser.FindChrome(os.Getenv("CHROME_PATH"), browser.DefaultCandidates(runtime.GOOS), browser.FileExists); err != nil {
		fmt.Fprintln(os.Stderr, "e2e требует Chrome:", err)
		return 1
	}

	dir, err := os.MkdirTemp("", "chromectl-e2e-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()

	binPath = filepath.Join(dir, "chromectl")
	build := exec.Command("go", "build", "-o", binPath, "github.com/Y91R/chromectl/cmd/chromectl")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "сборка chromectl:", err)
		return 1
	}
	return m.Run()
}

type result struct {
	stdout string
	stderr string
	code   int
}

type env struct {
	t        *testing.T
	stateDir string
	port     int
}

func newEnv(t *testing.T) *env {
	t.Helper()
	// Не t.TempDir: Chrome может дописывать профиль после остановки, и строгая
	// очистка TempDir уронила бы тест не по делу.
	dir, err := os.MkdirTemp("", "chromectl-e2e-state-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return &env{t: t, stateDir: dir, port: freePort(t)}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func (e *env) run(args ...string) result {
	e.t.Helper()
	return e.runEnv(nil, args...)
}

// runEnv запускает команду с дополнительными переменными окружения поверх текущих.
func (e *env) runEnv(extraEnv []string, args ...string) result {
	e.t.Helper()
	// Первый audit lighthouse скачивает пакет через npx — минуты, а не секунды.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	full := append([]string{"--port", strconv.Itoa(e.port)}, args...)
	cmd := exec.CommandContext(ctx, binPath, full...)
	cmd.Env = append(append(os.Environ(), "CHROMECTL_STATE_DIR="+e.stateDir), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			e.t.Fatalf("запуск chromectl %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func (e *env) mustRun(args ...string) result {
	e.t.Helper()
	r := e.run(args...)
	if r.code != 0 {
		e.t.Fatalf("chromectl %v: код %d\nstdout: %s\nstderr: %s", args, r.code, r.stdout, r.stderr)
	}
	return r
}

func (e *env) statePath() string {
	return filepath.Join(e.stateDir, strconv.Itoa(e.port), "state.json")
}

func (e *env) statePID() int {
	data, err := os.ReadFile(e.statePath())
	if err != nil {
		return 0
	}
	var st struct {
		PID int `json:"pid"`
	}
	_ = json.Unmarshal(data, &st)
	return st.PID
}

func (e *env) startBrowser() {
	e.t.Helper()
	e.mustRun("browser", "start", "--headless")
	pid := e.statePID()
	e.t.Cleanup(func() {
		_ = e.run("browser", "stop")
		// Страховка: упавший stop не должен оставлять Chrome после тестов.
		if pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
}

type pageInfo struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Selected bool   `json:"selected"`
}

func (e *env) pages() []pageInfo {
	e.t.Helper()
	var list []pageInfo
	require.NoError(e.t, json.Unmarshal([]byte(e.mustRun("pages", "list", "--json").stdout), &list))
	return list
}

func (e *env) selected() pageInfo {
	e.t.Helper()
	for _, p := range e.pages() {
		if p.Selected {
			return p
		}
	}
	e.t.Fatal("нет выбранной вкладки")
	return pageInfo{}
}

// pageTargetIDs читает вкладки напрямую из Chrome, мимо CLI, чтобы проверять
// побочные эффекты команд независимо от их собственного вывода.
func (e *env) pageTargetIDs() []string {
	e.t.Helper()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", e.port))
	require.NoError(e.t, err)
	defer func() { _ = resp.Body.Close() }()
	var targets []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	require.NoError(e.t, json.NewDecoder(resp.Body).Decode(&targets))
	var ids []string
	for _, tg := range targets {
		if tg.Type == "page" {
			ids = append(ids, tg.ID)
		}
	}
	return ids
}

type fixture struct {
	url   string
	count *atomic.Int64
}

func startFixture(t *testing.T) fixture {
	t.Helper()
	count := &atomic.Int64{}
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><title>A</title><h1>A</h1>`)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><title>B</title><h1>B</h1>`)
	})
	mux.HandleFunc("/count", func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		_, _ = fmt.Fprint(w, `<!doctype html><title>count</title>`)
	})
	// Без meta viewport мобильная эмуляция даёт layout viewport 980 px, как настоящий телефон.
	mux.HandleFunc("/mobile", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><meta name="viewport" content="width=device-width"><title>mobile</title>`)
	})
	mux.HandleFunc("/long", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><title>long</title><body style="margin:0"><div style="height:3000px">long</div>`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return fixture{url: srv.URL, count: count}
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func TestCommands_FailWithoutBrowser(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	for _, args := range [][]string{
		{"pages", "list"},
		{"navigate", "http://127.0.0.1:1/"},
		{"screenshot"},
		{"browser", "stop"},
	} {
		r := e.run(args...)
		assert.NotEqual(t, 0, r.code, "%v", args)
		assert.Empty(t, r.stdout, "%v", args)
		assert.Contains(t, r.stderr, "браузер не запущен, выполните chromectl browser start", "%v", args)
	}
}

func TestBrowserStatus_NotRunning(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	r := e.mustRun("browser", "status")
	assert.Contains(t, r.stdout, "браузер не запущен")

	r = e.mustRun("browser", "status", "--json")
	assert.JSONEq(t, `{"running":false}`, r.stdout)
}

func TestBrowser_StartStatusStop(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	e.startBrowser()
	var st struct {
		Running bool `json:"running"`
		Port    int  `json:"port"`
		PID     int  `json:"pid"`
		Pages   int  `json:"pages"`
	}
	require.NoError(t, json.Unmarshal([]byte(e.mustRun("browser", "status", "--json").stdout), &st))
	assert.True(t, st.Running)
	assert.Equal(t, e.port, st.Port)
	assert.Positive(t, st.PID)
	assert.Equal(t, 1, st.Pages)

	dirInfo, err := os.Stat(filepath.Dir(e.statePath()))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	fileInfo, err := os.Stat(e.statePath())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())

	r := e.mustRun("browser", "stop")
	assert.Contains(t, r.stdout, "браузер остановлен")
	assert.False(t, alive(st.PID), "процесс Chrome %d жив после stop", st.PID)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", e.port), 500*time.Millisecond)
	if err == nil {
		_ = conn.Close()
	}
	assert.Error(t, err, "порт открыт после stop")
	_, err = os.Stat(e.statePath())
	assert.True(t, errors.Is(err, fs.ErrNotExist), "state остался после stop: %v", err)
	assert.Contains(t, e.mustRun("browser", "status").stdout, "браузер не запущен")
}

func TestBrowserStart_SecondStartFails(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.startBrowser()
	pid := e.statePID()

	r := e.run("browser", "start", "--headless")

	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, fmt.Sprintf("браузер уже запущен: порт %d, pid %d", e.port, pid))
	assert.Equal(t, pid, e.statePID())
	assert.Len(t, e.pageTargetIDs(), 1)
}

func TestBrowserStart_PortBusyFails(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", e.port))
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	r := e.run("browser", "start", "--headless")

	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, fmt.Sprintf("порт %d уже занят", e.port))
	_, err = os.Stat(e.statePath())
	assert.True(t, errors.Is(err, fs.ErrNotExist), "state создан при занятом порте: %v", err)
}

func TestPages_Lifecycle(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	e.startBrowser()

	initial := e.pages()
	require.Len(t, initial, 1)
	assert.True(t, initial[0].Selected)

	var created pageInfo
	require.NoError(t, json.Unmarshal([]byte(e.mustRun("pages", "new", fx.url+"/a", "--json").stdout), &created))
	assert.NotEmpty(t, created.ID)
	assert.Len(t, e.pages(), 2)
	sel := e.selected()
	assert.Equal(t, created.ID, sel.ID)
	assert.Equal(t, fx.url+"/a", sel.URL)
	assert.Equal(t, "A", sel.Title)

	e.mustRun("pages", "select", initial[0].ID)
	assert.Equal(t, initial[0].ID, e.selected().ID)

	e.mustRun("pages", "select", created.ID)
	e.mustRun("pages", "close", created.ID)
	remaining := e.pages()
	require.Len(t, remaining, 1)
	assert.Equal(t, initial[0].ID, remaining[0].ID)
	assert.True(t, remaining[0].Selected, "после закрытия выбранной вкладки выбирается оставшаяся")

	r := e.run("pages", "close", initial[0].ID)
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "последнюю вкладку закрыть нельзя")
	assert.Len(t, e.pageTargetIDs(), 1)

	r = e.run("pages", "select", "NOPE")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "вкладка NOPE не найдена")
}

func TestCommands_KeepTabCount(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", fx.url+"/a")
	before := e.pageTargetIDs()
	shot := filepath.Join(t.TempDir(), "keep.png")

	for _, args := range [][]string{
		{"pages", "list"},
		{"pages", "select", before[0]},
		{"navigate", fx.url + "/b"},
		{"navigate", "--reload"},
		{"screenshot", "-o", shot},
		{"browser", "status"},
	} {
		e.mustRun(args...)
		assert.ElementsMatch(t, before, e.pageTargetIDs(), "после %v", args)
	}
}

func TestNavigate_HistoryAndReload(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", fx.url+"/a")

	r := e.run("navigate", "--back")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "нет предыдущей страницы в истории")

	e.mustRun("navigate", fx.url+"/b")
	assert.Equal(t, fx.url+"/b", e.selected().URL)
	e.mustRun("navigate", "--back")
	assert.Equal(t, fx.url+"/a", e.selected().URL)
	e.mustRun("navigate", "--forward")
	assert.Equal(t, fx.url+"/b", e.selected().URL)

	r = e.run("navigate", "--forward")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "нет следующей страницы в истории")

	e.mustRun("navigate", fx.url+"/count")
	assert.Equal(t, int64(1), fx.count.Load())
	e.mustRun("navigate", "--reload")
	assert.Equal(t, int64(2), fx.count.Load())
}

func TestNavigate_UnreachableURLFails(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.startBrowser()

	// Порт 1 Chrome отклоняет как небезопасный (ERR_UNSAFE_PORT) ещё до соединения,
	// поэтому берётся свободный закрытый порт.
	r := e.run("navigate", fmt.Sprintf("http://127.0.0.1:%d/", freePort(t)))

	assert.NotEqual(t, 0, r.code)
	assert.Empty(t, r.stdout)
	assert.Contains(t, r.stderr, "net::ERR_CONNECTION_REFUSED")
}

func TestNavigate_DialogClosedBeforeExit(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", fx.url+"/a")
	const confirmScript = `document.addEventListener('DOMContentLoaded', () => { document.title = String(confirm('продолжить?')) })`

	r := e.mustRun("navigate", fx.url+"/a?accept", "--init-script", confirmScript)
	assert.Contains(t, r.stdout, "диалог confirm: продолжить?")
	assert.Equal(t, "true", e.selected().Title)
	// Страница не осталась заблокированной: следующая команда отвечает.
	e.mustRun("screenshot", "-o", filepath.Join(t.TempDir(), "after-dialog.png"))

	e.mustRun("navigate", fx.url+"/a?dismiss", "--init-script", confirmScript, "--dialog", "dismiss")
	assert.Equal(t, "false", e.selected().Title)
}

func TestNavigate_InitScriptOnlyForOneCommand(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", fx.url+"/a")

	e.mustRun("navigate", fx.url+"/a?init", "--init-script", `document.addEventListener('DOMContentLoaded', () => { document.title = 'init' })`)
	assert.Equal(t, "init", e.selected().Title)

	e.mustRun("navigate", "--reload")
	assert.Equal(t, "A", e.selected().Title)
}

func TestScreenshot_Formats(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", fx.url+"/long")
	dir := t.TempDir()

	pngPath := filepath.Join(dir, "x.png")
	r := e.mustRun("screenshot", "-o", pngPath)
	assert.Contains(t, r.stdout, pngPath)
	viewport := pngHeight(t, pngPath)
	assert.Less(t, viewport, 3000)

	fullPath := filepath.Join(dir, "full.png")
	e.mustRun("screenshot", "--full-page", "-o", fullPath)
	assert.GreaterOrEqual(t, pngHeight(t, fullPath), 3000)

	jpegPath := filepath.Join(dir, "x.jpg")
	e.mustRun("screenshot", "--format", "jpeg", "--quality", "50", "-o", jpegPath)
	data, err := os.ReadFile(jpegPath)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}), "не JPEG")

	r = e.mustRun("screenshot")
	tmpPath := strings.TrimSpace(r.stdout)
	t.Cleanup(func() { _ = os.Remove(tmpPath) })
	assert.Positive(t, pngHeight(t, tmpPath))

	r = e.run("screenshot", "--format", "gif")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "формат gif не поддерживается: png, jpeg, webp")

	r = e.run("screenshot", "--quality", "50")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "--quality только для jpeg и webp")
}

func pngHeight(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	cfg, err := png.DecodeConfig(f)
	require.NoError(t, err, "%s — не PNG", path)
	return cfg.Height
}
