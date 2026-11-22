package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DarkIL1ght/M094Diploma/internal/app/session"
)

// SessionStore — session.Store на таблицах sessions и session_tokens (migrations/00001_sessions.sql).
// Postgres хранит время с точностью до микросекунды, поэтому прочитанное время может отличаться от записанного
// на наносекунды; приложение сравнивает время только с точностью до часов.
type SessionStore struct {
	pool *pgxpool.Pool
}

// NewSessionStore создаёт хранилище сессий поверх пула.
func NewSessionStore(pool *pgxpool.Pool) *SessionStore { return &SessionStore{pool: pool} }

var _ session.Store = (*SessionStore)(nil)

func (s *SessionStore) CreateSession(ctx context.Context, sess session.Session, token session.TokenHash) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`insert into sessions (id, display_name, created_at) values ($1, $2, $3)`,
			sess.ID, sess.DisplayName, sess.CreatedAt); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`insert into session_tokens (token_hash, session_id, created_at, last_seen_at) values ($1, $2, $3, $3)`,
			token[:], sess.ID, sess.CreatedAt)
		return err
	})
}

func (s *SessionStore) SessionByToken(ctx context.Context, token session.TokenHash) (session.Session, error) {
	var sess session.Session
	err := s.pool.QueryRow(ctx, `
		select s.id, s.display_name, s.created_at, s.recovery_issued_at, t.last_seen_at
		from session_tokens t join sessions s on s.id = t.session_id
		where t.token_hash = $1`, token[:]).
		Scan(&sess.ID, &sess.DisplayName, &sess.CreatedAt, &sess.RecoveryIssuedAt, &sess.TokenLastSeen)
	if isNoRows(err) {
		return session.Session{}, session.ErrNotFound
	}
	return utc(sess), err
}

func (s *SessionStore) AddToken(ctx context.Context, sessionID string, token session.TokenHash, now time.Time) error {
	_, err := s.pool.Exec(ctx,
		`insert into session_tokens (token_hash, session_id, created_at, last_seen_at) values ($1, $2, $3, $3)`,
		token[:], sessionID, now)
	if isCode(err, codeForeignKeyViolation) {
		return session.ErrNotFound // сессии нет (или очистка удалила её только что)
	}
	return err
}

func (s *SessionStore) TouchToken(ctx context.Context, token session.TokenHash, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `update session_tokens set last_seen_at = $2 where token_hash = $1`, token[:], now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return session.ErrNotFound
	}
	return nil
}

func (s *SessionStore) SetRecoveryKey(ctx context.Context, sessionID string, key session.KeyHash, issued time.Time) error {
	// Новый ключ затирает прежний: у сессии одна пара полей, поэтому старый ключ перестаёт работать сразу.
	tag, err := s.pool.Exec(ctx,
		`update sessions set recovery_key_hash = $2, recovery_issued_at = $3 where id = $1`,
		sessionID, key[:], issued)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return session.ErrNotFound
	}
	return nil
}

func (s *SessionStore) SessionByRecoveryKey(ctx context.Context, key session.KeyHash) (session.Session, error) {
	var sess session.Session
	err := s.pool.QueryRow(ctx,
		`select id, display_name, created_at, recovery_issued_at from sessions where recovery_key_hash = $1`, key[:]).
		Scan(&sess.ID, &sess.DisplayName, &sess.CreatedAt, &sess.RecoveryIssuedAt)
	if isNoRows(err) {
		return session.Session{}, session.ErrNotFound
	}
	return utc(sess), err
}

func (s *SessionStore) DeleteExpired(ctx context.Context, before time.Time) (int, error) {
	var deleted int
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `delete from session_tokens where last_seen_at < $1`, before); err != nil {
			return err
		}
		// Второй запрос видит удаление первого (одна транзакция). Сессия с ключом остаётся: ключ действует бессрочно.
		// Всё остальное пользователя (прогресс, тесты, чаты) уходит каскадом.
		tag, err := tx.Exec(ctx, `
			delete from sessions s
			where s.recovery_key_hash is null
			  and not exists (select 1 from session_tokens t where t.session_id = s.id)`)
		deleted = int(tag.RowsAffected())
		return err
	})
	return deleted, err
}

// DeleteUnused удаляет пустые сессии: ключа нет, первый вход не пройден, ни одним токеном не пользовались после
// выдачи и нет ни одной строки прогресса. Последнее важно: визит продлевается только через час, и студент,
// занимавшийся меньше часа, выглядел бы по токену нетронутым. Задания дня (daily_tasks) прогрессом не считаются:
// их создаёт сам сервер при первом запросе дня.
func (s *SessionStore) DeleteUnused(ctx context.Context, createdBefore time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		delete from sessions s
		where s.created_at < $1
		  and s.recovery_key_hash is null
		  and not s.onboarded
		  and not exists (select 1 from session_tokens t where t.session_id = s.id and t.last_seen_at <> t.created_at)
		  and not exists (select 1 from progress p where p.session_id = s.id)
		  and not exists (select 1 from xp_daily x where x.session_id = s.id)
		  and not exists (select 1 from section_marks m where m.session_id = s.id)
		  and not exists (select 1 from question_progress q where q.session_id = s.id)
		  and not exists (select 1 from test_attempts a where a.session_id = s.id)
		  and not exists (select 1 from kt_attempts k where k.session_id = s.id)
		  and not exists (select 1 from achievements a where a.session_id = s.id)
		  and not exists (select 1 from cosmetic_unlocks c where c.session_id = s.id)
		  and not exists (select 1 from daily_chests d where d.session_id = s.id)
		  and not exists (select 1 from plan_tasks p where p.session_id = s.id)
		  and not exists (select 1 from ai_chats c where c.session_id = s.id)`, createdBefore)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// utc приводит время к UTC: pgx отдаёт timestamptz в локальном часовом поясе процесса.
func utc(s session.Session) session.Session {
	s.CreatedAt = s.CreatedAt.UTC()
	s.TokenLastSeen = s.TokenLastSeen.UTC()
	if s.RecoveryIssuedAt != nil {
		t := s.RecoveryIssuedAt.UTC()
		s.RecoveryIssuedAt = &t
	}
	return s
}
