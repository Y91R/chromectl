// Package observability — логи и служебные пробы. Логгер один на процесс,
// формат структурный: сообщение — константа, переменное уходит в поля.
package observability

import (
	"log/slog"
	"os"
	"strings"
)

func NewLogger(level, service string) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(level)})
	return slog.New(h).With(slog.String("service", service))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
