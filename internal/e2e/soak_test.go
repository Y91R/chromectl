//go:build e2e

// Главная проверка комплекта (docs/plans/dapper-tinkering-raccoon.md, «Верификация»):
// 50 команд подряд не оставляют вкладок, подключённых сессий и процессов CLI.
package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// attachedTargets — target, к которым сейчас подключён какой-либо клиент CDP.
// Команда, забывшая отсоединиться, оставила бы здесь след.
func (e *env) attachedTargets() []string {
	e.t.Helper()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", e.port))
	require.NoError(e.t, err)
	defer func() { _ = resp.Body.Close() }()
	var version struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	require.NoError(e.t, json.NewDecoder(resp.Body).Decode(&version))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, version.WebSocketDebuggerURL, &websocket.DialOptions{HTTPClient: client})
	require.NoError(e.t, err)
	defer func() { _ = ws.CloseNow() }()
	ws.SetReadLimit(-1)

	require.NoError(e.t, ws.Write(ctx, websocket.MessageText, []byte(`{"id":1,"method":"Target.getTargets"}`)))
	for {
		_, data, err := ws.Read(ctx)
		require.NoError(e.t, err)
		var msg struct {
			ID     int `json:"id"`
			Result struct {
				TargetInfos []struct {
					TargetID string `json:"targetId"`
					Type     string `json:"type"`
					URL      string `json:"url"`
					Attached bool   `json:"attached"`
				} `json:"targetInfos"`
			} `json:"result"`
		}
		require.NoError(e.t, json.Unmarshal(data, &msg))
		if msg.ID != 1 {
			continue
		}
		var attached []string
		for _, info := range msg.Result.TargetInfos {
			if info.Attached && (info.Type == "page" || info.Type == "iframe") {
				attached = append(attached, info.Type+" "+info.URL)
			}
		}
		return attached
	}
}

func chromeRSS(t *testing.T, pid int) string {
	t.Helper()
	out, err := exec.Command("ps", "-o", "rss=", "-p", fmt.Sprint(pid)).Output()
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(out)) + " КБ"
}

func TestSoak_FiftyCommandsLeaveNoTabsSessionsOrProcesses(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", startFormFixture(t))
	snap := e.mustRun("snapshot").stdout
	send := uidOf(t, snap, "button", "Отправить")
	shot := filepath.Join(t.TempDir(), "soak.png")

	before := e.pageTargetIDs()
	pid := e.statePID()
	rssBefore := chromeRSS(t, pid)

	commands := [][]string{
		{"snapshot"},
		{"click", send},
		{"eval", "() => document.title"},
		{"screenshot", "-o", shot},
		{"console", "list"},
	}
	for i := 0; i < 50; i++ {
		e.mustRun(commands[i%len(commands)]...)
	}

	assert.ElementsMatch(t, before, e.pageTargetIDs(), "набор вкладок не изменился")
	assert.Empty(t, e.attachedTargets(), "ни одна команда не оставила подключённую сессию")

	// Параллельные e2e запускают тот же бинарь, поэтому ищутся только процессы этого
	// теста: run передаёт --port первым аргументом, порт у каждого теста свой.
	out, _ := exec.Command("pgrep", "-f", fmt.Sprintf("%s --port %d ", binPath, e.port)).Output()
	assert.Empty(t, strings.TrimSpace(string(out)), "процессов chromectl не осталось")

	t.Logf("RSS Chrome (главный процесс, pid %d): до %s, после 50 команд %s", pid, rssBefore, chromeRSS(t, pid))
}
