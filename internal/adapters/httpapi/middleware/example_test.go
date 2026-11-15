package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi/middleware"
)

// Цепочка оборачивает весь роутер: заголовки — снаружи, чтобы попасть и на ответы 403 и 413.
func Example() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/answers", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "принято") })

	handler := middleware.SecurityHeaders(middleware.DefaultCSP)(
		middleware.CheckOrigin([]string{"https://m094.example"})(
			middleware.LimitBody(64 << 10)(mux)))

	req := httptest.NewRequest(http.MethodPost, "/api/answers", strings.NewReader("{}"))
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	fmt.Println(rec.Code, rec.Header().Get("X-Frame-Options"))
	// Output: 403 DENY
}
