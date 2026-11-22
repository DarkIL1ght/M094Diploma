// Package pgtest — база для тестов хранилища. NewPool даёт тесту свою пустую схему с применёнными миграциями
// и удаляет её после теста, поэтому тесты можно гонять параллельно и на общей базе. База — TEST_DATABASE_URL
// (в CI есть, локально `make up` и `make test`); без неё тест пропускается.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/DarkIL1ght/M094Diploma/migrations"
)

// NewPool возвращает пул соединений к новой схеме с применёнными миграциями.
func NewPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool := NewEmptyPool(t)
	db := stdlib.OpenDBFromPool(pool) // закрытие *sql.DB пул не закрывает
	defer db.Close()
	p, err := migrations.NewProvider(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(context.Background()); err != nil {
		t.Fatalf("миграции: %v", err)
	}
	return pool
}

// NewEmptyPool — то же, но без миграций: для проверок того, как код ведёт себя на пустой или старой схеме.
func NewEmptyPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(context.Background()) })

	var b [6]byte
	_, _ = rand.Read(b[:])
	schema := "pgtest_" + hex.EncodeToString(b[:])
	if _, err := admin.Exec(ctx, "create schema "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "drop schema "+schema+" cascade"); err != nil {
			t.Errorf("не удалось удалить схему %s: %v", schema, err)
		}
	})

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
