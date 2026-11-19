package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi/respond"
)

// Recover превращает панику в обработчике в ответ 500 по контракту и запись в лог со стеком.
// Один сломанный запрос не должен ронять сервер.
//
// Если ответ уже начат (часть данных ушла клиенту, например поток ИИ по SSE), 500 уже не отправить:
// заголовки отправлены, а дописанный JSON ошибки испортил бы поток. Тогда соединение обрывается через
// http.ErrAbortHandler, и клиент видит обрыв, а не «успешный» обрезанный ответ. Тем же способом
// пробрасывается паника http.ErrAbortHandler самого обработчика (клиент ушёл посреди ответа).
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return recoverer{next: next, log: log}
	}
}

type recoverer struct {
	next http.Handler
	log  *slog.Logger
}

func (m recoverer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sw := &statusWriter{ResponseWriter: w}
	defer m.recoverPanic(sw, r)
	m.next.ServeHTTP(sw, r)
}

// recoverPanic вызывается напрямую через defer, поэтому recover() здесь работает.
func (m recoverer) recoverPanic(sw *statusWriter, r *http.Request) {
	rec := recover()
	if rec == nil {
		return
	}
	if rec == http.ErrAbortHandler {
		panic(rec)
	}
	m.log.ErrorContext(r.Context(), "паника в обработчике",
		"request_id", IDFrom(r.Context()), "panic", rec, "stack", string(debug.Stack()), "response_started", sw.Started())
	if sw.Started() {
		panic(http.ErrAbortHandler)
	}
	respond.Err(sw, http.StatusInternalServerError, "internal", "Внутренняя ошибка сервера")
}
