//go:build e2e

// e2e Э5: трейс, разборы, heap snapshot, Lighthouse. Ожидания — из
// docs/plans/etap-5-perf-lighthouse-heap.md, раздел «Тесты».
package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const perfHero = `<svg xmlns="http://www.w3.org/2000/svg" width="600" height="400"><rect width="600" height="400" fill="teal"/></svg>`

func startPerfFixture(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/hero.svg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = fmt.Fprint(w, perfHero)
	})
	mux.HandleFunc("/stable", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>stable</title><body style="margin:0">
<img src="/hero.svg" width="600" height="400" alt="герой"><p>текст</p>`)
	})
	mux.HandleFunc("/shift", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>shift</title><body style="margin:0">
<div id="gap" style="height:0"></div>
<img src="/hero.svg" width="600" height="400" alt="герой"><p>текст</p>
<script>
setTimeout(() => { document.getElementById('gap').style.height = '200px'; }, 300);
setTimeout(() => { const end = performance.now() + 120; while (performance.now() < end) {} }, 500);
</script>`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func metric(t *testing.T, out, name string) float64 {
	t.Helper()
	m := regexp.MustCompile(name + `: ([0-9.]+)`).FindStringSubmatch(out)
	require.NotNil(t, m, "нет %s в выводе:\n%s", name, out)
	v, err := strconv.ParseFloat(m[1], 64)
	require.NoError(t, err)
	return v
}

func TestPerfTrace_MetricsAfterReload(t *testing.T) {
	t.Parallel()
	base := startPerfFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", base+"/shift")
	before := e.pageTargetIDs()
	dir := t.TempDir()

	shiftTrace := filepath.Join(dir, "shift.json.gz")
	r := e.mustRun("perf", "trace", "--duration", "1s", "-o", shiftTrace)
	assert.Contains(t, r.stdout, "Трейс: "+base+"/shift")
	assert.Greater(t, metric(t, r.stdout, "LCP"), 0.0)
	assert.Greater(t, metric(t, r.stdout, "CLS"), 0.0)
	assert.NotContains(t, r.stdout, "Длинные задачи: нет")
	assert.Contains(t, r.stdout, shiftTrace)
	data, err := os.ReadFile(shiftTrace)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(data, []byte{0x1f, 0x8b}), "файл .gz сжат gzip")
	assert.ElementsMatch(t, before, e.pageTargetIDs(), "перезагрузка идёт в той же вкладке")

	assert.Contains(t, e.mustRun("perf", "insight", shiftTrace, "LayoutShifts").stdout, "окно 1:")

	e.mustRun("navigate", base+"/stable")
	r = e.mustRun("perf", "trace", "--duration", "1s", "-o", filepath.Join(dir, "stable.json"))
	assert.Equal(t, 0.0, metric(t, r.stdout, "CLS"))
}

func TestPerfInsight_WorksWithoutBrowser(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	path := filepath.Join(t.TempDir(), "nolcp.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"traceEvents":[{"name":"navigationStart","cat":"blink.user_timing","ts":1000000,"pid":1,"tid":1,
	  "args":{"frame":"F1","data":{"documentLoaderURL":"http://x/","isOutermostMainFrame":true,"navigationId":"N1"}}}]}`), 0o600))

	r := e.run("perf", "insight", path, "LCPBreakdown")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "LCP не найден в трейсе")

	r = e.run("perf", "insight", path, "DocumentLatency")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, `неизвестный разбор "DocumentLatency"`)

	assert.Contains(t, e.mustRun("perf", "insight", path, "LongTasks").stdout, "длинных задач нет")
}

func TestHeapSnapshot_WritesValidFile(t *testing.T) {
	t.Parallel()
	base := startPerfFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", base+"/stable")

	target := filepath.Join(t.TempDir(), "memory")
	r := e.mustRun("heap", "snapshot", "-o", target)

	path := target + ".heapsnapshot"
	assert.Contains(t, r.stdout, path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var snapshot struct {
		Snapshot struct {
			Meta struct {
				NodeFields []string `json:"node_fields"`
			} `json:"meta"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(data, &snapshot))
	assert.NotEmpty(t, snapshot.Snapshot.Meta.NodeFields)
}

func TestAuditLighthouse_ScoresReportsAndBrowserSurvives(t *testing.T) {
	t.Parallel()
	base := startPerfFixture(t)
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", base+"/stable")
	before := e.pageTargetIDs()
	dir := t.TempDir()

	r := e.mustRun("audit", "lighthouse", "--output-dir", dir)

	for _, category := range []string{"accessibility", "seo", "best-practices", "agentic-browsing"} {
		assert.Regexp(t, `(?m)^`+category+`: \d+$`, r.stdout)
	}
	assert.FileExists(t, filepath.Join(dir, "report.json"))
	assert.FileExists(t, filepath.Join(dir, "report.html"))
	assert.Contains(t, e.mustRun("browser", "status").stdout, "браузер запущен")
	assert.ElementsMatch(t, before, e.pageTargetIDs(), "Lighthouse не оставляет и не закрывает вкладки")

	r = e.run("audit", "lighthouse", "--mode", "snapshot")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "режим snapshot не поддерживается CLI Lighthouse")

	r = e.runEnv([]string{"PATH=/usr/bin:/bin"}, "audit", "lighthouse")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "для Lighthouse нужен Node.js (npx)")
}
