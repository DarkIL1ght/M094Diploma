package middleware

import "net/http"

// statusWriter запоминает, начат ли ответ, код и размер. Им пользуются AccessLog (для журнала)
// и Recover (чтобы знать, можно ли ещё ответить 500). Unwrap отдаёт исходный ResponseWriter
// http.ResponseController'у, поэтому Flush (потоки ИИ по SSE) проходит насквозь.
type statusWriter struct {
	http.ResponseWriter
	status int // 0 — ничего ещё не записано
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
		w.status = http.StatusOK // первая запись без WriteHeader — это неявный 200
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

// Flush для тех, кто проверяет интерфейс http.Flusher приведением типа.
func (w *statusWriter) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK // Flush отправляет заголовки: ответ начат
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Started — ответ уже начат: заголовки ушли клиенту, код ответа не поменять.
func (w *statusWriter) Started() bool { return w.status != 0 }

// Status — код ответа; пока ничего не записано, net/http ответит 200.
func (w *statusWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}
