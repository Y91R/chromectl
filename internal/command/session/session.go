// Package session — общее для команд, работающих с вкладкой: подключение по
// state, выбор вкладки, сессия CDP, страж диалогов, ожидание последствий действия.
//
// ADR: docs/adr/0009-cli-bez-demona.md — одна команда — одно подключение.
package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Y91R/chromectl/internal/browser"
	"github.com/Y91R/chromectl/internal/cdp"
	"github.com/Y91R/chromectl/internal/emulation"
	"github.com/Y91R/chromectl/internal/state"
)

const (
	detachTimeout = 2 * time.Second
	pollInterval  = 100 * time.Millisecond
)

type Env struct {
	Store *state.Store
	Port  int
	// Page — вкладка из --page вместо выбранной в state.
	Page string
}

type TargetInfo struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

func Connect(ctx context.Context, env Env) (state.State, *cdp.Client, error) {
	st, err := env.Store.Load()
	if err != nil {
		return state.State{}, nil, err
	}
	c, err := cdp.Dial(ctx, st.BrowserWSURL)
	if err != nil {
		if !browser.Alive(st.PID) {
			// Chrome закрыли мимо chromectl: устаревший state только мешал бы следующему start.
			if rmErr := env.Store.Remove(); rmErr != nil {
				return state.State{}, nil, rmErr
			}
			return state.State{}, nil, state.ErrNotRunning
		}
		return state.State{}, nil, fmt.Errorf("подключение к браузеру на порту %d: %w", st.Port, err)
	}
	return st, c, nil
}

func PageTargets(ctx context.Context, c *cdp.Client) ([]TargetInfo, error) {
	var res struct {
		TargetInfos []TargetInfo `json:"targetInfos"`
	}
	if err := c.Call(ctx, "", "Target.getTargets", nil, &res); err != nil {
		return nil, err
	}
	var result []TargetInfo
	for _, t := range res.TargetInfos {
		if t.Type == "page" {
			result = append(result, t)
		}
	}
	return result, nil
}

func FindTarget(targets []TargetInfo, id string) (TargetInfo, bool) {
	for _, t := range targets {
		if t.TargetID == id {
			return t, true
		}
	}
	return TargetInfo{}, false
}

func GetTargetInfo(ctx context.Context, c *cdp.Client, id string) (TargetInfo, error) {
	var res struct {
		TargetInfo TargetInfo `json:"targetInfo"`
	}
	if err := c.Call(ctx, "", "Target.getTargetInfo", map[string]any{"targetId": id}, &res); err != nil {
		return TargetInfo{}, err
	}
	return res.TargetInfo, nil
}

func ResolvePage(ctx context.Context, c *cdp.Client, env Env, st state.State) (string, error) {
	targets, err := PageTargets(ctx, c)
	if err != nil {
		return "", err
	}
	id := env.Page
	if id == "" {
		id = st.SelectedPage
	}
	if _, ok := FindTarget(targets, id); ok {
		return id, nil
	}
	if env.Page != "" {
		return "", fmt.Errorf("вкладка %s не найдена", id)
	}
	return "", fmt.Errorf("выбранная вкладка %s закрыта, выполните chromectl pages select <id>", id)
}

func Attach(ctx context.Context, c *cdp.Client, targetID string) (string, error) {
	var res struct {
		SessionID string `json:"sessionId"`
	}
	if err := c.Call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, &res); err != nil {
		return "", err
	}
	return res.SessionID, nil
}

// Detach отсоединяет сессию, не закрывая вкладку. Контекст команды к этому
// моменту может быть уже отменён, поэтому у отсоединения свой короткий таймаут.
func Detach(ctx context.Context, c *cdp.Client, sessionID string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), detachTimeout)
	defer cancel()
	_ = c.Call(ctx, "", "Target.detachFromTarget", map[string]any{"sessionId": sessionID}, nil)
}

func BlockedError(pageID string, err error) error {
	return fmt.Errorf("вкладка %s не отвечает — возможно, её блокирует диалог, оставшийся от другой команды; выполните chromectl navigate --reload: %w", pageID, err)
}

// DialogFlag — значение --dialog и то, задан ли флаг явно.
type DialogFlag struct {
	Value string
	Set   bool
}

// DialogPolicyFor выбирает ответ на диалог: явный --dialog, иначе одноразовая
// политика из `chromectl dialog`, иначе accept. consumed — политика из state
// израсходована, и state нужно сохранить.
func DialogPolicyFor(flag DialogFlag, st *state.State) (policy DialogPolicy, consumed bool) {
	if flag.Set {
		return ParseDialogPolicy(flag.Value), false
	}
	if st.PendingDialog != nil {
		policy = DialogPolicy{Accept: st.PendingDialog.Accept, PromptText: st.PendingDialog.PromptText}
		st.PendingDialog = nil
		return policy, true
	}
	return DialogPolicy{Accept: true}, false
}

// Page — подключение к выбранной вкладке на время одной команды.
type Page struct {
	Env       Env
	State     state.State
	Client    *cdp.Client
	ID        string
	SessionID string

	dialogs *Dialogs
	frames  map[string]string
	changes []func(*state.State)
	closed  bool
}

// Open подключается к вкладке. dialog == nil — команда только читает страницу и
// не расходует политику диалога; иначе на время команды включается страж диалогов.
func Open(ctx context.Context, env Env, dialog *DialogFlag) (*Page, error) {
	st, c, err := Connect(ctx, env)
	if err != nil {
		return nil, err
	}
	id, err := ResolvePage(ctx, c, env, st)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	sid, err := Attach(ctx, c, id)
	if err != nil {
		_ = c.Close()
		return nil, err
	}

	p := &Page{Env: env, State: st, Client: c, ID: id, SessionID: sid, frames: map[string]string{}}
	if err := emulation.Apply(ctx, c, sid, st.Emulation[id]); err != nil {
		_, _ = p.Close(ctx)
		return nil, err
	}
	if dialog == nil {
		return p, nil
	}
	pending := p.State.PendingDialog
	policy, consumed := DialogPolicyFor(*dialog, &p.State)
	if consumed {
		used := *pending
		p.changes = append(p.changes, func(st *state.State) { st.ClearPendingDialog(used) })
	}
	dialogs, err := WatchDialogs(ctx, c, sid, policy)
	if err != nil {
		_, _ = p.Close(ctx)
		return nil, BlockedError(id, err)
	}
	p.dialogs = dialogs
	return p, nil
}

// Change применяет изменение к State команды и запоминает его: Close накатит его
// на свежий state, не затирая то, что за время команды записали другие.
func (p *Page) Change(fn func(*state.State)) {
	fn(&p.State)
	p.changes = append(p.changes, fn)
}

// FrameSession возвращает сессию cross-origin iframe, подключаясь один раз за команду.
func (p *Page) FrameSession(ctx context.Context, targetID string) (string, error) {
	if targetID == p.ID {
		return p.SessionID, nil
	}
	if sid, ok := p.frames[targetID]; ok {
		return sid, nil
	}
	sid, err := Attach(ctx, p.Client, targetID)
	if err != nil {
		return "", err
	}
	p.frames[targetID] = sid
	return sid, nil
}

// Close останавливает страж диалогов, отсоединяет сессии, сохраняет изменённый
// state и возвращает диалоги, закрытые за время команды.
func (p *Page) Close(ctx context.Context) ([]DialogEvent, error) {
	if p.closed {
		return nil, nil
	}
	p.closed = true

	var events []DialogEvent
	var err error
	if p.dialogs != nil {
		events, err = p.dialogs.Stop()
	}
	for _, sid := range p.frames {
		Detach(ctx, p.Client, sid)
	}
	Detach(ctx, p.Client, p.SessionID)
	_ = p.Client.Close()

	if len(p.changes) > 0 {
		apply := func(st *state.State) {
			for _, fn := range p.changes {
				fn(st)
			}
		}
		if saveErr := p.Env.Store.Update(apply); saveErr != nil {
			err = errors.Join(err, saveErr)
		}
	}
	return events, err
}
