package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"

	_ "github.com/jackc/pgx/v5/stdlib" // драйвер pgx для database/sql
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/DarkIL1ght/M094Diploma/migrations"
)

// errMigrateUsage — неверные аргументы подкоманды migrate (код выхода 2).
var errMigrateUsage = errors.New("migrate: укажите up, down или status")

// migrate выполняет подкоманду `server migrate up|down|status` над базой dsn:
// up — применить все новые миграции, down — откатить последнюю, status — показать, что применено.
func migrate(ctx context.Context, dsn string, args []string, stdout io.Writer) error {
	if len(args) != 1 || (args[0] != "up" && args[0] != "down" && args[0] != "status") {
		return errMigrateUsage
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	// Блокировка в базе: два экземпляра сервера, запущенные одновременно, не применят миграции дважды.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	p, err := migrations.NewProvider(db, goose.WithSessionLocker(locker))
	if err != nil {
		return err
	}

	switch args[0] {
	case "up":
		res, err := p.Up(ctx)
		for _, r := range res {
			fmt.Fprintln(stdout, r)
		}
		if err == nil && len(res) == 0 {
			fmt.Fprintln(stdout, "новых миграций нет")
		}
		return err
	case "down":
		r, err := p.Down(ctx)
		if r != nil {
			fmt.Fprintln(stdout, r)
		}
		return err
	default:
		st, err := p.Status(ctx)
		for _, s := range st {
			applied := "не применена"
			if s.State == goose.StateApplied {
				applied = "применена " + s.AppliedAt.Format("2006-01-02 15:04:05Z07:00")
			}
			fmt.Fprintf(stdout, "%-28s %s\n", s.Source.Path, applied)
		}
		return err
	}
}
