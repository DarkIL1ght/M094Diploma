package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/postgres"
	"github.com/DarkIL1ght/M094Diploma/internal/adapters/postgres/pgtest"
	"github.com/DarkIL1ght/M094Diploma/internal/app/session"
	"github.com/DarkIL1ght/M094Diploma/internal/app/session/sessiontest"
)

// Тот же набор проверок, что у memstore: порт ведёт себя одинаково в памяти и в базе.
func TestSessionStoreContract(t *testing.T) {
	sessiontest.Run(t, func(t *testing.T) session.Store { return postgres.NewSessionStore(pgtest.NewPool(t)) })
}

var t0 = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)

func newEmpty(t *testing.T, pool *pgxpool.Pool, st *postgres.SessionStore, id string) {
	t.Helper()
	if err := st.CreateSession(context.Background(), session.Session{ID: id, DisplayName: "Тихий Лис", CreatedAt: t0}, session.HashToken(id)); err != nil {
		t.Fatal(err)
	}
}

// В базе есть то, чего нет в memstore: настройка onboarded и таблицы прогресса. Сессия, в которой есть хоть что-то
// из этого, пустой не считается, даже если токеном не пользовались после выдачи (визит продлевается только через час).
func TestDeleteUnusedKeepsSessionsWithProgress(t *testing.T) {
	pool := pgtest.NewPool(t)
	st := postgres.NewSessionStore(pool)
	ctx := context.Background()
	marks := map[string]string{
		"onboarded":   `update sessions set onboarded = true where id = $1`,
		"progress":    `insert into progress (session_id, xp) values ($1, 10)`,
		"xp_daily":    `insert into xp_daily (session_id, day, xp) values ($1, '2026-11-17', 10)`,
		"test":        `insert into test_attempts (session_id, mode, title, started_at) values ($1, 'mixed', 'Тест', now())`,
		"kt":          `insert into kt_attempts (session_id, track, started_at, deadline_at, max_points) values ($1, 'profile', now(), now() + interval '1 hour', 70)`,
		"plan":        `insert into plan_tasks (session_id, day, position, kind, title) values ($1, '2026-11-17', 1, 'lesson', 'Урок')`,
		"ai_chat":     `insert into ai_chats (session_id, title, created_at, updated_at) values ($1, 'Чат', now(), now())`,
		"achievement": `insert into achievements (session_id, achievement_id, level, unlocked_at, updated_at) values ($1, 'first', 1, now(), now())`,
	}
	newEmpty(t, pool, st, "empty")
	for id, sql := range marks {
		newEmpty(t, pool, st, id)
		if _, err := pool.Exec(ctx, sql, id); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	// задания дня создаёт сервер сам: прогрессом они не считаются
	newEmpty(t, pool, st, "daily_only")
	if _, err := pool.Exec(ctx, `insert into daily_tasks (session_id, day, position, kind, target, xp) values ($1, '2026-11-17', 1, 'solve_questions', 10, 20)`, "daily_only"); err != nil {
		t.Fatal(err)
	}

	n, err := st.DeleteUnused(ctx, t0.Add(24*time.Hour))
	if err != nil || n != 2 {
		t.Fatalf("должны удалиться две пустые сессии (empty и daily_only), удалено %d, %v", n, err)
	}
	for id := range marks {
		if _, err := st.SessionByToken(ctx, session.HashToken(id)); err != nil {
			t.Errorf("сессия с %s должна остаться: %v", id, err)
		}
	}
	var dailyLeft int
	_ = pool.QueryRow(ctx, `select count(*) from daily_tasks`).Scan(&dailyLeft)
	if dailyLeft != 0 {
		t.Fatal("задания дня удалённой сессии уходят каскадом")
	}
}

func TestDeletingSessionCascadesToUserData(t *testing.T) {
	pool := pgtest.NewPool(t)
	st := postgres.NewSessionStore(pool)
	ctx := context.Background()
	newEmpty(t, pool, st, "gone")
	if _, err := pool.Exec(ctx, `insert into progress (session_id, xp) values ('gone', 5)`); err != nil {
		t.Fatal(err)
	}
	// токен просрочен, ключа нет: DeleteExpired удаляет сессию, прогресс уходит каскадом
	if n, err := st.DeleteExpired(ctx, t0.Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("удалено %d, %v", n, err)
	}
	var left int
	if err := pool.QueryRow(ctx, `select count(*) from progress`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("прогресс удалённой сессии должен уйти каскадом: осталось %d, %v", left, err)
	}
}

func TestTimesComeBackInUTC(t *testing.T) {
	pool := pgtest.NewPool(t)
	st := postgres.NewSessionStore(pool)
	ctx := context.Background()
	almaty := time.FixedZone("Алматы", 5*3600)
	created := time.Date(2026, 11, 17, 14, 0, 0, 123456789, almaty) // наносекунды база округлит до микросекунд
	if err := st.CreateSession(ctx, session.Session{ID: "tz", DisplayName: "Ник", CreatedAt: created}, session.HashToken("tz")); err != nil {
		t.Fatal(err)
	}
	got, err := st.SessionByToken(ctx, session.HashToken("tz"))
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedAt.Location() != time.UTC || got.CreatedAt.Sub(created).Abs() >= time.Microsecond {
		t.Fatalf("время: %v (%v), записано %v", got.CreatedAt, got.CreatedAt.Location(), created)
	}
	if !got.TokenLastSeen.Equal(got.CreatedAt) {
		t.Fatalf("новый токен: последний визит = создание, получили %v и %v", got.TokenLastSeen, got.CreatedAt)
	}
}

func TestOpenRejectsBadURLWithoutLeakingPassword(t *testing.T) {
	_, err := postgres.Open(context.Background(), "postgres://u:секрет@::bad")
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if strings.Contains(err.Error(), "секрет") {
		t.Fatalf("пароль из строки подключения попал в ошибку: %v", err)
	}
}
