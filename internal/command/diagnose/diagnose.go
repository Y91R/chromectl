// Package diagnose — команды Э5: трейс производительности, разборы, heap snapshot,
// Lighthouse. Пакет назван не perf: это имя занято разбором трейса internal/perf.
//
// ADR: docs/adr/0009-cli-bez-demona.md — start и stop трейса не разделить
// без демона, запись идёт одной командой; Lighthouse запускается через npx.
package diagnose

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Y91R/chromectl/internal/cdp"
	"github.com/Y91R/chromectl/internal/command/session"
	"github.com/Y91R/chromectl/internal/perf"
)

const (
	lighthouseVersion  = "13.4.1"
	traceCollectWindow = time.Minute
)

// traceCategories — TracingDefaultCategories DevTools плюс JS sampling и скриншоты:
// трейс открывается в DevTools тем же набором.
var traceCategories = []string{
	"-*",
	"blink.console", "blink.user_timing", "loading", "devtools.timeline",
	"disabled-by-default-devtools.target-rundown", "disabled-by-default-devtools.timeline.frame",
	"disabled-by-default-devtools.timeline.stack", "disabled-by-default-devtools.timeline",
	"disabled-by-default-devtools.v8-source-rundown-sources", "disabled-by-default-devtools.v8-source-rundown",
	"disabled-by-default-layout_shift.debug", "disabled-by-default-v8.inspector",
	"disabled-by-default-v8.cpu_profiler.hires", "disabled-by-default-lighthouse",
	"v8.execute", "v8", "cppgc", "navigation,rail",
	"disabled-by-default-v8.cpu_profiler", "disabled-by-default-devtools.screenshot",
}

var lighthouseCategories = []string{"accessibility", "seo", "best-practices", "agentic-browsing"}

type TraceOptions struct {
	Reload   bool
	Duration time.Duration
	// Output — файл трейса; .gz сжимается. Пустой — временный файл.
	Output string
}

type TraceResult struct {
	Path    string       `json:"path"`
	Metrics perf.Metrics `json:"metrics"`
}

func Trace(ctx context.Context, env session.Env, opts TraceOptions) (res TraceResult, err error) {
	p, err := session.Open(ctx, env, &session.DialogFlag{})
	if err != nil {
		return TraceResult{}, err
	}
	defer func() {
		_, closeErr := p.Close(ctx)
		err = errors.Join(err, closeErr)
	}()

	info, err := session.GetTargetInfo(ctx, p.Client, p.ID)
	if err != nil {
		return TraceResult{}, err
	}
	if opts.Reload {
		// Трейс начинается на пустой странице, чтобы в него попала вся загрузка.
		if err := navigateAndWait(ctx, p, "about:blank", true); err != nil {
			return TraceResult{}, err
		}
	}

	sub := p.Client.Subscribe("", "Tracing.dataCollected", "Tracing.tracingComplete")
	defer sub.Close()
	collector := startTraceCollector(ctx, sub)
	if err := p.Client.Call(ctx, "", "Tracing.start", map[string]any{
		"traceConfig":  map[string]any{"includedCategories": traceCategories, "excludedCategories": []string{"*"}},
		"transferMode": "ReportEvents",
	}, nil); err != nil {
		return TraceResult{}, err
	}

	recordErr := record(ctx, p, info.URL, opts)
	if err := p.Client.Call(context.WithoutCancel(ctx), "", "Tracing.end", nil, nil); err != nil {
		return TraceResult{}, errors.Join(recordErr, err)
	}
	events, collectErr := collector.wait(ctx)
	if err := errors.Join(recordErr, collectErr); err != nil {
		return TraceResult{}, err
	}

	data, err := json.Marshal(map[string]any{"traceEvents": events})
	if err != nil {
		return TraceResult{}, fmt.Errorf("сериализация трейса: %w", err)
	}
	metrics, err := perf.Analyze(data)
	if err != nil {
		return TraceResult{}, err
	}
	path, err := writeTrace(opts.Output, data)
	if err != nil {
		return TraceResult{}, err
	}
	return TraceResult{Path: path, Metrics: metrics}, nil
}

func record(ctx context.Context, p *session.Page, url string, opts TraceOptions) error {
	if opts.Reload {
		if err := navigateAndWait(ctx, p, url, false); err != nil {
			return err
		}
	}
	timer := time.NewTimer(opts.Duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func navigateAndWait(ctx context.Context, p *session.Page, url string, allowBlank bool) error {
	var res struct {
		ErrorText string `json:"errorText"`
	}
	if err := p.Client.Call(ctx, p.SessionID, "Page.navigate", map[string]any{"url": url}, &res); err != nil {
		return err
	}
	if res.ErrorText != "" {
		return fmt.Errorf("переход на %s: %s", url, res.ErrorText)
	}
	return session.WaitDocumentReady(ctx, p.Client, p.SessionID, allowBlank)
}

// traceCollector забирает события трейса в фоне: за несколько секунд записи их
// десятки тысяч, и очередь подписки переполнилась бы, пока команда ждёт загрузку.
type traceCollector struct {
	done   chan struct{}
	events []json.RawMessage
	err    error
}

// startTraceCollector запускает сборщик. Горутина принадлежит Trace и завершается
// на tracingComplete, при закрытии подписки (defer в Trace) или отмене контекста.
func startTraceCollector(ctx context.Context, sub *cdp.Subscription) *traceCollector {
	tc := &traceCollector{done: make(chan struct{})}
	go func() {
		defer close(tc.done)
		for {
			select {
			case ev, open := <-sub.C:
				if !open {
					tc.err = fmt.Errorf("трейс неполный: %w", sub.Err())
					return
				}
				switch ev.Method {
				case "Tracing.dataCollected":
					var p struct {
						Value []json.RawMessage `json:"value"`
					}
					if err := json.Unmarshal(ev.Params, &p); err != nil {
						tc.err = fmt.Errorf("разбор трейса: %w", err)
						return
					}
					tc.events = append(tc.events, p.Value...)
				case "Tracing.tracingComplete":
					return
				}
			case <-ctx.Done():
				tc.err = ctx.Err()
				return
			}
		}
	}()
	return tc
}

func (tc *traceCollector) wait(ctx context.Context) ([]json.RawMessage, error) {
	timer := time.NewTimer(traceCollectWindow)
	defer timer.Stop()
	select {
	case <-tc.done:
		return tc.events, tc.err
	case <-timer.C:
		return nil, errors.New("трейс не завершился за минуту после Tracing.end")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func writeTrace(output string, data []byte) (string, error) {
	path := output
	if path == "" {
		f, err := os.CreateTemp("", "chromectl-trace-*.json")
		if err != nil {
			return "", fmt.Errorf("временный файл трейса: %w", err)
		}
		path = f.Name()
		_ = f.Close()
	}
	if strings.HasSuffix(path, ".gz") {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(data); err != nil {
			return "", fmt.Errorf("сжатие трейса: %w", err)
		}
		if err := zw.Close(); err != nil {
			return "", fmt.Errorf("сжатие трейса: %w", err)
		}
		data = buf.Bytes()
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("запись трейса: %w", err)
	}
	return path, nil
}

// Insight разбирает сохранённый трейс; браузер не нужен.
func Insight(path, name string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("чтение трейса: %w", err)
	}
	if bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return "", fmt.Errorf("распаковка трейса: %w", err)
		}
		if data, err = io.ReadAll(zr); err != nil {
			return "", fmt.Errorf("распаковка трейса: %w", err)
		}
	}
	metrics, err := perf.Analyze(data)
	if err != nil {
		return "", err
	}
	return perf.Insight(metrics, name)
}

// HeapSnapshot сохраняет снимок кучи вкладки в файл .heapsnapshot.
func HeapSnapshot(ctx context.Context, env session.Env, output string) (path string, err error) {
	path = output
	if !strings.HasSuffix(path, ".heapsnapshot") {
		path += ".heapsnapshot"
	}
	p, err := session.Open(ctx, env, nil)
	if err != nil {
		return "", err
	}
	defer func() {
		_, closeErr := p.Close(ctx)
		err = errors.Join(err, closeErr)
	}()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("файл heap snapshot: %w", err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()

	chunks := p.Client.Subscribe(p.SessionID, "HeapProfiler.addHeapSnapshotChunk")
	defer chunks.Close()
	if err := p.Client.Call(ctx, p.SessionID, "HeapProfiler.enable", nil, nil); err != nil {
		return "", err
	}
	// Чанки приходят событиями раньше ответа на takeHeapSnapshot, поэтому к его
	// возврату все они уже в очереди подписки.
	if err := p.Client.Call(ctx, p.SessionID, "HeapProfiler.takeHeapSnapshot", map[string]any{"reportProgress": false}, nil); err != nil {
		return "", err
	}
	for {
		select {
		case ev, open := <-chunks.C:
			if !open {
				return "", fmt.Errorf("heap snapshot неполный: %w", chunks.Err())
			}
			var chunk struct {
				Chunk string `json:"chunk"`
			}
			if err := json.Unmarshal(ev.Params, &chunk); err != nil {
				return "", fmt.Errorf("разбор чанка heap snapshot: %w", err)
			}
			if _, err := f.WriteString(chunk.Chunk); err != nil {
				return "", fmt.Errorf("запись heap snapshot: %w", err)
			}
		default:
			_ = p.Client.Call(ctx, p.SessionID, "HeapProfiler.disable", nil, nil)
			return path, nil
		}
	}
}

type LighthouseOptions struct {
	Device    string
	Mode      string
	OutputDir string
}

type CategoryScore struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Score *float64 `json:"score"`
}

type LighthouseResult struct {
	URL     string          `json:"url"`
	Mode    string          `json:"mode"`
	Device  string          `json:"device"`
	Scores  []CategoryScore `json:"scores"`
	Failed  int             `json:"failedAudits"`
	Passed  int             `json:"passedAudits"`
	Reports []string        `json:"reports"`
}

func Lighthouse(ctx context.Context, env session.Env, opts LighthouseOptions) (LighthouseResult, error) {
	if opts.Mode == "snapshot" {
		return LighthouseResult{}, errors.New("режим snapshot не поддерживается CLI Lighthouse: используйте --mode navigation")
	}
	if opts.Mode != "navigation" {
		return LighthouseResult{}, fmt.Errorf("--mode: navigation или snapshot, получено %q", opts.Mode)
	}
	if opts.Device != "desktop" && opts.Device != "mobile" {
		return LighthouseResult{}, fmt.Errorf("--device: desktop или mobile, получено %q", opts.Device)
	}
	npx, err := exec.LookPath("npx")
	if err != nil {
		return LighthouseResult{}, errors.New("для Lighthouse нужен Node.js (npx) в PATH")
	}

	st, c, err := session.Connect(ctx, env)
	if err != nil {
		return LighthouseResult{}, err
	}
	pageID, err := session.ResolvePage(ctx, c, env, st)
	if err != nil {
		_ = c.Close()
		return LighthouseResult{}, err
	}
	info, err := session.GetTargetInfo(ctx, c, pageID)
	_ = c.Close()
	if err != nil {
		return LighthouseResult{}, err
	}

	dir := opts.OutputDir
	if dir == "" {
		if dir, err = os.MkdirTemp("", "chromectl-lighthouse-"); err != nil {
			return LighthouseResult{}, err
		}
	} else if err := os.MkdirAll(dir, 0o700); err != nil {
		return LighthouseResult{}, err
	}

	args := []string{"--yes", "lighthouse@" + lighthouseVersion, info.URL,
		"--port=" + strconv.Itoa(st.Port),
		"--output=json", "--output=html",
		"--output-path=" + filepath.Join(dir, "report"),
		"--only-categories=" + strings.Join(lighthouseCategories, ","),
		"--max-wait-for-load=30000", "--quiet"}
	if opts.Device == "desktop" {
		args = append(args, "--form-factor=desktop", "--screenEmulation.mobile=false",
			"--screenEmulation.width=1350", "--screenEmulation.height=940", "--screenEmulation.deviceScaleFactor=1")
	} else {
		args = append(args, "--form-factor=mobile", "--screenEmulation.mobile=true",
			"--screenEmulation.width=412", "--screenEmulation.height=823", "--screenEmulation.deviceScaleFactor=1.75")
	}
	out, err := exec.CommandContext(ctx, npx, args...).CombinedOutput()
	if err != nil {
		return LighthouseResult{}, fmt.Errorf("lighthouse: %w\n%s", err, tail(string(out), 2000))
	}

	reports, err := normalizeReports(dir)
	if err != nil {
		return LighthouseResult{}, err
	}
	return parseReport(reports, opts)
}

// normalizeReports приводит имена отчётов к report.json и report.html: при
// нескольких форматах CLI добавляет к --output-path своё окончание.
func normalizeReports(dir string) ([]string, error) {
	var reports []string
	for _, ext := range []string{".json", ".html"} {
		want := filepath.Join(dir, "report"+ext)
		if _, err := os.Stat(want); err != nil {
			matches, _ := filepath.Glob(filepath.Join(dir, "report*"+ext))
			if len(matches) == 0 {
				return nil, fmt.Errorf("lighthouse не записал отчёт %s в %s", ext, dir)
			}
			if err := os.Rename(matches[0], want); err != nil {
				return nil, err
			}
		}
		reports = append(reports, want)
	}
	return reports, nil
}

func parseReport(reports []string, opts LighthouseOptions) (LighthouseResult, error) {
	data, err := os.ReadFile(reports[0])
	if err != nil {
		return LighthouseResult{}, err
	}
	var lhr struct {
		MainDocumentURL string `json:"mainDocumentUrl"`
		Categories      map[string]struct {
			Title string   `json:"title"`
			Score *float64 `json:"score"`
		} `json:"categories"`
		Audits map[string]struct {
			Score *float64 `json:"score"`
		} `json:"audits"`
	}
	if err := json.Unmarshal(data, &lhr); err != nil {
		return LighthouseResult{}, fmt.Errorf("разбор отчёта Lighthouse: %w", err)
	}
	res := LighthouseResult{URL: lhr.MainDocumentURL, Mode: opts.Mode, Device: opts.Device, Reports: reports}
	for _, id := range lighthouseCategories {
		if cat, ok := lhr.Categories[id]; ok {
			res.Scores = append(res.Scores, CategoryScore{ID: id, Title: cat.Title, Score: cat.Score})
		}
	}
	for _, a := range lhr.Audits {
		switch {
		case a.Score == nil:
		case *a.Score < 1:
			res.Failed++
		default:
			res.Passed++
		}
	}
	return res, nil
}

func (r LighthouseResult) Text() string {
	lines := []string{fmt.Sprintf("Lighthouse: %s (%s, %s)", r.URL, r.Mode, r.Device)}
	for _, s := range r.Scores {
		value := "—"
		if s.Score != nil {
			value = strconv.Itoa(int(*s.Score*100 + 0.5))
		}
		lines = append(lines, s.ID+": "+value)
	}
	lines = append(lines,
		fmt.Sprintf("аудитов: не пройдено %d, пройдено %d", r.Failed, r.Passed),
		"отчёты: "+strings.Join(r.Reports, ", "))
	return strings.Join(lines, "\n")
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
