package middleware_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi/middleware"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") })

func TestSecurityHeaders(t *testing.T) {
	notFound := http.HandlerFunc(http.NotFound)
	for name, tc := range map[string]struct {
		csp, wantCSP string
		next         http.Handler
	}{
		"политика по умолчанию": {"", middleware.DefaultCSP, ok},
		"своя политика":         {"default-src 'none'", "default-src 'none'", ok},
		"и на ответе с ошибкой": {"", middleware.DefaultCSP, notFound},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			middleware.SecurityHeaders(tc.csp)(tc.next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			want := map[string]string{
				"Content-Security-Policy": tc.wantCSP,
				"X-Content-Type-Options":  "nosniff",
				"Referrer-Policy":         "strict-origin-when-cross-origin",
				"X-Frame-Options":         "DENY",
				"Permissions-Policy":      "camera=(), microphone=(), geolocation=()",
			}
			for k, v := range want {
				if got := rec.Header().Get(k); got != v {
					t.Errorf("%s = %q, want %q", k, got, v)
				}
			}
		})
	}
}

func TestDefaultCSP(t *testing.T) {
	for _, d := range []string{"default-src 'self'", "script-src 'self'", "style-src 'self'", "img-src 'self' data:",
		"connect-src 'self'", "frame-ancestors 'none'", "base-uri 'self'", "form-action 'self'", "object-src 'none'"} {
		if !strings.Contains(middleware.DefaultCSP, d) {
			t.Errorf("в DefaultCSP нет %q", d)
		}
	}
}

func TestCheckOrigin(t *testing.T) {
	h := middleware.CheckOrigin([]string{"https://m094.example", "HTTP://localhost:8080/"})(ok)
	for name, tc := range map[string]struct {
		method, origin, fetchSite string
		want                      int
	}{
		"GET с чужого сайта":            {http.MethodGet, "https://evil.example", "cross-site", http.StatusOK},
		"HEAD с чужого сайта":           {http.MethodHead, "https://evil.example", "cross-site", http.StatusOK},
		"OPTIONS с чужого сайта":        {http.MethodOptions, "https://evil.example", "cross-site", http.StatusOK},
		"POST со своего сайта":          {http.MethodPost, "https://m094.example", "same-origin", http.StatusOK},
		"POST: регистр и слэш в списке": {http.MethodPost, "http://localhost:8080", "", http.StatusOK},
		"POST с чужого сайта":           {http.MethodPost, "https://evil.example", "cross-site", http.StatusForbidden},
		"чужой Origin важнее Sec-Fetch": {http.MethodPost, "https://evil.example", "same-origin", http.StatusForbidden},
		"POST с Origin: null":           {http.MethodPost, "null", "", http.StatusForbidden},
		"PUT с чужого сайта":            {http.MethodPut, "https://evil.example", "", http.StatusForbidden},
		"PATCH с чужого сайта":          {http.MethodPatch, "https://evil.example", "", http.StatusForbidden},
		"DELETE с чужого сайта":         {http.MethodDelete, "https://evil.example", "", http.StatusForbidden},
		"без Origin, same-origin":       {http.MethodPost, "", "same-origin", http.StatusOK},
		"без Origin, none":              {http.MethodPost, "", "none", http.StatusOK},
		"без Origin, cross-site":        {http.MethodPost, "", "cross-site", http.StatusForbidden},
		"без Origin, same-site":         {http.MethodPost, "", "same-site", http.StatusForbidden},
		"без Origin и Sec-Fetch-Site":   {http.MethodPost, "", "", http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/answers", strings.NewReader("{}"))
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("статус = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == http.StatusForbidden && rec.Body.String() == "ok" {
				t.Error("обработчик вызван для запрещённого запроса")
			}
		})
	}
}

// readAll — обработчик, читающий тело целиком: так поступит любой JSON-обработчик API.
// Превышение лимита он узнаёт по *http.MaxBytesError и отвечает 413.
var readAll = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "слишком большой запрос", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, "%d", len(body))
})

func TestLimitBody(t *testing.T) {
	const limit = 10
	h := middleware.LimitBody(limit)(readAll)
	for name, tc := range map[string]struct {
		body    string
		chunked bool // без Content-Length — лимит срабатывает только при чтении
		want    int
	}{
		"меньше лимита":                {"12345", false, http.StatusOK},
		"ровно лимит":                  {"1234567890", false, http.StatusOK},
		"больше лимита":                {"12345678901", false, http.StatusRequestEntityTooLarge},
		"больше лимита без длины":      {strings.Repeat("x", 100), true, http.StatusRequestEntityTooLarge},
		"меньше лимита без длины тела": {"12345", true, http.StatusOK},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			if tc.chunked {
				req.ContentLength = -1
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("статус = %d, want %d (тело ответа %q)", rec.Code, tc.want, rec.Body)
			}
			if tc.want == http.StatusOK && rec.Body.String() != fmt.Sprint(len(tc.body)) {
				t.Errorf("обработчик прочитал %s байт, want %d", rec.Body, len(tc.body))
			}
		})
	}
}

func TestLimitBodyRejectsByContentLength(t *testing.T) {
	// Заявленная длина больше лимита — 413 сразу, обработчик не вызывается.
	called := false
	h := middleware.LimitBody(10)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 11))))
	if rec.Code != http.StatusRequestEntityTooLarge || called {
		t.Fatalf("статус = %d, обработчик вызван: %v", rec.Code, called)
	}
}
