// Package perf считает метрики загрузки из трейса Chrome: LCP, FCP, TTFB, CLS и
// длинные задачи. Упрощённая замена движка insights DevTools frontend.
//
// ADR: docs/adr/0009-cli-bez-demona.md — трейс пишется одной командой,
// insights считаются на Go.
package perf

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Границы из web-vitals: длинная задача — дольше 50 мс; сессионное окно CLS
// закрывается разрывом больше 1 с или длиной больше 5 с. Время трейса — микросекунды.
const (
	longTaskMicros    = 50_000
	shiftGapMicros    = 1_000_000
	shiftWindowMicros = 5_000_000
)

var InsightNames = []string{"LCPBreakdown", "LayoutShifts", "LongTasks"}

type LongTask struct {
	StartMs    float64 `json:"startMs"`
	DurationMs float64 `json:"durationMs"`
}

type ShiftWindow struct {
	Score   float64 `json:"score"`
	Shifts  int     `json:"shifts"`
	StartMs float64 `json:"startMs"`
	EndMs   float64 `json:"endMs"`
}

type Metrics struct {
	URL       string        `json:"url"`
	LCPMs     float64       `json:"lcpMs,omitempty"`
	LCPType   string        `json:"lcpType,omitempty"`
	LCPNode   string        `json:"lcpNode,omitempty"`
	LCPSize   float64       `json:"lcpSize,omitempty"`
	FCPMs     float64       `json:"fcpMs,omitempty"`
	TTFBMs    float64       `json:"ttfbMs,omitempty"`
	CLS       float64       `json:"cls"`
	Windows   []ShiftWindow `json:"shiftWindows,omitempty"`
	LongTasks []LongTask    `json:"longTasks,omitempty"`
}

type event struct {
	Name string `json:"name"`
	Ts   int64  `json:"ts"`
	Dur  int64  `json:"dur"`
	Pid  int    `json:"pid"`
	Tid  int    `json:"tid"`
	Args struct {
		Frame string          `json:"frame"`
		Data  json.RawMessage `json:"data"`
	} `json:"args"`
}

func parseEvents(trace []byte) ([]event, error) {
	var wrapped struct {
		TraceEvents []event `json:"traceEvents"`
	}
	if err := json.Unmarshal(trace, &wrapped); err == nil && wrapped.TraceEvents != nil {
		return wrapped.TraceEvents, nil
	}
	var bare []event
	if err := json.Unmarshal(trace, &bare); err != nil {
		return nil, fmt.Errorf("трейс не разобран: %w", err)
	}
	return bare, nil
}

// Analyze разбирает трейс в формате {"traceEvents": [...]} или массив событий.
// В трейсе есть события чужих target (расширения, служебные страницы), поэтому
// метрики берутся только у последней навигации главного фрейма по http(s) или file.
func Analyze(trace []byte) (Metrics, error) {
	events, err := parseEvents(trace)
	if err != nil {
		return Metrics{}, err
	}

	var nav *event
	var navID string
	for i := range events {
		ev := &events[i]
		if ev.Name != "navigationStart" {
			continue
		}
		var d struct {
			URL       string `json:"documentLoaderURL"`
			Outermost bool   `json:"isOutermostMainFrame"`
			ID        string `json:"navigationId"`
		}
		if json.Unmarshal(ev.Args.Data, &d) != nil || !d.Outermost || !pageURL(d.URL) {
			continue
		}
		if nav == nil || ev.Ts >= nav.Ts {
			nav, navID = ev, d.ID
		}
	}
	if nav == nil {
		return Metrics{}, errors.New("в трейсе нет навигации главного фрейма")
	}
	var navData struct {
		URL string `json:"documentLoaderURL"`
	}
	_ = json.Unmarshal(nav.Args.Data, &navData)

	m := Metrics{URL: navData.URL}
	ms := func(ts int64) float64 { return float64(ts-nav.Ts) / 1000 }
	var lcpTs int64 = -1
	type shift struct {
		ts    int64
		score float64
	}
	var shifts []shift

	for _, ev := range events {
		switch ev.Name {
		case "largestContentfulPaint::Candidate":
			var d struct {
				NavigationID string  `json:"navigationId"`
				NodeName     string  `json:"nodeName"`
				Size         float64 `json:"size"`
				Type         string  `json:"type"`
			}
			if json.Unmarshal(ev.Args.Data, &d) == nil && d.NavigationID == navID && ev.Ts > lcpTs {
				lcpTs = ev.Ts
				m.LCPMs, m.LCPType, m.LCPNode, m.LCPSize = ms(ev.Ts), d.Type, d.NodeName, d.Size
			}
		case "firstContentfulPaint":
			var d struct {
				NavigationID string `json:"navigationId"`
			}
			if json.Unmarshal(ev.Args.Data, &d) == nil && d.NavigationID == navID && m.FCPMs == 0 {
				m.FCPMs = ms(ev.Ts)
			}
		case "ResourceReceiveResponse":
			var d struct {
				RequestID string `json:"requestId"`
				Timing    *struct {
					RequestTime         float64 `json:"requestTime"`
					ReceiveHeadersStart float64 `json:"receiveHeadersStart"`
				} `json:"timing"`
			}
			// У запроса документа requestId совпадает с navigationId.
			if json.Unmarshal(ev.Args.Data, &d) == nil && d.RequestID == navID && d.Timing != nil {
				m.TTFBMs = d.Timing.RequestTime*1000 + d.Timing.ReceiveHeadersStart - float64(nav.Ts)/1000
			}
		case "LayoutShift":
			var d struct {
				HadRecentInput bool    `json:"had_recent_input"`
				Weighted       float64 `json:"weighted_score_delta"`
			}
			if ev.Args.Frame == nav.Args.Frame && ev.Ts >= nav.Ts &&
				json.Unmarshal(ev.Args.Data, &d) == nil && !d.HadRecentInput {
				shifts = append(shifts, shift{ts: ev.Ts, score: d.Weighted})
			}
		case "RunTask":
			if ev.Pid == nav.Pid && ev.Tid == nav.Tid && ev.Ts >= nav.Ts && ev.Dur > longTaskMicros {
				m.LongTasks = append(m.LongTasks, LongTask{StartMs: ms(ev.Ts), DurationMs: float64(ev.Dur) / 1000})
			}
		}
	}

	sort.Slice(shifts, func(i, j int) bool { return shifts[i].ts < shifts[j].ts })
	var windowStart, lastShift int64
	for _, s := range shifts {
		if len(m.Windows) == 0 || s.ts-lastShift > shiftGapMicros || s.ts-windowStart > shiftWindowMicros {
			m.Windows = append(m.Windows, ShiftWindow{StartMs: ms(s.ts)})
			windowStart = s.ts
		}
		w := &m.Windows[len(m.Windows)-1]
		w.Score += s.score
		w.Shifts++
		w.EndMs = ms(s.ts)
		lastShift = s.ts
		m.CLS = math.Max(m.CLS, w.Score)
	}
	sort.Slice(m.LongTasks, func(i, j int) bool { return m.LongTasks[i].StartMs < m.LongTasks[j].StartMs })
	return m, nil
}

func pageURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "file://")
}

func millis(v float64) string {
	return strconv.FormatFloat(math.Round(v), 'f', -1, 64)
}

func score(v float64) string {
	return strconv.FormatFloat(math.Round(v*10000)/10000, 'f', -1, 64)
}

func number(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func Summary(m Metrics) string {
	lines := []string{"Трейс: " + m.URL}
	if m.LCPMs > 0 {
		lines = append(lines, fmt.Sprintf("LCP: %s мс (%s %s, %s px²)", millis(m.LCPMs), m.LCPType, m.LCPNode, number(m.LCPSize)))
	} else {
		lines = append(lines, "LCP: не найден")
	}
	if m.FCPMs > 0 {
		lines = append(lines, "FCP: "+millis(m.FCPMs)+" мс")
	} else {
		lines = append(lines, "FCP: не найден")
	}
	if m.TTFBMs > 0 {
		lines = append(lines, "TTFB: "+millis(m.TTFBMs)+" мс")
	} else {
		lines = append(lines, "TTFB: не найден")
	}
	lines = append(lines, "CLS: "+score(m.CLS))
	if len(m.LongTasks) > 0 {
		longest := 0.0
		for _, t := range m.LongTasks {
			longest = math.Max(longest, t.DurationMs)
		}
		lines = append(lines, fmt.Sprintf("Длинные задачи: %d, самая долгая %s мс", len(m.LongTasks), millis(longest)))
	} else {
		lines = append(lines, "Длинные задачи: нет")
	}
	lines = append(lines, "Разборы: "+strings.Join(InsightNames, ", "))
	return strings.Join(lines, "\n")
}

func Insight(m Metrics, name string) (string, error) {
	switch name {
	case "LCPBreakdown":
		if m.LCPMs <= 0 {
			return "", errors.New("LCP не найден в трейсе")
		}
		return fmt.Sprintf("LCP %s мс = TTFB %s мс + от первого байта до отрисовки %s мс\nэлемент: %s %s, %s px²",
			millis(m.LCPMs), millis(m.TTFBMs), millis(m.LCPMs-m.TTFBMs), m.LCPType, m.LCPNode, number(m.LCPSize)), nil
	case "LayoutShifts":
		if len(m.Windows) == 0 {
			return "сдвигов макета нет", nil
		}
		lines := []string{"CLS " + score(m.CLS) + " — худшее окно сдвигов"}
		for i, w := range m.Windows {
			lines = append(lines, fmt.Sprintf("окно %d: %s, сдвигов %d, %s–%s мс", i+1, score(w.Score), w.Shifts, millis(w.StartMs), millis(w.EndMs)))
		}
		return strings.Join(lines, "\n"), nil
	case "LongTasks":
		if len(m.LongTasks) == 0 {
			return "длинных задач нет", nil
		}
		lines := []string{fmt.Sprintf("длинных задач: %d", len(m.LongTasks))}
		for _, t := range m.LongTasks {
			lines = append(lines, millis(t.StartMs)+" мс: "+millis(t.DurationMs)+" мс")
		}
		return strings.Join(lines, "\n"), nil
	default:
		return "", fmt.Errorf("неизвестный разбор %q: %s", name, strings.Join(InsightNames, ", "))
	}
}
