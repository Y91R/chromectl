package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ожидания — docs/plans/podderzhka-codex.md: отдельная упаковка, общие скиллы и CLI.
func TestCodexPluginManifest(t *testing.T) {
	t.Parallel()

	var plugin struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Skills      string `json:"skills"`
		Local       bool   `json:"requires_local_executor"`
		Author      struct {
			Name string `json:"name"`
		} `json:"author"`
		Interface struct {
			DisplayName      string `json:"displayName"`
			ShortDescription string `json:"shortDescription"`
		} `json:"interface"`
	}
	readJSON(t, "../../.codex-plugin/plugin.json", &plugin)
	assert.Equal(t, "chromectl", plugin.Name)
	assert.NotEmpty(t, plugin.Version)
	assert.NotEmpty(t, plugin.Description)
	assert.Equal(t, "Y91R", plugin.Author.Name)
	assert.Equal(t, "./skills/", plugin.Skills)
	assert.True(t, plugin.Local, "плагину нужны локальные Go и Chrome")
	assert.Equal(t, "chromectl", plugin.Interface.DisplayName)
	assert.NotEmpty(t, plugin.Interface.ShortDescription)

	var claude struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	readJSON(t, "../../.claude-plugin/plugin.json", &claude)
	assert.Equal(t, claude.Name, plugin.Name)
	assert.Equal(t, claude.Version, plugin.Version, "один CLI должен иметь одну версию в обоих агентах")

	for _, skill := range []string{"browser", "browser-a11y", "browser-lcp", "browser-troubleshooting"} {
		_, err := os.Stat(filepath.Join("../..", plugin.Skills, skill, "SKILL.md"))
		assert.NoError(t, err, "общий скилл %s доступен Codex", skill)
	}
}

func TestCodexMarketplace(t *testing.T) {
	t.Parallel()

	var marketplace struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source struct {
				Source string `json:"source"`
				Path   string `json:"path"`
			} `json:"source"`
			Policy struct {
				Installation   string `json:"installation"`
				Authentication string `json:"authentication"`
			} `json:"policy"`
			Category string `json:"category"`
		} `json:"plugins"`
	}
	readJSON(t, "../../.agents/plugins/marketplace.json", &marketplace)
	assert.Equal(t, "chromectl", marketplace.Name)
	require.Len(t, marketplace.Plugins, 1)
	plugin := marketplace.Plugins[0]
	assert.Equal(t, "chromectl", plugin.Name)
	assert.Equal(t, "local", plugin.Source.Source)
	assert.Equal(t, "./", plugin.Source.Path, "источник относительно корня репозитория")
	assert.Equal(t, "AVAILABLE", plugin.Policy.Installation)
	assert.Equal(t, "ON_INSTALL", plugin.Policy.Authentication)
	assert.Equal(t, "Productivity", plugin.Category)
}
