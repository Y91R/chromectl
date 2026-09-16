package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Y91R/chromectl/internal/cdp"
)

// autoAttachQuiet — сколько ждать следующего iframe после последнего
// подключённого: Chrome присылает существующие OOPIF сразу после setAutoAttach.
const (
	autoAttachQuiet = 300 * time.Millisecond
	detachTimeout   = 2 * time.Second
)

// FrameRef — где разрешать узлы фрейма: target сессии, фрейм и его документ.
type FrameRef struct {
	TargetID string
	FrameID  string
	LoaderID string
}

func FrameKey(frameID, loaderID string) string {
	return frameID + ":" + loaderID
}

type frameTarget struct {
	id  string
	sid string
}

type frameInfo struct {
	id       string
	loaderID string
	parentID string
	target   frameTarget
}

type frameTreeResponse struct {
	Frame struct {
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
		LoaderID string `json:"loaderId"`
	} `json:"frame"`
	ChildFrames []frameTreeResponse `json:"childFrames"`
}

// Collect снимает снапшот вкладки вместе со всеми iframe, включая cross-origin:
// их документы живут в отдельных target и подключаются через setAutoAttach.
// Дерево вложенного фрейма подвешивается к узлу его <iframe>.
func Collect(ctx context.Context, c *cdp.Client, pageID, pageSID string, verbose bool) (*Node, map[string]FrameRef, error) {
	targets := []frameTarget{{id: pageID, sid: pageSID}}
	attached, err := attachFrames(ctx, c, pageSID)
	defer detachAll(ctx, c, attached)
	if err != nil {
		return nil, nil, err
	}
	for _, a := range attached {
		if a.isIframe {
			targets = append(targets, a.frameTarget)
		}
	}

	frames, order, err := collectFrames(ctx, c, targets)
	if err != nil {
		return nil, nil, err
	}

	roots := map[string]*Node{}
	refs := map[string]FrameRef{}
	owners := map[string]int64{}
	mainID := ""
	for _, id := range order {
		f := frames[id]
		if f.parentID == "" && f.target.id == pageID {
			mainID = id
		}
		frameOwners := map[int64]bool{}
		for _, childID := range order {
			if frames[childID].parentID != id {
				continue
			}
			if owner, err := frameOwner(ctx, c, f.target.sid, childID); err == nil {
				frameOwners[owner] = true
				owners[childID] = owner
			}
		}

		var tree struct {
			Nodes []AXNode `json:"nodes"`
		}
		if err := c.Call(ctx, f.target.sid, "Accessibility.getFullAXTree", map[string]any{"frameId": id}, &tree); err != nil {
			// Фрейм мог исчезнуть между getFrameTree и getFullAXTree: снапшот без него честнее отказа.
			if id == mainID {
				return nil, nil, err
			}
			continue
		}
		root, err := buildTree(tree.Nodes, verbose, frameOwners)
		if err != nil {
			if id == mainID {
				return nil, nil, err
			}
			continue
		}
		key := FrameKey(id, f.loaderID)
		setFrameKey(root, key)
		refs[key] = FrameRef{TargetID: f.target.id, FrameID: id, LoaderID: f.loaderID}
		roots[id] = root
	}

	mainRoot := roots[mainID]
	if mainRoot == nil {
		return nil, nil, errors.New("не удалось снять дерево доступности главного фрейма")
	}
	for _, id := range order {
		root := roots[id]
		if id == mainID || root == nil {
			continue
		}
		parentID := frames[id].parentID
		host := mainRoot
		if parentRoot := roots[parentID]; parentRoot != nil {
			if owner := findNode(parentRoot, parentRoot.FrameKey, owners[id]); owner != nil {
				host = owner
			}
		}
		host.Children = append(host.Children, root)
	}
	return mainRoot, refs, nil
}

func collectFrames(ctx context.Context, c *cdp.Client, targets []frameTarget) (map[string]*frameInfo, []string, error) {
	frames := map[string]*frameInfo{}
	var order []string
	for i, t := range targets {
		var res struct {
			FrameTree frameTreeResponse `json:"frameTree"`
		}
		if err := c.Call(ctx, t.sid, "Page.getFrameTree", nil, &res); err != nil {
			if i == 0 {
				return nil, nil, err
			}
			continue
		}
		var walk func(ft frameTreeResponse, parentID string)
		walk = func(ft frameTreeResponse, parentID string) {
			id := ft.Frame.ID
			if ft.Frame.ParentID != "" {
				parentID = ft.Frame.ParentID
			}
			existing, known := frames[id]
			switch {
			case !known:
				frames[id] = &frameInfo{id: id, loaderID: ft.Frame.LoaderID, parentID: parentID, target: t}
				order = append(order, id)
			case id == t.id:
				// Документ cross-origin iframe принадлежит его собственному target,
				// даже если родитель тоже перечислил этот фрейм.
				existing.loaderID = ft.Frame.LoaderID
				existing.target = t
				if parentID != "" {
					existing.parentID = parentID
				}
			}
			for _, child := range ft.ChildFrames {
				walk(child, id)
			}
		}
		walk(res.FrameTree, "")
	}
	return frames, order, nil
}

type attachedTarget struct {
	frameTarget
	isIframe bool
}

// attachFrames подключает cross-origin iframe вкладки и вложенных iframe.
func attachFrames(ctx context.Context, c *cdp.Client, sid string) ([]attachedTarget, error) {
	sub := c.Subscribe(sid, "Target.attachedToTarget")
	defer sub.Close()
	autoAttach := map[string]any{"autoAttach": true, "waitForDebuggerOnStart": false, "flatten": true}
	if err := c.Call(ctx, sid, "Target.setAutoAttach", autoAttach, nil); err != nil {
		return nil, err
	}

	var found []attachedTarget
collect:
	for {
		select {
		case ev, open := <-sub.C:
			if !open {
				break collect
			}
			var p struct {
				SessionID  string `json:"sessionId"`
				TargetInfo struct {
					TargetID string `json:"targetId"`
					Type     string `json:"type"`
				} `json:"targetInfo"`
			}
			if json.Unmarshal(ev.Params, &p) == nil {
				found = append(found, attachedTarget{
					frameTarget: frameTarget{id: p.TargetInfo.TargetID, sid: p.SessionID},
					isIframe:    p.TargetInfo.Type == "iframe",
				})
			}
		case <-time.After(autoAttachQuiet):
			break collect
		case <-ctx.Done():
			return found, ctx.Err()
		}
	}
	// autoAttach не выключается: Chrome при этом отсоединяет уже подключённые
	// iframe, и обход вложенных упал бы с «Session with given id not found».
	// Сессии отсоединяет detachAll в конце снапшота.
	direct := len(found)
	for i := 0; i < direct; i++ {
		if !found[i].isIframe {
			continue
		}
		nested, err := attachFrames(ctx, c, found[i].sid)
		found = append(found, nested...)
		if err != nil {
			return found, err
		}
	}
	return found, nil
}

func detachAll(ctx context.Context, c *cdp.Client, attached []attachedTarget) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), detachTimeout)
	defer cancel()
	for i := len(attached) - 1; i >= 0; i-- {
		_ = c.Call(ctx, "", "Target.detachFromTarget", map[string]any{"sessionId": attached[i].sid}, nil)
	}
}

func frameOwner(ctx context.Context, c *cdp.Client, sid, frameID string) (int64, error) {
	var res struct {
		BackendNodeID int64 `json:"backendNodeId"`
	}
	err := c.Call(ctx, sid, "DOM.getFrameOwner", map[string]any{"frameId": frameID}, &res)
	if err != nil {
		// DOM.getFrameOwner требует включённого домена DOM в этой сессии.
		if enableErr := c.Call(ctx, sid, "DOM.enable", nil, nil); enableErr != nil {
			return 0, err
		}
		err = c.Call(ctx, sid, "DOM.getFrameOwner", map[string]any{"frameId": frameID}, &res)
	}
	return res.BackendNodeID, err
}

func setFrameKey(n *Node, key string) {
	n.FrameKey = key
	for _, child := range n.Children {
		setFrameKey(child, key)
	}
}

func findNode(n *Node, frameKey string, backendNodeID int64) *Node {
	if backendNodeID == 0 {
		return nil
	}
	if n.FrameKey == frameKey && n.BackendNodeID == backendNodeID {
		return n
	}
	for _, child := range n.Children {
		if found := findNode(child, frameKey, backendNodeID); found != nil {
			return found
		}
	}
	return nil
}
