// Package netlog хранит и печатает сетевые запросы, захваченные за время одной
// команды: маскирование секретов, лимиты тел.
//
// ADR: docs/adr/0009-cli-bez-demona.md — истории сети до подключения
// нет, детали есть только у запросов из --capture.
package netlog

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// storedBodyLimit — сколько тела лежит в файле захвата; inlineBodyLimit — сколько
	// символов печатается.
	storedBodyLimit = 1024 * 1024
	inlineBodyLimit = 10000
	urlLimit        = 255
	truncated       = "... <обрезано>"
)

var ErrNoCapture = errors.New("нет захваченных запросов: выполните navigate или действие с --capture")

var secretHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"set-cookie":          true,
	"proxy-authorization": true,
}

// Request — захваченный запрос в том виде, в каком он лежит в файле захвата.
type Request struct {
	ReqID            int               `json:"reqid"`
	Method           string            `json:"method"`
	URL              string            `json:"url"`
	ResourceType     string            `json:"resourceType"`
	Status           int               `json:"status,omitempty"`
	Failure          string            `json:"failure,omitempty"`
	RequestHeaders   map[string]string `json:"requestHeaders,omitempty"`
	ResponseHeaders  map[string]string `json:"responseHeaders,omitempty"`
	RequestBody      string            `json:"requestBody,omitempty"`
	RequestBodyNote  string            `json:"requestBodyNote,omitempty"`
	ResponseBody     string            `json:"responseBody,omitempty"`
	ResponseBodyNote string            `json:"responseBodyNote,omitempty"`
}

// Timing — запись Performance API, когда захвата нет.
type Timing struct {
	URL           string `json:"url"`
	InitiatorType string `json:"initiatorType"`
	Status        int    `json:"status"`
}

func Redact(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	redacted := make(map[string]string, len(headers))
	for name, value := range headers {
		if secretHeaders[strings.ToLower(name)] {
			value = "<redacted>"
		}
		redacted[name] = value
	}
	return redacted
}

// StoredBody готовит тело к записи: текст до 1 МБ, бинарное — только размер.
func StoredBody(data []byte) (body, note string) {
	switch {
	case len(data) == 0:
		return "", "<пустое тело>"
	case !utf8.Valid(data):
		return "", fmt.Sprintf("<бинарные данные, %d байт>", len(data))
	case len(data) > storedBodyLimit:
		cut := data[:storedBodyLimit]
		for !utf8.Valid(cut) {
			cut = cut[:len(cut)-1]
		}
		return string(cut), fmt.Sprintf("<обрезано до %d из %d байт>", storedBodyLimit, len(data))
	default:
		return string(data), ""
	}
}

func status(r Request) string {
	switch {
	case r.Status > 0:
		return strconv.Itoa(r.Status)
	case r.Failure != "":
		return r.Failure
	default:
		return "pending"
	}
}

func Concise(r Request) string {
	url := r.URL
	if len(url) > urlLimit {
		url = url[:urlLimit] + truncated
	}
	return fmt.Sprintf("reqid=%d %s %s [%s]", r.ReqID, r.Method, url, status(r))
}

func Detail(r Request) string {
	lines := []string{
		"## Запрос " + r.Method + " " + r.URL,
		"Статус: " + status(r),
	}
	lines = appendHeaders(lines, "### Заголовки запроса", r.RequestHeaders)
	lines = appendBody(lines, "### Тело запроса", r.RequestBody, r.RequestBodyNote)
	lines = appendHeaders(lines, "### Заголовки ответа", r.ResponseHeaders)
	lines = appendBody(lines, "### Тело ответа", r.ResponseBody, r.ResponseBodyNote)
	if r.Failure != "" {
		lines = append(lines, "### Ошибка", r.Failure)
	}
	return strings.Join(lines, "\n")
}

func appendHeaders(lines []string, title string, headers map[string]string) []string {
	if len(headers) == 0 {
		return lines
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	lines = append(lines, title)
	for _, name := range names {
		lines = append(lines, name+": "+headers[name])
	}
	return lines
}

func appendBody(lines []string, title, body, note string) []string {
	if body == "" && note == "" {
		return lines
	}
	lines = append(lines, title)
	if body != "" {
		if runes := []rune(body); len(runes) > inlineBodyLimit {
			body = string(runes[:inlineBodyLimit]) + truncated
		}
		lines = append(lines, body)
	}
	if note != "" {
		lines = append(lines, note)
	}
	return lines
}

func ConciseTiming(t Timing) string {
	s := "?"
	if t.Status > 0 {
		s = strconv.Itoa(t.Status)
	}
	return fmt.Sprintf("- %s %s [%s]", TypeFromInitiator(t.InitiatorType), t.URL, s)
}

// TypeFromInitiator переводит initiatorType Performance API в типы ресурсов,
// по которым фильтрует network list.
func TypeFromInitiator(initiatorType string) string {
	switch initiatorType {
	case "navigation":
		return "document"
	case "fetch":
		return "fetch"
	case "xmlhttprequest":
		return "xhr"
	case "img", "image", "input":
		return "image"
	case "script":
		return "script"
	case "css":
		return "stylesheet"
	case "beacon":
		return "ping"
	case "video", "audio", "track":
		return "media"
	default:
		return "other"
	}
}

func capturePath(stateDir, pageID string) string {
	return filepath.Join(stateDir, "network", pageID+".jsonl")
}

// Save перезаписывает захват вкладки. Файл может содержать тела ответов,
// поэтому каталог 0700, файл 0600.
func Save(stateDir, pageID string, requests []Request) error {
	dir := filepath.Dir(capturePath(stateDir, pageID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("каталог захвата: %w", err)
	}
	tmp, err := os.CreateTemp(dir, pageID+".tmp-*")
	if err != nil {
		return fmt.Errorf("файл захвата: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	w := bufio.NewWriter(tmp)
	enc := json.NewEncoder(w)
	for _, r := range requests {
		if err := enc.Encode(r); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("запись захвата: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("запись захвата: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("запись захвата: %w", err)
	}
	if err := os.Rename(tmp.Name(), capturePath(stateDir, pageID)); err != nil {
		return fmt.Errorf("замена захвата: %w", err)
	}
	return nil
}

func Load(stateDir, pageID string) ([]Request, error) {
	f, err := os.Open(capturePath(stateDir, pageID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNoCapture
	}
	if err != nil {
		return nil, fmt.Errorf("чтение захвата: %w", err)
	}
	defer func() { _ = f.Close() }()

	var requests []Request
	dec := json.NewDecoder(f)
	for dec.More() {
		var r Request
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("разбор захвата: %w", err)
		}
		requests = append(requests, r)
	}
	return requests, nil
}
