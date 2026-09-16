package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Y91R/chromectl/internal/cdp"
	"github.com/Y91R/chromectl/internal/netlog"
)

const (
	captureQuiet   = 500 * time.Millisecond
	captureMaxWait = 10 * time.Second
	maxPostData    = 1024 * 1024
)

type capturedRequest struct {
	req         netlog.Request
	hasPostData bool
	finished    bool
	failed      bool
}

// Capture записывает сетевые запросы вкладки, пока команда работает. Истории
// до подключения CDP не отдаёт, поэтому захват начинается до действия.
type Capture struct {
	c          *cdp.Client
	sid        string
	unredacted bool
	sub        *cdp.Subscription
	order      []string
	requests   map[string]*capturedRequest
}

func StartCapture(ctx context.Context, c *cdp.Client, sid string, unredacted bool) (*Capture, error) {
	sub := c.Subscribe(sid,
		"Network.requestWillBeSent", "Network.requestWillBeSentExtraInfo",
		"Network.responseReceived", "Network.responseReceivedExtraInfo",
		"Network.loadingFinished", "Network.loadingFailed")
	if err := c.Call(ctx, sid, "Network.enable", map[string]any{"maxPostDataSize": maxPostData}, nil); err != nil {
		sub.Close()
		return nil, err
	}
	return &Capture{c: c, sid: sid, unredacted: unredacted, sub: sub, requests: map[string]*capturedRequest{}}, nil
}

// Close нужен, когда действие упало и Finish не вызывался.
func (cp *Capture) Close() {
	cp.sub.Close()
}

// Finish ждёт, пока сеть затихнет на 500 мс (не дольше 10 с), и забирает тела.
func (cp *Capture) Finish(ctx context.Context) ([]netlog.Request, error) {
	defer cp.sub.Close()
	deadline := time.After(captureMaxWait)
wait:
	for {
		select {
		case ev, open := <-cp.sub.C:
			if !open {
				if err := cp.sub.Err(); err != nil {
					return nil, err
				}
				break wait
			}
			cp.apply(ev)
		case <-time.After(captureQuiet):
			if cp.inFlight() == 0 {
				break wait
			}
		case <-deadline:
			break wait
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	result := make([]netlog.Request, 0, len(cp.order))
	for i, id := range cp.order {
		r := cp.requests[id]
		if r.finished {
			cp.fetchResponseBody(ctx, id, r)
		}
		if r.hasPostData && r.req.RequestBody == "" && r.req.RequestBodyNote == "" {
			cp.fetchPostData(ctx, id, r)
		}
		req := r.req
		req.ReqID = i + 1
		if !cp.unredacted {
			req.RequestHeaders = netlog.Redact(req.RequestHeaders)
			req.ResponseHeaders = netlog.Redact(req.ResponseHeaders)
		}
		result = append(result, req)
	}
	return result, nil
}

func (cp *Capture) inFlight() int {
	n := 0
	for _, id := range cp.order {
		if r := cp.requests[id]; !r.finished && !r.failed {
			n++
		}
	}
	return n
}

func (cp *Capture) entry(id string) *capturedRequest {
	r, ok := cp.requests[id]
	if !ok {
		r = &capturedRequest{}
		cp.requests[id] = r
	}
	return r
}

func (cp *Capture) apply(ev cdp.Event) {
	var p struct {
		RequestID string `json:"requestId"`
		Type      string `json:"type"`
		ErrorText string `json:"errorText"`
		Headers   map[string]string
		Request   struct {
			URL         string            `json:"url"`
			Method      string            `json:"method"`
			Headers     map[string]string `json:"headers"`
			PostData    string            `json:"postData"`
			HasPostData bool              `json:"hasPostData"`
		} `json:"request"`
		Response struct {
			Status  int               `json:"status"`
			Headers map[string]string `json:"headers"`
		} `json:"response"`
	}
	if json.Unmarshal(ev.Params, &p) != nil || p.RequestID == "" {
		return
	}
	r := cp.entry(p.RequestID)

	switch ev.Method {
	case "Network.requestWillBeSent":
		if r.req.URL == "" {
			cp.order = append(cp.order, p.RequestID)
		}
		r.req.URL = p.Request.URL
		r.req.Method = p.Request.Method
		r.req.ResourceType = strings.ToLower(p.Type)
		r.req.RequestHeaders = mergeHeaders(p.Request.Headers, r.req.RequestHeaders)
		r.hasPostData = p.Request.HasPostData
		if p.Request.PostData != "" {
			r.req.RequestBody, r.req.RequestBodyNote = netlog.StoredBody([]byte(p.Request.PostData))
		}
	case "Network.requestWillBeSentExtraInfo":
		// Здесь полные заголовки, с cookie, которых нет в requestWillBeSent.
		r.req.RequestHeaders = mergeHeaders(r.req.RequestHeaders, p.Headers)
	case "Network.responseReceived":
		r.req.Status = p.Response.Status
		if p.Type != "" {
			r.req.ResourceType = strings.ToLower(p.Type)
		}
		r.req.ResponseHeaders = mergeHeaders(p.Response.Headers, r.req.ResponseHeaders)
	case "Network.responseReceivedExtraInfo":
		r.req.ResponseHeaders = mergeHeaders(r.req.ResponseHeaders, p.Headers)
	case "Network.loadingFinished":
		r.finished = true
	case "Network.loadingFailed":
		r.failed = true
		r.req.Failure = p.ErrorText
	}
}

func mergeHeaders(base, extra map[string]string) map[string]string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}

func (cp *Capture) fetchResponseBody(ctx context.Context, id string, r *capturedRequest) {
	var res struct {
		Body          string `json:"body"`
		Base64Encoded bool   `json:"base64Encoded"`
	}
	if err := cp.c.Call(ctx, cp.sid, "Network.getResponseBody", map[string]any{"requestId": id}, &res); err != nil {
		r.req.ResponseBodyNote = "<тело недоступно>"
		return
	}
	data, err := decodeBody(res.Body, res.Base64Encoded)
	if err != nil {
		r.req.ResponseBodyNote = "<тело недоступно>"
		return
	}
	r.req.ResponseBody, r.req.ResponseBodyNote = netlog.StoredBody(data)
}

func (cp *Capture) fetchPostData(ctx context.Context, id string, r *capturedRequest) {
	var res struct {
		PostData      string `json:"postData"`
		Base64Encoded bool   `json:"base64Encoded"`
	}
	if err := cp.c.Call(ctx, cp.sid, "Network.getRequestPostData", map[string]any{"requestId": id}, &res); err != nil {
		r.req.RequestBodyNote = "<тело недоступно>"
		return
	}
	data, err := decodeBody(res.PostData, res.Base64Encoded)
	if err != nil {
		r.req.RequestBodyNote = "<тело недоступно>"
		return
	}
	r.req.RequestBody, r.req.RequestBodyNote = netlog.StoredBody(data)
}

func decodeBody(body string, base64Encoded bool) ([]byte, error) {
	if !base64Encoded {
		return []byte(body), nil
	}
	data, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return nil, errors.New("тело не декодируется из base64")
	}
	return data, nil
}
