// Command server — HTTP-сервер сайта подготовки к КТ.
//
//	server              запустить сервер (настройки — переменные окружения, см. internal/config)
//	server healthcheck  проверить, что запущенный сервер отвечает на /healthz (для HEALTHCHECK в Docker)
//	server migrate up|down|status  миграции схемы базы из DATABASE_URL: применить новые, откатить последнюю, показать
//	server seed [каталог]  загрузить учебный контент из seed (по умолчанию content/seed, формат — docs/seed.md)
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi"
	"github.com/DarkIL1ght/M094Diploma/internal/adapters/memstore"
	"github.com/DarkIL1ght/M094Diploma/internal/adapters/postgres"
	"github.com/DarkIL1ght/M094Diploma/internal/app/session"
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
		switch args[0] {
		case "healthcheck":
			if err := httpapi.CheckHealth(cfg.Addr); err != nil {
				fmt.Fprintf(stderr, "healthcheck: %v\n", err)
				return 1
			}
			return 0
		case "migrate":
			if cfg.DatabaseURL == "" {
				fmt.Fprintln(stderr, "migrate: задайте DATABASE_URL")
				return 2
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := migrate(ctx, cfg.DatabaseURL, args[1:], stdout); err != nil {
				fmt.Fprintln(stderr, err)
				if errors.Is(err, errMigrateUsage) {
					return 2
				}
				return 1
			}
			return 0
		case "seed":
			if cfg.DatabaseURL == "" {
				fmt.Fprintln(stderr, "seed: задайте DATABASE_URL")
				return 2
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := runSeed(ctx, cfg.DatabaseURL, args[1:], stdout); err != nil {
				fmt.Fprintln(stderr, err)
				if errors.Is(err, errSeedUsage) {
					return 2
				}
				return 1
			}
			return 0
		}
		fmt.Fprintf(stderr, "неизвестная команда %q (есть healthcheck, migrate и seed)\n", args[0])
		return 2
	}

	log := logging.New(stdout, cfg.LogLevel, cfg.LogFormat)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Хранилище: Postgres, если задан DATABASE_URL, иначе память процесса (make run, тесты, CI без базы).
	var store session.Store
	if cfg.DatabaseURL != "" {
		pool, err := postgres.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Error("не удалось подключиться к базе", "error", err)
			return 1
		}
		defer pool.Close()
		if err := checkSchema(ctx, pool); err != nil {
			log.Error("сервер не запущен", "error", err)
			return 1
		}
		store = postgres.NewSessionStore(pool)
	} else {
		log.Warn("DATABASE_URL не задан: сессии хранятся в памяти процесса и пропадут после перезапуска")
		store = memstore.New()
	}

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Error("не удалось занять адрес", "addr", cfg.Addr, "error", err)
		return 1
	}
	log.Info("сервер запущен", "listen", ln.Addr().String(), "config", cfg)

	sessions := session.New(store)
	// Раз в час удаляем просроченное и пустые сессии (каждый вход без cookie создаёт сессию).
	go sessions.RunCleanup(ctx, time.Hour, func(deleted int, err error) {
		if err != nil {
			log.Error("очистка сессий не удалась", "error", err)
		} else if deleted > 0 {
			log.Info("удалены брошенные сессии", "count", deleted)
		}
	})

	srv := httpapi.NewServer(httpapi.NewRouter(log, httpapi.Deps{Sessions: sessions, TrustProxy: cfg.TrustProxy}))
	if err := httpapi.Serve(ctx, srv, ln, cfg.ShutdownTimeout, log); err != nil {
		log.Error("сервер остановился с ошибкой", "error", err)
		return 1
	}
	return 0
}
