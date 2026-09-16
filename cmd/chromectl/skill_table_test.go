package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var backticked = regexp.MustCompile("`([^`]+)`")

// TestSkillTableMatchesCommandTree — таблица команд скилла /browser описывает ровно
// дерево cobra: новая команда без строки в таблице или устаревшая строка скилла
// уводят агента в сторону так же, как неправильная справка.
func TestSkillTableMatchesCommandTree(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../skills/browser/SKILL.md")
	require.NoError(t, err)

	inTable := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		firstCell := strings.SplitN(line, " | ", 2)[0]
		for _, m := range backticked.FindAllStringSubmatch(firstCell, -1) {
			if path := commandPath(m[1]); path != "" {
				inTable[path] = true
			}
		}
	}
	require.NotEmpty(t, inTable, "таблица команд не разобрана")

	inTree := map[string]bool{}
	walkLeaves(newRootCmd(), nil, func(path []string) {
		if joined := strings.Join(path, " "); joined != "version" {
			inTree[joined] = true
		}
	})

	assert.Empty(t, difference(inTree, inTable), "команды без строки в таблице скилла")
	assert.Empty(t, difference(inTable, inTree), "строки таблицы скилла без команды")
}

// commandPath берёт слова команды до первого флага, аргумента или альтернативы:
// "pages select <id>" → "pages select", "--back" → "".
func commandPath(cell string) string {
	var words []string
	for _, w := range strings.Fields(cell) {
		if strings.HasPrefix(w, "-") || strings.HasPrefix(w, "<") || strings.HasPrefix(w, "[") || strings.Contains(w, "|") {
			break
		}
		words = append(words, w)
	}
	return strings.Join(words, " ")
}

func difference(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
