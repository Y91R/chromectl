// Package inspect — команды Э4: консоль и сеть вкладки.
//
// ADR: docs/adr/0009-cli-bez-demona.md — консоль повторяет буфер Chrome
// (спайк Э1, допущение 3), истории сети нет — только Performance API и --capture.
package inspect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Y91R/chromectl/internal/command/session"
	"github.com/Y91R/chromectl/internal/console"
	"github.com/Y91R/chromectl/internal/netlog"
)

const (
	consoleQuiet   = 200 * time.Millisecond
	consoleMaxWait = 3 * time.Second
)

// resourceTypes — допустимые значения --types у network list.
var resourceTypes = map[string]bool{
	"document": true, "stylesheet": true, "image": true, "media": true, "font": true, "script": true,
	"texttrack": true, "xhr": true, "fetch": true, "prefetch": true, "eventsource": true, "websocket": true,
	"manifest": true, "signedexchange": true, "ping": true, "cspviolationreport": true, "preflight": true,
	"fedcm": true, "other": true,
}

const performanceScript = `JSON.stringify([...performance.getEntriesByType('navigation'), ...performance.getEntriesByType('resource')]
  .map(e => ({url: e.name, initiatorType: e.initiatorType, status: e.responseStatus || 0})))`

func ConsoleList(ctx context.Context, env session.Env, opts console.ListOptions) (string, error) {
	// Неверный фильтр не должен доходить до браузера.
	if _, err := console.List(nil, opts); err != nil {
		return "", err
	}
	messages, err := consoleMessages(ctx, env)
	if err != nil {
		return "", err
	}
	return console.List(messages, opts)
}

func ConsoleGet(ctx context.Context, env session.Env, id int) (console.Message, error) {
	messages, err := consoleMessages(ctx, env)
	if err != nil {
		return console.Message{}, err
	}
	for _, m := range messages {
		if m.ID == id {
			return m, nil
		}
	}
	return console.Message{}, fmt.Errorf("сообщение %d не найдено", id)
}

// consoleMessages получает буфер консоли, который Chrome повторяет новой сессии
// после Runtime.enable, и ждёт, пока повтор закончится.
func consoleMessages(ctx context.Context, env session.Env) (messages []console.Message, err error) {
	p, err := session.Open(ctx, env, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, closeErr := p.Close(ctx)
		err = errors.Join(err, closeErr)
	}()

	sub := p.Client.Subscribe(p.SessionID, "Runtime.consoleAPICalled", "Runtime.exceptionThrown")
	defer sub.Close()
	if err := p.Client.Call(ctx, p.SessionID, "Runtime.enable", nil, nil); err != nil {
		return nil, err
	}

	deadline := time.After(consoleMaxWait)
	for {
		select {
		case ev, open := <-sub.C:
			if !open {
				return nil, sub.Err()
			}
			var m console.Message
			if ev.Method == "Runtime.exceptionThrown" {
				m, err = console.FromException(ev.Params)
			} else {
				m, err = console.FromAPICalled(ev.Params)
			}
			if err != nil {
				return nil, err
			}
			messages = append(messages, m)
		case <-time.After(consoleQuiet):
			return console.Number(messages), nil
		case <-deadline:
			return console.Number(messages), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

type NetworkListOptions struct {
	Types    []string
	PageSize int
	PageIdx  int
}

func NetworkList(ctx context.Context, env session.Env, opts NetworkListOptions) (text string, err error) {
	types := map[string]bool{}
	for _, t := range opts.Types {
		if !resourceTypes[t] {
			return "", fmt.Errorf("неизвестный тип ресурса %q", t)
		}
		types[t] = true
	}
	if opts.PageSize < 0 || opts.PageIdx < 0 {
		return "", errors.New("--page-size и --page-idx не могут быть отрицательными")
	}

	p, err := session.Open(ctx, env, nil)
	if err != nil {
		return "", err
	}
	defer func() {
		_, closeErr := p.Close(ctx)
		err = errors.Join(err, closeErr)
	}()

	captured, err := netlog.Load(env.Store.Dir(), p.ID)
	switch {
	case err == nil:
		var lines []string
		for _, r := range page(filter(captured, types, func(r netlog.Request) string { return r.ResourceType }), opts) {
			lines = append(lines, netlog.Concise(r))
		}
		return withHeader("запросы последнего --capture:", lines, "захваченных запросов нет"), nil
	case !errors.Is(err, netlog.ErrNoCapture):
		return "", err
	}

	var res session.RemoteResult
	if err := p.Client.Call(ctx, p.SessionID, "Runtime.evaluate", map[string]any{
		"expression": performanceScript, "returnByValue": true,
	}, &res); err != nil {
		return "", err
	}
	if res.ExceptionDetails != nil {
		return "", res.ExceptionDetails
	}
	var encoded string
	var timings []netlog.Timing
	if err := json.Unmarshal(res.Result.Value, &encoded); err != nil {
		return "", fmt.Errorf("разбор Performance API: %w", err)
	}
	if err := json.Unmarshal([]byte(encoded), &timings); err != nil {
		return "", fmt.Errorf("разбор Performance API: %w", err)
	}
	var lines []string
	for _, t := range page(filter(timings, types, func(t netlog.Timing) string { return netlog.TypeFromInitiator(t.InitiatorType) }), opts) {
		lines = append(lines, netlog.ConciseTiming(t))
	}
	return withHeader("запросы из Performance API — заголовки и тела недоступны, повторите действие с --capture:", lines, "запросов нет"), nil
}

type NetworkGetOptions struct {
	ReqID        int
	RequestFile  string
	ResponseFile string
}

func NetworkGet(ctx context.Context, env session.Env, opts NetworkGetOptions) (req netlog.Request, text string, err error) {
	st, c, err := session.Connect(ctx, env)
	if err != nil {
		return netlog.Request{}, "", err
	}
	defer func() { _ = c.Close() }()
	pageID, err := session.ResolvePage(ctx, c, env, st)
	if err != nil {
		return netlog.Request{}, "", err
	}

	captured, err := netlog.Load(env.Store.Dir(), pageID)
	if err != nil {
		return netlog.Request{}, "", err
	}
	for _, r := range captured {
		if r.ReqID != opts.ReqID {
			continue
		}
		text = netlog.Detail(r)
		if opts.RequestFile != "" {
			if err := os.WriteFile(opts.RequestFile, []byte(r.RequestBody), 0o600); err != nil {
				return netlog.Request{}, "", fmt.Errorf("запись тела запроса: %w", err)
			}
			text += "\nТело запроса сохранено в " + opts.RequestFile
		}
		if opts.ResponseFile != "" {
			if err := os.WriteFile(opts.ResponseFile, []byte(r.ResponseBody), 0o600); err != nil {
				return netlog.Request{}, "", fmt.Errorf("запись тела ответа: %w", err)
			}
			text += "\nТело ответа сохранено в " + opts.ResponseFile
		}
		return r, text, nil
	}
	return netlog.Request{}, "", fmt.Errorf("запрос %d не найден в последнем захвате", opts.ReqID)
}

func filter[T any](items []T, types map[string]bool, typeOf func(T) string) []T {
	if len(types) == 0 {
		return items
	}
	var out []T
	for _, it := range items {
		if types[typeOf(it)] {
			out = append(out, it)
		}
	}
	return out
}

func page[T any](items []T, opts NetworkListOptions) []T {
	if opts.PageSize <= 0 {
		return items
	}
	start := min(opts.PageSize*opts.PageIdx, len(items))
	end := min(start+opts.PageSize, len(items))
	return items[start:end]
}

func withHeader(header string, lines []string, empty string) string {
	if len(lines) == 0 {
		return header + "\n" + empty
	}
	return header + "\n" + strings.Join(lines, "\n")
}
