// Package httpapi — HTTP-слой: маршрутизация, общие обёртки и запуск сервера.
// Контракт методов — api/openapi.yaml; обработчики подключаются сюда по мере появления сценариев.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi/middleware"
)

// NewRouter собирает обработчик сервера. Порядок обёрток снаружи внутрь: идентификатор запроса,
// журнал, перехват паник. Паника становится 500 внутри журнала, поэтому в нём виден настоящий код.
func NewRouter(log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/healthz", Methods{http.MethodGet: http.HandlerFunc(healthz)})
	mux.HandleFunc("/", notFound) // всё, что не нашлось: JSON по контракту вместо текста

	return wrap(mux, log)
}

// wrap надевает общие обёртки на обработчик.
func wrap(h http.Handler, log *slog.Logger) http.Handler {
	h = middleware.Recover(log)(h)
	h = middleware.AccessLog(log, "/healthz")(h)
	return middleware.RequestID(h)
}

// healthz — проверка живости для Docker, мониторинга и CI: процесс принимает запросы.
// Готовность базы появится вместе со слоем хранилища; здесь она не проверяется намеренно:
// упавшая база не должна приводить к перезапуску самого сервера.
func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "Не найдено")
}
