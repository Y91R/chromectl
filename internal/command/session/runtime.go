package session

import (
	"context"
	"encoding/json"

	"github.com/Y91R/chromectl/internal/snapshot"
	"github.com/Y91R/chromectl/internal/state"
)

// ExceptionDetails — исключение JS из Runtime.evaluate и Runtime.callFunctionOn.
type ExceptionDetails struct {
	Text      string `json:"text"`
	Exception *struct {
		Description string `json:"description"`
	} `json:"exception"`
}

func (e *ExceptionDetails) Error() string {
	if e.Exception != nil && e.Exception.Description != "" {
		return e.Exception.Description
	}
	return e.Text
}

type RemoteResult struct {
	Result struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	} `json:"result"`
	ExceptionDetails *ExceptionDetails `json:"exceptionDetails"`
}

// CallOn вызывает функцию с this = элемент и аргументами-значениями; result
// получает возвращённое значение.
func (p *Page) CallOn(ctx context.Context, el Element, function string, args []any, result any) error {
	arguments := make([]map[string]any, 0, len(args))
	for _, a := range args {
		arguments = append(arguments, map[string]any{"value": a})
	}
	var res RemoteResult
	if err := p.Client.Call(ctx, el.SessionID, "Runtime.callFunctionOn", map[string]any{
		"objectId":            el.ObjectID,
		"functionDeclaration": function,
		"arguments":           arguments,
		"returnByValue":       true,
		"awaitPromise":        true,
	}, &res); err != nil {
		return err
	}
	if res.ExceptionDetails != nil {
		return res.ExceptionDetails
	}
	if result != nil && len(res.Result.Value) > 0 {
		return json.Unmarshal(res.Result.Value, result)
	}
	return nil
}

// Snapshot снимает снапшот вкладки и запоминает маршруты uid в state. Узел того
// же документа сохраняет uid прежнего снапшота этой вкладки.
func (p *Page) Snapshot(ctx context.Context, verbose bool) (string, error) {
	root, frames, err := snapshot.Collect(ctx, p.Client, p.ID, p.SessionID, verbose)
	if err != nil {
		return "", err
	}

	previous := map[string]string{}
	for uid, ref := range p.State.UIDs {
		if ref.PageID == p.ID {
			previous[snapshot.Key(snapshot.FrameKey(ref.FrameID, ref.LoaderID), ref.BackendNodeID)] = uid
		}
	}

	// Номер снапшота выдаётся сразу под блокировкой state: у снапшотов соседних
	// команд должны быть разные префиксы uid.
	var seq int
	if err := p.Env.Store.Update(func(st *state.State) {
		st.SnapshotSeq++
		seq = st.SnapshotSeq
	}); err != nil {
		return "", err
	}
	p.State.SnapshotSeq = seq
	snapshot.AssignUIDs(root, seq, previous)

	uids := map[string]state.NodeRef{}

	var walk func(n *snapshot.Node)
	walk = func(n *snapshot.Node) {
		// Узел без DOM-узла (InlineTextBox) действию недоступен — маршрута к нему нет.
		if fr, ok := frames[n.FrameKey]; ok && n.BackendNodeID != 0 {
			uids[n.UID] = state.NodeRef{
				PageID:        p.ID,
				TargetID:      fr.TargetID,
				FrameID:       fr.FrameID,
				LoaderID:      fr.LoaderID,
				BackendNodeID: n.BackendNodeID,
			}
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(root)

	p.Change(func(st *state.State) {
		merged := map[string]state.NodeRef{}
		for uid, ref := range st.UIDs {
			if ref.PageID != p.ID {
				merged[uid] = ref
			}
		}
		for uid, ref := range uids {
			merged[uid] = ref
		}
		st.UIDs = merged
	})
	return snapshot.Format(root), nil
}
