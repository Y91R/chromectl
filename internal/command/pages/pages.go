// Package pages — команды Э1: браузер, вкладки, навигация, скриншот. Каждая
// функция открывает своё подключение к Chrome и закрывает его до возврата.
//
// ADR: docs/adr/0009-cli-bez-demona.md — одна команда — одно подключение.
package pages

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Y91R/chromectl/internal/cdp"
	"github.com/Y91R/chromectl/internal/command/session"
	"github.com/Y91R/chromectl/internal/state"
)

const pollInterval = 100 * time.Millisecond

var ErrLastPage = errors.New("последнюю вкладку закрыть нельзя")

type Env = session.Env

type Page struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Selected bool   `json:"selected"`
}

func List(ctx context.Context, env Env) ([]Page, error) {
	st, c, err := session.Connect(ctx, env)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()

	targets, err := session.PageTargets(ctx, c)
	if err != nil {
		return nil, err
	}
	result := make([]Page, 0, len(targets))
	for _, t := range targets {
		result = append(result, Page{ID: t.TargetID, Title: t.Title, URL: t.URL, Selected: t.TargetID == st.SelectedPage})
	}
	return result, nil
}

func New(ctx context.Context, env Env, url string, background bool, dialog session.DialogFlag) (Page, []session.DialogEvent, error) {
	st, c, err := session.Connect(ctx, env)
	if err != nil {
		return Page{}, nil, err
	}
	defer func() { _ = c.Close() }()

	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := c.Call(ctx, "", "Target.createTarget", map[string]any{"url": url, "background": background}, &created); err != nil {
		return Page{}, nil, err
	}
	sid, err := session.Attach(ctx, c, created.TargetID)
	if err != nil {
		return Page{}, nil, err
	}
	defer session.Detach(ctx, c, sid)

	pending := st.PendingDialog
	policy, consumed := session.DialogPolicyFor(dialog, &st)
	dialogs, err := session.WatchDialogs(ctx, c, sid, policy)
	if err != nil {
		return Page{}, nil, err
	}
	readyErr := session.WaitDocumentReady(ctx, c, sid, url == "about:blank")
	events, dialogErr := dialogs.Stop()
	if err := errors.Join(readyErr, dialogErr); err != nil {
		return Page{}, events, err
	}

	info, err := session.GetTargetInfo(ctx, c, created.TargetID)
	if err != nil {
		return Page{}, events, err
	}
	if err := env.Store.Update(func(s *state.State) {
		s.SelectedPage = created.TargetID
		if consumed {
			s.ClearPendingDialog(*pending)
		}
	}); err != nil {
		return Page{}, events, err
	}
	return Page{ID: info.TargetID, Title: info.Title, URL: info.URL, Selected: true}, events, nil
}

func Select(ctx context.Context, env Env, id string) (Page, error) {
	_, c, err := session.Connect(ctx, env)
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = c.Close() }()

	targets, err := session.PageTargets(ctx, c)
	if err != nil {
		return Page{}, err
	}
	target, ok := session.FindTarget(targets, id)
	if !ok {
		return Page{}, fmt.Errorf("вкладка %s не найдена", id)
	}
	if err := c.Call(ctx, "", "Target.activateTarget", map[string]any{"targetId": id}, nil); err != nil {
		return Page{}, err
	}
	if err := env.Store.Update(func(s *state.State) { s.SelectedPage = id }); err != nil {
		return Page{}, err
	}
	return Page{ID: target.TargetID, Title: target.Title, URL: target.URL, Selected: true}, nil
}

func Close(ctx context.Context, env Env, id string) error {
	_, c, err := session.Connect(ctx, env)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()

	targets, err := session.PageTargets(ctx, c)
	if err != nil {
		return err
	}
	if _, ok := session.FindTarget(targets, id); !ok {
		return fmt.Errorf("вкладка %s не найдена", id)
	}
	if len(targets) == 1 {
		return ErrLastPage
	}
	if err := c.Call(ctx, "", "Target.closeTarget", map[string]any{"targetId": id}, nil); err != nil {
		return err
	}
	remaining, err := waitTargetGone(ctx, c, id)
	if err != nil {
		return err
	}

	return env.Store.Update(func(s *state.State) {
		if s.SelectedPage == id {
			s.SelectedPage = remaining[0].TargetID
		}
	})
}

// waitTargetGone ждёт, пока закрытая вкладка исчезнет из списка: closeTarget
// отвечает раньше, чем Chrome её убирает, и следующая команда увидела бы её.
func waitTargetGone(ctx context.Context, c *cdp.Client, id string) ([]session.TargetInfo, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		targets, err := session.PageTargets(ctx, c)
		if err != nil {
			return nil, err
		}
		if _, ok := session.FindTarget(targets, id); !ok && len(targets) > 0 {
			return targets, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("вкладка %s не закрылась: %w", id, ctx.Err())
		case <-ticker.C:
		}
	}
}
