// Package snapshot превращает дерево доступности Chrome в текстовый снапшот с uid
// вида `uid=1_2 textbox "Имя" focusable focused value="Иван"`.
package snapshot

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
)

// AXNode — узел ответа Accessibility.getFullAXTree.
type AXNode struct {
	NodeID           string       `json:"nodeId"`
	Ignored          bool         `json:"ignored"`
	Role             *AXValue     `json:"role"`
	Name             *AXValue     `json:"name"`
	Description      *AXValue     `json:"description"`
	Value            *AXValue     `json:"value"`
	Properties       []AXProperty `json:"properties"`
	ParentID         string       `json:"parentId"`
	ChildIDs         []string     `json:"childIds"`
	BackendDOMNodeID int64        `json:"backendDOMNodeId"`
	FrameID          string       `json:"frameId"`
}

type AXValue struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type AXProperty struct {
	Name  string  `json:"name"`
	Value AXValue `json:"value"`
}

// Node — узел снапшота. Attrs хранит только то, что печатается после роли и
// имени: true, строку или число.
type Node struct {
	UID           string
	Role          string
	Name          string
	Attrs         map[string]any
	Children      []*Node
	BackendNodeID int64
	// FrameKey различает документы: одинаковый backendNodeId в разных фреймах —
	// разные узлы. Заполняет сборщик.
	FrameKey string
}

var (
	controlRoles = set("button", "checkbox", "ColorWell", "combobox", "DisclosureTriangle", "listbox", "menu", "menubar",
		"menuitem", "menuitemcheckbox", "menuitemradio", "radio", "scrollbar", "searchbox", "slider", "spinbutton",
		"switch", "tab", "textbox", "tree", "treeitem")
	textOnlyRoles = set("LineBreak", "text", "InlineTextBox", "StaticText")
	leafRoles     = set("doc-cover", "graphics-symbol", "img", "image", "Meter", "scrollbar", "slider", "separator", "progressbar")

	userStringProperties = []string{"keyshortcuts", "roledescription", "valuetext"}
	booleanProperties    = []string{"disabled", "expanded", "focused", "modal", "multiline", "multiselectable", "readonly", "required", "selected"}
	tristateProperties   = []string{"checked", "pressed"}
	rangeProperties      = []string{"level", "valuemax", "valuemin"}
	tokenProperties      = []string{"autocomplete", "haspopup", "invalid", "orientation"}

	// У выставленного флага печатается и его «способность».
	booleanPropertyMap = map[string]string{
		"disabled": "disableable",
		"expanded": "expandable",
		"focused":  "focusable",
		"selected": "selectable",
	}
)

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}

type axNode struct {
	payload  *AXNode
	role     string
	name     string
	props    map[string]AXValue
	children []*axNode

	focusableChild *bool
}

func newAXNode(p *AXNode) *axNode {
	n := &axNode{payload: p, role: "Unknown", props: map[string]AXValue{}}
	if p.Role != nil {
		n.role = stringValue(p.Role.Value)
	}
	if p.Name != nil {
		n.name = stringValue(p.Name.Value)
	}
	for _, prop := range p.Properties {
		n.props[prop.Name] = prop.Value
	}
	return n
}

func (n *axNode) boolProp(name string) bool {
	v, ok := n.props[name]
	if !ok {
		return false
	}
	var b bool
	return json.Unmarshal(v.Value, &b) == nil && b
}

func (n *axNode) tokenProp(name string) string {
	v, ok := n.props[name]
	if !ok {
		return ""
	}
	return stringValue(v.Value)
}

func (n *axNode) focusable() bool      { return n.boolProp("focusable") }
func (n *axNode) hidden() bool         { return n.boolProp("hidden") }
func (n *axNode) richlyEditable() bool { return n.tokenProp("editable") == "richtext" }
func (n *axNode) editable() bool       { return n.tokenProp("editable") != "" }
func (n *axNode) isControl() bool      { return controlRoles[n.role] }

func (n *axNode) isPlainTextField() bool {
	if n.richlyEditable() {
		return false
	}
	return n.editable() || n.role == "textbox" || n.role == "searchbox"
}

func (n *axNode) hasFocusableChild() bool {
	if n.focusableChild == nil {
		found := false
		for _, c := range n.children {
			if c.focusable() || c.hasFocusableChild() {
				found = true
				break
			}
		}
		n.focusableChild = &found
	}
	return *n.focusableChild
}

func (n *axNode) isLeaf() bool {
	switch {
	case len(n.children) == 0, n.isPlainTextField(), textOnlyRoles[n.role], leafRoles[n.role]:
		return true
	case n.hasFocusableChild():
		return false
	case n.focusable() && n.name != "":
		return true
	default:
		return n.role == "heading" && n.name != ""
	}
}

func (n *axNode) isInteresting(insideControl bool) bool {
	switch {
	case n.payload.Ignored || n.hidden():
		return false
	case n.focusable() || n.richlyEditable(), n.isControl():
		return true
	case insideControl:
		return false
	default:
		return n.isLeaf() && n.name != ""
	}
}

// Build строит дерево снапшота. Без verbose остаются только «интересные» узлы,
// остальные схлопываются в своих детей.
func Build(nodes []AXNode, verbose bool) (*Node, error) {
	return buildTree(nodes, verbose, nil)
}

// buildTree — Build, где узлы-владельцы iframe остаются в снапшоте всегда: к ним
// подвешивается дерево вложенного фрейма.
func buildTree(nodes []AXNode, verbose bool, frameOwners map[int64]bool) (*Node, error) {
	if len(nodes) == 0 {
		return nil, errors.New("дерево доступности пустое")
	}
	byID := make(map[string]*axNode, len(nodes))
	for i := range nodes {
		byID[nodes[i].NodeID] = newAXNode(&nodes[i])
	}
	var root *axNode
	for i := range nodes {
		n := byID[nodes[i].NodeID]
		for _, id := range nodes[i].ChildIDs {
			if child, ok := byID[id]; ok {
				n.children = append(n.children, child)
			}
		}
		if root == nil && nodes[i].ParentID == "" {
			root = n
		}
	}
	if root == nil {
		return nil, errors.New("в дереве доступности нет корня")
	}

	var interesting map[*axNode]bool
	if !verbose {
		interesting = map[*axNode]bool{root: true}
		collectInteresting(root, false, interesting)
		for _, n := range byID {
			if frameOwners[n.payload.BackendDOMNodeID] {
				interesting[n] = true
			}
		}
	}
	serialized := serialize(root, interesting)
	if len(serialized) != 1 {
		return nil, errors.New("корень дерева доступности не попал в снапшот")
	}
	return serialized[0], nil
}

func collectInteresting(n *axNode, insideControl bool, interesting map[*axNode]bool) {
	if n.isInteresting(insideControl) {
		interesting[n] = true
	}
	if n.isLeaf() {
		return
	}
	insideControl = insideControl || n.isControl()
	for _, c := range n.children {
		collectInteresting(c, insideControl, interesting)
	}
}

// serialize возвращает узел с детьми, а неинтересный узел заменяет его детьми.
// interesting == nil означает verbose: попадают все узлы.
func serialize(n *axNode, interesting map[*axNode]bool) []*Node {
	var children []*Node
	for _, c := range n.children {
		children = append(children, serialize(c, interesting)...)
	}
	if interesting != nil && !interesting[n] {
		return children
	}
	return []*Node{{
		Role:          n.role,
		Name:          n.name,
		Attrs:         n.attrs(),
		Children:      children,
		BackendNodeID: n.payload.BackendDOMNodeID,
	}}
}

func (n *axNode) attrs() map[string]any {
	attrs := map[string]any{}
	if n.payload.Value != nil {
		if v := scalar(n.payload.Value.Value); v != nil && v != "" {
			attrs["value"] = v
		}
	}
	if n.payload.Description != nil {
		if d := stringValue(n.payload.Description.Value); d != "" {
			attrs["description"] = d
		}
	}
	for _, key := range userStringProperties {
		if s := n.tokenProp(key); s != "" {
			attrs[key] = s
		}
	}
	for _, key := range booleanProperties {
		if key == "focused" && n.role == "RootWebArea" {
			continue
		}
		if n.boolProp(key) {
			attrs[key] = true
		}
	}
	for _, key := range tristateProperties {
		switch n.tokenProp(key) {
		case "mixed":
			attrs[key] = "mixed"
		case "true":
			attrs[key] = true
		}
	}
	for _, key := range rangeProperties {
		if v, ok := n.props[key]; ok {
			if num := scalar(v.Value); num != nil {
				attrs[key] = num
			}
		}
	}
	for _, key := range tokenProperties {
		if s := n.tokenProp(key); s != "" && s != "false" {
			attrs[key] = s
		}
	}
	// У option в дереве доступности нет value, а select выбирает по тексту.
	if n.role == "option" && n.name != "" {
		attrs["value"] = n.name
	}
	return attrs
}

func stringValue(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

func scalar(raw json.RawMessage) any {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return f
	}
	return nil
}

// AssignUIDs раздаёт uid вида <snapshotID>_<n> в порядке обхода. Узел, чей ключ
// FrameKey+backendNodeId есть в previous, сохраняет прежний uid. Возвращает
// ключи и uid всех узлов этого снапшота.
func AssignUIDs(root *Node, snapshotID int, previous map[string]string) map[string]string {
	seen := map[string]string{}
	counter := 0
	prefix := strconv.Itoa(snapshotID) + "_"

	var walk func(n *Node)
	walk = func(n *Node) {
		key := Key(n.FrameKey, n.BackendNodeID)
		switch uid, known := seen[key]; {
		case n.BackendNodeID == 0:
			// У узла без DOM-узла (InlineTextBox) ключ «фрейм_0» общий для всех
			// таких узлов: переиспользовать нечего, и в карту он не попадает.
			n.UID = prefix + strconv.Itoa(counter)
			counter++
		case known:
			n.UID = uid
		default:
			if prev, ok := previous[key]; ok {
				n.UID = prev
			} else {
				n.UID = prefix + strconv.Itoa(counter)
				counter++
			}
			seen[key] = n.UID
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return seen
}

func Key(frameKey string, backendNodeID int64) string {
	return frameKey + "_" + strconv.FormatInt(backendNodeID, 10)
}

func Format(root *Node) string {
	var b strings.Builder
	format(&b, root, 0)
	return b.String()
}

func format(b *strings.Builder, n *Node, depth int) {
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString("uid=")
	b.WriteString(n.UID)
	if n.Role != "" {
		b.WriteByte(' ')
		if n.Role == "none" {
			b.WriteString("ignored")
		} else {
			b.WriteString(n.Role)
		}
	}
	if n.Name != "" {
		b.WriteString(` "` + n.Name + `"`)
	}

	keys := make([]string, 0, len(n.Attrs))
	for k := range n.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch v := n.Attrs[k].(type) {
		case bool:
			if !v {
				continue
			}
			if mapped, ok := booleanPropertyMap[k]; ok {
				b.WriteString(" " + mapped)
			}
			b.WriteString(" " + k)
		case string:
			b.WriteString(" " + k + `="` + v + `"`)
		case float64:
			b.WriteString(" " + k + `="` + strconv.FormatFloat(v, 'f', -1, 64) + `"`)
		}
	}
	b.WriteByte('\n')

	for _, c := range n.Children {
		format(b, c, depth+1)
	}
}
