// Package session — анонимные сессии без регистрации и перенос прогресса ключом восстановления.
//
// Браузер получает случайный токен (cookie sid). Сервер хранит только его SHA-256: утечка базы
// не даёт войти в чужую сессию. Ключ восстановления — отдельный секрет из 80 бит, он тоже лежит
// в базе только хэшем и показывается пользователю один раз. Хранилище — порт Store: реализация
// на Postgres появится вместе со слоем хранилища (S-03, B-04), для тестов и запуска без базы есть
// adapters/memstore.
package session

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"time"
)

var (
	// ErrNotFound — токена или ключа нет, либо срок токена истёк. Причины не различаются намеренно.
	ErrNotFound = errors.New("сессия не найдена")
	// ErrInvalidKey — ключ восстановления не подошёл: неверный формат или такого ключа нет.
	ErrInvalidKey = errors.New("ключ восстановления не подошёл")
)

const (
	// DefaultTTL — сколько токен живёт без обращений. Каждый вход (GET /api/me) продлевает срок.
	DefaultTTL = 180 * 24 * time.Hour
	// touchEvery — как часто обновлять «последний визит», чтобы не писать в базу на каждый запрос.
	touchEvery = time.Hour
)

// Session — анонимный пользователь.
type Session struct {
	ID          string
	DisplayName string // случайный ник для лиги недели
	CreatedAt   time.Time
	// RecoveryIssuedAt — когда выпущен действующий ключ восстановления; nil — ключа нет.
	RecoveryIssuedAt *time.Time
	// TokenLastSeen — последний визит с этим токеном (до текущего запроса). Заполняет Store.SessionByToken.
	TokenLastSeen time.Time
}

// Store — то, что сервису нужно от хранилища. Реализации обязаны проходить sessiontest.Run.
type Store interface {
	// CreateSession создаёт сессию вместе с первым токеном одной операцией.
	CreateSession(ctx context.Context, s Session, token TokenHash) error
	// SessionByToken находит сессию по хэшу токена; нет такого — ErrNotFound.
	SessionByToken(ctx context.Context, token TokenHash) (Session, error)
	// AddToken привязывает ещё один токен (ещё один браузер) к сессии.
	AddToken(ctx context.Context, sessionID string, token TokenHash, now time.Time) error
	// TouchToken обновляет последний визит токена.
	TouchToken(ctx context.Context, token TokenHash, now time.Time) error
	// SetRecoveryKey заменяет ключ восстановления сессии: прежний перестаёт работать.
	SetRecoveryKey(ctx context.Context, sessionID string, key KeyHash, issued time.Time) error
	// SessionByRecoveryKey находит сессию по хэшу ключа; нет такого — ErrNotFound.
	SessionByRecoveryKey(ctx context.Context, key KeyHash) (Session, error)
}

// Service — сценарии работы с сессиями.
type Service struct {
	store Store
	now   func() time.Time
	rand  io.Reader
	ttl   time.Duration
}

// Option настраивает Service (в основном для тестов).
type Option func(*Service)

// WithClock подменяет часы.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// WithRand подменяет источник случайности.
func WithRand(r io.Reader) Option { return func(s *Service) { s.rand = r } }

// WithTTL меняет срок жизни токена без обращений.
func WithTTL(d time.Duration) Option { return func(s *Service) { s.ttl = d } }

// New создаёт сервис поверх хранилища.
func New(store Store, opts ...Option) *Service {
	s := &Service{store: store, now: time.Now, rand: rand.Reader, ttl: DefaultTTL}
	for _, o := range opts {
		o(s)
	}
	return s
}

// TTL — срок жизни токена; по нему HTTP-слой ставит Max-Age cookie.
func (s *Service) TTL() time.Duration { return s.ttl }

// Start создаёт новую сессию и возвращает её с токеном для cookie. Токен нигде, кроме ответа, не хранится.
func (s *Service) Start(ctx context.Context) (Session, string, error) {
	id, err := randomHex(s.rand, 16)
	if err != nil {
		return Session{}, "", err
	}
	name, err := displayName(s.rand)
	if err != nil {
		return Session{}, "", err
	}
	token, err := newToken(s.rand)
	if err != nil {
		return Session{}, "", err
	}
	sess := Session{ID: id, DisplayName: name, CreatedAt: s.now().UTC()}
	if err := s.store.CreateSession(ctx, sess, HashToken(token)); err != nil {
		return Session{}, "", err
	}
	return sess, token, nil
}

// Resolve находит сессию по токену из cookie. Неизвестный, плохо оформленный и просроченный токен
// дают ErrNotFound. Последний визит обновляется не чаще раза в час.
func (s *Service) Resolve(ctx context.Context, token string) (Session, error) {
	if !validToken(token) {
		return Session{}, ErrNotFound
	}
	h := HashToken(token)
	sess, err := s.store.SessionByToken(ctx, h)
	if err != nil {
		return Session{}, err
	}
	now := s.now()
	if now.Sub(sess.TokenLastSeen) > s.ttl {
		return Session{}, ErrNotFound
	}
	if now.Sub(sess.TokenLastSeen) > touchEvery {
		if err := s.store.TouchToken(ctx, h, now.UTC()); err != nil {
			return Session{}, err
		}
	}
	return sess, nil
}

// IssueRecoveryKey выпускает новый ключ восстановления; прежний перестаёт работать.
// Ключ возвращается открытым один раз, в хранилище уходит только его хэш.
func (s *Service) IssueRecoveryKey(ctx context.Context, sessionID string) (string, time.Time, error) {
	key, err := newRecoveryKey(s.rand)
	if err != nil {
		return "", time.Time{}, err
	}
	issued := s.now().UTC()
	h, _ := HashKey(key)
	if err := s.store.SetRecoveryKey(ctx, sessionID, h, issued); err != nil {
		return "", time.Time{}, err
	}
	return key, issued, nil
}

// Restore привязывает новый браузер к сессии, для которой выпущен ключ: возвращает сессию и новый токен.
// Ключ при этом не сгорает: им можно подключить ещё одно устройство, пока не выпущен новый.
func (s *Service) Restore(ctx context.Context, key string) (Session, string, error) {
	h, ok := HashKey(key)
	if !ok {
		return Session{}, "", ErrInvalidKey
	}
	sess, err := s.store.SessionByRecoveryKey(ctx, h)
	if errors.Is(err, ErrNotFound) {
		return Session{}, "", ErrInvalidKey
	}
	if err != nil {
		return Session{}, "", err
	}
	token, err := newToken(s.rand)
	if err != nil {
		return Session{}, "", err
	}
	if err := s.store.AddToken(ctx, sess.ID, HashToken(token), s.now().UTC()); err != nil {
		return Session{}, "", err
	}
	return sess, token, nil
}
