package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// NewServer создаёт http.Server с безопасными таймаутами. WriteTimeout не задан намеренно:
// он оборвал бы долгие потоки ИИ (SSE). Срок записи обработчики при необходимости ставят сами
// через http.ResponseController.SetWriteDeadline.
func NewServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
}

// Serve принимает соединения на ln, пока ctx не отменён, потом останавливает сервер корректно:
// перестаёт принимать новые соединения и ждёт текущие запросы не дольше grace. Что не успело —
// обрывается. Возвращает nil при штатной остановке и ошибку, если сервер упал сам.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, grace time.Duration, log *slog.Logger) error {
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err // сервер остановился сам, до сигнала
	case <-ctx.Done():
	}

	log.Info("останавливаем сервер", "grace", grace.String())
	shutCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Warn("не все запросы завершились вовремя, обрываем", "error", err)
		_ = srv.Close()
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("сервер остановлен")
	return nil
}
