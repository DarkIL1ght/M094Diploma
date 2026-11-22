// Package postgres — хранилище на Postgres через pgx (B-04). Пул соединений создаёт Open, реализации портов
// приложения (пока session.Store) лежат рядом. Схема — migrations/ (S-03), описание таблиц — docs/db.md.
// Тесты на настоящей базе: пакет pgtest даёт каждому тесту свою пустую схему с применёнными миграциями.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Open подключается к базе по строке dsn и проверяет соединение. Параметры пула (pool_max_conns и другие)
// можно задать прямо в строке подключения; по умолчанию pgx берёт max(4, число CPU) соединений.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL: %w", err) // сама строка с паролем в ошибку не попадает
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("база недоступна: %w", err)
	}
	return pool, nil
}

// Коды ошибок Postgres, которые хранилище переводит в ошибки приложения.
const (
	codeForeignKeyViolation = "23503"
)

func isCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
