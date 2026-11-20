package migrations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Таблицы схемы после всех миграций. Новая миграция с таблицей — новая строка здесь.
var wantTables = []string{
	"achievements", "ai_chats", "ai_explanations", "ai_messages", "ai_usage_daily",
	"cosmetic_unlocks", "daily_chests", "daily_tasks", "kt_attempts", "kt_tasks",
	"notifications", "plan_tasks", "progress", "question_options", "question_progress",
	"questions", "section_marks", "sections", "session_tokens", "sessions",
	"shared_materials", "test_attempts", "test_questions", "topics", "xp_daily",
}

// testDB подключается к TEST_DATABASE_URL в отдельной пустой схеме и удаляет её после теста,
// поэтому тест можно гонять на общей базе и сколько угодно раз. Без TEST_DATABASE_URL тест пропускается.
func testDB(t *testing.T) *sql.DB {
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
	schema := "migrations_test_" + hex.EncodeToString(b[:])
	if _, err := admin.Exec(ctx, "create schema "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "drop schema "+schema+" cascade"); err != nil {
			t.Errorf("не удалось удалить схему %s: %v", schema, err)
		}
	})

	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { db.Close() })
	return db
}

// tables — таблицы текущей схемы, кроме служебной таблицы goose.
func tables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`select tablename from pg_tables
		where schemaname = current_schema() and tablename <> 'goose_db_version' order by tablename`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// Все миграции применяются вверх, откатываются до нуля без остатков и применяются снова:
// так проверяется, что каждый Down честно отменяет свой Up.
func TestUpDownUp(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	p, err := NewProvider(db)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("up: %v", err)
	}
	if got := tables(t, db); !slices.Equal(got, wantTables) {
		t.Fatalf("таблицы после up:\n got %v\nwant %v", got, wantTables)
	}

	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("down до нуля: %v", err)
	}
	if got := tables(t, db); len(got) != 0 {
		t.Fatalf("после down остались таблицы: %v", got)
	}
	var types int
	if err := db.QueryRow(`select count(*) from pg_type t join pg_namespace n on n.oid = t.typnamespace
		where n.nspname = current_schema() and t.typtype = 'd'`).Scan(&types); err != nil {
		t.Fatal(err)
	}
	if types != 0 {
		t.Fatalf("после down остались домены: %d", types)
	}

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("повторный up: %v", err)
	}
	if v, err := p.GetDBVersion(ctx); err != nil || v != int64(len(sources(t))) {
		t.Fatalf("версия после повторного up: %d, %v", v, err)
	}
}

func sources(t *testing.T) []string {
	t.Helper()
	entries, err := files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			out = append(out, e.Name())
		}
	}
	return out
}

// У каждого внешнего ключа есть индекс, который начинается с его первой колонки: без него каскад при удалении
// сессии и проверка restrict читают таблицу целиком. Частичный индекс не считается.
func TestForeignKeysIndexed(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	p, err := NewProvider(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(ctx, `select c.conrelid::regclass::text || '.' || a.attname
		from pg_constraint c
		join pg_attribute a on a.attrelid = c.conrelid and a.attnum = c.conkey[1]
		where c.contype = 'f' and c.connamespace = current_schema()::regnamespace
		  and not exists (select 1 from pg_index i
		                  where i.indrelid = c.conrelid and i.indkey[0] = c.conkey[1] and i.indpred is null)
		order by 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var fk string
		if err := rows.Scan(&fk); err != nil {
			t.Fatal(err)
		}
		t.Errorf("внешний ключ без индекса: %s", fk)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// Ограничения, на которые полагаются сценарии: без них ошибку в коде поймали бы только данные в проде.
func TestConstraints(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	p, err := NewProvider(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) error {
		_, err := db.ExecContext(ctx, q, args...)
		return err
	}
	must := func(q string, args ...any) {
		t.Helper()
		if err := exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustFail := func(why, q string, args ...any) {
		t.Helper()
		if err := exec(q, args...); err == nil {
			t.Errorf("%s: запрос прошёл, а не должен: %s", why, q)
		}
	}
	hash := func(b byte) []byte { return slices.Repeat([]byte{b}, 32) }

	must(`insert into sessions (id, display_name, created_at) values ('s1', 'Быстрый Барс', now())`)
	must(`insert into session_tokens values ($1, 's1', now(), now())`, hash(1))
	mustFail("ключ без даты выпуска", `update sessions set recovery_key_hash = $1 where id = 's1'`, hash(2))
	mustFail("хэш не той длины", `insert into session_tokens values ('\x01', 's1', now(), now())`)
	mustFail("неизвестная тема оформления", `update sessions set theme = 'neon' where id = 's1'`)

	must(`insert into topics (ext_id, subject, title, position, sum_hash) values ('topic-sorting', 'algorithms', 'Сортировки', 1, 'h')`)
	mustFail("предмет вопроса не совпадает с предметом темы",
		`insert into questions (ext_id, subject, topic_id, kind, text_md, explanation_md, sum_hash)
		 select 'q-1', 'databases', id, 'single', 'Вопрос', 'Пояснение', 'h' from topics`)
	mustFail("неизвестный предмет",
		`insert into questions (ext_id, subject, kind, text_md, explanation_md, sum_hash)
		 values ('q-2', 'physics', 'single', 'Вопрос', 'Пояснение', 'h')`)

	// незавершённый тест одного режима без темы — один, хотя topic_id у обоих NULL
	must(`insert into test_attempts (session_id, mode, title, started_at) values ('s1', 'mixed', 'Смешанный тест', now())`)
	mustFail("второй незавершённый смешанный тест",
		`insert into test_attempts (session_id, mode, title, started_at) values ('s1', 'mixed', 'Смешанный тест', now())`)
	mustFail("завершённый тест без итога", `update test_attempts set status = 'finished'`)

	must(`insert into kt_attempts (session_id, track, started_at, deadline_at, max_points)
		values ('s1', 'research', now(), now() + interval '235 minutes', 150)`)
	mustFail("вторая незавершённая попытка КТ",
		`insert into kt_attempts (session_id, track, started_at, deadline_at, max_points)
		 values ('s1', 'profile', now(), now() + interval '100 minutes', 70)`)

	// пересчёт плана переставляет задачи одним запросом: уникальность проверяется в конце транзакции
	must(`insert into plan_tasks (session_id, day, position, kind, title)
		values ('s1', '2027-01-10', 1, 'lesson', 'Урок'), ('s1', '2027-01-10', 2, 'test', 'Тест')`)
	must(`update plan_tasks set position = 3 - position`)

	// удаление сессии уносит всё, что ей принадлежит
	must(`delete from sessions where id = 's1'`)
	var left int
	if err := db.QueryRowContext(ctx, `select (select count(*) from session_tokens) + (select count(*) from test_attempts)
		+ (select count(*) from kt_attempts)`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("после удаления сессии осталось строк: %d", left)
	}
}
