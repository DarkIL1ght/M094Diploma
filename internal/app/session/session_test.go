package session_test

import (
	"context"
	"errors"
	"math/rand"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/memstore"
	"github.com/DarkIL1ght/M094Diploma/internal/app/session"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newService(t *testing.T) (*session.Service, *memstore.Store, *clock) {
	t.Helper()
	st := memstore.New()
	c := &clock{t: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
	// детерминированная случайность: тесты воспроизводимы, но токены всё равно разные
	svc := session.New(st, session.WithClock(c.now), session.WithRand(rand.New(rand.NewSource(1))))
	return svc, st, c
}

func TestStartAndResolve(t *testing.T) {
	svc, st, _ := newService(t)
	ctx := context.Background()
	sess, token, err := svc.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 || sess.ID == "" || !strings.Contains(sess.DisplayName, " ") {
		t.Fatalf("неожиданная сессия: %+v, токен %q", sess, token)
	}
	got, err := svc.Resolve(ctx, token)
	if err != nil || got.ID != sess.ID {
		t.Fatalf("Resolve: %+v, %v", got, err)
	}
	if h := session.HashToken(token); len(st.TokenHashes()) != 1 || st.TokenHashes()[0] != h {
		t.Fatal("в хранилище должен быть ровно хэш токена")
	}
}

func TestResolveRejectsGarbageAndUnknown(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	for _, tok := range []string{"", "короткий", strings.Repeat("a", 43), strings.Repeat("!", 43), strings.Repeat("A", 44)} {
		if _, err := svc.Resolve(ctx, tok); !errors.Is(err, session.ErrNotFound) {
			t.Errorf("токен %q: %v", tok, err)
		}
	}
}

func TestTokenExpiresAfterTTLWithoutVisits(t *testing.T) {
	svc, _, c := newService(t)
	ctx := context.Background()
	_, token, _ := svc.Start(ctx)

	c.t = c.t.Add(session.DefaultTTL - time.Hour)
	if _, err := svc.Resolve(ctx, token); err != nil {
		t.Fatalf("до истечения срока: %v", err)
	}
	// визит продлил срок: ещё почти полный срок можно не заходить
	c.t = c.t.Add(session.DefaultTTL - time.Hour)
	if _, err := svc.Resolve(ctx, token); err != nil {
		t.Fatalf("после продления: %v", err)
	}
	c.t = c.t.Add(session.DefaultTTL + time.Hour)
	if _, err := svc.Resolve(ctx, token); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("после истечения срока: %v", err)
	}
}

func TestRecoveryKeyFormat(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	sess, _, _ := svc.Start(ctx)
	seen := map[string]bool{}
	re := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{4}(-[0-9A-HJKMNP-TV-Z]{4}){3}$`)
	for i := 0; i < 50; i++ {
		key, issued, err := svc.IssueRecoveryKey(ctx, sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !re.MatchString(key) || seen[key] || issued.IsZero() {
			t.Fatalf("плохой ключ %q (повтор: %v)", key, seen[key])
		}
		seen[key] = true
	}
}

func TestRestoreMovesProgressToAnotherBrowser(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	sessA, tokenA, _ := svc.Start(ctx)
	key, _, err := svc.IssueRecoveryKey(ctx, sessA.ID)
	if err != nil {
		t.Fatal(err)
	}

	got, tokenB, err := svc.Restore(ctx, key)
	if err != nil || got.ID != sessA.ID {
		t.Fatalf("Restore: %+v, %v", got, err)
	}
	if tokenB == tokenA {
		t.Fatal("новый браузер должен получить свой токен")
	}
	for _, tok := range []string{tokenA, tokenB} {
		if s, err := svc.Resolve(ctx, tok); err != nil || s.ID != sessA.ID {
			t.Fatalf("токен %q должен вести в одну сессию: %+v, %v", tok[:6], s, err)
		}
	}
	// ключ не сгорает: можно подключить третье устройство
	if _, tokenC, err := svc.Restore(ctx, key); err != nil || tokenC == tokenB {
		t.Fatalf("повторное использование ключа: %v", err)
	}
}

func TestRestoreAcceptsHandwrittenKey(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	sess, _, _ := svc.Start(ctx)
	key, _, _ := svc.IssueRecoveryKey(ctx, sess.ID)
	variants := []string{
		strings.ToLower(key),
		strings.ReplaceAll(key, "-", ""),
		strings.ReplaceAll(key, "-", " "),
		"  " + key + "\n",
	}
	for _, v := range variants {
		if _, _, err := svc.Restore(ctx, v); err != nil {
			t.Errorf("ключ %q: %v", v, err)
		}
	}
	// 0 и O, 1 и I/L путаются при переписывании: ключ с подменой всё равно подходит
	confusable := strings.NewReplacer("0", "O", "1", "I").Replace(key)
	if _, _, err := svc.Restore(ctx, confusable); err != nil {
		t.Errorf("ключ с заменой похожих символов %q: %v", confusable, err)
	}
}

func TestRestoreRejectsWrongKeys(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	sess, _, _ := svc.Start(ctx)
	key, _, _ := svc.IssueRecoveryKey(ctx, sess.ID)
	other := "0000-0000-0000-0000"
	if key == other {
		other = "1111-1111-1111-1111"
	}
	for _, bad := range []string{"", "abc", other, key + "X", key[:len(key)-1], "ЖЖЖЖ-ЖЖЖЖ-ЖЖЖЖ-ЖЖЖЖ", "UUUU-UUUU-UUUU-UUUU"} {
		if _, _, err := svc.Restore(ctx, bad); !errors.Is(err, session.ErrInvalidKey) {
			t.Errorf("ключ %q: ожидали ErrInvalidKey, получили %v", bad, err)
		}
	}
}

func TestNewKeyInvalidatesOldOne(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	sess, _, _ := svc.Start(ctx)
	oldKey, _, _ := svc.IssueRecoveryKey(ctx, sess.ID)
	newKey, _, _ := svc.IssueRecoveryKey(ctx, sess.ID)
	if _, _, err := svc.Restore(ctx, oldKey); !errors.Is(err, session.ErrInvalidKey) {
		t.Fatalf("старый ключ: %v", err)
	}
	if _, _, err := svc.Restore(ctx, newKey); err != nil {
		t.Fatalf("новый ключ: %v", err)
	}
}

func TestIssueKeyForUnknownSession(t *testing.T) {
	svc, _, _ := newService(t)
	if _, _, err := svc.IssueRecoveryKey(context.Background(), "нет-такой"); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("ожидали ErrNotFound, получили %v", err)
	}
}

func TestDisplayNamesVary(t *testing.T) {
	st := memstore.New()
	svc := session.New(st)
	names := map[string]bool{}
	for i := 0; i < 40; i++ {
		s, _, err := svc.Start(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[s.DisplayName] = true
	}
	if len(names) < 10 {
		t.Fatalf("ники почти не меняются: %d разных из 40", len(names))
	}
}

func TestCleanupRemovesEmptySessionsAfterADay(t *testing.T) {
	svc, st, c := newService(t)
	ctx := context.Background()
	_, emptyTok, _ := svc.Start(ctx)
	used, usedTok, _ := svc.Start(ctx)

	c.t = c.t.Add(2 * time.Hour)
	if _, err := svc.Resolve(ctx, usedTok); err != nil { // визит позже часа продлевает токен
		t.Fatal(err)
	}
	c.t = c.t.Add(session.UnusedTTL - 3*time.Hour)
	if n, err := svc.Cleanup(ctx); err != nil || n != 0 {
		t.Fatalf("сутки ещё не прошли, удалять нечего: %d, %v", n, err)
	}
	c.t = c.t.Add(2 * time.Hour)
	if n, err := svc.Cleanup(ctx); err != nil || n != 1 {
		t.Fatalf("через сутки удаляется одна пустая сессия: %d, %v", n, err)
	}
	if _, err := svc.Resolve(ctx, emptyTok); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("пустая сессия: %v", err)
	}
	if got, err := svc.Resolve(ctx, usedTok); err != nil || got.ID != used.ID {
		t.Fatalf("сессия, которой пользовались, остаётся: %+v, %v", got, err)
	}
	if len(st.TokenHashes()) != 1 {
		t.Fatalf("в хранилище должен остаться один токен, осталось %d", len(st.TokenHashes()))
	}
}

func TestCleanupRemovesAbandonedSessionsButKeepsKeyedOnes(t *testing.T) {
	svc, _, c := newService(t)
	ctx := context.Background()
	_, abandonedTok, _ := svc.Start(ctx)
	keyed, keyedTok, _ := svc.Start(ctx)
	key, _, err := svc.IssueRecoveryKey(ctx, keyed.ID)
	if err != nil {
		t.Fatal(err)
	}
	c.t = c.t.Add(2 * time.Hour)
	for _, tok := range []string{abandonedTok, keyedTok} { // обеими пользовались, пустыми они не считаются
		if _, err := svc.Resolve(ctx, tok); err != nil {
			t.Fatal(err)
		}
	}

	c.t = c.t.Add(session.DefaultTTL + time.Hour) // семь месяцев без визитов
	n, err := svc.Cleanup(ctx)
	if err != nil || n != 1 {
		t.Fatalf("удаляется только брошенная сессия без ключа: %d, %v", n, err)
	}
	if _, err := svc.Resolve(ctx, abandonedTok); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("брошенная сессия: %v", err)
	}
	if _, err := svc.Resolve(ctx, keyedTok); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("старый токен сессии с ключом тоже истёк: %v", err)
	}
	got, _, err := svc.Restore(ctx, key)
	if err != nil || got.ID != keyed.ID {
		t.Fatalf("по ключу можно вернуться и через семь месяцев: %+v, %v", got, err)
	}
}

// storeLosingSession имитирует очистку, удалившую сессию между SessionByRecoveryKey и AddToken.
type storeLosingSession struct{ session.Store }

func (storeLosingSession) AddToken(context.Context, string, session.TokenHash, time.Time) error {
	return session.ErrNotFound
}

func TestRestoreRacingCleanupIsInvalidKeyNotInternalError(t *testing.T) {
	st := memstore.New()
	svc := session.New(st)
	ctx := context.Background()
	sess, _, _ := svc.Start(ctx)
	key, _, _ := svc.IssueRecoveryKey(ctx, sess.ID)

	racing := session.New(storeLosingSession{Store: st})
	if _, _, err := racing.Restore(ctx, key); !errors.Is(err, session.ErrInvalidKey) {
		t.Fatalf("ожидали ErrInvalidKey (клиент получит 404, а не 500), получили %v", err)
	}
}

func TestRunCleanupTicksAndStopsOnContext(t *testing.T) {
	svc, _, _ := newService(t)
	ctx, cancel := context.WithCancel(context.Background())
	reports := make(chan error, 8)
	done := make(chan struct{})
	go func() {
		svc.RunCleanup(ctx, 5*time.Millisecond, func(_ int, err error) { reports <- err })
		close(done)
	}()
	for i := 0; i < 2; i++ {
		select {
		case err := <-reports:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("очистка не запускается по таймеру")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunCleanup не остановился по отмене контекста")
	}
}
