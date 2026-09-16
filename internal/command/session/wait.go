package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Y91R/chromectl/internal/cdp"
)

const (
	probeTimeout       = 500 * time.Millisecond
	expectNavigationIn = 100 * time.Millisecond
	navigationTimeout  = 3 * time.Second
	stableDOMTimeout   = 3 * time.Second
)

const stableDOMScript = `new Promise(resolve => {
  let timer;
  const done = () => { observer.disconnect(); resolve(true); };
  const observer = new MutationObserver(() => { clearTimeout(timer); timer = setTimeout(done, 100); });
  observer.observe(document, {subtree: true, childList: true, attributes: true, characterData: true});
  timer = setTimeout(done, 100);
})`

// WaitDocumentReady опрашивает document.readyState короткими вызовами: пока
// открыт диалог, страница на них не отвечает, а страж диалогов успевает его закрыть.
func WaitDocumentReady(ctx context.Context, c *cdp.Client, sid string, allowBlank bool) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if documentReady(ctx, c, sid, allowBlank) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("страница не загрузилась: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func documentReady(ctx context.Context, c *cdp.Client, sid string, allowBlank bool) bool {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	var res struct {
		Result struct {
			Value []string `json:"value"`
		} `json:"result"`
	}
	err := c.Call(probeCtx, sid, "Runtime.evaluate", map[string]any{
		"expression":    "[document.readyState, location.href]",
		"returnByValue": true,
	}, &res)
	if err != nil || len(res.Result.Value) != 2 {
		return false
	}
	return res.Result.Value[0] == "complete" && (allowBlank || res.Result.Value[1] != "about:blank")
}

// WaitMainFrameNavigated ждёт смены документа главного фрейма. Событие load для
// этого не годится: страница, восстановленная из bfcache при --back, его не шлёт.
func WaitMainFrameNavigated(ctx context.Context, navigated *cdp.Subscription) error {
	for {
		select {
		case ev, open := <-navigated.C:
			if !open {
				return fmt.Errorf("подписка на навигацию закрыта: %w", navigated.Err())
			}
			var p struct {
				Frame struct {
					ParentID string `json:"parentId"`
				} `json:"frame"`
			}
			if err := json.Unmarshal(ev.Params, &p); err == nil && p.Frame.ParentID == "" {
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("переход не состоялся: %w", ctx.Err())
		}
	}
}

// NavigationWatch следит, не запустило ли действие навигацию. Создаётся до действия.
type NavigationWatch struct {
	c         *cdp.Client
	sid       string
	started   *cdp.Subscription
	navigated *cdp.Subscription
	mainFrame string
}

func WatchNavigation(ctx context.Context, c *cdp.Client, sid string) (*NavigationWatch, error) {
	started := c.Subscribe(sid, "Page.frameStartedLoading")
	navigated := c.Subscribe(sid, "Page.frameNavigated")
	var tree struct {
		FrameTree struct {
			Frame struct {
				ID string `json:"id"`
			} `json:"frame"`
		} `json:"frameTree"`
	}
	if err := c.Call(ctx, sid, "Page.getFrameTree", nil, &tree); err != nil {
		started.Close()
		navigated.Close()
		return nil, err
	}
	return &NavigationWatch{c: c, sid: sid, started: started, navigated: navigated, mainFrame: tree.FrameTree.Frame.ID}, nil
}

// Close нужен, когда действие упало и Settle не вызывался.
func (w *NavigationWatch) Close() {
	w.started.Close()
	w.navigated.Close()
}

// Settle ждёт последствий действия: если за
// 100 мс началась навигация главного фрейма — её (не дольше 3 с), затем, пока
// DOM не перестанет меняться 100 мс (не дольше 3 с). Не уложившаяся навигация
// или неуспокоившийся DOM ошибкой действия не считаются.
func (w *NavigationWatch) Settle(ctx context.Context) error {
	defer w.started.Close()
	defer w.navigated.Close()

	if w.navigationStarted(ctx) {
		navCtx, cancel := context.WithTimeout(ctx, navigationTimeout)
		err := WaitMainFrameNavigated(navCtx, w.navigated)
		if err == nil {
			err = WaitDocumentReady(navCtx, w.c, w.sid, true)
		}
		cancel()
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
	}

	stableCtx, cancel := context.WithTimeout(ctx, stableDOMTimeout)
	defer cancel()
	_ = w.c.Call(stableCtx, w.sid, "Runtime.evaluate", map[string]any{
		"expression": stableDOMScript, "awaitPromise": true, "returnByValue": true,
	}, nil)
	return ctx.Err()
}

func (w *NavigationWatch) navigationStarted(ctx context.Context) bool {
	timer := time.NewTimer(expectNavigationIn)
	defer timer.Stop()
	for {
		select {
		case ev, open := <-w.started.C:
			if !open {
				return false
			}
			var p struct {
				FrameID string `json:"frameId"`
			}
			if json.Unmarshal(ev.Params, &p) == nil && p.FrameID == w.mainFrame {
				return true
			}
		case <-timer.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}
