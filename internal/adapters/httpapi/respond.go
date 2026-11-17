package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
)

// errorBody — тело ошибки по контракту (схема Error в api/openapi.yaml).
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON отдаёт значение как JSON. Ошибка записи — это обрыв у клиента, обработчику делать с ней нечего.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError отдаёт ошибку в формате контракта. Коды — из перечисления Error.code.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Code: code, Message: message})
}

// Methods выбирает обработчик по методу запроса. Лишний метод получает 405 в формате контракта
// и заголовок Allow (стандартный ServeMux отвечает на него текстом, а не JSON). HEAD обслуживается
// обработчиком GET, как в net/http. Маршруты регистрируют без метода в шаблоне:
//
//	mux.Handle("/api/me", Methods{http.MethodGet: getMe, http.MethodPatch: ...})
type Methods map[string]http.Handler

func (m Methods) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	if method == http.MethodHead {
		if _, ok := m[http.MethodHead]; !ok {
			method = http.MethodGet
		}
	}
	if h, ok := m[method]; ok {
		h.ServeHTTP(w, r)
		return
	}
	allowed := make([]string, 0, len(m)+1)
	for k := range m {
		allowed = append(allowed, k)
	}
	if _, ok := m[http.MethodGet]; ok && !slices.Contains(allowed, http.MethodHead) {
		allowed = append(allowed, http.MethodHead)
	}
	slices.Sort(allowed)
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Метод не поддерживается")
}
