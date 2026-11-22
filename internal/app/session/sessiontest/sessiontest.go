// Package sessiontest — общий набор проверок для реализаций session.Store.
// Подключается из тестов адаптера: sessiontest.Run(t, func(t *testing.T) session.Store { ... }).
package sessiontest

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

	t.Run("очистка просроченных", func(t *testing.T) {
		st := newStore(t)
		// s1: единственный токен просрочен, но есть ключ; s2: просрочен один из двух; s3: свежий токен;
		// s4: единственный токен просрочен, ключа нет
		_ = st.CreateSession(ctx, mk("s1"), session.HashToken("s1-old"))
		k1, _ := session.HashKey("0000-0000-0000-0001")
		_ = st.SetRecoveryKey(ctx, "s1", k1, t0)
		_ = st.CreateSession(ctx, mk("s2"), session.HashToken("s2-old"))
		_ = st.AddToken(ctx, "s2", session.HashToken("s2-new"), t0.Add(48*time.Hour))
		fresh := session.HashToken("fresh")
		_ = st.CreateSession(ctx, mk("s3"), fresh)
		_ = st.TouchToken(ctx, fresh, t0.Add(48*time.Hour))
		_ = st.CreateSession(ctx, mk("s4"), session.HashToken("s4-old"))

		n, err := st.DeleteExpired(ctx, t0.Add(24*time.Hour))
		if err != nil || n != 1 {
			t.Fatalf("должна удалиться одна сессия (s4), получили %d, %v", n, err)
		}
		for _, tok := range []string{"s1-old", "s2-old", "s4-old"} {
			if _, err := st.SessionByToken(ctx, session.HashToken(tok)); !errors.Is(err, session.ErrNotFound) {
				t.Fatalf("просроченный токен %s должен исчезнуть: %v", tok, err)
			}
		}
		if got, err := st.SessionByRecoveryKey(ctx, k1); err != nil || got.ID != "s1" {
			t.Fatalf("сессия с ключом остаётся, ключ работает: %+v, %v", got, err)
		}
		if err := st.AddToken(ctx, "s1", session.HashToken("s1-new"), t0.Add(72*time.Hour)); err != nil {
			t.Fatalf("к сессии с ключом можно подключить новый браузер: %v", err)
		}
		if got, err := st.SessionByToken(ctx, session.HashToken("s2-new")); err != nil || got.ID != "s2" {
			t.Fatalf("сессия с одним живым токеном остаётся: %+v, %v", got, err)
		}
		if _, err := st.SessionByToken(ctx, fresh); err != nil {
			t.Fatalf("свежая сессия: %v", err)
		}
		if err := st.AddToken(ctx, "s4", session.HashToken("s4-new"), t0.Add(72*time.Hour)); !errors.Is(err, session.ErrNotFound) {
			t.Fatalf("удалённой сессии больше нет: %v", err)
		}
		if n, err := st.DeleteExpired(ctx, t0.Add(24*time.Hour)); err != nil || n != 0 {
			t.Fatalf("повторная очистка ничего не удаляет: %d, %v", n, err)
		}
	})

	t.Run("очистка пустых", func(t *testing.T) {
		st := newStore(t)
		day := t0.Add(24 * time.Hour)
		// empty: создана давно, токеном ни разу не пользовались, ключа нет — удаляется
		_ = st.CreateSession(ctx, mk("empty"), session.HashToken("empty"))
		// used: токеном пользовались после выдачи
		_ = st.CreateSession(ctx, mk("used"), session.HashToken("used"))
		_ = st.TouchToken(ctx, session.HashToken("used"), t0.Add(2*time.Hour))
		// keyed: есть ключ восстановления
		_ = st.CreateSession(ctx, mk("keyed"), session.HashToken("keyed"))
		k, _ := session.HashKey("0000-0000-0000-0003")
		_ = st.SetRecoveryKey(ctx, "keyed", k, t0)
		// young: создана позже границы
		young := session.Session{ID: "young", DisplayName: "Тихий Лис", CreatedAt: day.Add(time.Minute)}
		_ = st.CreateSession(ctx, young, session.HashToken("young"))

		n, err := st.DeleteUnused(ctx, day)
		if err != nil || n != 1 {
			t.Fatalf("должна удалиться одна пустая сессия, получили %d, %v", n, err)
		}
		if _, err := st.SessionByToken(ctx, session.HashToken("empty")); !errors.Is(err, session.ErrNotFound) {
			t.Fatalf("пустая сессия и её токен удаляются: %v", err)
		}
		for _, tok := range []string{"used", "keyed", "young"} {
			if got, err := st.SessionByToken(ctx, session.HashToken(tok)); err != nil || got.ID != tok {
				t.Fatalf("сессия %s остаётся: %+v, %v", tok, got, err)
			}
		}
		if n, err := st.DeleteUnused(ctx, day); err != nil || n != 0 {
			t.Fatalf("повторная очистка ничего не удаляет: %d, %v", n, err)
		}
	})

	t.Run("параллельный доступ", func(t *testing.T) {
		st := newStore(t)
		const workers = 50
		first := session.HashToken("first")
		if err := st.CreateSession(ctx, mk("shared"), first); err != nil {
			t.Fatal(err)
		}
		tokens := make([]session.TokenHash, workers)
		keys := make([]session.KeyHash, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			tokens[i] = session.HashToken(fmt.Sprintf("tok-%d", i))
			keys[i], _ = session.HashKey(fmt.Sprintf("%016d", i))
			wg.Add(1)
			go func() {
				defer wg.Done()
				now := t0.Add(time.Duration(i) * time.Second)
				// собственные сессии создаются параллельно с работой над общей
				if err := st.CreateSession(ctx, mk(fmt.Sprintf("own-%d", i)), session.HashToken(fmt.Sprintf("own-tok-%d", i))); err != nil {
					t.Error(err)
				}
				if err := st.AddToken(ctx, "shared", tokens[i], now); err != nil {
					t.Error(err)
				}
				if err := st.TouchToken(ctx, tokens[i], now.Add(time.Minute)); err != nil {
					t.Error(err)
				}
				if err := st.SetRecoveryKey(ctx, "shared", keys[i], now); err != nil {
					t.Error(err)
				}
				if _, err := st.SessionByToken(ctx, first); err != nil {
					t.Error(err)
				}
				_, _ = st.SessionByRecoveryKey(ctx, keys[i])
			}()
		}
		wg.Wait()

		for i, tok := range tokens {
			if got, err := st.SessionByToken(ctx, tok); err != nil || got.ID != "shared" {
				t.Fatalf("токен %d не находит общую сессию: %+v, %v", i, got, err)
			}
		}
		valid := 0
		for _, k := range keys {
			if got, err := st.SessionByRecoveryKey(ctx, k); err == nil && got.ID == "shared" {
				valid++
			}
		}
		if valid != 1 {
			t.Fatalf("после гонки должен действовать ровно один ключ, действует %d", valid)
		}
		for i := 0; i < workers; i++ {
			if _, err := st.SessionByToken(ctx, session.HashToken(fmt.Sprintf("own-tok-%d", i))); err != nil {
				t.Fatalf("собственная сессия %d потерялась: %v", i, err)
			}
		}
	})
}
