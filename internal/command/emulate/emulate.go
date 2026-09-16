// Package emulate — команды Э3: эмуляция вкладки и размер окна.
//
// ADR: docs/adr/0009-cli-bez-demona.md — настройки эмуляции хранятся в
// state и применяются при каждом подключении к вкладке.
package emulate

import (
	"context"
	"errors"
	"reflect"

	"github.com/Y91R/chromectl/internal/command/session"
	"github.com/Y91R/chromectl/internal/emulation"
	"github.com/Y91R/chromectl/internal/state"
)

// Emulate меняет эмуляцию выбранной вкладки и возвращает итоговые настройки.
// Пустые Changes только показывают текущие.
func Emulate(ctx context.Context, env session.Env, ch emulation.Changes) (settings state.Emulation, err error) {
	// Неверное значение не должно доходить до браузера.
	if _, err := emulation.Merge(state.Emulation{}, ch); err != nil {
		return state.Emulation{}, err
	}

	p, err := session.Open(ctx, env, nil)
	if err != nil {
		return state.Emulation{}, err
	}
	defer func() {
		_, closeErr := p.Close(ctx)
		err = errors.Join(err, closeErr)
	}()

	next, err := emulation.Merge(p.State.Emulation[p.ID], ch)
	if err != nil {
		return state.Emulation{}, err
	}
	if ch == (emulation.Changes{}) {
		return next, nil
	}
	if err := emulation.Apply(ctx, p.Client, p.SessionID, next); err != nil {
		return state.Emulation{}, err
	}
	// Override размеров, в отличие от остальных, Chrome не снимает при отключении
	// сессии (спайк Э3): сброшенный viewport снимается явно.
	if p.State.Emulation[p.ID].Viewport != nil && next.Viewport == nil {
		if err := p.Client.Call(ctx, p.SessionID, "Emulation.clearDeviceMetricsOverride", nil, nil); err != nil {
			return state.Emulation{}, err
		}
	}

	p.Change(func(st *state.State) {
		if reflect.DeepEqual(next, state.Emulation{}) {
			delete(st.Emulation, p.ID)
			return
		}
		if st.Emulation == nil {
			st.Emulation = map[string]state.Emulation{}
		}
		st.Emulation[p.ID] = next
	})
	return next, nil
}

// Resize меняет размер содержимого окна вкладки. В отличие от viewport из
// emulate, размер окна сохраняется в самом Chrome и между командами.
func Resize(ctx context.Context, env session.Env, width, height int) (err error) {
	if width <= 0 || height <= 0 {
		return errors.New("ширина и высота должны быть положительными")
	}
	p, err := session.Open(ctx, env, nil)
	if err != nil {
		return err
	}
	defer func() {
		_, closeErr := p.Close(ctx)
		err = errors.Join(err, closeErr)
	}()

	var window struct {
		WindowID int `json:"windowId"`
		Bounds   struct {
			WindowState string `json:"windowState"`
		} `json:"bounds"`
	}
	if err := p.Client.Call(ctx, "", "Browser.getWindowForTarget", map[string]any{"targetId": p.ID}, &window); err != nil {
		return err
	}
	if state := window.Bounds.WindowState; state != "" && state != "normal" {
		if err := p.Client.Call(ctx, "", "Browser.setWindowBounds", map[string]any{
			"windowId": window.WindowID, "bounds": map[string]any{"windowState": "normal"},
		}, nil); err != nil {
			return err
		}
	}

	err = p.Client.Call(ctx, "", "Browser.setContentsSize", map[string]any{
		"windowId": window.WindowID, "width": width, "height": height,
	}, nil)
	if err == nil {
		return nil
	}
	// Chrome без Browser.setContentsSize: в headless у окна нет рамок, и размер
	// окна совпадает с содержимым (спайк Э1, допущение 6).
	return p.Client.Call(ctx, "", "Browser.setWindowBounds", map[string]any{
		"windowId": window.WindowID,
		"bounds":   map[string]any{"width": width, "height": height, "windowState": "normal"},
	}, nil)
}
