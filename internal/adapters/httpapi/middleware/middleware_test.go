package middleware

import (
	"bytes"
	"encoding/json"
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
