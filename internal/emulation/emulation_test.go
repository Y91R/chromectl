package emulation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Y91R/chromectl/internal/emulation"
	"github.com/Y91R/chromectl/internal/state"
)

func str(s string) *string {
	return &s
}

func TestMerge_ParsesViewport(t *testing.T) {
	t.Parallel()
	cases := map[string]state.Viewport{
		"390x844x3,mobile,touch": {Width: 390, Height: 844, DeviceScaleFactor: 3, Mobile: true, Touch: true},
		"800x600":                {Width: 800, Height: 600, DeviceScaleFactor: 1},
		"844x390x2,landscape":    {Width: 844, Height: 390, DeviceScaleFactor: 2, Landscape: true},
	}
	for value, want := range cases {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			got, err := emulation.Merge(state.Emulation{}, emulation.Changes{Viewport: str(value)})
			require.NoError(t, err)
			require.NotNil(t, got.Viewport)
			assert.Equal(t, want, *got.Viewport)
		})
	}
}

func TestMerge_RejectsInvalidValues(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		changes emulation.Changes
		message string
	}{
		"нет высоты":           {emulation.Changes{Viewport: str("390x")}, `неверная высота viewport ""`},
		"нулевая ширина":       {emulation.Changes{Viewport: str("0x600")}, `неверная ширина viewport "0"`},
		"нулевая плотность":    {emulation.Changes{Viewport: str("390x844x0")}, `неверная плотность пикселей "0"`},
		"CPU больше 20":        {emulation.Changes{CPU: str("21")}, "--cpu должен быть от 1 до 20, получено 21"},
		"CPU не число":         {emulation.Changes{CPU: str("быстро")}, `--cpu должен быть от 1 до 20, получено "быстро"`},
		"неизвестная сеть":     {emulation.Changes{Network: str("5G")}, `неизвестный профиль сети "5G": Offline, Slow 3G, Fast 3G, Slow 4G, Fast 4G или none`},
		"широта вне диапазона": {emulation.Changes{Geolocation: str("91,0")}, `неверная широта "91": нужна от -90 до 90`},
		"долгота вне диапазона": {emulation.Changes{Geolocation: str("0,181")},
			`неверная долгота "181": нужна от -180 до 180`},
		"заголовки не объект": {emulation.Changes{Headers: str("[1]")}, "--headers должен быть JSON-объектом"},
		"неизвестная тема":    {emulation.Changes{ColorScheme: str("blue")}, `--color-scheme: dark, light или auto, получено "blue"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := emulation.Merge(state.Emulation{}, tc.changes)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
		})
	}
}

func TestMerge_ChangesOnlyPassedSettings(t *testing.T) {
	t.Parallel()
	current := state.Emulation{ColorScheme: "dark", CPURate: 4, Headers: map[string]string{"X-Test": "1"}}

	got, err := emulation.Merge(current, emulation.Changes{
		Viewport:    str("390x844"),
		Network:     str("Slow 3G"),
		Geolocation: str("55.75,37.62"),
		UserAgent:   str("chromectl-test"),
	})

	require.NoError(t, err)
	assert.Equal(t, state.Emulation{
		Viewport:    &state.Viewport{Width: 390, Height: 844, DeviceScaleFactor: 1},
		ColorScheme: "dark",
		Network:     "Slow 3G",
		CPURate:     4,
		UserAgent:   "chromectl-test",
		Geolocation: &state.Geolocation{Latitude: 55.75, Longitude: 37.62},
		Headers:     map[string]string{"X-Test": "1"},
	}, got)
	assert.Equal(t, "dark", current.ColorScheme, "Merge не меняет исходные настройки")
}

func TestMerge_ResetValues(t *testing.T) {
	t.Parallel()
	current := state.Emulation{
		Viewport:    &state.Viewport{Width: 390, Height: 844, DeviceScaleFactor: 3},
		ColorScheme: "dark",
		Network:     "Offline",
		CPURate:     4,
		UserAgent:   "chromectl-test",
		Geolocation: &state.Geolocation{Latitude: 1, Longitude: 2},
		Headers:     map[string]string{"X-Test": "1"},
	}

	got, err := emulation.Merge(current, emulation.Changes{
		Viewport:    str(""),
		ColorScheme: str("auto"),
		Network:     str("none"),
		CPU:         str("1"),
		UserAgent:   str(""),
		Geolocation: str(""),
		Headers:     str(""),
	})

	require.NoError(t, err)
	assert.Equal(t, state.Emulation{}, got)
}

func TestMerge_ParsesHeadersAndLightScheme(t *testing.T) {
	t.Parallel()

	got, err := emulation.Merge(state.Emulation{}, emulation.Changes{
		Headers:     str(`{"X-Test":"1","Authorization":"Bearer t"}`),
		ColorScheme: str("light"),
		CPU:         str("2.5"),
	})

	require.NoError(t, err)
	assert.Equal(t, state.Emulation{
		ColorScheme: "light",
		CPURate:     2.5,
		Headers:     map[string]string{"X-Test": "1", "Authorization": "Bearer t"},
	}, got)
}

func TestNetworkConditions_Presets(t *testing.T) {
	t.Parallel()
	cases := map[string]emulation.Conditions{
		"Offline": {Offline: true},
		"Slow 3G": {Latency: 2000, Download: 50000, Upload: 50000},
		"Fast 3G": {Latency: 562.5, Download: 180000, Upload: 84375},
		"Slow 4G": {Latency: 562.5, Download: 180000, Upload: 84375},
		"Fast 4G": {Latency: 165, Download: 1012500, Upload: 168750},
	}
	for name, want := range cases {
		got, ok := emulation.NetworkConditions(name)
		assert.True(t, ok, name)
		assert.InDelta(t, want.Latency, got.Latency, 0.001, name)
		assert.InDelta(t, want.Download, got.Download, 0.001, name)
		assert.InDelta(t, want.Upload, got.Upload, 0.001, name)
		assert.Equal(t, want.Offline, got.Offline, name)
	}

	_, ok := emulation.NetworkConditions("5G")
	assert.False(t, ok)
}

func TestDescribe(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "без эмуляции", emulation.Describe(state.Emulation{}))
	assert.Equal(t, `viewport 390x844x3,mobile,touch; тема dark; сеть Slow 3G; CPU ×4; геолокация 55.75,37.62; user agent "bot"; заголовки: X-Test`,
		emulation.Describe(state.Emulation{
			Viewport:    &state.Viewport{Width: 390, Height: 844, DeviceScaleFactor: 3, Mobile: true, Touch: true},
			ColorScheme: "dark",
			Network:     "Slow 3G",
			CPURate:     4,
			UserAgent:   "bot",
			Geolocation: &state.Geolocation{Latitude: 55.75, Longitude: 37.62},
			Headers:     map[string]string{"X-Test": "1"},
		}))
}
