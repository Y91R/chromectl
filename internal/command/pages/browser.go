package pages

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Y91R/chromectl/internal/browser"
	"github.com/Y91R/chromectl/internal/cdp"
	"github.com/Y91R/chromectl/internal/command/session"
	"github.com/Y91R/chromectl/internal/state"
)

const (
	closeCallTimeout = 5 * time.Second
	exitTimeout      = 10 * time.Second
	killTimeout      = 5 * time.Second
)

type BrowserInfo struct {
	Running bool `json:"running"`
	Port    int  `json:"port,omitempty"`
	PID     int  `json:"pid,omitempty"`
	Pages   int  `json:"pages,omitempty"`
}

type StartOptions struct {
	ChromePath string
	Headless   bool
	// Profile — каталог профиля; пустой — рядом со state этого порта.
	Profile string
}

func Start(ctx context.Context, env Env, opts StartOptions) (BrowserInfo, error) {
	st, err := env.Store.Load()
	switch {
	case err == nil && browser.Alive(st.PID):
		return BrowserInfo{}, fmt.Errorf("браузер уже запущен: порт %d, pid %d", st.Port, st.PID)
	case err == nil:
		if err := env.Store.Remove(); err != nil {
			return BrowserInfo{}, err
		}
	case !errors.Is(err, state.ErrNotRunning):
		return BrowserInfo{}, err
	}

	profile := opts.Profile
	if profile == "" {
		profile = filepath.Join(env.Store.Dir(), "profile")
	}
	proc, err := browser.Launch(ctx, opts.ChromePath, env.Port, profile, opts.Headless)
	if err != nil {
		return BrowserInfo{}, err
	}

	info, err := register(ctx, env, proc)
	if err != nil {
		_ = browser.Kill(proc.PID)
		return BrowserInfo{}, err
	}
	return info, nil
}

func register(ctx context.Context, env Env, proc browser.Process) (BrowserInfo, error) {
	c, err := cdp.Dial(ctx, proc.BrowserWSURL)
	if err != nil {
		return BrowserInfo{}, err
	}
	defer func() { _ = c.Close() }()

	// Порт открывается раньше, чем появляется стартовая вкладка.
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var targets []session.TargetInfo
	for {
		targets, err = session.PageTargets(ctx, c)
		if err != nil {
			return BrowserInfo{}, err
		}
		if len(targets) > 0 {
			break
		}
		select {
		case <-ctx.Done():
			return BrowserInfo{}, fmt.Errorf("запущенный Chrome не открыл ни одной вкладки: %w", ctx.Err())
		case <-ticker.C:
		}
	}

	st := state.State{
		Port:         proc.Port,
		PID:          proc.PID,
		UserDataDir:  proc.UserDataDir,
		BrowserWSURL: proc.BrowserWSURL,
		SelectedPage: targets[0].TargetID,
	}
	if err := env.Store.Save(st); err != nil {
		return BrowserInfo{}, err
	}
	return BrowserInfo{Running: true, Port: proc.Port, PID: proc.PID, Pages: len(targets)}, nil
}

func Stop(ctx context.Context, env Env) error {
	st, err := env.Store.Load()
	if err != nil {
		return err
	}

	if browser.Alive(st.PID) {
		if c, err := cdp.Dial(ctx, st.BrowserWSURL); err == nil {
			callCtx, cancel := context.WithTimeout(ctx, closeCallTimeout)
			// Chrome закрывает соединение, не всегда успев ответить: ошибка здесь ожидаема,
			// а результат проверяет ожидание выхода процесса ниже.
			_ = c.Call(callCtx, "", "Browser.close", nil, nil)
			cancel()
			_ = c.Close()
		}

		waitCtx, cancel := context.WithTimeout(ctx, exitTimeout)
		exitErr := browser.WaitExit(waitCtx, st.PID)
		cancel()
		if exitErr != nil {
			if err := browser.Kill(st.PID); err != nil {
				return err
			}
			killCtx, cancel := context.WithTimeout(ctx, killTimeout)
			defer cancel()
			if err := browser.WaitExit(killCtx, st.PID); err != nil {
				return err
			}
		}
	}
	return env.Store.Remove()
}

func Status(ctx context.Context, env Env) (BrowserInfo, error) {
	st, c, err := session.Connect(ctx, env)
	if errors.Is(err, state.ErrNotRunning) {
		return BrowserInfo{}, nil
	}
	if err != nil {
		return BrowserInfo{}, err
	}
	defer func() { _ = c.Close() }()

	targets, err := session.PageTargets(ctx, c)
	if err != nil {
		return BrowserInfo{}, err
	}
	return BrowserInfo{Running: true, Port: st.Port, PID: st.PID, Pages: len(targets)}, nil
}
