package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || cfg.DatabaseURL != "" || cfg.LogLevel != slog.LevelInfo ||
		cfg.LogFormat != "json" || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("неожиданные значения по умолчанию: %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"ADDR":             " 127.0.0.1:9000 ",
		"DATABASE_URL":     "postgres://u:secret@db/x",
		"LOG_LEVEL":        "debug",
		"LOG_FORMAT":       "TEXT",
		"SHUTDOWN_TIMEOUT": "3s",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:9000" || cfg.DatabaseURL == "" || cfg.LogLevel != slog.LevelDebug ||
		cfg.LogFormat != "text" || cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("переопределение не применилось: %+v", cfg)
	}
}

func TestLoadReportsAllErrors(t *testing.T) {
	_, err := Load(env(map[string]string{"LOG_LEVEL": "loud", "LOG_FORMAT": "xml", "SHUTDOWN_TIMEOUT": "-1s"}))
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	for _, want := range []string{"LOG_LEVEL", "LOG_FORMAT", "SHUTDOWN_TIMEOUT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("в ошибке нет %s: %v", want, err)
		}
	}
}

func TestLogValueHidesDatabaseURL(t *testing.T) {
	cfg, _ := Load(env(map[string]string{"DATABASE_URL": "postgres://u:secret@db/x"}))
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("старт", "config", cfg)
	out := buf.String()
	if strings.Contains(out, "secret") || strings.Contains(out, "postgres://") {
		t.Fatalf("строка подключения попала в лог: %s", out)
	}
	if !strings.Contains(out, `"database":true`) {
		t.Fatalf("нет признака базы: %s", out)
	}
}
