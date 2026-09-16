// Package input — команды Э2, действующие на элементы по uid из снапшота:
// клик, наведение, перетаскивание, заполнение, клавиатура, загрузка файла.
//
// ADR: docs/adr/0009-cli-bez-demona.md — одна команда — одно подключение.
package input

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Y91R/chromectl/internal/command/session"
	"github.com/Y91R/chromectl/internal/netlog"
)

const dragInterceptTimeout = time.Second

// Common — флаги, общие для всех действий.
type Common struct {
	Dialog   session.DialogFlag
	Snapshot bool
	// Capture — записать сетевые запросы за время действия; Unredacted — не
	// маскировать Authorization и cookie.
	Capture    bool
	Unredacted bool
}

type Result struct {
	Dialogs  []session.DialogEvent `json:"dialogs"`
	Snapshot string                `json:"snapshot,omitempty"`
}

// run открывает вкладку, выполняет действие, ждёт его последствий и при
// необходимости снимает свежий снапшот.
func run(ctx context.Context, env session.Env, common Common, action func(p *session.Page) error) (res Result, err error) {
	p, err := session.Open(ctx, env, &common.Dialog)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		events, closeErr := p.Close(ctx)
		res.Dialogs = events
		err = errors.Join(err, closeErr)
	}()

	nav, err := session.WatchNavigation(ctx, p.Client, p.SessionID)
	if err != nil {
		return Result{}, err
	}
	var capture *session.Capture
	if common.Capture {
		if capture, err = session.StartCapture(ctx, p.Client, p.SessionID, common.Unredacted); err != nil {
			nav.Close()
			return Result{}, err
		}
	}
	if err := action(p); err != nil {
		nav.Close()
		if capture != nil {
			capture.Close()
		}
		return Result{}, err
	}
	if err := nav.Settle(ctx); err != nil {
		return Result{}, err
	}
	if capture != nil {
		requests, err := capture.Finish(ctx)
		if err != nil {
			return Result{}, err
		}
		if err := netlog.Save(p.Env.Store.Dir(), p.ID, requests); err != nil {
			return Result{}, err
		}
	}
	if common.Snapshot {
		text, err := p.Snapshot(ctx, false)
		if err != nil {
			return Result{}, err
		}
		return Result{Snapshot: text}, nil
	}
	return Result{}, nil
}

func mouse(ctx context.Context, p *session.Page, sid, eventType string, x, y float64, clickCount int) error {
	params := map[string]any{"type": eventType, "x": x, "y": y}
	if eventType != "mouseMoved" {
		params["button"] = "left"
		params["clickCount"] = clickCount
	}
	if eventType == "mousePressed" {
		params["buttons"] = 1
	}
	return p.Client.Call(ctx, sid, "Input.dispatchMouseEvent", params, nil)
}

func center(ctx context.Context, p *session.Page, uid string) (session.Element, float64, float64, error) {
	el, err := p.Element(ctx, uid)
	if err != nil {
		return session.Element{}, 0, 0, err
	}
	box, err := p.Box(ctx, el)
	if err != nil {
		return session.Element{}, 0, 0, err
	}
	x, y := box.Center()
	return el, x, y, nil
}

func clickAt(ctx context.Context, p *session.Page, sid string, x, y float64, count int) error {
	if err := mouse(ctx, p, sid, "mouseMoved", x, y, 0); err != nil {
		return err
	}
	for i := 1; i <= count; i++ {
		if err := mouse(ctx, p, sid, "mousePressed", x, y, i); err != nil {
			return err
		}
		if err := mouse(ctx, p, sid, "mouseReleased", x, y, i); err != nil {
			return err
		}
	}
	return nil
}

func Click(ctx context.Context, env session.Env, common Common, uid string, double bool) (Result, error) {
	return run(ctx, env, common, func(p *session.Page) error {
		el, x, y, err := center(ctx, p, uid)
		if err != nil {
			return err
		}
		count := 1
		if double {
			count = 2
		}
		return clickAt(ctx, p, el.SessionID, x, y, count)
	})
}

func Hover(ctx context.Context, env session.Env, common Common, uid string) (Result, error) {
	return run(ctx, env, common, func(p *session.Page) error {
		el, x, y, err := center(ctx, p, uid)
		if err != nil {
			return err
		}
		return mouse(ctx, p, el.SessionID, "mouseMoved", x, y, 0)
	})
}

// Drag перетаскивает мышью. HTML5 drag-and-drop в headless Chrome без
// перехвата не доходит до drop, поэтому перехваченные данные доставляются
// событиями Input.dispatchDragEvent.
func Drag(ctx context.Context, env session.Env, common Common, fromUID, toUID string) (Result, error) {
	return run(ctx, env, common, func(p *session.Page) error {
		from, _, _, err := center(ctx, p, fromUID)
		if err != nil {
			return err
		}
		to, tx, ty, err := center(ctx, p, toUID)
		if err != nil {
			return err
		}
		if from.SessionID != to.SessionID {
			return errors.New("перетаскивание между разными cross-origin iframe не поддерживается")
		}
		// Прокрутка ко второму элементу могла сдвинуть первый.
		_, fx, fy, err := center(ctx, p, fromUID)
		if err != nil {
			return err
		}
		sid := from.SessionID

		intercepted := p.Client.Subscribe(sid, "Input.dragIntercepted")
		defer intercepted.Close()
		if err := p.Client.Call(ctx, sid, "Input.setInterceptDrags", map[string]any{"enabled": true}, nil); err != nil {
			return err
		}
		defer func() {
			_ = p.Client.Call(context.WithoutCancel(ctx), sid, "Input.setInterceptDrags", map[string]any{"enabled": false}, nil)
		}()

		if err := mouse(ctx, p, sid, "mouseMoved", fx, fy, 0); err != nil {
			return err
		}
		if err := mouse(ctx, p, sid, "mousePressed", fx, fy, 1); err != nil {
			return err
		}
		// Без зажатой кнопки в mouseMoved Chrome не считает движение перетаскиванием.
		for _, point := range [][2]float64{{(fx + tx) / 2, (fy + ty) / 2}, {tx, ty}} {
			if err := p.Client.Call(ctx, sid, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved", "x": point[0], "y": point[1], "button": "left", "buttons": 1,
			}, nil); err != nil {
				return err
			}
		}

		select {
		case ev, open := <-intercepted.C:
			if open {
				var drag struct {
					Data map[string]any `json:"data"`
				}
				if err := jsonUnmarshal(ev.Params, &drag); err != nil {
					return err
				}
				for _, t := range []string{"dragEnter", "dragOver", "drop"} {
					if err := p.Client.Call(ctx, sid, "Input.dispatchDragEvent", map[string]any{
						"type": t, "x": tx, "y": ty, "data": drag.Data,
					}, nil); err != nil {
						return err
					}
				}
			}
		case <-time.After(dragInterceptTimeout):
		case <-ctx.Done():
			return ctx.Err()
		}
		return mouse(ctx, p, sid, "mouseReleased", tx, ty, 1)
	})
}

const fillProbe = `function(value) {
  const el = this;
  if (el instanceof HTMLSelectElement) {
    const option = Array.from(el.options).find(o => o.label === value || o.text === value || o.value === value);
    if (!option) return {kind: 'select', error: 'в списке нет варианта "' + value + '"'};
    el.value = option.value;
    el.dispatchEvent(new Event('input', {bubbles: true}));
    el.dispatchEvent(new Event('change', {bubbles: true}));
    return {kind: 'select'};
  }
  if (el instanceof HTMLInputElement && (el.type === 'checkbox' || el.type === 'radio')) {
    return {kind: 'toggle', checked: el.checked, radio: el.type === 'radio'};
  }
  const role = el.getAttribute ? el.getAttribute('role') : null;
  if (role === 'checkbox' || role === 'radio' || role === 'switch') {
    return {kind: 'toggle', checked: el.getAttribute('aria-checked') === 'true', radio: role === 'radio'};
  }
  if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) {
    el.focus();
    el.select();
    return {kind: 'text'};
  }
  if (el.isContentEditable) {
    el.focus();
    const range = document.createRange();
    range.selectNodeContents(el);
    const selection = getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    return {kind: 'text'};
  }
  return {kind: 'text', error: 'элемент не принимает ввод текста'};
}`

const clearAndNotify = `function() {
  if ('value' in this) { this.value = ''; } else { this.textContent = ''; }
  this.dispatchEvent(new Event('input', {bubbles: true}));
  this.dispatchEvent(new Event('change', {bubbles: true}));
}`

const notifyChange = `function() { this.dispatchEvent(new Event('change', {bubbles: true})); }`

func fillElement(ctx context.Context, p *session.Page, uid, value string) error {
	el, err := p.Element(ctx, uid)
	if err != nil {
		return err
	}
	var probe struct {
		Kind    string `json:"kind"`
		Checked bool   `json:"checked"`
		Radio   bool   `json:"radio"`
		Error   string `json:"error"`
	}
	if err := p.CallOn(ctx, el, fillProbe, []any{value}, &probe); err != nil {
		return err
	}
	if probe.Error != "" {
		return fmt.Errorf("%s: %s", uid, probe.Error)
	}

	switch probe.Kind {
	case "select":
		return nil
	case "toggle":
		if value != "true" && value != "false" {
			return fmt.Errorf(`%s: для чекбокса и радиокнопки нужно значение "true" или "false", получено %q`, uid, value)
		}
		want := value == "true"
		if probe.Checked == want {
			return nil
		}
		if probe.Radio && !want {
			return fmt.Errorf("%s: радиокнопку нельзя снять значением false — выберите другую", uid)
		}
		return p.CallOn(ctx, el, `function() { this.click(); }`, nil, nil)
	default:
		if value == "" {
			return p.CallOn(ctx, el, clearAndNotify, nil, nil)
		}
		// insertText — настоящий ввод: страница получает beforeinput/input, как от клавиатуры.
		if err := p.Client.Call(ctx, el.SessionID, "Input.insertText", map[string]any{"text": value}, nil); err != nil {
			return err
		}
		return p.CallOn(ctx, el, notifyChange, nil, nil)
	}
}

func Fill(ctx context.Context, env session.Env, common Common, uid, value string) (Result, error) {
	return run(ctx, env, common, func(p *session.Page) error {
		return fillElement(ctx, p, uid, value)
	})
}

type fillPair struct {
	uid   string
	value string
}

func FillForm(ctx context.Context, env session.Env, common Common, pairs []string) (Result, error) {
	// Разбор до подключения: опечатка в одной паре не должна заполнить половину формы.
	parsed := make([]fillPair, 0, len(pairs))
	for _, pair := range pairs {
		uid, value, ok := strings.Cut(pair, "=")
		if !ok || uid == "" {
			return Result{}, fmt.Errorf("ожидалось uid=значение, получено %q", pair)
		}
		parsed = append(parsed, fillPair{uid: uid, value: value})
	}
	return run(ctx, env, common, func(p *session.Page) error {
		for _, f := range parsed {
			if err := fillElement(ctx, p, f.uid, f.value); err != nil {
				return err
			}
		}
		return nil
	})
}

// Type печатает текст в элемент с фокусом посимвольно, с событиями клавиатуры.
func Type(ctx context.Context, env session.Env, common Common, text, submitKey string) (Result, error) {
	var submit combo
	if submitKey != "" {
		var err error
		if submit, err = parseCombo(submitKey); err != nil {
			return Result{}, err
		}
	}
	return run(ctx, env, common, func(p *session.Page) error {
		for _, r := range text {
			def, err := keyFor(string(r))
			if err != nil {
				return err
			}
			if err := pressKey(ctx, p, p.SessionID, combo{key: def}); err != nil {
				return err
			}
		}
		if submitKey != "" {
			return pressKey(ctx, p, p.SessionID, submit)
		}
		return nil
	})
}

func Press(ctx context.Context, env session.Env, common Common, key string) (Result, error) {
	c, err := parseCombo(key)
	if err != nil {
		return Result{}, err
	}
	return run(ctx, env, common, func(p *session.Page) error {
		return pressKey(ctx, p, p.SessionID, c)
	})
}

func Upload(ctx context.Context, env session.Env, common Common, uid string, files []string) (Result, error) {
	paths := make([]string, 0, len(files))
	for _, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			return Result{}, err
		}
		if _, err := os.Stat(abs); err != nil {
			return Result{}, fmt.Errorf("файл для загрузки: %w", err)
		}
		paths = append(paths, abs)
	}
	return run(ctx, env, common, func(p *session.Page) error {
		return upload(ctx, p, uid, paths)
	})
}

func upload(ctx context.Context, p *session.Page, uid string, paths []string) error {
	el, err := p.Element(ctx, uid)
	if err != nil {
		return err
	}
	var isFileInput bool
	if err := p.CallOn(ctx, el, `function() { return this instanceof HTMLInputElement && this.type === 'file'; }`, nil, &isFileInput); err != nil {
		return err
	}
	if isFileInput {
		return p.Client.Call(ctx, el.SessionID, "DOM.setFileInputFiles", map[string]any{
			"files": paths, "backendNodeId": el.Ref.BackendNodeID,
		}, nil)
	}

	// Не input[type=file]: элемент может открывать выбор файла по клику.
	sid := el.SessionID
	opened := p.Client.Subscribe(sid, "Page.fileChooserOpened")
	defer opened.Close()
	if err := p.Client.Call(ctx, sid, "Page.setInterceptFileChooserDialog", map[string]any{"enabled": true}, nil); err != nil {
		return err
	}
	defer func() {
		_ = p.Client.Call(context.WithoutCancel(ctx), sid, "Page.setInterceptFileChooserDialog", map[string]any{"enabled": false}, nil)
	}()
	box, err := p.Box(ctx, el)
	if err != nil {
		return err
	}
	x, y := box.Center()
	if err := clickAt(ctx, p, sid, x, y, 1); err != nil {
		return err
	}
	select {
	case ev, open := <-opened.C:
		if !open {
			return opened.Err()
		}
		var chooser struct {
			BackendNodeID int64 `json:"backendNodeId"`
		}
		if err := jsonUnmarshal(ev.Params, &chooser); err != nil {
			return err
		}
		return p.Client.Call(ctx, sid, "DOM.setFileInputFiles", map[string]any{
			"files": paths, "backendNodeId": chooser.BackendNodeID,
		}, nil)
	case <-time.After(3 * time.Second):
		return fmt.Errorf("%s не принимает файл: это не input[type=file], и клик по нему не открыл выбор файла", uid)
	case <-ctx.Done():
		return ctx.Err()
	}
}
