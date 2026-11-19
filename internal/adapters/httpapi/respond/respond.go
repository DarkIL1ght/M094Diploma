// Package respond — запись ответов HTTP-слоя в формате контракта (api/openapi.yaml).
// Формат ошибки описан здесь один раз: его используют и обработчики, и общие обёртки (middleware).
package respond

import (
	"encoding/json"
	"net/http"
)

// Error — тело ошибки по контракту (схема Error в api/openapi.yaml).
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JSON отдаёт значение как JSON. Ошибка записи — это обрыв у клиента, обработчику делать с ней нечего.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Err отдаёт ошибку в формате контракта. Коды — из перечисления Error.code.
func Err(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, Error{Code: code, Message: message})
}
