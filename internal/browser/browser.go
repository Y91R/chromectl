// Package browser находит бинарь Chrome, запускает его отдельным процессом с
// debug-портом и останавливает.
//
// ADR: docs/adr/0009-cli-bez-demona.md — резидентный процесс один, сам Chrome.
package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	logFile      = "chromectl-chrome.log"
	pollInterval = 100 * time.Millisecond
	probeTimeout = time.Second
)

type Process struct {
	PID          int
	Port         int
	UserDataDir  string
	BrowserWSURL string
}

// ParseBrowserWSURL достаёт адрес browser websocket из ответа /json/version и
// проверяет, что он указывает на ожидаемый порт.
func ParseBrowserWSURL(body []byte, port int) (string, error) {
	var version struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(body, &version); err != nil {
		return "", fmt.Errorf("ответ /json/version не JSON: %w", err)
	}
	prefix := fmt.Sprintf("ws://127.0.0.1:%d/devtools/browser/", port)
	if !strings.HasPrefix(version.WebSocketDebuggerURL, prefix) {
		return "", fmt.Errorf("debug-порт %d вернул адрес %q, ожидался %s…", port, version.WebSocketDebuggerURL, prefix)
	}
	return version.WebSocketDebuggerURL, nil
}

// FindChrome возвращает путь к Chrome: из CHROME_PATH, иначе первый существующий
// из стандартных путей.
func FindChrome(envPath string, candidates []string, exists func(string) bool) (string, error) {
	if envPath != "" {
		if exists(envPath) {
			return envPath, nil
		}
		return "", fmt.Errorf("CHROME_PATH=%s: файл не найден", envPath)
	}
	for _, c := range candidates {
		if exists(c) {
			return c, nil
		}
	}
	return "", errors.New("исполняемый файл Chrome не найден, укажите путь в CHROME_PATH")
}

func DefaultCandidates(goos string) []string {
	switch goos {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
		}
	default:
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		}
	}
}

func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func LaunchArgs(port int, userDataDir string, headless bool) []string {
	args := []string{
		"--remote-debugging-port=" + strconv.Itoa(port),
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--no-default-browser-check",
	}
	if headless {
		args = append(args, "--headless=new")
	}
	return append(args, "about:blank")
}

// Launch запускает Chrome в отдельной сессии процессов, чтобы он пережил
// завершение chromectl, и ждёт, пока debug-порт ответит.
func Launch(ctx context.Context, chromePath string, port int, userDataDir string, headless bool) (Process, error) {
	// На занятом порту Chrome молча откроется без debug-порта, а /json/version
	// ответит чужой процесс — подключились бы не к своему браузеру.
	if portBusy(port) {
		return Process{}, fmt.Errorf("порт %d уже занят: выберите другой --port", port)
	}
	if err := os.MkdirAll(userDataDir, 0o700); err != nil {
		return Process{}, fmt.Errorf("каталог профиля: %w", err)
	}

	logPath := filepath.Join(userDataDir, logFile)
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return Process{}, fmt.Errorf("лог Chrome: %w", err)
	}
	defer func() { _ = log.Close() }()

	cmd := exec.Command(chromePath, LaunchArgs(port, userDataDir, headless)...)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return Process{}, fmt.Errorf("запуск %s: %w", chromePath, err)
	}
	pid := cmd.Process.Pid

	wsURL, err := waitReady(ctx, pid, port)
	if err != nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_, _ = syscall.Wait4(pid, nil, 0, nil)
		return Process{}, fmt.Errorf("%w (лог: %s)", err, logPath)
	}
	return Process{PID: pid, Port: port, UserDataDir: userDataDir, BrowserWSURL: wsURL}, nil
}

func portBusy(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// waitReady опрашивает /json/version: файл DevToolsActivePort Chrome пишет
// только для --remote-debugging-port=0, а порт здесь явный.
func waitReady(ctx context.Context, pid, port int) (string, error) {
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: probeTimeout}
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		// Родитель Chrome — этот процесс, поэтому ранний выход виден через wait4:
		// например, профиль уже занят другим экземпляром.
		var status syscall.WaitStatus
		if wpid, err := syscall.Wait4(pid, &status, syscall.WNOHANG, nil); err == nil && wpid == pid {
			return "", fmt.Errorf("процесс Chrome завершился при старте с кодом %d", status.ExitStatus())
		}

		if wsURL, err := probeVersion(ctx, client, url, port); err == nil {
			return wsURL, nil
		}

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("debug-порт %d не открылся: %w", port, ctx.Err())
		case <-ticker.C:
		}
	}
}

func probeVersion(ctx context.Context, client *http.Client, url string, port int) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return ParseBrowserWSURL(body, port)
}

// Alive сообщает, существует ли процесс. Родитель Chrome к моменту вызова уже
// завершился, поэтому зомби не остаётся и сигнал 0 отвечает честно.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func WaitExit(ctx context.Context, pid int) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for Alive(pid) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("процесс %d не завершился: %w", pid, ctx.Err())
		case <-ticker.C:
		}
	}
	return nil
}

func Kill(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("SIGKILL %d: %w", pid, err)
	}
	return nil
}
