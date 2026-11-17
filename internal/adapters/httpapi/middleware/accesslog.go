package middleware

import (
	"log/slog"
	"net/http"
	"slices"
	"time"
)

// statusWriter запоминает код и размер ответа. Unwrap отдаёт исходный ResponseWriter
// http.ResponseController'у, поэтому Flush (потоки ИИ по SSE) проходит насквозь.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush для тех, кто проверяет интерфейс http.Flusher приведением типа.
func (w *statusWriter) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// AccessLog пишет одну строку на запрос: метод, путь, код, размер, время, идентификатор запроса.
// Запросы к путям из quiet (проверки живости раз в несколько секунд) идут на уровне debug.
// Тело, заголовки и строка запроса не логируются: в них могут быть cookie и ключ восстановления.
func AccessLog(log *slog.Logger, quiet ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w}
			defer func() {
				level := slog.LevelInfo
				if slices.Contains(quiet, r.URL.Path) {
					level = slog.LevelDebug
				}
				status := sw.status
				if status == 0 {
					status = http.StatusOK
				}
				log.LogAttrs(r.Context(), level, "запрос",
					slog.String("request_id", IDFrom(r.Context())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", status),
					slog.Int64("bytes", sw.bytes),
					slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
				)
			}()
			next.ServeHTTP(sw, r)
		})
	}
}
