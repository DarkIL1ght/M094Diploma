package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/postgres/pgtest"
)

func TestSeedNeedsDatabaseAndOneDir(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"seed"}, func(string) string { return "" }, &out, &errb); code != 2 ||
		!strings.Contains(errb.String(), "DATABASE_URL") {
		t.Fatalf("без DATABASE_URL: код %d, stderr %q", code, errb.String())
	}
	errb.Reset()
	env := func(k string) string {
		if k == "DATABASE_URL" {
			return "postgres://localhost:1/none"
		}
		return ""
	}
	if code := run([]string{"seed", "a", "b"}, env, &out, &errb); code != 2 || !strings.Contains(errb.String(), "seed [каталог]") {
		t.Fatalf("два каталога: код %d, stderr %q", code, errb.String())
	}
}

// Пример из репозитория проходит проверку и загружается; повторная загрузка ничего не меняет.
func TestSeedLoadsRepositoryExample(t *testing.T) {
	pool := pgtest.NewPool(t)
	ctx := context.Background()
	var out bytes.Buffer
	if err := seed(ctx, pool, "../../content/seed", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "вопросы: добавлено") {
		t.Errorf("отчёт: %s", out.String())
	}
	out.Reset()
	if err := seed(ctx, pool, "../../content/seed", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "изменений нет") {
		t.Errorf("повторная загрузка: %s", out.String())
	}
}
