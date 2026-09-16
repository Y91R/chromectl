package input

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Y91R/chromectl/internal/command/session"
)

type keyDef struct {
	Key     string
	Code    string
	KeyCode int
	Text    string
}

type modifierDef struct {
	keyDef
	bit int
}

// Биты — Input.dispatchKeyEvent.modifiers: Alt=1, Ctrl=2, Meta=4, Shift=8.
var modifiers = map[string]modifierDef{
	"Alt":     {keyDef{Key: "Alt", Code: "AltLeft", KeyCode: 18}, 1},
	"Control": {keyDef{Key: "Control", Code: "ControlLeft", KeyCode: 17}, 2},
	"Meta":    {keyDef{Key: "Meta", Code: "MetaLeft", KeyCode: 91}, 4},
	"Shift":   {keyDef{Key: "Shift", Code: "ShiftLeft", KeyCode: 16}, 8},
}

var namedKeys = map[string]keyDef{
	"Enter":      {Key: "Enter", Code: "Enter", KeyCode: 13, Text: "\r"},
	"Tab":        {Key: "Tab", Code: "Tab", KeyCode: 9},
	"Escape":     {Key: "Escape", Code: "Escape", KeyCode: 27},
	"Backspace":  {Key: "Backspace", Code: "Backspace", KeyCode: 8},
	"Delete":     {Key: "Delete", Code: "Delete", KeyCode: 46},
	"Insert":     {Key: "Insert", Code: "Insert", KeyCode: 45},
	"Space":      {Key: " ", Code: "Space", KeyCode: 32, Text: " "},
	"ArrowUp":    {Key: "ArrowUp", Code: "ArrowUp", KeyCode: 38},
	"ArrowDown":  {Key: "ArrowDown", Code: "ArrowDown", KeyCode: 40},
	"ArrowLeft":  {Key: "ArrowLeft", Code: "ArrowLeft", KeyCode: 37},
	"ArrowRight": {Key: "ArrowRight", Code: "ArrowRight", KeyCode: 39},
	"Home":       {Key: "Home", Code: "Home", KeyCode: 36},
	"End":        {Key: "End", Code: "End", KeyCode: 35},
	"PageUp":     {Key: "PageUp", Code: "PageUp", KeyCode: 33},
	"PageDown":   {Key: "PageDown", Code: "PageDown", KeyCode: 34},
}

func init() {
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("F%d", i)
		namedKeys[name] = keyDef{Key: name, Code: name, KeyCode: 111 + i}
	}
}

// keyFor — клавиша по имени (Enter, ArrowUp, F5) или одному символу.
func keyFor(name string) (keyDef, error) {
	if def, ok := namedKeys[name]; ok {
		return def, nil
	}
	if utf8.RuneCountInString(name) != 1 {
		return keyDef{}, fmt.Errorf("неизвестная клавиша %q", name)
	}
	r, _ := utf8.DecodeRuneInString(name)
	switch {
	case r == ' ':
		return namedKeys["Space"], nil
	case r == '\n' || r == '\r':
		return namedKeys["Enter"], nil
	case r < unicode.MaxASCII && unicode.IsLetter(r):
		upper := unicode.ToUpper(r)
		return keyDef{Key: name, Code: "Key" + string(upper), KeyCode: int(upper), Text: name}, nil
	case r >= '0' && r <= '9':
		return keyDef{Key: name, Code: "Digit" + name, KeyCode: int(r), Text: name}, nil
	default:
		return keyDef{Key: name, Text: name}, nil
	}
}

type combo struct {
	modifiers []modifierDef
	key       keyDef
}

// parseCombo разбирает "Control+Shift+R" и "Control++" (клавиша плюс).
func parseCombo(s string) (combo, error) {
	if s == "" {
		return combo{}, fmt.Errorf("неизвестная клавиша %q", s)
	}
	keyName := s
	var modNames []string
	switch {
	case s == "+":
	case strings.HasSuffix(s, "++"):
		keyName = "+"
		modNames = strings.Split(strings.TrimSuffix(s, "++"), "+")
	default:
		parts := strings.Split(s, "+")
		keyName = parts[len(parts)-1]
		modNames = parts[:len(parts)-1]
	}

	var c combo
	for _, name := range modNames {
		mod, ok := modifiers[name]
		if !ok {
			return combo{}, fmt.Errorf("неизвестный модификатор %q: допустимы Control, Shift, Alt, Meta", name)
		}
		c.modifiers = append(c.modifiers, mod)
	}
	key, err := keyFor(keyName)
	if err != nil {
		return combo{}, err
	}
	c.key = key
	return c, nil
}

func pressKey(ctx context.Context, p *session.Page, sid string, c combo) error {
	bits := 0
	shift, command := false, false
	for _, mod := range c.modifiers {
		bits |= mod.bit
		if err := keyEvent(ctx, p, sid, "rawKeyDown", mod.keyDef, bits, ""); err != nil {
			return err
		}
		switch mod.Key {
		case "Shift":
			shift = true
		case "Control", "Alt", "Meta":
			command = true
		}
	}

	key := c.key
	if shift && utf8.RuneCountInString(key.Key) == 1 {
		key.Key = strings.ToUpper(key.Key)
		key.Text = strings.ToUpper(key.Text)
	}
	// С Control, Alt или Meta символ не вставляется: это сочетание, а не ввод.
	text := key.Text
	downType := "keyDown"
	if command || text == "" {
		text = ""
		downType = "rawKeyDown"
	}
	if err := keyEvent(ctx, p, sid, downType, key, bits, text); err != nil {
		return err
	}
	if err := keyEvent(ctx, p, sid, "keyUp", key, bits, ""); err != nil {
		return err
	}

	for i := len(c.modifiers) - 1; i >= 0; i-- {
		mod := c.modifiers[i]
		bits &^= mod.bit
		if err := keyEvent(ctx, p, sid, "keyUp", mod.keyDef, bits, ""); err != nil {
			return err
		}
	}
	return nil
}

func keyEvent(ctx context.Context, p *session.Page, sid, eventType string, def keyDef, bits int, text string) error {
	params := map[string]any{
		"type":                  eventType,
		"key":                   def.Key,
		"code":                  def.Code,
		"windowsVirtualKeyCode": def.KeyCode,
		"modifiers":             bits,
	}
	if text != "" {
		params["text"] = text
		params["unmodifiedText"] = text
	}
	return p.Client.Call(ctx, sid, "Input.dispatchKeyEvent", params, nil)
}

func jsonUnmarshal(data json.RawMessage, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("разбор события: %w", err)
	}
	return nil
}
