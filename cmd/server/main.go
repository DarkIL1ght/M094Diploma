// Command server — HTTP-сервер сайта подготовки к КТ.
//
//	server              запустить сервер (настройки — переменные окружения, см. internal/config)
//	server healthcheck  проверить, что запущенный сервер отвечает на /healthz (для HEALTHCHECK в Docker)
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi"
	"github.com/DarkIL1ght/M094Diploma/internal/config"
	"github.com/DarkIL1ght/M094Diploma/internal/logging"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run возвращает код выхода процесса. Логи идут в stdout (так их собирает Docker), ошибки запуска — в stderr.
func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "настройки: %v\n", err)
		return 2
	}
	if len(args) > 0 {
		if args[0] == "healthcheck" {
			if err := httpapi.CheckHealth(cfg.Addr); err != nil {
				fmt.Fprintf(stderr, "healthcheck: %v\n", err)
				return 1
			}
			return 0
		}
		fmt.Fprintf(stderr, "неизвестная команда %q (есть только healthcheck)\n", args[0])
		return 2
	}

	log := logging.New(stdout, cfg.LogLevel, cfg.LogFormat)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Error("не удалось занять адрес", "addr", cfg.Addr, "error", err)
		return 1
	}
	log.Info("сервер запущен", "listen", ln.Addr().String(), "config", cfg)

	srv := httpapi.NewServer(httpapi.NewRouter(log))
	if err := httpapi.Serve(ctx, srv, ln, cfg.ShutdownTimeout, log); err != nil {
		log.Error("сервер остановился с ошибкой", "error", err)
		return 1
	}
	return 0
}
