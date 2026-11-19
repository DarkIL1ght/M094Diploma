package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi/respond"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func do(h http.Handler, method, path string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	h := NewRouter(testLogger(&bytes.Buffer{}))
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		rec := do(h, method, "/healthz", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: код %d", method, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("%s: Content-Type %q", method, ct)
		}
	}
	var body map[string]string
	if err := json.Unmarshal(do(h, http.MethodGet, "/healthz", nil).Body.Bytes(), &body); err != nil || body["status"] != "ok" {
		t.Fatalf("тело: %v, %v", body, err)
	}
}

func TestErrorsFollowContract(t *testing.T) {
	h := NewRouter(testLogger(&bytes.Buffer{}))
	cases := []struct {
		method, path string
		status       int
		code         string
	}{
		{http.MethodGet, "/api/nothing", http.StatusNotFound, "not_found"},
		{http.MethodGet, "/", http.StatusNotFound, "not_found"},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed, "method_not_allowed"},
	}
	for _, c := range cases {
		rec := do(h, c.method, c.path, nil)
		var e respond.Error
		if rec.Code != c.status || json.Unmarshal(rec.Body.Bytes(), &e) != nil || e.Code != c.code || e.Message == "" {
			t.Errorf("%s %s: код %d, тело %q", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
	if allow := do(h, http.MethodPost, "/healthz", nil).Header().Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("Allow = %q", allow)
	}
}

func TestMethodsDispatch(t *testing.T) {
	ok := func(s string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(s)) })
	}
	m := Methods{http.MethodGet: ok("get"), http.MethodPut: ok("put")}
	if got := do(m, http.MethodPut, "/x", nil).Body.String(); got != "put" {
		t.Errorf("PUT: %q", got)
	}
	rec := do(m, http.MethodDelete, "/x", nil)
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD, PUT" {
		t.Errorf("DELETE: %d, Allow %q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestRouterSetsRequestIDAndLogs(t *testing.T) {
	var buf bytes.Buffer
	h := NewRouter(testLogger(&buf))
	rec := do(h, http.MethodGet, "/api/x", map[string]string{"X-Request-ID": "abc-123"})
	if rec.Header().Get("X-Request-ID") != "abc-123" {
		t.Fatalf("заголовок: %q", rec.Header().Get("X-Request-ID"))
	}
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("лог не JSON: %v: %q", err, buf.String())
	}
	if line["request_id"] != "abc-123" || line["status"] != float64(404) || line["path"] != "/api/x" || line["level"] != "INFO" {
		t.Fatalf("запись журнала: %v", line)
	}
}

func TestHealthzIsQuietInLog(t *testing.T) {
	var buf bytes.Buffer
	h := NewRouter(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	do(h, http.MethodGet, "/healthz", nil)
	if buf.Len() != 0 {
		t.Fatalf("проверка живости засоряет лог на уровне info: %q", buf.String())
	}
}

func TestPanicBecomes500AndIsLogged(t *testing.T) {
	var buf bytes.Buffer
	log := testLogger(&buf)
	mux := http.NewServeMux()
	mux.HandleFunc("/boom", func(http.ResponseWriter, *http.Request) { panic("сломалось") })
	h := wrap(mux, log)
	rec := do(h, http.MethodGet, "/boom", nil)
	var e respond.Error
	if rec.Code != http.StatusInternalServerError || json.Unmarshal(rec.Body.Bytes(), &e) != nil || e.Code != "internal" {
		t.Fatalf("код %d, тело %q", rec.Code, rec.Body.String())
	}
	out := buf.String()
	if !strings.Contains(out, "паника в обработчике") || !strings.Contains(out, "сломалось") || !strings.Contains(out, `"status":500`) {
		t.Fatalf("в логе нет паники или кода 500: %s", out)
	}
	if strings.Contains(rec.Body.String(), "сломалось") {
		t.Fatal("текст паники не должен уходить клиенту")
	}
}
