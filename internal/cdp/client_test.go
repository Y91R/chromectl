package cdp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Y91R/chromectl/internal/cdp"
)

type wireMsg struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
}

type fakePeer struct {
	t  *testing.T
	ws *websocket.Conn
}

// dialFake поднимает websocket-сервер вместо Chrome и возвращает подключённый
// к нему клиент и серверную сторону соединения.
func dialFake(t *testing.T) (*cdp.Client, *fakePeer) {
	t.Helper()
	conns := make(chan *websocket.Conn, 1)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ws.SetReadLimit(-1)
		conns <- ws
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := cdp.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	select {
	case ws := <-conns:
		return client, &fakePeer{t: t, ws: ws}
	case <-time.After(2 * time.Second):
		t.Fatal("клиент не подключился к фейковому серверу")
		return nil, nil
	}
}

func (p *fakePeer) read() wireMsg {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := p.ws.Read(ctx)
	require.NoError(p.t, err)
	var msg wireMsg
	require.NoError(p.t, json.Unmarshal(data, &msg))
	return msg
}

func (p *fakePeer) send(raw string) {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(p.t, p.ws.Write(ctx, websocket.MessageText, []byte(raw)))
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestCall_SendsRequestAndDecodesResult(t *testing.T) {
	t.Parallel()
	client, peer := dialFake(t)

	var got struct {
		Value int `json:"value"`
	}
	errc := make(chan error, 1)
	go func() {
		errc <- client.Call(testCtx(t), "S1", "Runtime.evaluate", map[string]any{"expression": "1"}, &got)
	}()

	req := peer.read()
	assert.Equal(t, "Runtime.evaluate", req.Method)
	assert.Equal(t, "S1", req.SessionID)
	assert.JSONEq(t, `{"expression":"1"}`, string(req.Params))
	assert.NotZero(t, req.ID)
	peer.send(fmt.Sprintf(`{"id":%d,"sessionId":"S1","result":{"value":42}}`, req.ID))

	require.NoError(t, <-errc)
	assert.Equal(t, 42, got.Value)
}

func TestCall_NilParamsSendsEmptyObject(t *testing.T) {
	t.Parallel()
	client, peer := dialFake(t)

	errc := make(chan error, 1)
	go func() { errc <- client.Call(testCtx(t), "", "Target.getTargets", nil, nil) }()

	req := peer.read()
	assert.JSONEq(t, `{}`, string(req.Params))
	assert.Empty(t, req.SessionID)
	peer.send(fmt.Sprintf(`{"id":%d,"result":{}}`, req.ID))
	require.NoError(t, <-errc)
}

func TestCall_ProtocolErrorIsTyped(t *testing.T) {
	t.Parallel()
	client, peer := dialFake(t)

	errc := make(chan error, 1)
	go func() {
		errc <- client.Call(testCtx(t), "", "Target.attachToTarget", map[string]any{"targetId": "X"}, nil)
	}()
	req := peer.read()
	peer.send(fmt.Sprintf(`{"id":%d,"error":{"code":-32000,"message":"No target with given id found"}}`, req.ID))

	err := <-errc
	var cdpErr *cdp.Error
	require.True(t, errors.As(err, &cdpErr), "ожидалась *cdp.Error, получено %v", err)
	assert.Equal(t, "Target.attachToTarget", cdpErr.Method)
	assert.Equal(t, -32000, cdpErr.Code)
	assert.Equal(t, "No target with given id found", cdpErr.Message)
}

func TestCall_StopsOnContextDeadline(t *testing.T) {
	t.Parallel()
	client, _ := dialFake(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := client.Call(ctx, "", "Page.enable", nil, nil)

	assert.True(t, errors.Is(err, context.DeadlineExceeded), "ожидался DeadlineExceeded, получено %v", err)
}

func TestCall_MatchesOutOfOrderResponses(t *testing.T) {
	t.Parallel()
	client, peer := dialFake(t)

	type outcome struct {
		value string
		err   error
	}
	call := func(method string, out chan<- outcome) {
		var res struct {
			V string `json:"v"`
		}
		err := client.Call(testCtx(t), "", method, nil, &res)
		out <- outcome{res.V, err}
	}
	first := make(chan outcome, 1)
	go call("A.first", first)
	reqA := peer.read()
	second := make(chan outcome, 1)
	go call("B.second", second)
	reqB := peer.read()

	peer.send(fmt.Sprintf(`{"id":%d,"result":{"v":"b"}}`, reqB.ID))
	peer.send(fmt.Sprintf(`{"id":%d,"result":{"v":"a"}}`, reqA.ID))

	a, b := <-first, <-second
	require.NoError(t, a.err)
	require.NoError(t, b.err)
	assert.Equal(t, "a", a.value)
	assert.Equal(t, "b", b.value)
}

func TestSubscribe_FiltersBySessionAndMethod(t *testing.T) {
	t.Parallel()
	client, peer := dialFake(t)
	sub := client.Subscribe("S1", "Page.javascriptDialogOpening")
	defer sub.Close()

	peer.send(`{"method":"Page.loadEventFired","sessionId":"S1","params":{}}`)
	peer.send(`{"method":"Page.javascriptDialogOpening","sessionId":"S2","params":{"message":"чужой"}}`)
	peer.send(`{"method":"Page.javascriptDialogOpening","sessionId":"S1","params":{"message":"свой"}}`)

	select {
	case ev := <-sub.C:
		assert.Equal(t, "Page.javascriptDialogOpening", ev.Method)
		assert.Equal(t, "S1", ev.SessionID)
		assert.JSONEq(t, `{"message":"свой"}`, string(ev.Params))
	case <-time.After(2 * time.Second):
		t.Fatal("событие не доставлено")
	}
	select {
	case ev := <-sub.C:
		t.Fatalf("доставлено лишнее событие: %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestClose_FailsPendingCallAndSubscriptions(t *testing.T) {
	t.Parallel()
	client, peer := dialFake(t)
	sub := client.Subscribe("S1", "Page.loadEventFired")

	errc := make(chan error, 1)
	go func() { errc <- client.Call(testCtx(t), "", "Page.enable", nil, nil) }()
	peer.read()
	require.NoError(t, client.Close())

	err := <-errc
	assert.True(t, errors.Is(err, cdp.ErrClosed), "ожидалась ErrClosed, получено %v", err)
	_, open := <-sub.C
	assert.False(t, open, "канал подписки должен закрыться")
	assert.True(t, errors.Is(sub.Err(), cdp.ErrClosed), "ожидалась ErrClosed, получено %v", sub.Err())
	assert.True(t, errors.Is(client.Call(testCtx(t), "", "Page.enable", nil, nil), cdp.ErrClosed))
}

func TestConnectionDrop_FailsPendingCall(t *testing.T) {
	t.Parallel()
	client, peer := dialFake(t)

	errc := make(chan error, 1)
	go func() { errc <- client.Call(testCtx(t), "", "Page.enable", nil, nil) }()
	peer.read()
	require.NoError(t, peer.ws.Close(websocket.StatusGoingAway, "браузер закрыт"))

	err := <-errc
	assert.True(t, errors.Is(err, cdp.ErrClosed), "ожидалась ErrClosed, получено %v", err)
}

func TestSubscribe_OverflowClosesWithError(t *testing.T) {
	t.Parallel()
	client, peer := dialFake(t)
	sub := client.Subscribe("S1", "Network.dataReceived")
	defer sub.Close()

	for i := 0; i < 10000; i++ {
		peer.send(`{"method":"Network.dataReceived","sessionId":"S1","params":{}}`)
	}

	received := 0
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, open := <-sub.C:
			if !open {
				assert.Less(t, received, 10000)
				assert.True(t, errors.Is(sub.Err(), cdp.ErrEventOverflow), "ожидалась ErrEventOverflow, получено %v", sub.Err())
				return
			}
			received++
		case <-deadline:
			t.Fatalf("канал не закрылся при переполнении, получено %d событий", received)
		}
	}
}
