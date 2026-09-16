package perf_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Y91R/chromectl/internal/perf"
)

// traceEvents собирает трейс из событий той же формы, что пишет Chrome 153 (спайк
// Э5). Время — микросекунды, как в ts трейса.
func traceEvents(events ...string) []byte {
	return []byte(`{"traceEvents":[` + strings.Join(events, ",") + `]}`)
}

func navigationStart(ts int, frame, navID, url string, pid, tid int) string {
	return fmt.Sprintf(`{"name":"navigationStart","cat":"blink.user_timing","ts":%d,"pid":%d,"tid":%d,
	  "args":{"frame":%q,"data":{"documentLoaderURL":%q,"isOutermostMainFrame":true,"isLoadingMainFrame":true,"navigationId":%q}}}`,
		ts, pid, tid, frame, url, navID)
}

func lcpCandidate(ts int, frame, navID, node, kind string, size int) string {
	return fmt.Sprintf(`{"name":"largestContentfulPaint::Candidate","cat":"loading,rail,devtools.timeline","ts":%d,"pid":20,"tid":2,
	  "args":{"frame":%q,"data":{"candidateIndex":1,"isOutermostMainFrame":true,"navigationId":%q,"nodeName":%q,"size":%d,"type":%q}}}`,
		ts, frame, navID, node, size, kind)
}

func layoutShift(ts int, frame string, score float64, recentInput bool) string {
	return fmt.Sprintf(`{"name":"LayoutShift","cat":"loading","ts":%d,"pid":20,"tid":2,
	  "args":{"frame":%q,"data":{"had_recent_input":%v,"is_main_frame":true,"score":%v,"weighted_score_delta":%v}}}`,
		ts, frame, recentInput, score, score)
}

func runTask(ts, dur, pid, tid int) string {
	return fmt.Sprintf(`{"name":"RunTask","cat":"disabled-by-default-devtools.timeline","ts":%d,"dur":%d,"pid":%d,"tid":%d,"args":{}}`,
		ts, dur, pid, tid)
}

func fullTrace() []byte {
	return traceEvents(
		navigationStart(1_000_000, "F-ext", "N-ext", "chrome-extension://x/", 10, 1),
		navigationStart(2_000_000, "F1", "N-blank", "", 20, 2),
		navigationStart(3_000_000, "F1", "N1", "http://x/", 20, 2),
		`{"name":"ResourceReceiveResponse","cat":"devtools.timeline","ts":3005000,"pid":30,"tid":3,
		  "args":{"data":{"requestId":"N1","statusCode":200,"timing":{"requestTime":3.0005,"receiveHeadersStart":40.5,"receiveHeadersEnd":41}}}}`,
		`{"name":"firstContentfulPaint","cat":"loading,rail,devtools.timeline","ts":3200000,"pid":20,"tid":2,
		  "args":{"frame":"F1","data":{"navigationId":"N1"}}}`,
		lcpCandidate(3_200_000, "F1", "N1", "P", "text", 666),
		lcpCandidate(3_100_000, "F-ext", "N-ext", "IMG", "image", 99999),
		lcpCandidate(3_450_000, "F1", "N1", "IMG", "image", 240000),
		layoutShift(3_300_000, "F1", 0.1, false),
		layoutShift(3_400_000, "F-ext", 0.7, false),
		layoutShift(3_800_000, "F1", 0.05, false),
		layoutShift(5_500_000, "F1", 0.2, false),
		layoutShift(5_600_000, "F1", 0.9, true),
		runTask(2_500_000, 90_000, 20, 2),
		runTask(4_000_000, 120_000, 20, 2),
		runTask(4_200_000, 30_000, 20, 2),
		runTask(4_300_000, 200_000, 30, 3),
	)
}

func TestAnalyze_MetricsOfMainNavigationOnly(t *testing.T) {
	t.Parallel()

	got, err := perf.Analyze(fullTrace())

	require.NoError(t, err)
	assert.Equal(t, "http://x/", got.URL)
	assert.InDelta(t, 450, got.LCPMs, 0.001, "последний кандидат своей навигации")
	assert.Equal(t, "image", got.LCPType)
	assert.Equal(t, "IMG", got.LCPNode)
	assert.InDelta(t, 240000, got.LCPSize, 0.001)
	assert.InDelta(t, 200, got.FCPMs, 0.001)
	assert.InDelta(t, 41, got.TTFBMs, 0.001, "requestTime·1000 + receiveHeadersStart − navigationStart")
	assert.InDelta(t, 0.2, got.CLS, 0.0001, "худшее сессионное окно; сдвиги после ввода и чужих фреймов не считаются")
	require.Len(t, got.Windows, 2)
	assert.InDelta(t, 0.15, got.Windows[0].Score, 0.0001)
	assert.Equal(t, 2, got.Windows[0].Shifts)
	assert.InDelta(t, 300, got.Windows[0].StartMs, 0.001)
	assert.InDelta(t, 800, got.Windows[0].EndMs, 0.001)
	assert.Equal(t, []perf.LongTask{{StartMs: 1000, DurationMs: 120}}, got.LongTasks)
}

func TestAnalyze_SessionWindowIsCappedAtFiveSeconds(t *testing.T) {
	t.Parallel()
	events := []string{navigationStart(1_000_000, "F1", "N1", "http://x/", 20, 2)}
	for i := 0; i < 7; i++ {
		events = append(events, layoutShift(1_000_000+i*900_000, "F1", 0.1, false))
	}

	got, err := perf.Analyze(traceEvents(events...))

	require.NoError(t, err)
	assert.InDelta(t, 0.6, got.CLS, 0.0001, "шесть сдвигов укладываются в 5 с, седьмой открывает новое окно")
	require.Len(t, got.Windows, 2)
	assert.Equal(t, 6, got.Windows[0].Shifts)
}

func TestAnalyze_AcceptsBareEventArray(t *testing.T) {
	t.Parallel()

	got, err := perf.Analyze([]byte(`[` + navigationStart(1_000_000, "F1", "N1", "http://x/", 20, 2) + `]`))

	require.NoError(t, err)
	assert.Equal(t, "http://x/", got.URL)
	assert.Zero(t, got.CLS)
}

func TestAnalyze_Errors(t *testing.T) {
	t.Parallel()

	_, err := perf.Analyze([]byte(`не json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "трейс не разобран")

	_, err = perf.Analyze(traceEvents(navigationStart(1_000_000, "F1", "N-blank", "", 20, 2)))
	require.Error(t, err)
	assert.Equal(t, "в трейсе нет навигации главного фрейма", err.Error())
}

func TestSummary(t *testing.T) {
	t.Parallel()
	m, err := perf.Analyze(fullTrace())
	require.NoError(t, err)

	assert.Equal(t, `Трейс: http://x/
LCP: 450 мс (image IMG, 240000 px²)
FCP: 200 мс
TTFB: 41 мс
CLS: 0.2
Длинные задачи: 1, самая долгая 120 мс
Разборы: LCPBreakdown, LayoutShifts, LongTasks`, perf.Summary(m))

	assert.Equal(t, `Трейс: http://y/
LCP: не найден
FCP: не найден
TTFB: не найден
CLS: 0
Длинные задачи: нет
Разборы: LCPBreakdown, LayoutShifts, LongTasks`, perf.Summary(perf.Metrics{URL: "http://y/"}))
}

func TestInsight(t *testing.T) {
	t.Parallel()
	m, err := perf.Analyze(fullTrace())
	require.NoError(t, err)

	got, err := perf.Insight(m, "LCPBreakdown")
	require.NoError(t, err)
	assert.Equal(t, "LCP 450 мс = TTFB 41 мс + от первого байта до отрисовки 409 мс\nэлемент: image IMG, 240000 px²", got)

	got, err = perf.Insight(m, "LayoutShifts")
	require.NoError(t, err)
	assert.Equal(t, "CLS 0.2 — худшее окно сдвигов\nокно 1: 0.15, сдвигов 2, 300–800 мс\nокно 2: 0.2, сдвигов 1, 2500–2500 мс", got)

	got, err = perf.Insight(m, "LongTasks")
	require.NoError(t, err)
	assert.Equal(t, "длинных задач: 1\n1000 мс: 120 мс", got)
}

func TestInsight_EmptyAndErrors(t *testing.T) {
	t.Parallel()
	empty := perf.Metrics{URL: "http://y/"}

	_, err := perf.Insight(empty, "LCPBreakdown")
	require.Error(t, err)
	assert.Equal(t, "LCP не найден в трейсе", err.Error())

	got, err := perf.Insight(empty, "LayoutShifts")
	require.NoError(t, err)
	assert.Equal(t, "сдвигов макета нет", got)

	got, err = perf.Insight(empty, "LongTasks")
	require.NoError(t, err)
	assert.Equal(t, "длинных задач нет", got)

	_, err = perf.Insight(empty, "DocumentLatency")
	require.Error(t, err)
	assert.Equal(t, `неизвестный разбор "DocumentLatency": LCPBreakdown, LayoutShifts, LongTasks`, err.Error())
}
