// Package middleware — обёртки http.Handler, общие для всех маршрутов: идентификатор запроса,
// журнал запросов, перехват паник.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
)

// HeaderRequestID — заголовок с идентификатором запроса: приходит от прокси или создаётся здесь.
const HeaderRequestID = "X-Request-ID"

type ctxKey struct{}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// RequestID кладёт идентификатор запроса в контекст и в заголовок ответа. Пришедший от клиента
// идентификатор принимается, только если он короткий и состоит из безопасных символов
// (его пишут в логи, поэтому управляющие символы и длинные строки отбрасываются).
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !validRequestID.MatchString(id) {
			id = newRequestID()
		}
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

// IDFrom возвращает идентификатор запроса из контекста; вне запроса — пустая строка.
func IDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand не возвращает ошибку на поддерживаемых платформах
	return hex.EncodeToString(b[:])
}
