// Package memstore — хранилище сессий в памяти процесса. Нужно тестам и запуску без базы
// (DATABASE_URL пуст): после перезапуска сервера всё пропадает. Postgres-реализация — в слое хранилища (B-04).
package memstore

import (
	"context"
	"sync"
	"time"

	"github.com/DarkIL1ght/M094Diploma/internal/app/session"
)

type tokenRec struct {
	sessionID string
	lastSeen  time.Time
}

// Store — session.Store в памяти.
type Store struct {
	mu       sync.Mutex
	sessions map[string]*session.Session
	tokens   map[session.TokenHash]tokenRec
	keys     map[session.KeyHash]string // хэш ключа → id сессии
	keyOf    map[string]session.KeyHash // id сессии → её действующий хэш ключа
	hasKey   map[string]bool
}

// New создаёт пустое хранилище.
func New() *Store {
	return &Store{
		sessions: map[string]*session.Session{},
		tokens:   map[session.TokenHash]tokenRec{},
		keys:     map[session.KeyHash]string{},
		keyOf:    map[string]session.KeyHash{},
		hasKey:   map[string]bool{},
	}
}

var _ session.Store = (*Store)(nil)

func (m *Store) CreateSession(_ context.Context, s session.Session, token session.TokenHash) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := s
	m.sessions[s.ID] = &cp
	m.tokens[token] = tokenRec{sessionID: s.ID, lastSeen: s.CreatedAt}
	return nil
}

func (m *Store) SessionByToken(_ context.Context, token session.TokenHash) (session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.tokens[token]
	if !ok {
		return session.Session{}, session.ErrNotFound
	}
	s := *m.sessions[rec.sessionID]
	s.TokenLastSeen = rec.lastSeen
	return s, nil
}

func (m *Store) AddToken(_ context.Context, sessionID string, token session.TokenHash, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[sessionID]; !ok {
		return session.ErrNotFound
	}
	m.tokens[token] = tokenRec{sessionID: sessionID, lastSeen: now}
	return nil
}

func (m *Store) TouchToken(_ context.Context, token session.TokenHash, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.tokens[token]
	if !ok {
		return session.ErrNotFound
	}
	rec.lastSeen = now
	m.tokens[token] = rec
	return nil
}

func (m *Store) SetRecoveryKey(_ context.Context, sessionID string, key session.KeyHash, issued time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return session.ErrNotFound
	}
	if m.hasKey[sessionID] {
		delete(m.keys, m.keyOf[sessionID]) // прежний ключ перестаёт работать
	}
	m.keys[key] = sessionID
	m.keyOf[sessionID] = key
	m.hasKey[sessionID] = true
	t := issued
	s.RecoveryIssuedAt = &t
	return nil
}

func (m *Store) SessionByRecoveryKey(_ context.Context, key session.KeyHash) (session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.keys[key]
	if !ok {
		return session.Session{}, session.ErrNotFound
	}
	return *m.sessions[id], nil
}

// TokenHashes возвращает все хранимые хэши токенов: для проверки в тестах, что открытых токенов в хранилище нет.
func (m *Store) TokenHashes() []session.TokenHash {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]session.TokenHash, 0, len(m.tokens))
	for h := range m.tokens {
		out = append(out, h)
	}
	return out
}
