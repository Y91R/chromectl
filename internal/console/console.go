// Package console разбирает сообщения консоли из событий CDP и печатает их в
// виде `msgid=3 [log] обычное 42 (2 args)`.
package console

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// knownTypes — допустимые значения --types у console list.
var knownTypes = map[string]bool{
	"log": true, "debug": true, "info": true, "error": true, "warn": true, "dir": true, "dirxml": true,
	"table": true, "trace": true, "clear": true, "startGroup": true, "startGroupCollapsed": true,
	"endGroup": true, "assert": true, "profile": true, "profileEnd": true, "count": true, "timeEnd": true,
	"verbose": true, "issue": true,
}

type Message struct {
	ID        int      `json:"id"`
	Type      string   `json:"type"`
	Text      string   `json:"text"`
	Args      []string `json:"args,omitempty"`
	Stack     []string `json:"stack,omitempty"`
	Timestamp float64  `json:"-"`
}

type ListOptions struct {
	Types    []string
	PageSize int
	PageIdx  int
}

type remoteObject struct {
	Type                string          `json:"type"`
	Value               json.RawMessage `json:"value"`
	UnserializableValue string          `json:"unserializableValue"`
	Description         string          `json:"description"`
}

type stackTrace struct {
	CallFrames []struct {
		FunctionName string `json:"functionName"`
		URL          string `json:"url"`
		LineNumber   int    `json:"lineNumber"`
		ColumnNumber int    `json:"columnNumber"`
	} `json:"callFrames"`
}

func (s *stackTrace) lines() []string {
	if s == nil {
		return nil
	}
	lines := make([]string, 0, len(s.CallFrames))
	for _, f := range s.CallFrames {
		name := f.FunctionName
		if name == "" {
			name = "<anonymous>"
		}
		// CDP нумерует строки и столбцы с нуля, DevTools показывает с единицы.
		lines = append(lines, fmt.Sprintf("at %s (%s:%d:%d)", name, f.URL, f.LineNumber+1, f.ColumnNumber+1))
	}
	return lines
}

func (o remoteObject) String() string {
	switch {
	case len(o.Value) > 0:
		var s string
		if json.Unmarshal(o.Value, &s) == nil {
			return s
		}
		return string(o.Value)
	case o.UnserializableValue != "":
		return o.UnserializableValue
	case o.Type == "undefined":
		return "undefined"
	default:
		return o.Description
	}
}

// FromAPICalled — сообщение из Runtime.consoleAPICalled.
func FromAPICalled(params json.RawMessage) (Message, error) {
	var p struct {
		Type       string         `json:"type"`
		Timestamp  float64        `json:"timestamp"`
		Args       []remoteObject `json:"args"`
		StackTrace *stackTrace    `json:"stackTrace"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return Message{}, fmt.Errorf("разбор сообщения консоли: %w", err)
	}
	args := make([]string, 0, len(p.Args))
	for _, a := range p.Args {
		args = append(args, a.String())
	}
	// CDP называет console.warn "warning", а CLI показывает и фильтрует его как "warn".
	if p.Type == "warning" {
		p.Type = "warn"
	}
	return Message{
		Type:      p.Type,
		Text:      strings.Join(args, " "),
		Args:      args,
		Stack:     p.StackTrace.lines(),
		Timestamp: p.Timestamp,
	}, nil
}

// FromException — необработанное исключение из Runtime.exceptionThrown.
func FromException(params json.RawMessage) (Message, error) {
	var p struct {
		Timestamp        float64 `json:"timestamp"`
		ExceptionDetails struct {
			Text       string        `json:"text"`
			Exception  *remoteObject `json:"exception"`
			StackTrace *stackTrace   `json:"stackTrace"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return Message{}, fmt.Errorf("разбор исключения: %w", err)
	}
	text := p.ExceptionDetails.Text
	if ex := p.ExceptionDetails.Exception; ex != nil && ex.Description != "" {
		text, _, _ = strings.Cut(ex.Description, "\n")
	}
	return Message{
		Type:      "error",
		Text:      text,
		Stack:     p.ExceptionDetails.StackTrace.lines(),
		Timestamp: p.Timestamp,
	}, nil
}

// Number упорядочивает сообщения по времени и нумерует с 1.
func Number(messages []Message) []Message {
	numbered := append([]Message{}, messages...)
	sort.SliceStable(numbered, func(i, j int) bool { return numbered[i].Timestamp < numbered[j].Timestamp })
	for i := range numbered {
		numbered[i].ID = i + 1
	}
	return numbered
}

func List(messages []Message, opts ListOptions) (string, error) {
	types := map[string]bool{}
	for _, t := range opts.Types {
		if !knownTypes[t] {
			return "", fmt.Errorf("неизвестный тип сообщения %q", t)
		}
		types[t] = true
	}
	if opts.PageSize < 0 || opts.PageIdx < 0 {
		return "", fmt.Errorf("--page-size и --page-idx не могут быть отрицательными")
	}

	var filtered []Message
	for _, m := range messages {
		if len(types) == 0 || types[m.Type] {
			filtered = append(filtered, m)
		}
	}
	if opts.PageSize > 0 {
		start := min(opts.PageSize*opts.PageIdx, len(filtered))
		end := min(start+opts.PageSize, len(filtered))
		filtered = filtered[start:end]
	}
	if len(filtered) == 0 {
		return "сообщений нет", nil
	}

	var lines []string
	for i := 0; i < len(filtered); {
		m := filtered[i]
		count := 1
		for i+count < len(filtered) && sameMessage(filtered[i+count], m) {
			count++
		}
		line := "msgid=" + strconv.Itoa(m.ID) + " [" + m.Type + "] " + m.Text + " (" + strconv.Itoa(len(m.Args)) + " args)"
		if count > 1 {
			line += " [" + strconv.Itoa(count) + " times]"
		}
		lines = append(lines, line)
		i += count
	}
	return strings.Join(lines, "\n"), nil
}

func sameMessage(a, b Message) bool {
	return a.Type == b.Type && a.Text == b.Text && len(a.Args) == len(b.Args)
}

func Detail(m Message) string {
	lines := []string{
		"ID: " + strconv.Itoa(m.ID),
		"Сообщение: " + m.Type + "> " + m.Text,
	}
	if len(m.Args) > 0 {
		lines = append(lines, "### Аргументы")
		for i, a := range m.Args {
			lines = append(lines, fmt.Sprintf("Arg #%d: %s", i, a))
		}
	}
	if len(m.Stack) > 0 {
		lines = append(lines, "### Стек")
		lines = append(lines, m.Stack...)
	}
	return strings.Join(lines, "\n")
}
