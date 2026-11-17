package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover превращает панику в обработчике в ответ 500 с телом Error из контракта и запись в лог со стеком.
// Один сломанный запрос не должен ронять сервер. http.ErrAbortHandler (клиент ушёл посреди ответа)
// пробрасывается дальше: так net/http понимает, что соединение надо оборвать.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				log.ErrorContext(r.Context(), "паника в обработчике",
					"request_id", IDFrom(r.Context()), "panic", rec, "stack", string(debug.Stack()))
				// Если ответ уже начат, заголовок не поменять; лучше оборвать, чем отдать обрезанный JSON.
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"code":"internal","message":"Внутренняя ошибка сервера"}` + "\n"))
			}()
			next.ServeHTTP(w, r)
		})
	}
}
