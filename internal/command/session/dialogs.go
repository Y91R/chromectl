package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Y91R/chromectl/internal/cdp"
)

const enableTimeout = 3 * time.Second

// DialogPolicy — ответ на диалог, открывшийся во время команды.
//
// ADR: docs/adr/0009-cli-bez-demona.md — диалог, оставленный открытым,
// блокирует страницу, а следующая команда его не видит; закрывает та, при которой он открылся.
type DialogPolicy struct {
	Accept     bool
	PromptText string
}

func ParseDialogPolicy(s string) DialogPolicy {
	switch s {
	case "", "accept":
		return DialogPolicy{Accept: true}
	case "dismiss":
		return DialogPolicy{Accept: false}
	default:
		return DialogPolicy{Accept: true, PromptText: s}
	}
}

func (p DialogPolicy) String() string {
	switch {
	case !p.Accept:
		return "dismiss"
	case p.PromptText != "":
		return "accept: " + p.PromptText
	default:
		return "accept"
	}
}

type DialogEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Action  string `json:"action"`
}

// Dialogs закрывает диалоги в фоне: действие вроде клика по кнопке с confirm()
// не вернёт ответ, пока диалог открыт, поэтому ждать его в том же потоке нельзя.
type Dialogs struct {
	c      *cdp.Client
	sid    string
	sub    *cdp.Subscription
	policy DialogPolicy

	mu     sync.Mutex
	events []DialogEvent
	err    error

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

func WatchDialogs(ctx context.Context, c *cdp.Client, sid string, policy DialogPolicy) (*Dialogs, error) {
	sub := c.Subscribe(sid, "Page.javascriptDialogOpening")
	enableCtx, cancel := context.WithTimeout(ctx, enableTimeout)
	defer cancel()
	if err := c.Call(enableCtx, sid, "Page.enable", nil, nil); err != nil {
		sub.Close()
		return nil, err
	}

	d := &Dialogs{c: c, sid: sid, sub: sub, policy: policy, stop: make(chan struct{}), done: make(chan struct{})}
	go d.run(ctx)
	return d, nil
}

// run принадлежит Dialogs и завершается по Stop, по отмене контекста команды
// или при закрытии соединения.
func (d *Dialogs) run(ctx context.Context) {
	defer close(d.done)
	for {
		select {
		case ev, open := <-d.sub.C:
			if !open {
				if err := d.sub.Err(); err != nil && !errors.Is(err, cdp.ErrClosed) {
					d.fail(err)
				}
				return
			}
			d.handle(ctx, ev.Params)
		case <-d.stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (d *Dialogs) handle(ctx context.Context, params json.RawMessage) {
	var dialog struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(params, &dialog); err != nil {
		d.fail(fmt.Errorf("разбор диалога: %w", err))
		return
	}
	p := map[string]any{"accept": d.policy.Accept}
	if d.policy.PromptText != "" {
		p["promptText"] = d.policy.PromptText
	}
	if err := d.c.Call(ctx, d.sid, "Page.handleJavaScriptDialog", p, nil); err != nil {
		d.fail(fmt.Errorf("закрытие диалога %s %q: %w", dialog.Type, dialog.Message, err))
		return
	}
	d.mu.Lock()
	d.events = append(d.events, DialogEvent{Type: dialog.Type, Message: dialog.Message, Action: d.policy.String()})
	d.mu.Unlock()
}

func (d *Dialogs) fail(err error) {
	d.mu.Lock()
	d.err = errors.Join(d.err, err)
	d.mu.Unlock()
}

// Stop останавливает стража и возвращает закрытые диалоги. Повторный вызов безопасен.
func (d *Dialogs) Stop() ([]DialogEvent, error) {
	d.stopOnce.Do(func() { close(d.stop) })
	<-d.done
	d.sub.Close()

	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]DialogEvent{}, d.events...), d.err
}
