// Package middleware — защитные обёртки HTTP: заголовки безопасности (CSP и др.), проверка Origin
// для изменяющих запросов и лимит размера тела.
//
// Подключаются цепочкой вокруг всего роутера; SecurityHeaders — снаружи, чтобы заголовки были
// и на ответах 403 и 413 от внутренних обёрток:
//
//	handler := middleware.SecurityHeaders(middleware.DefaultCSP)(
//		middleware.CheckOrigin([]string{"https://m094.example"})(
//			middleware.LimitBody(64 << 10)(mux)))
package middleware

import (
	"net/http"
	"strings"
)

// DefaultCSP — политика для сайта без сборки: всё только со своего домена, картинки ещё и data:,
// встраивание во фреймы и плагины запрещены.
const DefaultCSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'; object-src 'none'"

// SecurityHeaders ставит защитные заголовки на каждый ответ. csp — Content-Security-Policy,
// пусто — DefaultCSP.
func SecurityHeaders(csp string) func(http.Handler) http.Handler {
	if csp == "" {
		csp = DefaultCSP
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", csp)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			next.ServeHTTP(w, r)
		})
	}
}

// CheckOrigin пропускает изменяющие запросы (всё, кроме GET, HEAD и OPTIONS) только со своих сайтов,
// чтобы чужая страница не могла действовать от имени пользователя (CSRF).
// Origin должен быть в allowed — список нужно передать целиком, включая адрес самого сайта:
// браузер шлёт Origin и в запросах со своей страницы. Без Origin решает Sec-Fetch-Site: same-origin
// и none (адресная строка, закладка) — можно, остальное и отсутствие заголовка — 403.
func CheckOrigin(allowed []string) func(http.Handler) http.Handler {
	trusted := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		trusted[normalizeOrigin(o)] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !safeMethod(r.Method) && !sameSite(r, trusted) {
				http.Error(w, "запрос с чужого сайта", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func sameSite(r *http.Request, trusted map[string]bool) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return trusted[normalizeOrigin(origin)]
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	}
	return false
}

// normalizeOrigin приводит origin к виду, в котором его шлёт браузер: схема и хост в нижнем регистре, без «/».
func normalizeOrigin(o string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(o), "/"))
}

// LimitBody ограничивает тело запроса maxBytes байтами. Заявленный Content-Length больше лимита —
// сразу 413. Без длины (chunked) чтение сверх лимита вернёт обработчику *http.MaxBytesError —
// на неё он отвечает 413.
func LimitBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				http.Error(w, "слишком большой запрос", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
