package httpapi

import (
	"sync"
	"time"
)

// Limiter — ограничитель частоты с фиксированным окном: не больше limit событий за window на ключ
// (обычно IP-адрес). Живёт в памяти процесса; при нескольких экземплярах сервера счётчики не общие.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	hits   map[string][]time.Time
	calls  int
}

// NewLimiter создаёт ограничитель.
func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, now: time.Now, hits: map[string][]time.Time{}}
}

// prune убирает устаревшие записи ключа и возвращает оставшиеся. Вызывать под замком.
func (l *Limiter) prune(key string, now time.Time) []time.Time {
	l.calls++
	if l.calls%1024 == 0 { // изредка чистим и чужие ключи, чтобы карта не росла бесконечно
		for k := range l.hits {
			l.pruneKey(k, now)
		}
	}
	return l.pruneKey(key, now)
}

func (l *Limiter) pruneKey(key string, now time.Time) []time.Time {
	hs := l.hits[key]
	i := 0
	for i < len(hs) && now.Sub(hs[i]) >= l.window {
		i++
	}
	hs = hs[i:]
	if len(hs) == 0 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = hs
	return hs
}

// retryAfter — через сколько освободится место, если лимит исчерпан.
func (l *Limiter) retryAfter(hs []time.Time, now time.Time) time.Duration {
	return hs[0].Add(l.window).Sub(now)
}

// Blocked сообщает, исчерпан ли лимит, и через сколько он освободится. Событие не записывает.
func (l *Limiter) Blocked(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	hs := l.prune(key, now)
	if len(hs) >= l.limit {
		return l.retryAfter(hs, now), true
	}
	return 0, false
}

// Hit записывает событие.
func (l *Limiter) Hit(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.prune(key, now)
	l.hits[key] = append(l.hits[key], now)
}

// Take записывает событие, если лимит не исчерпан, иначе возвращает, через сколько повторить.
func (l *Limiter) Take(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	hs := l.prune(key, now)
	if len(hs) >= l.limit {
		return l.retryAfter(hs, now), false
	}
	l.hits[key] = append(hs, now)
	return 0, true
}
