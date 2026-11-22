package postgres

import (
	"context"
	"slices"
	"testing"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/postgres/pgtest"
)

// Каждая таблица со ссылкой на sessions должна быть учтена в DeleteUnused: либо как прогресс, либо явно исключена.
// Тест падает, если новая миграция добавила таблицу пользователя, а список забыли обновить.
func TestEveryTableReferencingSessionsIsClassified(t *testing.T) {
	pool := pgtest.NewPool(t)
	rows, err := pool.Query(context.Background(), `
		select c.conrelid::regclass::text
		from pg_constraint c
		where c.contype = 'f' and c.confrelid = 'sessions'::regclass`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var referencing []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		referencing = append(referencing, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(referencing) == 0 {
		t.Fatal("не найдено ни одной таблицы со ссылкой на sessions: запрос сломан")
	}
	known := append(slices.Clone(progressTables), notProgressTables...)
	for _, table := range referencing {
		if !slices.Contains(known, table) {
			t.Errorf("таблица %s ссылается на sessions, но не учтена в DeleteUnused: добавьте её в progressTables "+
				"(строка в ней — прогресс) или в notProgressTables", table)
		}
	}
	for _, table := range known {
		if !slices.Contains(referencing, table) {
			t.Errorf("таблица %s есть в списках DeleteUnused, но не ссылается на sessions: уберите её из списка", table)
		}
	}
}
