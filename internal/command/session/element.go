package session

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/Y91R/chromectl/internal/state"
)

var ErrStaleSnapshot = errors.New("снапшот устарел, выполните chromectl snapshot")

// Element — DOM-узел, найденный по uid, в сессии своего target.
type Element struct {
	UID       string
	Ref       state.NodeRef
	SessionID string
	ObjectID  string
}

// Element находит узел по uid из последнего снапшота. Узел документа, который
// перезагрузили или сменили переходом, DOM.resolveNode уже не находит — это и
// есть признак устаревшего снапшота.
func (p *Page) Element(ctx context.Context, uid string) (Element, error) {
	ref, ok := p.State.UIDs[uid]
	if !ok {
		return Element{}, fmt.Errorf("uid %s не найден в последнем снапшоте", uid)
	}
	if ref.PageID != p.ID {
		return Element{}, fmt.Errorf("uid %s из снапшота вкладки %s, а команда работает с вкладкой %s", uid, ref.PageID, p.ID)
	}

	sid, err := p.FrameSession(ctx, ref.TargetID)
	if err != nil {
		return Element{}, fmt.Errorf("%w (iframe %s недоступен: %v)", ErrStaleSnapshot, ref.TargetID, err)
	}

	var res struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := p.Client.Call(ctx, sid, "DOM.resolveNode", map[string]any{"backendNodeId": ref.BackendNodeID}, &res); err != nil {
		return Element{}, fmt.Errorf("%w (%v)", ErrStaleSnapshot, err)
	}
	return Element{UID: uid, Ref: ref, SessionID: sid, ObjectID: res.Object.ObjectID}, nil
}

// Box — прямоугольник элемента в координатах окна его target, CSS-пиксели.
type Box struct {
	X, Y, Width, Height float64
}

func (b Box) Center() (x, y float64) {
	return b.X + b.Width/2, b.Y + b.Height/2
}

// Box прокручивает элемент в видимую область и возвращает его рамку.
func (p *Page) Box(ctx context.Context, el Element) (Box, error) {
	params := map[string]any{"backendNodeId": el.Ref.BackendNodeID}
	if err := p.Client.Call(ctx, el.SessionID, "DOM.scrollIntoViewIfNeeded", params, nil); err != nil {
		return Box{}, fmt.Errorf("прокрутка к %s: %w", el.UID, err)
	}
	var res struct {
		Quads [][]float64 `json:"quads"`
	}
	if err := p.Client.Call(ctx, el.SessionID, "DOM.getContentQuads", params, &res); err != nil {
		return Box{}, fmt.Errorf("рамка %s: %w", el.UID, err)
	}
	if len(res.Quads) == 0 || len(res.Quads[0]) != 8 {
		return Box{}, fmt.Errorf("элемент %s не отображается на странице", el.UID)
	}
	q := res.Quads[0]
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for i := 0; i < 8; i += 2 {
		minX, maxX = math.Min(minX, q[i]), math.Max(maxX, q[i])
		minY, maxY = math.Min(minY, q[i+1]), math.Max(maxY, q[i+1])
	}
	return Box{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}, nil
}
