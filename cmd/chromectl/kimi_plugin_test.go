package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Ожидания — docs/plans/podderzhka-codex.md: Kimi использует общие скиллы и CLI.
func TestKimiPluginManifest(t *testing.T) {
	t.Parallel()

	var plugin struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Skills      string `json:"skills"`
		Author      struct {
			Name string `json:"name"`
		} `json:"author"`
		Interface struct {
			DisplayName      string `json:"displayName"`
			ShortDescription string `json:"shortDescription"`
		} `json:"interface"`
	}
	readJSON(t, "../../kimi.plugin.json", &plugin)
	assert.Equal(t, "chromectl", plugin.Name)
	assert.NotEmpty(t, plugin.Version)
	assert.NotEmpty(t, plugin.Description)
	assert.Equal(t, "Y91R", plugin.Author.Name)
	assert.Equal(t, "./skills/", plugin.Skills)
	assert.Equal(t, "chromectl", plugin.Interface.DisplayName)
	assert.NotEmpty(t, plugin.Interface.ShortDescription)

	var claude struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	readJSON(t, "../../.claude-plugin/plugin.json", &claude)
	assert.Equal(t, claude.Name, plugin.Name)
	assert.Equal(t, claude.Version, plugin.Version, "один CLI должен иметь одну версию во всех агентах")

	for _, skill := range []string{"browser", "browser-a11y", "browser-lcp", "browser-troubleshooting"} {
		_, err := os.Stat(filepath.Join("../..", plugin.Skills, skill, "SKILL.md"))
		assert.NoError(t, err, "общий скилл %s доступен Kimi Code", skill)
	}
}
