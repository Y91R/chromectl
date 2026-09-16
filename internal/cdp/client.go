// Package cdp — тонкий клиент Chrome DevTools Protocol поверх websocket: вызов
// метода, ответ по id, подписка на события на время одной команды.
//
// ADR: docs/adr/0009-cli-bez-demona.md — одна команда — одно подключение.
package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
)

// eventBuffer ограничивает события, которые подписчик ещё не забрал. Переполнение
// закрывает подписку с ошибкой: молча потерянное событие дало бы неполный
// результат без признака неполноты.
const eventBuffer = 4096

var (
	ErrClosed        = errors.New("соединение с Chrome закрыто")
	ErrEventOverflow = errors.New("очередь событий CDP переполнена")
)

type Error struct {
	Method  string
	Code    int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s (%d)", e.Method, e.Message, e.Code)
}

type Event struct {
	Method    string
	SessionID string
	Params    json.RawMessage
}

type wireError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type wireMessage struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *wireError      `json:"error,omitempty"`
}

type response struct {
	result json.RawMessage
	err    *wireError
}

type Client struct {
	ws     *websocket.Conn
	nextID atomic.Int64

	mu       sync.Mutex
	pending  map[int64]chan response
	subs     map[*Subscription]struct{}
	closed   bool
	closeErr error

	done       chan struct{}
	readDone   chan struct{}
	cancelRead context.CancelFunc
}

func Dial(ctx context.Context, browserWSURL string) (*Client, error) {
	// Debug-порт всегда локальный: прокси из окружения только сломал бы подключение.
	ws, _, err := websocket.Dial(ctx, browserWSURL, &websocket.DialOptions{
		HTTPClient: &http.Client{Transport: &http.Transport{Proxy: nil}},
	})
	if err != nil {
		return nil, fmt.Errorf("подключение к Chrome %s: %w", browserWSURL, err)
	}
	// Скриншоты и чанки heap snapshot превышают стандартный лимит в 32 КБ.
	ws.SetReadLimit(-1)

	// Чтение живёт до Close, а не до дедлайна подключения.
	readCtx, cancelRead := context.WithCancel(context.WithoutCancel(ctx))
	c := &Client{
		ws:         ws,
		pending:    map[int64]chan response{},
		subs:       map[*Subscription]struct{}{},
		done:       make(chan struct{}),
		readDone:   make(chan struct{}),
		cancelRead: cancelRead,
	}
	go c.readLoop(readCtx)
	return c, nil
}

// readLoop принадлежит Client и завершается, когда соединение закрыто — через
// Close или со стороны Chrome.
func (c *Client) readLoop(ctx context.Context) {
	defer close(c.readDone)
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			c.shutdown(fmt.Errorf("%w: %v", ErrClosed, err))
			return
		}
		var msg wireMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		c.dispatch(msg)
	}
}

func (c *Client) dispatch(msg wireMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if msg.ID != 0 {
		if ch, ok := c.pending[msg.ID]; ok {
			delete(c.pending, msg.ID)
			ch <- response{result: msg.Result, err: msg.Error}
		}
		return
	}

	ev := Event{Method: msg.Method, SessionID: msg.SessionID, Params: msg.Params}
	for sub := range c.subs {
		if !sub.matches(ev) {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
			c.finishLocked(sub, ErrEventOverflow)
		}
	}
}

func (c *Client) shutdown(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	c.closeErr = err
	for sub := range c.subs {
		c.finishLocked(sub, err)
	}
	close(c.done)
}

// Close не ждёт закрывающего рукопожатия: команда завершается, и вкладке от
// этого ничего не будет — сессия в Chrome отсоединится сама.
func (c *Client) Close() error {
	c.shutdown(ErrClosed)
	c.cancelRead()
	_ = c.ws.CloseNow()
	<-c.readDone
	return nil
}

func (c *Client) Call(ctx context.Context, sessionID, method string, params, result any) error {
	if params == nil {
		params = struct{}{}
	}
	rawParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("%s: параметры: %w", method, err)
	}

	id := c.nextID.Add(1)
	ch := make(chan response, 1)
	c.mu.Lock()
	if c.closed {
		closeErr := c.closeErr
		c.mu.Unlock()
		return fmt.Errorf("%s: %w", method, closeErr)
	}
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	data, err := json.Marshal(wireMessage{ID: id, Method: method, SessionID: sessionID, Params: rawParams})
	if err != nil {
		return fmt.Errorf("%s: запрос: %w", method, err)
	}
	if err := c.ws.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("%s: отправка: %w", method, err)
	}

	select {
	case resp := <-ch:
		if resp.err != nil {
			return &Error{Method: method, Code: resp.err.Code, Message: resp.err.Message}
		}
		if result != nil && len(resp.result) > 0 {
			if err := json.Unmarshal(resp.result, result); err != nil {
				return fmt.Errorf("%s: разбор ответа: %w", method, err)
			}
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", method, ctx.Err())
	case <-c.done:
		c.mu.Lock()
		closeErr := c.closeErr
		c.mu.Unlock()
		return fmt.Errorf("%s: %w", method, closeErr)
	}
}

// Subscribe доставляет события указанной сессии ("" — уровень браузера) с
// перечисленными методами. Подписываться нужно до действия, которое их вызывает.
func (c *Client) Subscribe(sessionID string, methods ...string) *Subscription {
	ch := make(chan Event, eventBuffer)
	sub := &Subscription{C: ch, ch: ch, client: c, sessionID: sessionID, methods: map[string]struct{}{}}
	for _, m := range methods {
		sub.methods[m] = struct{}{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		sub.err = c.closeErr
		close(ch)
		return sub
	}
	c.subs[sub] = struct{}{}
	return sub
}

func (c *Client) finishLocked(sub *Subscription, err error) {
	if _, ok := c.subs[sub]; !ok {
		return
	}
	delete(c.subs, sub)
	sub.err = err
	close(sub.ch)
}

type Subscription struct {
	C <-chan Event

	ch        chan Event
	client    *Client
	sessionID string
	methods   map[string]struct{}
	err       error
}

func (s *Subscription) matches(ev Event) bool {
	if ev.SessionID != s.sessionID {
		return false
	}
	if len(s.methods) == 0 {
		return true
	}
	_, ok := s.methods[ev.Method]
	return ok
}

// Err объясняет, почему закрылся канал C: nil после Close, ErrClosed или
// ErrEventOverflow.
func (s *Subscription) Err() error {
	s.client.mu.Lock()
	defer s.client.mu.Unlock()
	return s.err
}

func (s *Subscription) Close() {
	s.client.mu.Lock()
	defer s.client.mu.Unlock()
	s.client.finishLocked(s, nil)
}
