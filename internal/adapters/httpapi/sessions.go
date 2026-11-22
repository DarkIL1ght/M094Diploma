package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi/middleware"
	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi/respond"
	"github.com/DarkIL1ght/M094Diploma/internal/app/session"
)

// CookieName — cookie анонимной сессии (см. api/openapi.yaml, securitySchemes).
const CookieName = "sid"

const (
	maxRestoreBody = 1 << 10 // тело запроса восстановления — один короткий ключ

	// Лимиты по адресу. Мобильные операторы и сети университетов отдают сотни и тысячи абонентов
	// через один IP, поэтому лимиты мягкие: они останавливают скрипт, а не людей.
	newSessionsMax = 300 // новых сессий с одного адреса в час
	restoreFails   = 10  // неверных ключей с одного адреса за restoreWindow
	restoreWindow  = 15 * time.Minute
	// Общий потолок неверных ключей на весь сервер: защита от подбора с множества адресов.
	restoreFailsAll = 1000 // за минуту
)

// Sessions — HTTP-слой анонимных сессий: cookie, вход, ключ восстановления.
// Другие маршруты оборачивают себя в Require, чтобы получить текущую сессию.
type Sessions struct {
	svc          *session.Service
	log          *slog.Logger
	trustProxy   bool
	newSessions  *Limiter
	restoreFails *Limiter // по адресу клиента
	restoreAll   *Limiter // на весь сервер
	warnProxy    sync.Once
}

// NewSessions создаёт HTTP-слой сессий. trustProxy — доверять заголовкам X-Forwarded-* от обратного прокси
// (Caddy): по ним определяется адрес клиента и HTTPS. Включать, только если сервер не доступен напрямую.
// За прокси без trustProxy все запросы выглядят как один адрес, а cookie ставится без Secure.
func NewSessions(svc *session.Service, log *slog.Logger, trustProxy bool) *Sessions {
	return &Sessions{
		svc: svc, log: log, trustProxy: trustProxy,
		newSessions:  NewLimiter(newSessionsMax, time.Hour),
		restoreFails: NewLimiter(restoreFails, restoreWindow),
		restoreAll:   NewLimiter(restoreFailsAll, time.Minute),
	}
}

// Routes регистрирует маршруты сессий.
func (a *Sessions) Routes(mux *http.ServeMux) {
	mux.Handle("/api/me", Methods{http.MethodGet: http.HandlerFunc(a.me)})
	mux.Handle("/api/session/recovery-key", Methods{http.MethodPost: a.Require(http.HandlerFunc(a.recoveryKey))})
	mux.Handle("/api/session/restore", Methods{http.MethodPost: middleware.RequireJSON(http.HandlerFunc(a.restore))})
}

type ctxKey struct{}

// SessionFrom возвращает сессию текущего запроса; работает внутри обработчиков под Require.
func SessionFrom(ctx context.Context) (session.Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(session.Session)
	return s, ok
}

// Require пропускает запрос дальше, только если в cookie действующая сессия; иначе 401 no_session.
// Клиент в ответ на 401 вызывает GET /api/me и повторяет запрос.
func (a *Sessions) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := a.fromCookie(r)
		switch {
		case err == nil:
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess)))
		case errors.Is(err, session.ErrNotFound):
			respond.Err(w, http.StatusUnauthorized, "no_session", "Сессия не найдена")
		default:
			a.internal(w, r, err)
		}
	})
}

func (a *Sessions) fromCookie(r *http.Request) (session.Session, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return session.Session{}, session.ErrNotFound
	}
	return a.svc.Resolve(r.Context(), c.Value)
}

// me: GET /api/me. Есть действующая cookie — отдаёт профиль и продлевает её, иначе создаёт новую сессию.
func (a *Sessions) me(w http.ResponseWriter, r *http.Request) {
	a.warnIfBehindUntrustedProxy(r)
	sess, err := a.fromCookie(r)
	if err == nil {
		c, _ := r.Cookie(CookieName)
		a.setCookie(w, r, c.Value)
		writeSecret(w, http.StatusOK, newMe(sess, false))
		return
	}
	if !errors.Is(err, session.ErrNotFound) {
		a.internal(w, r, err)
		return
	}
	if retry, ok := a.newSessions.Take(a.limitKey(r)); !ok {
		tooManyRequests(w, retry)
		return
	}
	sess, token, err := a.svc.Start(r.Context())
	if err != nil {
		a.internal(w, r, err)
		return
	}
	a.setCookie(w, r, token)
	writeSecret(w, http.StatusOK, newMe(sess, true))
}

// recoveryKey: POST /api/session/recovery-key. Новый ключ показывается один раз, прежний перестаёт работать.
func (a *Sessions) recoveryKey(w http.ResponseWriter, r *http.Request) {
	sess, _ := SessionFrom(r.Context())
	key, issued, err := a.svc.IssueRecoveryKey(r.Context(), sess.ID)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	writeSecret(w, http.StatusOK, map[string]any{"key": key, "issued_at": issued})
}

// restore: POST /api/session/restore. Подобрать 80 бит нельзя, но перебор не должен нагружать сервер:
// неверные ключи ограничены по адресу и на весь сервер. Место в лимите занимается ДО проверки ключа
// (Reserve атомарен), иначе параллельные запросы прошли бы все разом, пока ни один неудачный не записан;
// подошедший ключ и ошибки не по вине ключа место возвращают.
func (a *Sessions) restore(w http.ResponseWriter, r *http.Request) {
	relIP, retry, ok := a.restoreFails.Reserve(a.limitKey(r))
	if !ok {
		tooManyRequests(w, retry)
		return
	}
	relAll, retry, ok := a.restoreAll.Reserve("*")
	if !ok {
		relIP()
		tooManyRequests(w, retry)
		return
	}
	release := func() { relIP(); relAll() }

	r.Body = http.MaxBytesReader(w, r.Body, maxRestoreBody)
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		release() // это не неверный ключ
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			respond.Err(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Тело запроса слишком большое")
			return
		}
		respond.Err(w, http.StatusBadRequest, "bad_request", "Ожидается JSON с полем key")
		return
	}
	sess, token, err := a.svc.Restore(r.Context(), body.Key)
	if errors.Is(err, session.ErrInvalidKey) {
		a.log.InfoContext(r.Context(), "ключ восстановления не подошёл") // место в лимите остаётся занятым
		respond.Err(w, http.StatusNotFound, "invalid_key", "Ключ не подошёл")
		return
	}
	release()
	if err != nil {
		a.internal(w, r, err)
		return
	}
	a.setCookie(w, r, token)
	writeSecret(w, http.StatusOK, newMe(sess, false))
}

func (a *Sessions) internal(w http.ResponseWriter, r *http.Request, err error) {
	a.log.ErrorContext(r.Context(), "ошибка сессии", "error", err)
	respond.Err(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка сервера")
}

func (a *Sessions) setCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: token, Path: "/",
		MaxAge:   int(a.svc.TTL() / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.secure(r),
	})
}

func (a *Sessions) secure(r *http.Request) bool {
	return r.TLS != nil || (a.trustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
}

// clientIP — адрес клиента. За доверенным прокси берётся последний адрес из последней строки X-Forwarded-For:
// его дописывает сам прокси, а всё левее подставляет клиент (в том числе отдельной строкой заголовка).
func (a *Sessions) clientIP(r *http.Request) string {
	if a.trustProxy {
		if vals := r.Header.Values("X-Forwarded-For"); len(vals) > 0 {
			parts := strings.Split(vals[len(vals)-1], ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limitKey — ключ для ограничения частоты: IPv4 как есть, IPv6 по префиксу /64. Одному клиенту провайдер
// выдаёт целую подсеть /64, и по отдельным адресам лимит обходился бы сменой адреса внутри неё.
func (a *Sessions) limitKey(r *http.Request) string {
	addr := a.clientIP(r)
	ip := net.ParseIP(addr)
	if ip == nil {
		return addr
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// warnIfBehindUntrustedProxy пишет в лог один раз, если приходят заголовки прокси, а TRUST_PROXY выключен:
// за прокси все пользователи тогда делят один адрес (и один лимит), а cookie ставится без Secure.
func (a *Sessions) warnIfBehindUntrustedProxy(r *http.Request) {
	if a.trustProxy {
		return
	}
	if r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("X-Forwarded-Proto") == "" {
		return
	}
	a.warnProxy.Do(func() {
		a.log.Warn("запросы приходят через прокси, а TRUST_PROXY выключен: все пользователи делят один адрес для лимитов, cookie ставится без Secure; задайте TRUST_PROXY=true, если сервер доступен только через прокси")
	})
}

// writeSecret отдаёт JSON с токенами и ключами: такие ответы нельзя кэшировать.
func writeSecret(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Cache-Control", "no-store")
	respond.JSON(w, status, v)
}

func tooManyRequests(w http.ResponseWriter, retry time.Duration) {
	secs := int(retry.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	respond.Err(w, http.StatusTooManyRequests, "rate_limited", "Слишком много запросов, попробуйте позже")
}

// meResponse — профиль по контракту (схема Me). Уровень, XP и серия появятся с геймификацией (B-09),
// пока это значения нового пользователя; настройки — по умолчанию, их изменение — в задаче профиля.
type meResponse struct {
	Created     bool       `json:"created"`
	Onboarded   bool       `json:"onboarded"`
	DisplayName string     `json:"display_name"`
	Level       int        `json:"level"`
	XP          int        `json:"xp"`
	XPToNext    int        `json:"xp_to_next"`
	StreakDays  int        `json:"streak_days"`
	BestStreak  int        `json:"best_streak"`
	Settings    meSettings `json:"settings"`
}

type meSettings struct {
	Theme       string    `json:"theme"`
	Accent      string    `json:"accent"`
	Pattern     string    `json:"pattern"`
	AvatarFrame *string   `json:"avatar_frame"`
	FocusMode   bool      `json:"focus_mode"`
	ExamDate    *string   `json:"exam_date"`
	DailyGoal   dailyGoal `json:"daily_goal"`
}

type dailyGoal struct {
	Type  string `json:"type"`
	Value int    `json:"value"`
}

func newMe(s session.Session, created bool) meResponse {
	return meResponse{
		Created: created, Onboarded: false, DisplayName: s.DisplayName,
		Level: 1, XPToNext: 100,
		Settings: meSettings{Theme: "system", Accent: "accent-1", Pattern: "pattern-1", DailyGoal: dailyGoal{Type: "minutes", Value: 30}},
	}
}
