package pages

import (
	"context"
	"errors"
	"fmt"

	"github.com/Y91R/chromectl/internal/cdp"
	"github.com/Y91R/chromectl/internal/command/session"
	"github.com/Y91R/chromectl/internal/emulation"
	"github.com/Y91R/chromectl/internal/netlog"
	"github.com/Y91R/chromectl/internal/state"
)

type NavigateOptions struct {
	URL         string
	Back        bool
	Forward     bool
	Reload      bool
	IgnoreCache bool
	InitScript  string
	Dialog      session.DialogFlag
	// Capture — записать сетевые запросы навигации; Unredacted — не маскировать секреты.
	Capture    bool
	Unredacted bool
}

type NavigateResult struct {
	URL     string                `json:"url"`
	Title   string                `json:"title"`
	Dialogs []session.DialogEvent `json:"dialogs"`
}

func (o NavigateOptions) validate() error {
	modes := 0
	for _, set := range []bool{o.URL != "", o.Back, o.Forward, o.Reload} {
		if set {
			modes++
		}
	}
	if modes != 1 {
		return errors.New("укажите URL или один из флагов --back, --forward, --reload")
	}
	return nil
}

func Navigate(ctx context.Context, env Env, opts NavigateOptions) (NavigateResult, error) {
	if err := opts.validate(); err != nil {
		return NavigateResult{}, err
	}
	st, c, err := session.Connect(ctx, env)
	if err != nil {
		return NavigateResult{}, err
	}
	defer func() { _ = c.Close() }()

	id, err := session.ResolvePage(ctx, c, env, st)
	if err != nil {
		return NavigateResult{}, err
	}
	sid, err := session.Attach(ctx, c, id)
	if err != nil {
		return NavigateResult{}, err
	}
	defer session.Detach(ctx, c, sid)

	// Эмуляция до перехода: заголовки и сеть должны действовать уже на запрос документа.
	if err := emulation.Apply(ctx, c, sid, st.Emulation[id]); err != nil {
		return NavigateResult{}, err
	}

	pending := st.PendingDialog
	policy, consumed := session.DialogPolicyFor(opts.Dialog, &st)
	if consumed {
		used := *pending
		if err := env.Store.Update(func(s *state.State) { s.ClearPendingDialog(used) }); err != nil {
			return NavigateResult{}, err
		}
	}

	navigated := c.Subscribe(sid, "Page.frameNavigated")
	defer navigated.Close()

	dialogs, err := session.WatchDialogs(ctx, c, sid, policy)
	if err != nil {
		if !opts.Reload || !errors.Is(err, context.DeadlineExceeded) {
			return NavigateResult{}, session.BlockedError(id, err)
		}
		return reloadBlocked(ctx, c, sid, id, opts, policy)
	}

	var capture *session.Capture
	if opts.Capture {
		if capture, err = session.StartCapture(ctx, c, sid, opts.Unredacted); err != nil {
			_, _ = dialogs.Stop()
			return NavigateResult{}, err
		}
	}
	navErr := navigateAndWait(ctx, c, sid, navigated, opts)
	if capture != nil {
		if navErr != nil {
			capture.Close()
		} else if requests, err := capture.Finish(ctx); err != nil {
			navErr = err
		} else {
			navErr = netlog.Save(env.Store.Dir(), id, requests)
		}
	}
	events, dialogErr := dialogs.Stop()
	if err := errors.Join(navErr, dialogErr); err != nil {
		return NavigateResult{Dialogs: events}, err
	}
	return navigateResult(ctx, c, id, events)
}

func navigateAndWait(ctx context.Context, c *cdp.Client, sid string, navigated *cdp.Subscription, opts NavigateOptions) error {
	if opts.InitScript != "" {
		if err := c.Call(ctx, sid, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": opts.InitScript}, nil); err != nil {
			return err
		}
	}
	sameDocument, err := startNavigation(ctx, c, sid, opts)
	if err != nil {
		return err
	}
	if !sameDocument {
		if err := session.WaitMainFrameNavigated(ctx, navigated); err != nil {
			return err
		}
	}
	return session.WaitDocumentReady(ctx, c, sid, true)
}

// reloadBlocked перезагружает вкладку, которая не отвечает на Page.enable:
// так выглядит диалог, оставшийся от другой сессии, и Page.reload его снимает (спайк Э1).
func reloadBlocked(ctx context.Context, c *cdp.Client, sid, id string, opts NavigateOptions, policy session.DialogPolicy) (NavigateResult, error) {
	if err := c.Call(ctx, sid, "Page.reload", map[string]any{"ignoreCache": opts.IgnoreCache}, nil); err != nil {
		return NavigateResult{}, err
	}
	dialogs, err := session.WatchDialogs(ctx, c, sid, policy)
	if err != nil {
		return NavigateResult{}, fmt.Errorf("вкладка %s не ожила после перезагрузки: %w", id, err)
	}
	readyErr := session.WaitDocumentReady(ctx, c, sid, true)
	events, dialogErr := dialogs.Stop()
	if err := errors.Join(readyErr, dialogErr); err != nil {
		return NavigateResult{Dialogs: events}, err
	}
	return navigateResult(ctx, c, id, events)
}

func navigateResult(ctx context.Context, c *cdp.Client, id string, dialogs []session.DialogEvent) (NavigateResult, error) {
	info, err := session.GetTargetInfo(ctx, c, id)
	if err != nil {
		return NavigateResult{}, err
	}
	return NavigateResult{URL: info.URL, Title: info.Title, Dialogs: dialogs}, nil
}

func startNavigation(ctx context.Context, c *cdp.Client, sid string, opts NavigateOptions) (sameDocument bool, err error) {
	switch {
	case opts.URL != "":
		var res struct {
			LoaderID  string `json:"loaderId"`
			ErrorText string `json:"errorText"`
		}
		if err := c.Call(ctx, sid, "Page.navigate", map[string]any{"url": opts.URL}, &res); err != nil {
			return false, err
		}
		if res.ErrorText != "" {
			return false, fmt.Errorf("переход на %s: %s", opts.URL, res.ErrorText)
		}
		// Переход внутри документа (например, по якорю) не создаёт нового загрузчика.
		return res.LoaderID == "", nil

	case opts.Back || opts.Forward:
		var history struct {
			CurrentIndex int `json:"currentIndex"`
			Entries      []struct {
				ID int `json:"id"`
			} `json:"entries"`
		}
		if err := c.Call(ctx, sid, "Page.getNavigationHistory", nil, &history); err != nil {
			return false, err
		}
		target := history.CurrentIndex + 1
		if opts.Back {
			target = history.CurrentIndex - 1
		}
		if target < 0 {
			return false, errors.New("нет предыдущей страницы в истории")
		}
		if target >= len(history.Entries) {
			return false, errors.New("нет следующей страницы в истории")
		}
		err := c.Call(ctx, sid, "Page.navigateToHistoryEntry", map[string]any{"entryId": history.Entries[target].ID}, nil)
		return false, err

	default:
		err := c.Call(ctx, sid, "Page.reload", map[string]any{"ignoreCache": opts.IgnoreCache}, nil)
		return false, err
	}
}
