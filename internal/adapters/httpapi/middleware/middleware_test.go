package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDGeneratedAndSanitized(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = IDFrom(r.Context()) }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if len(seen) != 16 || rec.Header().Get(HeaderRequestID) != seen {
		t.Fatalf("сгенерированный id: %q, заголовок %q", seen, rec.Header().Get(HeaderRequestID))
	}

	for _, bad := range []string{"с пробелом", "line\nbreak", strings.Repeat("a", 65), "a/b"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(HeaderRequestID, bad)
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get(HeaderRequestID); got == bad || len(got) != 16 {
			t.Errorf("плохой id %q принят как %q", bad, got)
		}
	}
	if IDFrom(httptest.NewRequest(http.MethodGet, "/", nil).Context()) != "" {
		t.Error("вне запроса id должен быть пустым")
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flushRecorder) Flush() { f.flushed = true }

func TestAccessLogKeepsFlushForSSE(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	h := AccessLog(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("event: start\n\n"))
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("Flush через ResponseController: %v", err)
		}
		if f, ok := w.(http.Flusher); !ok {
			t.Error("обёртка должна оставаться http.Flusher")
		} else {
			f.Flush()
		}
	}))
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))
	if !rec.flushed {
		t.Fatal("Flush не дошёл до исходного ResponseWriter")
	}
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil || line["status"] != float64(200) || line["bytes"] != float64(14) {
		t.Fatalf("запись журнала: %v (%v)", line, err)
	}
}

func TestAccessLogDoesNotLogQueryString(t *testing.T) {
	var buf bytes.Buffer
	h := AccessLog(slog.New(slog.NewJSONHandler(&buf, nil)))(http.NotFoundHandler())
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/search?q=секрет", nil))
	if strings.Contains(buf.String(), "секрет") {
		t.Fatalf("строка запроса попала в лог: %s", buf.String())
	}
}

func TestRecoverRethrowsAbortHandler(t *testing.T) {
	h := Recover(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if r := recover(); r != http.ErrAbortHandler {
			t.Fatalf("ожидалась паника ErrAbortHandler, получили %v", r)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func quietLogger(buf *bytes.Buffer) *slog.Logger { return slog.New(slog.NewJSONHandler(buf, nil)) }

func TestRecoverBeforeAnyWriteAnswers500(t *testing.T) {
	var logs bytes.Buffer
	h := Recover(quietLogger(&logs))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("x") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var e struct{ Code, Message string }
	if rec.Code != http.StatusInternalServerError || json.Unmarshal(rec.Body.Bytes(), &e) != nil || e.Code != "internal" {
		t.Fatalf("код %d, тело %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), `"response_started":false`) {
		t.Fatalf("в логе нет признака response_started=false: %s", logs.String())
	}
}

// Паника после начала записи: JSON ошибки к уже отправленной части не клеим, соединение обрываем.
func TestRecoverAfterPartialWriteAbortsInsteadOfAppendingError(t *testing.T) {
	var logs bytes.Buffer
	h := Recover(quietLogger(&logs))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("часть"))
		panic("x")
	}))
	rec := httptest.NewRecorder()
	func() {
		defer func() {
			if r := recover(); r != http.ErrAbortHandler {
				t.Fatalf("ожидали panic(http.ErrAbortHandler), получили %v", r)
			}
		}()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	}()
	if body := rec.Body.String(); body != "часть" {
		t.Fatalf("к обрезанному ответу ничего дописываться не должно, тело %q", body)
	}
	if strings.Contains(rec.Body.String(), `"code":"internal"`) {
		t.Fatal("JSON ошибки приклеился к потоку")
	}
	if !strings.Contains(logs.String(), `"response_started":true`) || !strings.Contains(logs.String(), "паника в обработчике") {
		t.Fatalf("паника должна быть записана в лог: %s", logs.String())
	}
}

func TestRecoverAbortsAfterFlushOnRealServer(t *testing.T) {
	var logs bytes.Buffer
	h := Recover(quietLogger(&logs))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "event: start\n\n")
		_ = http.NewResponseController(w).Flush()
		panic("сломалось посреди потока")
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err == nil {
		t.Fatalf("клиент должен увидеть обрыв, а не успешный ответ; тело %q", body)
	}
	if strings.Contains(string(body), `"code":"internal"`) {
		t.Fatalf("JSON ошибки приклеился к потоку: %q", body)
	}
}

func TestStatusWriterStartedAndStatus(t *testing.T) {
	w := &statusWriter{ResponseWriter: httptest.NewRecorder()}
	if w.Started() || w.Status() != http.StatusOK {
		t.Fatal("до записи: не начат, код по умолчанию 200")
	}
	w.WriteHeader(http.StatusTeapot)
	w.WriteHeader(http.StatusOK) // второй вызов не меняет запомненный код
	if !w.Started() || w.Status() != http.StatusTeapot {
		t.Fatalf("после WriteHeader: %v %d", w.Started(), w.Status())
	}
	f := &statusWriter{ResponseWriter: httptest.NewRecorder()}
	f.Flush()
	if !f.Started() || f.Status() != http.StatusOK {
		t.Fatal("Flush отправляет заголовки: ответ начат с 200")
	}
}
