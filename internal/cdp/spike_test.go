//go:build spike

// Спайк Э1: проверяет на реальном Chrome допущения, на которых стоит модель
// «одна команда — одно подключение». Каждый подтест — одно допущение из
// docs/plans/etap-1-brauzer-i-vkladki.md; каждое новое подключение имитирует
// отдельный вызов CLI. У каждого подтеста своя вкладка, чтобы заблокированная
// диалогом страница не портила соседние проверки. В make verify не входит.
//
// Запуск: go test -tags spike ./internal/cdp/ -run TestSpike -v -count=1
//
// ADR: docs/adr/0009-cli-bez-demona.md — результаты пишутся в раздел «Результаты спайка».
package cdp_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const callTimeout = 10 * time.Second

type message struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type conn struct {
	t       *testing.T
	ws      *websocket.Conn
	nextID  atomic.Int64
	mu      sync.Mutex
	waiters map[int64]chan message
	events  chan message
	done    chan struct{}
}

func dial(t *testing.T, wsURL string) *conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient: &http.Client{Transport: &http.Transport{Proxy: nil}},
	})
	if err != nil {
		t.Fatalf("подключение к %s: %v", wsURL, err)
	}
	ws.SetReadLimit(64 << 20)

	c := &conn{
		t:       t,
		ws:      ws,
		waiters: map[int64]chan message{},
		events:  make(chan message, 4096),
		done:    make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// readLoop принадлежит conn и завершается, когда close закрывает websocket.
func (c *conn) readLoop() {
	defer close(c.done)
	for {
		_, data, err := c.ws.Read(context.Background())
		if err != nil {
			return
		}
		var msg message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.ID != 0 {
			c.mu.Lock()
			ch := c.waiters[msg.ID]
			delete(c.waiters, msg.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
			continue
		}
		select {
		case c.events <- msg:
		default:
			c.t.Logf("буфер событий переполнен, потеряно %s", msg.Method)
		}
	}
}

func (c *conn) close() {
	_ = c.ws.Close(websocket.StatusNormalClosure, "")
	<-c.done
}

func (c *conn) callTimeout(timeout time.Duration, sessionID, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan message, 1)
	c.mu.Lock()
	c.waiters[id] = ch
	c.mu.Unlock()

	if params == nil {
		params = struct{}{}
	}
	rawParams, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(message{ID: id, Method: method, Params: rawParams, SessionID: sessionID})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := c.ws.Write(ctx, websocket.MessageText, data); err != nil {
		return nil, fmt.Errorf("%s: запись: %w", method, err)
	}
	select {
	case msg := <-ch:
		if msg.Error != nil {
			return nil, fmt.Errorf("%s: %d %s", method, msg.Error.Code, msg.Error.Message)
		}
		return msg.Result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("%s: таймаут ответа за %s", method, timeout)
	}
}

func (c *conn) call(sessionID, method string, params any) (json.RawMessage, error) {
	return c.callTimeout(callTimeout, sessionID, method, params)
}

func (c *conn) mustCall(sessionID, method string, params any) json.RawMessage {
	c.t.Helper()
	res, err := c.call(sessionID, method, params)
	if err != nil {
		c.t.Fatal(err)
	}
	return res
}

func (c *conn) waitEvent(timeout time.Duration, sessionID, method string, match func(json.RawMessage) bool) (json.RawMessage, error) {
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-c.events:
			if msg.Method != method || msg.SessionID != sessionID {
				continue
			}
			if match == nil || match(msg.Params) {
				return msg.Params, nil
			}
		case <-deadline:
			return nil, fmt.Errorf("событие %s не пришло за %s", method, timeout)
		}
	}
}

func (c *conn) attach(targetID string) string {
	c.t.Helper()
	var res struct {
		SessionID string `json:"sessionId"`
	}
	unmarshal(c.t, c.mustCall("", "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}), &res)
	return res.SessionID
}

func (c *conn) detach(sessionID string) {
	c.t.Helper()
	c.mustCall("", "Target.detachFromTarget", map[string]any{"sessionId": sessionID})
}

func (c *conn) evalTimeout(timeout time.Duration, sessionID, expression string) (json.RawMessage, error) {
	raw, err := c.callTimeout(timeout, sessionID, "Runtime.evaluate", map[string]any{
		"expression": expression, "returnByValue": true, "awaitPromise": true,
	})
	if err != nil {
		return nil, err
	}
	var res struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		return nil, fmt.Errorf("исключение в %q: %s", expression, res.ExceptionDetails)
	}
	return res.Result.Value, nil
}

func (c *conn) eval(sessionID, expression string) json.RawMessage {
	c.t.Helper()
	v, err := c.evalTimeout(callTimeout, sessionID, expression)
	if err != nil {
		c.t.Fatal(err)
	}
	return v
}

func (c *conn) pageTargets() []string {
	c.t.Helper()
	var res struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
		} `json:"targetInfos"`
	}
	unmarshal(c.t, c.mustCall("", "Target.getTargets", nil), &res)
	var ids []string
	for _, info := range res.TargetInfos {
		if info.Type == "page" {
			ids = append(ids, info.TargetID)
		}
	}
	return ids
}

func unmarshal(t *testing.T, data json.RawMessage, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("разбор %s: %v", data, err)
	}
}

type browser struct {
	port  string
	wsURL string
}

func startChrome(t *testing.T) browser {
	t.Helper()
	path := os.Getenv("CHROME_PATH")
	if path == "" {
		path = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	}
	dir, err := os.MkdirTemp("", "chromectl-spike-")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path, "--headless=new", "--remote-debugging-port=0", "--user-data-dir="+dir,
		"--no-first-run", "--no-default-browser-check", "about:blank")
	if err := cmd.Start(); err != nil {
		t.Fatalf("запуск Chrome (%s): %v", path, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = os.RemoveAll(dir)
	})

	portFile := filepath.Join(dir, "DevToolsActivePort")
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		f, err := os.Open(portFile)
		if err == nil {
			sc := bufio.NewScanner(f)
			var lines []string
			for sc.Scan() {
				lines = append(lines, sc.Text())
			}
			_ = f.Close()
			if len(lines) >= 2 {
				return browser{port: lines[0], wsURL: "ws://127.0.0.1:" + lines[0] + lines[1]}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Chrome не открыл debug-порт: нет %s", portFile)
	return browser{}
}

func startFixtures(t *testing.T) string {
	t.Helper()
	frame := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><input id="inner" aria-label="inner">`)
	}))
	t.Cleanup(frame.Close)
	// localhost и 127.0.0.1 — разные сайты, поэтому iframe уходит в отдельный процесс (OOPIF).
	frameURL := strings.Replace(frame.URL, "127.0.0.1", "localhost", 1)

	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Маркеры консоли — ASCII: Chrome может экранировать не-ASCII в JSON, и
		// поиск подстроки дал бы ложный отрицательный результат.
		_, _ = fmt.Fprintf(w, `<!doctype html><title>spike</title>
<button id="btn">btn</button>
<iframe id="f" src="%s/frame"></iframe>
<script>console.log("marker-log-before-cli"); console.error("marker-error-before-cli");</script>`, frameURL)
	}))
	t.Cleanup(page.Close)
	return page.URL
}

// newTab открывает вкладку отдельным подключением и дожидается загрузки, как это
// сделала бы команда pages new.
func newTab(t *testing.T, b browser, url string) string {
	t.Helper()
	c := dial(t, b.wsURL)
	defer c.close()
	var created struct {
		TargetID string `json:"targetId"`
	}
	unmarshal(t, c.mustCall("", "Target.createTarget", map[string]any{"url": url}), &created)
	sid := c.attach(created.TargetID)
	deadline := time.Now().Add(callTimeout)
	for string(c.eval(sid, "document.readyState")) != `"complete"` {
		if time.Now().After(deadline) {
			t.Fatal("страница не загрузилась")
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.detach(sid)
	return created.TargetID
}

func TestSpike(t *testing.T) {
	b := startChrome(t)
	pageURL := startFixtures(t)

	t.Run("1_TabSurvivesDetachAndDisconnect", func(t *testing.T) {
		tid := newTab(t, b, pageURL)
		c0 := dial(t, b.wsURL)
		before := c0.pageTargets()
		c0.close()

		c := dial(t, b.wsURL)
		sid := c.attach(tid)
		c.eval(sid, "1")
		c.detach(sid)
		c.close()

		c2 := dial(t, b.wsURL)
		defer c2.close()
		after := c2.pageTargets()
		if len(after) != len(before) || !contains(after, tid) {
			t.Fatalf("вкладки до %v, после %v, ожидалась %s", before, after, tid)
		}

		// Контроль: закрытие websocket без явного detach тоже не должно закрывать вкладку.
		c3 := dial(t, b.wsURL)
		sid3 := c3.attach(tid)
		c3.eval(sid3, "1")
		c3.close()
		c4 := dial(t, b.wsURL)
		defer c4.close()
		if got := c4.pageTargets(); !contains(got, tid) {
			t.Fatalf("вкладка закрылась после обрыва websocket без detach: %v", got)
		}
	})

	t.Run("2_BackendNodeIDStableAcrossSessions", func(t *testing.T) {
		tid := newTab(t, b, pageURL)
		c := dial(t, b.wsURL)
		sid := c.attach(tid)
		btnBackendID := backendNodeID(t, c, sid, "#btn")

		var ax struct {
			Nodes []struct {
				BackendDOMNodeID int64 `json:"backendDOMNodeId"`
			} `json:"nodes"`
		}
		unmarshal(t, c.mustCall(sid, "Accessibility.getFullAXTree", nil), &ax)
		inAX := false
		for _, n := range ax.Nodes {
			if n.BackendDOMNodeID == btnBackendID {
				inAX = true
			}
		}
		t.Logf("backendNodeId кнопки %d, есть в AX-дереве страницы: %v, узлов AX: %d", btnBackendID, inAX, len(ax.Nodes))

		c.mustCall(sid, "Target.setAutoAttach", map[string]any{"autoAttach": true, "waitForDebuggerOnStart": false, "flatten": true})
		params, err := c.waitEvent(5*time.Second, sid, "Target.attachedToTarget", func(p json.RawMessage) bool {
			return strings.Contains(string(p), `"type":"iframe"`)
		})
		if err != nil {
			t.Fatalf("OOPIF не подключился через setAutoAttach: %v", err)
		}
		var attached struct {
			SessionID  string `json:"sessionId"`
			TargetInfo struct {
				TargetID string `json:"targetId"`
			} `json:"targetInfo"`
		}
		unmarshal(t, params, &attached)
		frameTargetID := attached.TargetInfo.TargetID
		innerBackendID := backendNodeID(t, c, attached.SessionID, "#inner")
		t.Logf("OOPIF target %s, backendNodeId поля %d", frameTargetID, innerBackendID)
		c.detach(sid)
		c.close()

		c2 := dial(t, b.wsURL)
		defer c2.close()
		sid2 := c2.attach(tid)
		if got := idOfBackendNode(t, c2, sid2, btnBackendID); got != "btn" {
			t.Fatalf("в новой сессии backendNodeId %d указывает на %q, ожидалось btn", btnBackendID, got)
		}
		fsid := c2.attach(frameTargetID)
		if got := idOfBackendNode(t, c2, fsid, innerBackendID); got != "inner" {
			t.Fatalf("в новой сессии OOPIF backendNodeId %d указывает на %q, ожидалось inner", innerBackendID, got)
		}
	})

	t.Run("3_ConsoleReplayedToNewSession", func(t *testing.T) {
		tid := newTab(t, b, pageURL)
		c := dial(t, b.wsURL)
		defer c.close()

		sid := c.attach(tid)
		c.mustCall(sid, "Runtime.enable", nil)
		_, runtimeErr := c.waitEvent(2*time.Second, sid, "Runtime.consoleAPICalled", func(p json.RawMessage) bool {
			return strings.Contains(string(p), "marker-log-before-cli")
		})
		t.Logf("Runtime.enable повторяет консоль: %v (%v)", runtimeErr == nil, runtimeErr)
		c.detach(sid)

		sid2 := c.attach(tid)
		c.mustCall(sid2, "Console.enable", nil)
		_, consoleErr := c.waitEvent(2*time.Second, sid2, "Console.messageAdded", func(p json.RawMessage) bool {
			return strings.Contains(string(p), "marker-log-before-cli")
		})
		t.Logf("Console.enable повторяет консоль: %v (%v)", consoleErr == nil, consoleErr)
		c.detach(sid2)

		sid3 := c.attach(tid)
		c.mustCall(sid3, "Log.enable", nil)
		_, logErr := c.waitEvent(2*time.Second, sid3, "Log.entryAdded", nil)
		t.Logf("Log.enable отдаёт записи: %v (%v)", logErr == nil, logErr)
		c.detach(sid3)

		if runtimeErr != nil && consoleErr != nil {
			t.Fatal("ни Runtime.enable, ни Console.enable не отдали сообщения, залогированные до подключения")
		}
	})

	t.Run("4_EmulationResetOnDetach", func(t *testing.T) {
		const query = "matchMedia('(prefers-color-scheme: dark)').matches"
		tid := newTab(t, b, pageURL)
		c := dial(t, b.wsURL)
		sid := c.attach(tid)
		c.mustCall(sid, "Emulation.setEmulatedMedia", map[string]any{
			"features": []map[string]string{{"name": "prefers-color-scheme", "value": "dark"}},
		})
		if got := string(c.eval(sid, query)); got != "true" {
			t.Fatalf("эмуляция не применилась в своей сессии: %s", got)
		}
		c.detach(sid)
		c.close()

		c2 := dial(t, b.wsURL)
		defer c2.close()
		sid2 := c2.attach(tid)
		got := string(c2.eval(sid2, query))
		t.Logf("тёмная тема после отключения сессии: %s", got)
		if got != "false" {
			t.Fatalf("эмуляция пережила отключение сессии: %s", got)
		}
	})

	t.Run("5_DialogInvisibleToNewSession_ReloadUnblocks", func(t *testing.T) {
		tid := newTab(t, b, pageURL)
		c := dial(t, b.wsURL)
		sid := c.attach(tid)
		c.mustCall(sid, "Page.enable", nil)
		c.mustCall(sid, "Runtime.evaluate", map[string]any{"expression": "setTimeout(() => alert('spike'), 0)"})
		if _, err := c.waitEvent(5*time.Second, sid, "Page.javascriptDialogOpening", nil); err != nil {
			t.Fatalf("диалог не открылся: %v", err)
		}
		c.detach(sid)
		c.close()

		c2 := dial(t, b.wsURL)
		defer c2.close()
		sid2 := c2.attach(tid)
		_, blockedErr := c2.evalTimeout(2*time.Second, sid2, "1")
		t.Logf("страница после отключения первой сессии заблокирована диалогом: %v (%v)", blockedErr != nil, blockedErr)

		_, handleBeforeEnableErr := c2.callTimeout(3*time.Second, sid2, "Page.handleJavaScriptDialog", map[string]any{"accept": true})
		t.Logf("handleJavaScriptDialog без Page.enable: %v", handleBeforeEnableErr)

		handleErr := handleBeforeEnableErr
		if handleBeforeEnableErr != nil {
			_, enableErr := c2.callTimeout(3*time.Second, sid2, "Page.enable", nil)
			_, eventErr := c2.waitEvent(3*time.Second, sid2, "Page.javascriptDialogOpening", nil)
			_, handleErr = c2.callTimeout(3*time.Second, sid2, "Page.handleJavaScriptDialog", map[string]any{"accept": true})
			t.Logf("Page.enable: %v; повтор события: %v; handleJavaScriptDialog после enable: %v", enableErr, eventErr, handleErr)
		}

		// Допущение опровергнуто (ADR-0009): тест фиксирует фактическое поведение и
		// выход из тупика. Позеленевшее закрытие диалога из новой сессии значит, что
		// Chrome изменился и решение о страже диалогов стоит пересмотреть.
		if blockedErr == nil || handleErr == nil {
			t.Fatalf("поведение Chrome изменилось: страница заблокирована=%v, диалог закрыт новой сессией=%v — пересмотреть ADR-0009",
				blockedErr != nil, handleErr == nil)
		}

		c3 := dial(t, b.wsURL)
		defer c3.close()
		sid3 := c3.attach(tid)
		if _, err := c3.callTimeout(3*time.Second, sid3, "Page.reload", nil); err != nil {
			t.Fatalf("Page.reload заблокированной вкладки: %v", err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := c3.evalTimeout(time.Second, sid3, "1"); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("после Page.reload страница не отвечает")
			}
		}
	})

	t.Run("6_SetWindowBoundsInHeadless", func(t *testing.T) {
		tid := newTab(t, b, pageURL)
		c := dial(t, b.wsURL)
		var win struct {
			WindowID int `json:"windowId"`
		}
		unmarshal(t, c.mustCall("", "Browser.getWindowForTarget", map[string]any{"targetId": tid}), &win)
		c.mustCall("", "Browser.setWindowBounds", map[string]any{
			"windowId": win.WindowID,
			"bounds":   map[string]any{"width": 800, "height": 600, "windowState": "normal"},
		})
		c.close()

		c2 := dial(t, b.wsURL)
		defer c2.close()
		sid := c2.attach(tid)
		outer := string(c2.eval(sid, "outerWidth"))
		inner := string(c2.eval(sid, "innerWidth"))
		t.Logf("после setWindowBounds 800x600 в новой сессии: outerWidth=%s innerWidth=%s", outer, inner)
		if outer != "800" {
			t.Fatalf("outerWidth=%s, ожидалось 800", outer)
		}
	})

	t.Run("7_LighthouseReusesRunningChrome", func(t *testing.T) {
		tid := newTab(t, b, pageURL)
		c0 := dial(t, b.wsURL)
		before := c0.pageTargets()
		c0.close()

		npx, err := exec.LookPath("npx")
		if err != nil {
			t.Fatal("npx не найден: Lighthouse без Node.js не проверить")
		}
		out := filepath.Join(t.TempDir(), "lh.json")
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, npx, "--yes", "lighthouse@12", pageURL,
			"--port="+b.port, "--output=json", "--output-path="+out,
			"--only-categories=seo", "--quiet")
		combined, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("lighthouse: %v\n%s", err, tail(string(combined), 2000))
		}

		var report struct {
			Categories map[string]struct {
				Score *float64 `json:"score"`
			} `json:"categories"`
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		seo, ok := report.Categories["seo"]
		if !ok || seo.Score == nil {
			t.Fatalf("в отчёте нет оценки seo: %s", tail(string(data), 500))
		}

		c := dial(t, b.wsURL)
		defer c.close()
		after := c.pageTargets()
		t.Logf("seo=%.2f; вкладки до %d, после %d", *seo.Score, len(before), len(after))
		if len(after) != len(before) || !contains(after, tid) {
			t.Fatalf("Lighthouse изменил набор вкладок: до %v, после %v", before, after)
		}
	})
}

// Спайк Э3: какие override эмуляции снимаются при отключении сессии. Допущение 4
// Э1 проверяло только тему; e2e Э3 показал, что ширина viewport после сброса
// осталась. Каждый override — на своей вкладке, итог — в логе теста.
func TestSpikeOverridesOnDetach(t *testing.T) {
	b := startChrome(t)
	pageURL := startFixtures(t)

	type override struct {
		name   string
		set    func(c *conn, sid string)
		probe  string
		expect bool
	}
	overrides := []override{
		{
			name: "setEmulatedMedia (тема)",
			set: func(c *conn, sid string) {
				c.mustCall(sid, "Emulation.setEmulatedMedia", map[string]any{
					"features": []map[string]string{{"name": "prefers-color-scheme", "value": "dark"}},
				})
			},
			probe: "matchMedia('(prefers-color-scheme: dark)').matches",
		},
		{
			name: "setDeviceMetricsOverride (viewport)",
			set: func(c *conn, sid string) {
				c.mustCall(sid, "Emulation.setDeviceMetricsOverride", map[string]any{
					"width": 390, "height": 844, "deviceScaleFactor": 1, "mobile": false,
				})
			},
			probe: "innerWidth + 'x' + innerHeight",
		},
		{
			name: "setTouchEmulationEnabled",
			set: func(c *conn, sid string) {
				c.mustCall(sid, "Emulation.setTouchEmulationEnabled", map[string]any{"enabled": true, "maxTouchPoints": 1})
			},
			probe: "navigator.maxTouchPoints",
		},
		{
			name: "setUserAgentOverride",
			set: func(c *conn, sid string) {
				c.mustCall(sid, "Emulation.setUserAgentOverride", map[string]any{"userAgent": "spike-agent"})
			},
			probe: "navigator.userAgent",
		},
		{
			name: "emulateNetworkConditions (offline)",
			set: func(c *conn, sid string) {
				c.mustCall(sid, "Network.enable", nil)
				c.mustCall(sid, "Network.emulateNetworkConditions", map[string]any{
					"offline": true, "latency": 0, "downloadThroughput": 0, "uploadThroughput": 0,
				})
			},
			probe: "navigator.onLine",
		},
	}

	var survived []string
	for _, o := range overrides {
		tid := newTab(t, b, pageURL)

		c0 := dial(t, b.wsURL)
		s0 := c0.attach(tid)
		before := string(c0.eval(s0, o.probe))
		c0.detach(s0)
		c0.close()

		c := dial(t, b.wsURL)
		sid := c.attach(tid)
		o.set(c, sid)
		during := string(c.eval(sid, o.probe))
		c.detach(sid)
		c.close()

		c2 := dial(t, b.wsURL)
		sid2 := c2.attach(tid)
		after := string(c2.eval(sid2, o.probe))
		c2.detach(sid2)
		c2.close()

		kept := after == during && during != before
		t.Logf("%-40s до %s, с override %s, после отключения %s, сохранился: %v", o.name, before, during, after, kept)
		if during == before {
			t.Errorf("%s: override не применился в своей сессии (%s)", o.name, during)
		}
		if kept {
			survived = append(survived, o.name)
		}
	}
	t.Logf("переживают отключение сессии: %v", survived)
}

// Спайк Э5: какие события пишет Chrome 153 в трейс с категориями DevTools и какие
// у них поля. По выводу написан разбор метрик в internal/perf; трейс содержит события
// чужих target (расширения, служебные страницы), их нужно отсекать по фрейму.
func TestSpikeTrace(t *testing.T) {
	b := startChrome(t)

	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hero.svg" {
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = fmt.Fprint(w, `<svg xmlns="http://www.w3.org/2000/svg" width="600" height="400"><rect width="600" height="400" fill="teal"/></svg>`)
			return
		}
		_, _ = fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>trace</title><body style="margin:0">
<div id="shift" style="height:0"></div>
<img src="/hero.svg" width="600" height="400">
<p>текст</p>
<script>
setTimeout(() => { document.getElementById('shift').style.height = '200px'; }, 300);
setTimeout(() => { const end = performance.now() + 120; while (performance.now() < end) {} }, 500);
</script>`)
	}))
	t.Cleanup(page.Close)

	c := dial(t, b.wsURL)
	defer c.close()
	var created struct {
		TargetID string `json:"targetId"`
	}
	unmarshal(t, c.mustCall("", "Target.createTarget", map[string]any{"url": "about:blank"}), &created)
	sid := c.attach(created.TargetID)
	c.mustCall(sid, "Page.enable", nil)

	categories := []string{"-*", "blink.console", "blink.user_timing", "loading", "devtools.timeline",
		"disabled-by-default-devtools.target-rundown", "disabled-by-default-devtools.timeline.frame",
		"disabled-by-default-devtools.timeline.stack", "disabled-by-default-devtools.timeline",
		"disabled-by-default-devtools.v8-source-rundown-sources", "disabled-by-default-devtools.v8-source-rundown",
		"disabled-by-default-layout_shift.debug", "disabled-by-default-v8.inspector",
		"disabled-by-default-v8.cpu_profiler.hires", "disabled-by-default-lighthouse", "v8.execute", "v8", "cppgc",
		"navigation,rail", "disabled-by-default-v8.cpu_profiler", "disabled-by-default-devtools.screenshot"}

	complete := make(chan struct{})
	var chunks []json.RawMessage
	var mu sync.Mutex
	go func() {
		for msg := range c.events {
			switch msg.Method {
			case "Tracing.dataCollected":
				var p struct {
					Value []json.RawMessage `json:"value"`
				}
				_ = json.Unmarshal(msg.Params, &p)
				mu.Lock()
				chunks = append(chunks, p.Value...)
				mu.Unlock()
			case "Tracing.tracingComplete":
				close(complete)
				return
			}
		}
	}()

	c.mustCall("", "Tracing.start", map[string]any{
		"traceConfig":  map[string]any{"includedCategories": categories, "excludedCategories": []string{"*"}},
		"transferMode": "ReportEvents",
	})
	c.mustCall(sid, "Page.navigate", map[string]any{"url": page.URL})
	time.Sleep(2500 * time.Millisecond)
	c.mustCall("", "Tracing.end", nil)
	select {
	case <-complete:
	case <-time.After(30 * time.Second):
		t.Fatal("трейс не завершился")
	}

	mu.Lock()
	defer mu.Unlock()
	// Трейс весит мегабайты — в репозиторий он не попадает, только во временный каталог.
	out := filepath.Join(os.TempDir(), "chromectl-spike-trace.json")
	data, err := json.Marshal(map[string]any{"traceEvents": chunks})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		t.Fatal(err)
	}

	interesting := map[string]int{}
	for _, raw := range chunks {
		var ev struct {
			Name string          `json:"name"`
			Cat  string          `json:"cat"`
			Ts   int64           `json:"ts"`
			Dur  int64           `json:"dur"`
			Args json.RawMessage `json:"args"`
		}
		_ = json.Unmarshal(raw, &ev)
		switch ev.Name {
		case "navigationStart", "largestContentfulPaint::Candidate", "LayoutShift", "ResourceSendRequest",
			"ResourceReceiveResponse", "firstContentfulPaint", "markLoadEventEnd":
			if interesting[ev.Name] < 3 {
				t.Logf("%s cat=%s ts=%d args=%s", ev.Name, ev.Cat, ev.Ts, tail(string(ev.Args), 600))
			}
			interesting[ev.Name]++
		case "RunTask":
			if ev.Dur > 50_000 {
				if interesting["RunTask>50ms"] < 3 {
					t.Logf("RunTask cat=%s ts=%d dur=%d", ev.Cat, ev.Ts, ev.Dur)
				}
				interesting["RunTask>50ms"]++
			}
		}
	}
	t.Logf("событий всего %d, интересных: %v, файл %s (%d байт)", len(chunks), interesting, out, len(data))
}

func backendNodeID(t *testing.T, c *conn, sessionID, selector string) int64 {
	t.Helper()
	var doc struct {
		Root struct {
			NodeID int64 `json:"nodeId"`
		} `json:"root"`
	}
	unmarshal(t, c.mustCall(sessionID, "DOM.getDocument", map[string]any{"depth": 0}), &doc)
	var q struct {
		NodeID int64 `json:"nodeId"`
	}
	unmarshal(t, c.mustCall(sessionID, "DOM.querySelector", map[string]any{"nodeId": doc.Root.NodeID, "selector": selector}), &q)
	if q.NodeID == 0 {
		t.Fatalf("узел %s не найден", selector)
	}
	var d struct {
		Node struct {
			BackendNodeID int64 `json:"backendNodeId"`
		} `json:"node"`
	}
	unmarshal(t, c.mustCall(sessionID, "DOM.describeNode", map[string]any{"nodeId": q.NodeID}), &d)
	return d.Node.BackendNodeID
}

func idOfBackendNode(t *testing.T, c *conn, sessionID string, backendID int64) string {
	t.Helper()
	res, err := c.call(sessionID, "DOM.resolveNode", map[string]any{"backendNodeId": backendID})
	if err != nil {
		t.Fatalf("DOM.resolveNode в новой сессии без getDocument: %v", err)
	}
	var r struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	unmarshal(t, res, &r)
	var v struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	unmarshal(t, c.mustCall(sessionID, "Runtime.callFunctionOn", map[string]any{
		"objectId": r.Object.ObjectID, "functionDeclaration": "function() { return this.id }", "returnByValue": true,
	}), &v)
	return v.Result.Value
}

func contains(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
