package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/postgres"
	"github.com/DarkIL1ght/M094Diploma/internal/app/content"
)

// defaultSeedDir — каталог seed относительно рабочего каталога: в репозитории и в образе (/content/seed).
const defaultSeedDir = "content/seed"

// errSeedUsage — неверные аргументы подкоманды seed (код выхода 2).
var errSeedUsage = errors.New("seed: укажите не больше одного каталога: server seed [каталог]")

// runSeed выполняет подкоманду `server seed [каталог]` над базой dsn. Схема должна быть применена заранее.
func runSeed(ctx context.Context, dsn string, args []string, stdout io.Writer) error {
	if len(args) > 1 {
		return errSeedUsage
	}
	dir := defaultSeedDir
	if len(args) == 1 {
		dir = args[0]
	}
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := checkSchema(ctx, pool); err != nil {
		return err
	}
	return seed(ctx, pool, dir, stdout)
}

// seed загружает контент из каталога dir и печатает отчёт.
func seed(ctx context.Context, pool *pgxpool.Pool, dir string, stdout io.Writer) error {
	rep, err := content.Load(ctx, os.DirFS(dir), postgres.NewContentStore(pool))
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, rep)
	if !rep.Changed() {
		fmt.Fprintln(stdout, "изменений нет")
	}
	return nil
}
