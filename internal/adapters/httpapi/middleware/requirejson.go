package middleware

import (
	"mime"
	"net/http"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi/respond"
)

// RequireJSON пропускает только запросы с Content-Type: application/json, иначе отвечает 415.
// Подключать к изменяющим маршрутам, которые читают JSON из тела.
//
// Зачем: HTML-форма с enctype="text/plain" с чужого сайта умеет отправить тело, которое является
// валидным JSON, без предварительного запроса (preflight) и с cookie жертвы в рамках top-level
// навигации. Тип application/json форма отправить не может: для него браузер требует CORS preflight.
// Так закрывается подделка запросов (CSRF), пока нет общей проверки Origin (B-17).
func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
			respond.Err(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Ожидается Content-Type: application/json")
			return
		}
		next.ServeHTTP(w, r)
	})
}
