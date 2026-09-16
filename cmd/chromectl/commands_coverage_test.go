package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryCommandHasE2ECase — гардрейл ADR-0008: команда без кейса не имеет
// спецификации поведения. Ищется полный путь аргументов ("pages", "list"), а не
// одно слово: иначе будущий `console list` засчитал бы кейс `pages list`.
func TestEveryCommandHasE2ECase(t *testing.T) {
	t.Parallel()
	sources := readTestSources(t, "../../internal/e2e/*_test.go", "*_test.go")

	var leaves, missing []string
	walkLeaves(newRootCmd(), nil, func(path []string) {
		leaves = append(leaves, strings.Join(path, " "))
		if !strings.Contains(sources, quotedArgs(path)) {
			missing = append(missing, strings.Join(path, " "))
		}
	})

	// Пустой обход значит, что проверка сломалась, а не что всё покрыто.
	require.NotEmpty(t, leaves, "не найдено ни одной команды")
	assert.Empty(t, missing, "команды без e2e-кейса (или unit-кейса в cmd/chromectl)")
}

func walkLeaves(cmd *cobra.Command, parent []string, visit func([]string)) {
	for _, child := range cmd.Commands() {
		path := append(append([]string{}, parent...), child.Name())
		if len(child.Commands()) == 0 && child.Runnable() {
			visit(path)
			continue
		}
		walkLeaves(child, path, visit)
	}
}

func quotedArgs(path []string) string {
	return `"` + strings.Join(path, `", "`) + `"`
}

func readTestSources(t *testing.T, patterns ...string) string {
	t.Helper()
	var b strings.Builder
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		require.NoError(t, err)
		for _, f := range files {
			// Свой файл не считается: пример пути в комментарии засчитал бы команду.
			if filepath.Base(f) == "commands_coverage_test.go" {
				continue
			}
			data, err := os.ReadFile(f)
			require.NoError(t, err)
			b.Write(data)
		}
	}
	return b.String()
}
