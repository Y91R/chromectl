//go:build e2e

// e2e Э4, консоль: сообщения, залогированные до вызова CLI, видны без демона.
// Ожидания — из docs/plans/etap-4-konsol-i-set.md.
package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const consolePage = `<!doctype html><meta charset="utf-8"><title>Консоль</title>
<script>
console.error("до CLI");
console.log("обычное", 42);
for (let i = 0; i < 3; i++) console.warn("повтор");
setTimeout(() => { throw new Error("бум"); }, 0);
</script>`

func startConsoleFixture(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, consolePage)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/"
}

func TestConsole_ListShowsMessagesLoggedBeforeCLI(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", startConsoleFixture(t))
	e.mustRun("eval", "() => new Promise(r => setTimeout(() => r(1), 300))")

	list := e.mustRun("console", "list").stdout
	assert.Contains(t, list, "[error] до CLI (1 args)")
	assert.Contains(t, list, "[log] обычное 42 (2 args)")
	assert.Contains(t, list, "[warn] повтор (1 args) [3 times]")
	assert.Regexp(t, `\[error\] (Uncaught )?Error: бум`, list)

	errorsOnly := e.mustRun("console", "list", "--types", "error").stdout
	assert.Contains(t, errorsOnly, "до CLI")
	assert.NotContains(t, errorsOnly, "[log]")

	m := regexp.MustCompile(`msgid=(\d+) \[log\] обычное`).FindStringSubmatch(list)
	require.NotNil(t, m, list)
	detail := e.mustRun("console", "get", m[1]).stdout
	assert.Contains(t, detail, "Сообщение: log> обычное 42")
	assert.Contains(t, detail, "Arg #1: 42")
}

func TestConsole_Errors(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.startBrowser()
	e.mustRun("pages", "new", startConsoleFixture(t))

	r := e.run("console", "get", "999")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, "сообщение 999 не найдено")

	r = e.run("console", "list", "--types", "шум")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.stderr, `неизвестный тип сообщения "шум"`)
}
