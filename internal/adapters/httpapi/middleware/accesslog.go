package middleware

import (
	"log/slog"
	"net/http"
	"slices"
	"time"
)

// AccessLog пишет одну строку на запрос: метод, путь, код, размер, время, идентификатор запроса.
// Запросы к путям из quiet (проверки живости раз в несколько секунд) идут на уровне debug.
// Тело, заголовки и строка запроса не логируются: в них могут быть cookie и ключ восстановления.
func AccessLog(log *slog.Logger, quiet ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return accessLogger{next: next, log: log, quiet: quiet}
	}
}

type accessLogger struct {
	next  http.Handler
	log   *slog.Logger
	quiet []string
}

func (m accessLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	sw := &statusWriter{ResponseWriter: w}
	defer m.logRequest(r, sw, start)
	m.next.ServeHTTP(sw, r)
}

func (m accessLogger) logRequest(r *http.Request, sw *statusWriter, start time.Time) {
	level := slog.LevelInfo
	if slices.Contains(m.quiet, r.URL.Path) {
		level = slog.LevelDebug
	}
	m.log.LogAttrs(r.Context(), level, "запрос",
		slog.String("request_id", IDFrom(r.Context())),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.Int("status", sw.Status()),
		slog.Int64("bytes", sw.bytes),
		slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
	)
}
