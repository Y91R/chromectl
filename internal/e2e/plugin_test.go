//go:build e2e

// Обёртка плагина bin/chromectl: у пользователя плагина нет собранного бинаря, есть
// только исходники. Ожидания — из docs/plans/nested-painting-pebble.md, шаг 4.
package e2e_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginWrapper_BuildsOnFirstCallAndReusesBinary(t *testing.T) {
	t.Parallel()
	root := copyPluginSources(t)

	first := runWrapper(t, root, os.Getenv("PATH"), "version")
	require.Equal(t, 0, first.code, first.stderr)
	assert.Equal(t, "0.1.0\n", first.stdout)

	built, err := os.Stat(filepath.Join(root, "build", "chromectl"))
	require.NoError(t, err)

	second := runWrapper(t, root, os.Getenv("PATH"), "version")
	require.Equal(t, 0, second.code, second.stderr)
	assert.Equal(t, "0.1.0\n", second.stdout)

	again, err := os.Stat(filepath.Join(root, "build", "chromectl"))
	require.NoError(t, err)
	assert.Equal(t, built.ModTime(), again.ModTime(), "второй вызов пересобрал бинарь")
}

func TestPluginWrapper_WithoutGoFails(t *testing.T) {
	t.Parallel()
	root := copyPluginSources(t)

	res := runWrapper(t, root, "/usr/bin:/bin", "version")
	assert.NotEqual(t, 0, res.code)
	assert.Empty(t, res.stdout)
	assert.Contains(t, res.stderr, "нужен Go")
}

func copyPluginSources(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs("../..")
	require.NoError(t, err)
	dst := t.TempDir()
	for _, dir := range []string{"bin", "cmd", "internal", ".claude-plugin"} {
		require.NoError(t, os.CopyFS(filepath.Join(dst, dir), os.DirFS(filepath.Join(src, dir))))
	}
	for _, file := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(src, file))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dst, file), data, 0o644))
	}
	return dst
}

func runWrapper(t *testing.T, root, path string, args ...string) result {
	t.Helper()
	cmd := exec.Command(filepath.Join(root, "bin", "chromectl"), args...)
	cmd.Env = append(os.Environ(), "PATH="+path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		require.True(t, errors.As(err, &exitErr), "запуск обёртки: %v", err)
		code = exitErr.ExitCode()
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}
