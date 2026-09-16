// Package script — команды Э2, читающие страницу и выполняющие в ней код:
// снапшот, eval, ожидание текста и политика следующего диалога.
//
// ADR: docs/adr/0009-cli-bez-demona.md — одна команда — одно подключение.
package script

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Y91R/chromectl/internal/command/session"
	"github.com/Y91R/chromectl/internal/state"
)

const probeTimeout = 500 * time.Millisecond

type SnapshotResult struct {
	Snapshot string `json:"snapshot,omitempty"`
	Path     string `json:"path,omitempty"`
}

func Snapshot(ctx context.Context, env session.Env, verbose bool, output string) (res SnapshotResult, err error) {
	p, err := session.Open(ctx, env, nil)
	if err != nil {
		return SnapshotResult{}, err
	}
	defer func() {
		_, closeErr := p.Close(ctx)
		err = errors.Join(err, closeErr)
	}()

	text, err := p.Snapshot(ctx, verbose)
	if err != nil {
		return SnapshotResult{}, err
	}
	if output == "" {
		return SnapshotResult{Snapshot: text}, nil
	}
	if err := os.WriteFile(output, []byte(text), 0o600); err != nil {
		return SnapshotResult{}, fmt.Errorf("запись снапшота: %w", err)
	}
	return SnapshotResult{Path: output}, nil
}

type EvalOptions struct {
	Function string
	// Args — uid элементов, которые передаются функции аргументами.
	Args   []string
	Output string
	Dialog session.DialogFlag
}

type EvalResult struct {
	// Value — результат JSON.stringify; "undefined",
	// если функция ничего не вернула.
	Value   string                `json:"value,omitempty"`
	Path    string                `json:"path,omitempty"`
	Dialogs []session.DialogEvent `json:"dialogs"`
}

func Eval(ctx context.Context, env session.Env, opts EvalOptions) (res EvalResult, err error) {
	p, err := session.Open(ctx, env, &opts.Dialog)
	if err != nil {
		return EvalResult{}, err
	}
	defer func() {
		events, closeErr := p.Close(ctx)
		res.Dialogs = events
		err = errors.Join(err, closeErr)
	}()

	nav, err := session.WatchNavigation(ctx, p.Client, p.SessionID)
	if err != nil {
		return EvalResult{}, err
	}
	value, err := evaluate(ctx, p, opts)
	if err != nil {
		nav.Close()
		return EvalResult{}, err
	}
	if err := nav.Settle(ctx); err != nil {
		return EvalResult{}, err
	}

	if opts.Output == "" {
		return EvalResult{Value: value}, nil
	}
	if err := os.WriteFile(opts.Output, []byte(value), 0o600); err != nil {
		return EvalResult{}, fmt.Errorf("запись результата: %w", err)
	}
	return EvalResult{Path: opts.Output}, nil
}

func evaluate(ctx context.Context, p *session.Page, opts EvalOptions) (string, error) {
	var res session.RemoteResult
	if len(opts.Args) == 0 {
		if err := p.Client.Call(ctx, p.SessionID, "Runtime.evaluate", map[string]any{
			"expression":    "(async () => JSON.stringify(await (" + opts.Function + ")()))()",
			"awaitPromise":  true,
			"returnByValue": true,
		}, &res); err != nil {
			return "", err
		}
	} else {
		elements := make([]session.Element, 0, len(opts.Args))
		arguments := make([]map[string]any, 0, len(opts.Args))
		for _, uid := range opts.Args {
			el, err := p.Element(ctx, uid)
			if err != nil {
				return "", err
			}
			if len(elements) > 0 && el.SessionID != elements[0].SessionID {
				return "", errors.New("элементы из разных cross-origin iframe нельзя передать в одну функцию")
			}
			elements = append(elements, el)
			arguments = append(arguments, map[string]any{"objectId": el.ObjectID})
		}
		if err := p.Client.Call(ctx, elements[0].SessionID, "Runtime.callFunctionOn", map[string]any{
			"objectId":            elements[0].ObjectID,
			"functionDeclaration": "async function(...args) { return JSON.stringify(await (" + opts.Function + ")(...args)); }",
			"arguments":           arguments,
			"awaitPromise":        true,
			"returnByValue":       true,
		}, &res); err != nil {
			return "", err
		}
	}

	if res.ExceptionDetails != nil {
		return "", fmt.Errorf("исключение в функции: %w", res.ExceptionDetails)
	}
	var text string
	if json.Unmarshal(res.Result.Value, &text) != nil {
		return "undefined", nil
	}
	return text, nil
}

// SetDialog запоминает ответ на диалог для следующей команды без явного --dialog.
func SetDialog(env session.Env, accept bool, promptText string) error {
	if !accept && promptText != "" {
		return errors.New("--text только для accept: текст вводится в prompt")
	}
	return env.Store.Update(func(st *state.State) {
		st.PendingDialog = &state.DialogPolicy{Accept: accept, PromptText: promptText}
	})
}

func WaitFor(ctx context.Context, env session.Env, texts []string, timeout time.Duration) (found string, err error) {
	p, err := session.Open(ctx, env, nil)
	if err != nil {
		return "", err
	}
	defer func() {
		_, closeErr := p.Close(ctx)
		err = errors.Join(err, closeErr)
	}()

	encoded, err := json.Marshal(texts)
	if err != nil {
		return "", err
	}
	expression := "((texts) => { const body = document.body ? document.body.innerText : ''; return texts.find(t => body.includes(t)) ?? null; })(" + string(encoded) + ")"

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if text, ok := probeText(waitCtx, p, expression); ok {
			return text, nil
		}
		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", notAppeared(texts, timeout)
		case <-ticker.C:
		}
	}
}

func probeText(ctx context.Context, p *session.Page, expression string) (string, bool) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	var res session.RemoteResult
	err := p.Client.Call(probeCtx, p.SessionID, "Runtime.evaluate", map[string]any{
		"expression": expression, "returnByValue": true,
	}, &res)
	if err != nil || res.ExceptionDetails != nil {
		return "", false
	}
	// null разбирается в пустую строку без ошибки — это «не найдено», а не находка.
	var text *string
	if json.Unmarshal(res.Result.Value, &text) != nil || text == nil {
		return "", false
	}
	return *text, true
}

func notAppeared(texts []string, timeout time.Duration) error {
	if len(texts) == 1 {
		return fmt.Errorf("текст %q не появился за %s", texts[0], timeout)
	}
	quoted := make([]string, len(texts))
	for i, t := range texts {
		quoted[i] = fmt.Sprintf("%q", t)
	}
	return fmt.Errorf("ни один из текстов %s не появился за %s", strings.Join(quoted, ", "), timeout)
}
