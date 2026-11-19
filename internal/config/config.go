// Package config читает настройки сервера из переменных окружения.
// Значения по умолчанию подходят для локальной разработки: без единой переменной сервер стартует.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Config — настройки процесса.
type Config struct {
	// Addr — адрес, который слушает сервер (ADDR). Формат net.Listen: ":8080", "127.0.0.1:8080".
	Addr string
	// DatabaseURL — строка подключения к Postgres (DATABASE_URL). Пусто — сервер работает без базы:
	// до слоя хранилища (S-03, B-04) она ему не нужна. В логи не попадает.
	DatabaseURL string
	// LogLevel — уровень логов (LOG_LEVEL): debug, info, warn, error.
	LogLevel slog.Level
	// LogFormat — формат логов (LOG_FORMAT): json для сервера, text для чтения глазами.
	LogFormat string
	// TrustProxy — доверять заголовкам X-Forwarded-For и X-Forwarded-Proto обратного прокси (TRUST_PROXY=true).
	// Включать, только когда сервер не доступен напрямую, иначе адрес клиента можно подделать.
	TrustProxy bool
	// ShutdownTimeout — сколько ждать завершения запросов при остановке (SHUTDOWN_TIMEOUT, например 10s).
	ShutdownTimeout time.Duration
}

// Load читает настройки через getenv (обычно os.Getenv). Все ошибки собираются вместе,
// чтобы неверный запуск исправлялся за один раз.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:            ":8080",
		DatabaseURL:     strings.TrimSpace(getenv("DATABASE_URL")),
		LogLevel:        slog.LevelInfo,
		LogFormat:       "json",
		ShutdownTimeout: 10 * time.Second,
	}
	var errs []error
	if v := strings.TrimSpace(getenv("ADDR")); v != "" {
		cfg.Addr = v
	}
	if v := strings.TrimSpace(getenv("LOG_LEVEL")); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			errs = append(errs, fmt.Errorf("LOG_LEVEL=%q: допустимо debug, info, warn, error", v))
		}
	}
	if v := strings.ToLower(strings.TrimSpace(getenv("LOG_FORMAT"))); v != "" {
		if v != "json" && v != "text" {
			errs = append(errs, fmt.Errorf("LOG_FORMAT=%q: допустимо json или text", v))
		} else {
			cfg.LogFormat = v
		}
	}
	if v := strings.TrimSpace(getenv("TRUST_PROXY")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("TRUST_PROXY=%q: допустимо true или false", v))
		}
		cfg.TrustProxy = b
	}
	if v := strings.TrimSpace(getenv("SHUTDOWN_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT=%q: нужна положительная длительность, например 10s", v))
		} else {
			cfg.ShutdownTimeout = d
		}
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LogValue для slog: показывает настройки без секретов (строку подключения к базе скрывает).
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("addr", c.Addr),
		slog.Bool("database", c.DatabaseURL != ""),
		slog.String("log_level", c.LogLevel.String()),
		slog.String("log_format", c.LogFormat),
		slog.Bool("trust_proxy", c.TrustProxy),
		slog.String("shutdown_timeout", c.ShutdownTimeout.String()),
	)
}
