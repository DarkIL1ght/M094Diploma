// Package logging создаёт slog-логгер по настройкам процесса.
package logging

import (
	"io"
	"log/slog"
)

// New возвращает логгер: format "json" (для сервера и Docker) или "text" (для чтения в консоли).
func New(w io.Writer, level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if format == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}
