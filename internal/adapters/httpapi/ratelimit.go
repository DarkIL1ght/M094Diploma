package httpapi

import (
	"sync"
	"time"
)

// Limiter — ограничитель частоты со скользящим окном: не больше limit событий за последние window на ключ
// (обычно адрес клиента). Для каждого ключа хранится журнал меток времени, поэтому всплесков на границе
// окна, как у фиксированного окна, нет. Живёт в памяти процесса; при нескольких экземплярах сервера
// счётчики не общие.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	hits   map[string][]hit
	seq    uint64
	calls  int
}

type hit struct {
	at time.Time
	id uint64 // чтобы Reserve мог вернуть именно своё место
}

// NewLimiter создаёт ограничитель.
func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, now: time.Now, hits: map[string][]hit{}}
}

// prune убирает устаревшие записи ключа и возвращает оставшиеся. Вызывать под замком.
func (l *Limiter) prune(key string, now time.Time) []hit {
	l.calls++
	if l.calls%1024 == 0 { // изредка чистим и чужие ключи, чтобы карта не росла бесконечно
		for k := range l.hits {
			l.pruneKey(k, now)
		}
	}
	return l.pruneKey(key, now)
}

func (l *Limiter) pruneKey(key string, now time.Time) []hit {
	hs := l.hits[key]
	i := 0
	for i < len(hs) && now.Sub(hs[i].at) >= l.window {
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

// Reserve атомарно занимает место: проверка лимита и запись события идут под одним замком, поэтому
// параллельные запросы не могут пройти все разом, пока ни один ещё не записан. release отдаёт место
// обратно (например, когда ключ подошёл и попытка не считается неудачной); повторный вызов ничего не делает.
// Если лимит исчерпан, ok=false, а retry — через сколько освободится место.
func (l *Limiter) Reserve(key string) (release func(), retry time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	hs := l.prune(key, now)
	if len(hs) >= l.limit {
		return func() {}, hs[0].at.Add(l.window).Sub(now), false
	}
	l.seq++
	id := l.seq
	l.hits[key] = append(hs, hit{at: now, id: id})
	var once sync.Once
	return func() { once.Do(func() { l.release(key, id) }) }, 0, true
}

func (l *Limiter) release(key string, id uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	hs := l.hits[key]
	for i := range hs {
		if hs[i].id == id {
			hs = append(hs[:i], hs[i+1:]...)
			break
		}
	}
	if len(hs) == 0 {
		delete(l.hits, key)
		return
	}
	l.hits[key] = hs
}

// Take занимает место без возврата: для событий, которые считаются всегда.
func (l *Limiter) Take(key string) (time.Duration, bool) {
	_, retry, ok := l.Reserve(key)
	return retry, ok
}
