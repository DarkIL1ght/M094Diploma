package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/postgres/pgtest"
)

func TestCheckSchemaRefusesPendingMigrations(t *testing.T) {
	ctx := context.Background()
	empty := pgtest.NewEmptyPool(t)
	err := checkSchema(ctx, empty)
	if err == nil || !strings.Contains(err.Error(), "migrate up") {
		t.Fatalf("на пустой схеме сервер не должен стартовать, получили %v", err)
	}
	if err := checkSchema(ctx, pgtest.NewPool(t)); err != nil {
		t.Fatalf("на схеме со всеми миграциями: %v", err)
	}
}

func TestRunFailsWhenDatabaseUnreachable(t *testing.T) {
	env := func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "postgres://u:секрет@127.0.0.1:1/db?sslmode=disable&connect_timeout=2"
		case "ADDR":
			return "127.0.0.1:0"
		}
		return ""
	}
	var out, errb bytes.Buffer
	if code := run(nil, env, &out, &errb); code != 1 || !strings.Contains(out.String(), "не удалось подключиться к базе") {
		t.Fatalf("код %d, лог %q", code, out.String())
	}
	if strings.Contains(out.String(), "секрет") {
		t.Fatalf("пароль из DATABASE_URL попал в лог: %s", out.String())
	}
	if strings.Contains(out.String(), "сервер запущен") {
		t.Fatal("без базы сервер не должен занимать порт")
	}
}
