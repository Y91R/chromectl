package browser_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Y91R/chromectl/internal/browser"
)

func TestParseBrowserWSURL_ReadsDebuggerURL(t *testing.T) {
	t.Parallel()
	body := []byte(`{
   "Browser": "Chrome/153.0.8010.36",
   "Protocol-Version": "1.3",
   "webSocketDebuggerUrl": "ws://127.0.0.1:9478/devtools/browser/615065a7-3932-424c-880c-ffc9ba30579c"
}`)

	got, err := browser.ParseBrowserWSURL(body, 9478)

	require.NoError(t, err)
	assert.Equal(t, "ws://127.0.0.1:9478/devtools/browser/615065a7-3932-424c-880c-ffc9ba30579c", got)
}

func TestParseBrowserWSURL_RejectsUnexpectedResponse(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"не JSON":         `DevTools listening`,
		"нет адреса":      `{"Browser":"Chrome/153.0.8010.36"}`,
		"чужой порт":      `{"webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/x"}`,
		"не browser-путь": `{"webSocketDebuggerUrl":"ws://127.0.0.1:9478/devtools/page/x"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := browser.ParseBrowserWSURL([]byte(body), 9478)
			assert.Error(t, err)
		})
	}
}

func TestFindChrome_PrefersEnvPath(t *testing.T) {
	t.Parallel()
	exists := func(p string) bool { return p == "/opt/chrome" || p == "/usr/bin/google-chrome" }

	got, err := browser.FindChrome("/opt/chrome", []string{"/usr/bin/google-chrome"}, exists)

	require.NoError(t, err)
	assert.Equal(t, "/opt/chrome", got)
}

func TestFindChrome_EnvPathMissingIsError(t *testing.T) {
	t.Parallel()
	exists := func(p string) bool { return p == "/usr/bin/google-chrome" }

	_, err := browser.FindChrome("/opt/nope", []string{"/usr/bin/google-chrome"}, exists)

	require.Error(t, err)
	assert.Equal(t, "CHROME_PATH=/opt/nope: файл не найден", err.Error())
}

func TestFindChrome_FirstExistingCandidate(t *testing.T) {
	t.Parallel()
	exists := func(p string) bool { return p == "/b" || p == "/c" }

	got, err := browser.FindChrome("", []string{"/a", "/b", "/c"}, exists)

	require.NoError(t, err)
	assert.Equal(t, "/b", got)
}

func TestFindChrome_NothingFound(t *testing.T) {
	t.Parallel()

	_, err := browser.FindChrome("", []string{"/a"}, func(string) bool { return false })

	require.Error(t, err)
	assert.Equal(t, "исполняемый файл Chrome не найден, укажите путь в CHROME_PATH", err.Error())
}

func TestLaunchArgs_Headless(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{
		"--remote-debugging-port=9222",
		"--user-data-dir=/tmp/profile",
		"--no-first-run",
		"--no-default-browser-check",
		"--headless=new",
		"about:blank",
	}, browser.LaunchArgs(9222, "/tmp/profile", true))
}

func TestLaunchArgs_Headed(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{
		"--remote-debugging-port=9333",
		"--user-data-dir=/tmp/profile",
		"--no-first-run",
		"--no-default-browser-check",
		"about:blank",
	}, browser.LaunchArgs(9333, "/tmp/profile", false))
}
