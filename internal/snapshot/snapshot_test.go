package snapshot_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Y91R/chromectl/internal/snapshot"
)

// formTree — ответ Accessibility.getFullAXTree в форме CDP для страницы с
// заголовком, полем, чекбоксом и кнопкой внутри безымянного div.
const formTree = `[
 {"nodeId":"1","ignored":false,"role":{"type":"internalRole","value":"RootWebArea"},"name":{"type":"computedString","value":"Форма"},
  "properties":[{"name":"focusable","value":{"type":"booleanOrUndefined","value":true}},{"name":"focused","value":{"type":"booleanOrUndefined","value":true}}],
  "childIds":["2","6"],"backendDOMNodeId":1},
 {"nodeId":"2","ignored":false,"role":{"type":"role","value":"generic"},"name":{"type":"computedString","value":""},"parentId":"1","childIds":["3","4","5","9"],"backendDOMNodeId":2},
 {"nodeId":"3","ignored":false,"role":{"type":"role","value":"heading"},"name":{"type":"computedString","value":"Анкета"},
  "properties":[{"name":"level","value":{"type":"integer","value":1}}],"parentId":"2","childIds":["31"],"backendDOMNodeId":3},
 {"nodeId":"31","ignored":false,"role":{"type":"internalRole","value":"StaticText"},"name":{"type":"computedString","value":"Анкета"},"parentId":"3","childIds":[],"backendDOMNodeId":31},
 {"nodeId":"4","ignored":false,"role":{"type":"role","value":"textbox"},"name":{"type":"computedString","value":"Имя"},"value":{"type":"string","value":"Иван"},
  "properties":[{"name":"focusable","value":{"type":"booleanOrUndefined","value":true}},{"name":"focused","value":{"type":"booleanOrUndefined","value":true}},
   {"name":"editable","value":{"type":"token","value":"plaintext"}},{"name":"invalid","value":{"type":"token","value":"false"}}],
  "parentId":"2","childIds":[],"backendDOMNodeId":4},
 {"nodeId":"5","ignored":false,"role":{"type":"role","value":"checkbox"},"name":{"type":"computedString","value":"Согласен"},
  "properties":[{"name":"focusable","value":{"type":"booleanOrUndefined","value":true}},{"name":"checked","value":{"type":"tristate","value":"true"}}],
  "parentId":"2","childIds":[],"backendDOMNodeId":5},
 {"nodeId":"9","ignored":false,"role":{"type":"role","value":"button"},"name":{"type":"computedString","value":"Отправить"},
  "properties":[{"name":"focusable","value":{"type":"booleanOrUndefined","value":true}},{"name":"disabled","value":{"type":"boolean","value":true}},
   {"name":"haspopup","value":{"type":"token","value":"menu"}},{"name":"pressed","value":{"type":"tristate","value":"mixed"}}],
  "parentId":"2","childIds":["91"],"backendDOMNodeId":9},
 {"nodeId":"91","ignored":false,"role":{"type":"internalRole","value":"StaticText"},"name":{"type":"computedString","value":"Отправить"},"parentId":"9","childIds":[],"backendDOMNodeId":91},
 {"nodeId":"6","ignored":true,"role":{"type":"role","value":"none"},"parentId":"1","childIds":[],"backendDOMNodeId":6}
]`

func parse(t *testing.T, raw string) []snapshot.AXNode {
	t.Helper()
	var nodes []snapshot.AXNode
	require.NoError(t, json.Unmarshal([]byte(raw), &nodes))
	return nodes
}

func setFrameKey(n *snapshot.Node, key string) {
	n.FrameKey = key
	for _, c := range n.Children {
		setFrameKey(c, key)
	}
}

func TestFormat_InterestingNodesOnly(t *testing.T) {
	t.Parallel()
	root, err := snapshot.Build(parse(t, formTree), false)
	require.NoError(t, err)
	setFrameKey(root, "F1")
	snapshot.AssignUIDs(root, 1, nil)

	assert.Equal(t, `uid=1_0 RootWebArea "Форма"
  uid=1_1 heading "Анкета" level="1"
  uid=1_2 textbox "Имя" focusable focused value="Иван"
  uid=1_3 checkbox "Согласен" checked
  uid=1_4 button "Отправить" disableable disabled haspopup="menu" pressed="mixed"
`, snapshot.Format(root))
}

func TestFormat_Verbose(t *testing.T) {
	t.Parallel()
	root, err := snapshot.Build(parse(t, formTree), true)
	require.NoError(t, err)
	setFrameKey(root, "F1")
	snapshot.AssignUIDs(root, 1, nil)

	assert.Equal(t, `uid=1_0 RootWebArea "Форма"
  uid=1_1 generic
    uid=1_2 heading "Анкета" level="1"
      uid=1_3 StaticText "Анкета"
    uid=1_4 textbox "Имя" focusable focused value="Иван"
    uid=1_5 checkbox "Согласен" checked
    uid=1_6 button "Отправить" disableable disabled haspopup="menu" pressed="mixed"
      uid=1_7 StaticText "Отправить"
  uid=1_8 ignored
`, snapshot.Format(root))
}

func TestFormat_OptionValueIsItsName(t *testing.T) {
	t.Parallel()
	const tree = `[
 {"nodeId":"1","role":{"type":"internalRole","value":"RootWebArea"},"name":{"type":"computedString","value":"Город"},"childIds":["2"],"backendDOMNodeId":1},
 {"nodeId":"2","role":{"type":"role","value":"combobox"},"name":{"type":"computedString","value":"Город"},
  "properties":[{"name":"focusable","value":{"type":"booleanOrUndefined","value":true}},{"name":"expanded","value":{"type":"booleanOrUndefined","value":false}}],
  "parentId":"1","childIds":["3"],"backendDOMNodeId":2},
 {"nodeId":"3","role":{"type":"role","value":"option"},"name":{"type":"computedString","value":"Москва"},
  "properties":[{"name":"selected","value":{"type":"booleanOrUndefined","value":true}}],"parentId":"2","childIds":[],"backendDOMNodeId":3}
]`
	root, err := snapshot.Build(parse(t, tree), true)
	require.NoError(t, err)
	setFrameKey(root, "F1")
	snapshot.AssignUIDs(root, 1, nil)

	assert.Equal(t, `uid=1_0 RootWebArea "Город"
  uid=1_1 combobox "Город"
    uid=1_2 option "Москва" selectable selected value="Москва"
`, snapshot.Format(root))
}

func TestAssignUIDs_ReusesUIDsOfKnownNodes(t *testing.T) {
	t.Parallel()
	first, err := snapshot.Build(parse(t, formTree), false)
	require.NoError(t, err)
	setFrameKey(first, "F1")
	known := snapshot.AssignUIDs(first, 1, nil)

	var nodes []snapshot.AXNode
	require.NoError(t, json.Unmarshal([]byte(formTree), &nodes))
	nodes[1].ChildIDs = []string{"3", "10", "4", "5", "9"}
	nodes = append(nodes, parse(t, `[{"nodeId":"10","role":{"type":"role","value":"link"},"name":{"type":"computedString","value":"Помощь"},
  "properties":[{"name":"focusable","value":{"type":"booleanOrUndefined","value":true}}],"parentId":"2","childIds":[],"backendDOMNodeId":10}]`)...)
	second, err := snapshot.Build(nodes, false)
	require.NoError(t, err)
	setFrameKey(second, "F1")

	seen := snapshot.AssignUIDs(second, 2, known)

	assert.Equal(t, `uid=1_0 RootWebArea "Форма"
  uid=1_1 heading "Анкета" level="1"
  uid=2_0 link "Помощь"
  uid=1_2 textbox "Имя" focusable focused value="Иван"
  uid=1_3 checkbox "Согласен" checked
  uid=1_4 button "Отправить" disableable disabled haspopup="menu" pressed="mixed"
`, snapshot.Format(second))
	assert.Len(t, seen, 6)
	assert.Equal(t, "2_0", seen["F1_10"])
}

func TestAssignUIDs_SameBackendIDInOtherFrameIsNewNode(t *testing.T) {
	t.Parallel()
	root, err := snapshot.Build(parse(t, `[{"nodeId":"1","role":{"type":"internalRole","value":"RootWebArea"},"name":{"type":"computedString","value":"iframe"},"childIds":[],"backendDOMNodeId":4}]`), false)
	require.NoError(t, err)
	setFrameKey(root, "F2")

	seen := snapshot.AssignUIDs(root, 2, map[string]string{"F1_4": "1_2"})

	assert.Equal(t, "2_0", root.UID)
	assert.Equal(t, map[string]string{"F2_4": "2_0"}, seen)
}

// Регрессия: у InlineTextBox нет DOM-узла (backendDOMNodeId 0), и все такие узлы
// получали один uid 2_3 — ключ «фрейм_0» совпадал.
func TestAssignUIDs_NodesWithoutDOMNodeGetDistinctUIDs(t *testing.T) {
	t.Parallel()
	const tree = `[
 {"nodeId":"1","role":{"type":"internalRole","value":"RootWebArea"},"name":{"type":"computedString","value":"Текст"},"childIds":["2","3"],"backendDOMNodeId":1},
 {"nodeId":"2","role":{"type":"internalRole","value":"InlineTextBox"},"name":{"type":"computedString","value":"первый"},"parentId":"1","childIds":[]},
 {"nodeId":"3","role":{"type":"internalRole","value":"InlineTextBox"},"name":{"type":"computedString","value":"второй"},"parentId":"1","childIds":[]}
]`
	root, err := snapshot.Build(parse(t, tree), true)
	require.NoError(t, err)
	setFrameKey(root, "F1")

	seen := snapshot.AssignUIDs(root, 2, map[string]string{"F1_0": "1_7"})

	assert.Equal(t, `uid=2_0 RootWebArea "Текст"
  uid=2_1 InlineTextBox "первый"
  uid=2_2 InlineTextBox "второй"
`, snapshot.Format(root))
	assert.Equal(t, map[string]string{"F1_1": "2_0"}, seen)
}

func TestBuild_EmptyTreeFails(t *testing.T) {
	t.Parallel()

	_, err := snapshot.Build(nil, false)

	assert.Error(t, err)
}
