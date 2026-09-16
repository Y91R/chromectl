package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPluginManifests — репозиторий устанавливается как плагин Claude Code: манифест
// плагина и маркетплейса указывают на корень, а скиллы лежат там, где их ищет плагин.
func TestPluginManifests(t *testing.T) {
	t.Parallel()

	var plugin struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	readJSON(t, "../../.claude-plugin/plugin.json", &plugin)
	assert.Equal(t, "chromectl", plugin.Name)
	assert.NotEmpty(t, plugin.Version)

	var marketplace struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	readJSON(t, "../../.claude-plugin/marketplace.json", &marketplace)
	assert.Equal(t, "chromectl", marketplace.Name)
	require.Len(t, marketplace.Plugins, 1)
	assert.Equal(t, "chromectl", marketplace.Plugins[0].Name)
	assert.Equal(t, "./", marketplace.Plugins[0].Source)

	for _, skill := range []string{"browser", "browser-a11y", "browser-lcp", "browser-troubleshooting"} {
		data, err := os.ReadFile("../../skills/" + skill + "/SKILL.md")
		if !assert.NoError(t, err, skill) {
			continue
		}
		assert.True(t, strings.HasPrefix(string(data), "---\nname: "+skill+"\n"), "frontmatter скилла %s", skill)
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, v))
}
