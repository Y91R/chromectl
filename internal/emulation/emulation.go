// Package emulation разбирает настройки эмуляции и применяет их к сессии вкладки.
//
// ADR: docs/adr/0009-cli-bez-demona.md — override живёт только пока
// сессия подключена, поэтому применяется при каждом подключении.
package emulation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Y91R/chromectl/internal/cdp"
	"github.com/Y91R/chromectl/internal/state"
)

const viewportFormat = "<ширина>x<высота>[x<плотность>][,mobile][,touch][,landscape]"

// Changes — значения флагов emulate; nil — флаг не передан, настройка не меняется.
type Changes struct {
	Viewport    *string
	ColorScheme *string
	Network     *string
	CPU         *string
	UserAgent   *string
	Geolocation *string
	Headers     *string
}

// Conditions — параметры Network.emulateNetworkConditions: задержка в мс,
// пропускная способность в байтах в секунду.
type Conditions struct {
	Offline  bool
	Latency  float64
	Download float64
	Upload   float64
}

// networkNames — порядок в сообщениях.
var (
	networkNames = []string{"Offline", "Slow 3G", "Fast 3G", "Slow 4G", "Fast 4G"}
	presets      = map[string]Conditions{
		"Offline": {Offline: true},
		"Slow 3G": {Latency: 400 * 5, Download: 500 * 1000 / 8 * 0.8, Upload: 500 * 1000 / 8 * 0.8},
		"Fast 3G": {Latency: 150 * 3.75, Download: 1.6 * 1000 * 1000 / 8 * 0.9, Upload: 750 * 1000 / 8 * 0.9},
		"Slow 4G": {Latency: 150 * 3.75, Download: 1.6 * 1000 * 1000 / 8 * 0.9, Upload: 750 * 1000 / 8 * 0.9},
		"Fast 4G": {Latency: 60 * 2.75, Download: 9 * 1000 * 1000 / 8 * 0.9, Upload: 1.5 * 1000 * 1000 / 8 * 0.9},
	}
)

func NetworkConditions(name string) (Conditions, bool) {
	c, ok := presets[name]
	return c, ok
}

// Merge применяет переданные флаги к текущим настройкам. Сброс — явным значением:
// пустая строка, `auto` для темы, `none` для сети, `1` для CPU.
func Merge(current state.Emulation, ch Changes) (state.Emulation, error) {
	next := current
	if current.Viewport != nil {
		vp := *current.Viewport
		next.Viewport = &vp
	}
	if current.Geolocation != nil {
		geo := *current.Geolocation
		next.Geolocation = &geo
	}
	if current.Headers != nil {
		next.Headers = make(map[string]string, len(current.Headers))
		for k, v := range current.Headers {
			next.Headers[k] = v
		}
	}

	if ch.Viewport != nil {
		if *ch.Viewport == "" {
			next.Viewport = nil
		} else {
			vp, err := parseViewport(*ch.Viewport)
			if err != nil {
				return state.Emulation{}, err
			}
			next.Viewport = &vp
		}
	}
	if ch.ColorScheme != nil {
		switch v := *ch.ColorScheme; v {
		case "", "auto":
			next.ColorScheme = ""
		case "dark", "light":
			next.ColorScheme = v
		default:
			return state.Emulation{}, fmt.Errorf("--color-scheme: dark, light или auto, получено %q", v)
		}
	}
	if ch.Network != nil {
		switch v := *ch.Network; {
		case v == "" || v == "none":
			next.Network = ""
		case presets[v] != Conditions{} || v == "Offline":
			next.Network = v
		default:
			return state.Emulation{}, fmt.Errorf("неизвестный профиль сети %q: %s или none", v, strings.Join(networkNames, ", "))
		}
	}
	if ch.CPU != nil {
		v := *ch.CPU
		rate, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return state.Emulation{}, fmt.Errorf("--cpu должен быть от 1 до 20, получено %q", v)
		}
		if rate < 1 || rate > 20 {
			return state.Emulation{}, fmt.Errorf("--cpu должен быть от 1 до 20, получено %s", v)
		}
		next.CPURate = rate
		if rate == 1 {
			next.CPURate = 0
		}
	}
	if ch.UserAgent != nil {
		next.UserAgent = *ch.UserAgent
	}
	if ch.Geolocation != nil {
		if *ch.Geolocation == "" {
			next.Geolocation = nil
		} else {
			geo, err := parseGeolocation(*ch.Geolocation)
			if err != nil {
				return state.Emulation{}, err
			}
			next.Geolocation = &geo
		}
	}
	if ch.Headers != nil {
		if *ch.Headers == "" {
			next.Headers = nil
		} else {
			var headers map[string]string
			if err := json.Unmarshal([]byte(*ch.Headers), &headers); err != nil {
				return state.Emulation{}, fmt.Errorf("--headers должен быть JSON-объектом со строковыми значениями: %w", err)
			}
			next.Headers = headers
			if len(headers) == 0 {
				next.Headers = nil
			}
		}
	}
	return next, nil
}

func parseViewport(v string) (state.Viewport, error) {
	dims, tags, _ := strings.Cut(v, ",")
	parts := strings.Split(dims, "x")
	part := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}

	width, err := strconv.Atoi(part(0))
	if err != nil || width <= 0 {
		return state.Viewport{}, fmt.Errorf("неверная ширина viewport %q: ожидается %s", part(0), viewportFormat)
	}
	height, err := strconv.Atoi(part(1))
	if err != nil || height <= 0 {
		return state.Viewport{}, fmt.Errorf("неверная высота viewport %q: ожидается %s", part(1), viewportFormat)
	}
	vp := state.Viewport{Width: width, Height: height, DeviceScaleFactor: 1}
	if len(parts) > 2 {
		dpr, err := strconv.ParseFloat(part(2), 64)
		if err != nil || dpr <= 0 {
			return state.Viewport{}, fmt.Errorf("неверная плотность пикселей %q: нужно положительное число", part(2))
		}
		vp.DeviceScaleFactor = dpr
	}
	if tags != "" {
		for _, tag := range strings.Split(tags, ",") {
			switch tag {
			case "mobile":
				vp.Mobile = true
			case "touch":
				vp.Touch = true
			case "landscape":
				vp.Landscape = true
			default:
				return state.Viewport{}, fmt.Errorf("неизвестный признак viewport %q: mobile, touch или landscape", tag)
			}
		}
	}
	return vp, nil
}

func parseGeolocation(v string) (state.Geolocation, error) {
	latStr, lonStr, _ := strings.Cut(v, ",")
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil || lat < -90 || lat > 90 {
		return state.Geolocation{}, fmt.Errorf("неверная широта %q: нужна от -90 до 90", latStr)
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil || lon < -180 || lon > 180 {
		return state.Geolocation{}, fmt.Errorf("неверная долгота %q: нужна от -180 до 180", lonStr)
	}
	return state.Geolocation{Latitude: lat, Longitude: lon}, nil
}

// Apply выставляет override в сессии. Сбрасывать нечего: всё, что не задано,
// сбросилось при отключении прошлой сессии.
func Apply(ctx context.Context, c *cdp.Client, sid string, s state.Emulation) error {
	call := func(method string, params any) error {
		if err := c.Call(ctx, sid, method, params, nil); err != nil {
			return fmt.Errorf("эмуляция: %w", err)
		}
		return nil
	}

	if vp := s.Viewport; vp != nil {
		orientation := map[string]any{"type": "portraitPrimary", "angle": 0}
		if vp.Landscape {
			orientation = map[string]any{"type": "landscapePrimary", "angle": 90}
		}
		if err := call("Emulation.setDeviceMetricsOverride", map[string]any{
			"width": vp.Width, "height": vp.Height, "deviceScaleFactor": vp.DeviceScaleFactor,
			"mobile": vp.Mobile, "screenOrientation": orientation,
		}); err != nil {
			return err
		}
		if vp.Touch {
			if err := call("Emulation.setTouchEmulationEnabled", map[string]any{"enabled": true, "maxTouchPoints": 1}); err != nil {
				return err
			}
		}
	}
	if s.ColorScheme != "" {
		if err := call("Emulation.setEmulatedMedia", map[string]any{
			"features": []map[string]string{{"name": "prefers-color-scheme", "value": s.ColorScheme}},
		}); err != nil {
			return err
		}
	}
	if s.Network != "" || len(s.Headers) > 0 {
		if err := call("Network.enable", nil); err != nil {
			return err
		}
	}
	if s.Network != "" {
		cond := presets[s.Network]
		if err := call("Network.emulateNetworkConditions", map[string]any{
			"offline": cond.Offline, "latency": cond.Latency,
			"downloadThroughput": cond.Download, "uploadThroughput": cond.Upload,
		}); err != nil {
			return err
		}
	}
	if s.CPURate > 1 {
		if err := call("Emulation.setCPUThrottlingRate", map[string]any{"rate": s.CPURate}); err != nil {
			return err
		}
	}
	if s.UserAgent != "" {
		if err := call("Emulation.setUserAgentOverride", map[string]any{"userAgent": s.UserAgent}); err != nil {
			return err
		}
	}
	if geo := s.Geolocation; geo != nil {
		if err := call("Emulation.setGeolocationOverride", map[string]any{
			"latitude": geo.Latitude, "longitude": geo.Longitude, "accuracy": 1,
		}); err != nil {
			return err
		}
	}
	if len(s.Headers) > 0 {
		if err := call("Network.setExtraHTTPHeaders", map[string]any{"headers": s.Headers}); err != nil {
			return err
		}
	}
	return nil
}

func Describe(s state.Emulation) string {
	var parts []string
	if vp := s.Viewport; vp != nil {
		text := fmt.Sprintf("viewport %dx%dx%s", vp.Width, vp.Height, formatNumber(vp.DeviceScaleFactor))
		for _, tag := range []struct {
			on   bool
			name string
		}{{vp.Mobile, "mobile"}, {vp.Touch, "touch"}, {vp.Landscape, "landscape"}} {
			if tag.on {
				text += "," + tag.name
			}
		}
		parts = append(parts, text)
	}
	if s.ColorScheme != "" {
		parts = append(parts, "тема "+s.ColorScheme)
	}
	if s.Network != "" {
		parts = append(parts, "сеть "+s.Network)
	}
	if s.CPURate > 1 {
		parts = append(parts, "CPU ×"+formatNumber(s.CPURate))
	}
	if geo := s.Geolocation; geo != nil {
		parts = append(parts, "геолокация "+formatNumber(geo.Latitude)+","+formatNumber(geo.Longitude))
	}
	if s.UserAgent != "" {
		parts = append(parts, fmt.Sprintf("user agent %q", s.UserAgent))
	}
	if len(s.Headers) > 0 {
		names := make([]string, 0, len(s.Headers))
		for name := range s.Headers {
			names = append(names, name)
		}
		sort.Strings(names)
		parts = append(parts, "заголовки: "+strings.Join(names, ", "))
	}
	if len(parts) == 0 {
		return "без эмуляции"
	}
	return strings.Join(parts, "; ")
}

func formatNumber(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
