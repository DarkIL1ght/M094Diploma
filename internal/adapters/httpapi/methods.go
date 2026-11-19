package httpapi

import (
	"net/http"
	"slices"
	"strings"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi/respond"
)

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
	respond.Err(w, http.StatusMethodNotAllowed, "method_not_allowed", "Метод не поддерживается")
}
