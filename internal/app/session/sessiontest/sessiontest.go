// Package sessiontest — общий набор проверок для реализаций session.Store.
// Подключается из тестов адаптера: sessiontest.Run(t, func(t *testing.T) session.Store { ... }).
package sessiontest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DarkIL1ght/M094Diploma/internal/app/session"
)

// Run проверяет контракт хранилища. newStore возвращает пустое хранилище для каждого вложенного теста.
func Run(t *testing.T, newStore func(t *testing.T) session.Store) {
	ctx := context.Background()
	t0 := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	mk := func(id string) session.Session {
		return session.Session{ID: id, DisplayName: "Быстрый Барс", CreatedAt: t0}
	}

	t.Run("создание и поиск по токену", func(t *testing.T) {
		st := newStore(t)
		tok := session.HashToken("t1")
		if err := st.CreateSession(ctx, mk("s1"), tok); err != nil {
			t.Fatal(err)
		}
		got, err := st.SessionByToken(ctx, tok)
		if err != nil || got.ID != "s1" || got.DisplayName != "Быстрый Барс" || !got.TokenLastSeen.Equal(t0) {
			t.Fatalf("получили %+v, %v", got, err)
		}
		if _, err := st.SessionByToken(ctx, session.HashToken("other")); !errors.Is(err, session.ErrNotFound) {
			t.Fatalf("неизвестный токен: %v", err)
		}
	})

	t.Run("второй токен и обновление визита", func(t *testing.T) {
		st := newStore(t)
		a, b := session.HashToken("a"), session.HashToken("b")
		_ = st.CreateSession(ctx, mk("s1"), a)
		if err := st.AddToken(ctx, "s1", b, t0.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		got, err := st.SessionByToken(ctx, b)
		if err != nil || got.ID != "s1" || !got.TokenLastSeen.Equal(t0.Add(time.Hour)) {
			t.Fatalf("второй токен: %+v, %v", got, err)
		}
		if err := st.TouchToken(ctx, a, t0.Add(2*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if got, _ := st.SessionByToken(ctx, a); !got.TokenLastSeen.Equal(t0.Add(2 * time.Hour)) {
			t.Fatalf("визит не обновился: %v", got.TokenLastSeen)
		}
		if got, _ := st.SessionByToken(ctx, b); !got.TokenLastSeen.Equal(t0.Add(time.Hour)) {
			t.Fatal("обновление одного токена задело другой")
		}
		if err := st.AddToken(ctx, "нет-такой", session.HashToken("c"), t0); !errors.Is(err, session.ErrNotFound) {
			t.Fatalf("токен к несуществующей сессии: %v", err)
		}
		if err := st.TouchToken(ctx, session.HashToken("нет"), t0); !errors.Is(err, session.ErrNotFound) {
			t.Fatalf("визит неизвестного токена: %v", err)
		}
	})

	t.Run("ключ восстановления заменяется", func(t *testing.T) {
		st := newStore(t)
		_ = st.CreateSession(ctx, mk("s1"), session.HashToken("a"))
		_ = st.CreateSession(ctx, mk("s2"), session.HashToken("b"))
		k1, _ := session.HashKey("0000-0000-0000-0001")
		k2, _ := session.HashKey("0000-0000-0000-0002")
		if err := st.SetRecoveryKey(ctx, "s1", k1, t0); err != nil {
			t.Fatal(err)
		}
		got, err := st.SessionByRecoveryKey(ctx, k1)
		if err != nil || got.ID != "s1" || got.RecoveryIssuedAt == nil || !got.RecoveryIssuedAt.Equal(t0) {
			t.Fatalf("по ключу: %+v, %v", got, err)
		}
		if err := st.SetRecoveryKey(ctx, "s1", k2, t0.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if _, err := st.SessionByRecoveryKey(ctx, k1); !errors.Is(err, session.ErrNotFound) {
			t.Fatalf("старый ключ должен перестать работать: %v", err)
		}
		if got, err := st.SessionByRecoveryKey(ctx, k2); err != nil || got.ID != "s1" {
			t.Fatalf("новый ключ: %+v, %v", got, err)
		}
		if err := st.SetRecoveryKey(ctx, "нет-такой", k1, t0); !errors.Is(err, session.ErrNotFound) {
			t.Fatalf("ключ несуществующей сессии: %v", err)
		}
		s1, _ := st.SessionByToken(ctx, session.HashToken("a"))
		if s1.RecoveryIssuedAt == nil || !s1.RecoveryIssuedAt.Equal(t0.Add(time.Minute)) {
			t.Fatalf("момент выпуска не сохранился: %v", s1.RecoveryIssuedAt)
		}
		s2, _ := st.SessionByToken(ctx, session.HashToken("b"))
		if s2.RecoveryIssuedAt != nil {
			t.Fatal("у другой сессии ключа нет")
		}
	})
}
