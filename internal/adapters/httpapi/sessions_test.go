package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DarkIL1ght/M094Diploma/internal/adapters/httpapi/respond"
	"github.com/DarkIL1ght/M094Diploma/internal/adapters/memstore"
	"github.com/DarkIL1ght/M094Diploma/internal/app/session"
)

func newSessionServer(t *testing.T, trustProxy bool) (*httptest.Server, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	svc := session.New(memstore.New())
	srv := httptest.NewServer(NewRouter(testLogger(&logs), Deps{Sessions: svc, TrustProxy: trustProxy}))
	t.Cleanup(srv.Close)
	return srv, &logs
}

// newServerWith собирает сервер вручную: можно подменить хранилище и лимиты до запуска.
func newServerWith(t *testing.T, trustProxy bool, store session.Store, tune func(*Sessions)) (*httptest.Server, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	log := testLogger(&logs)
	a := NewSessions(session.New(store), log, trustProxy)
	if tune != nil {
		tune(a)
	}
	mux := http.NewServeMux()
	a.Routes(mux)
	srv := httptest.NewServer(wrap(mux, log))
	t.Cleanup(srv.Close)
	return srv, &logs
}

// browser — клиент со своей банкой cookie: один «браузер».
func browser(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

// doRequest выполняет запрос и не трогает *testing.T: его можно вызывать из горутин, где t.Fatal запрещён.
func doRequest(c *http.Client, method, url, body string, hdr map[string]string) (*http.Response, []byte, error) {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		return nil, nil, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp, b, err
}

// call — doRequest для основной горутины теста: ошибка сразу останавливает тест.
func call(t *testing.T, c *http.Client, method, url, body string, hdr map[string]string) (*http.Response, []byte) {
	t.Helper()
	resp, b, err := doRequest(c, method, url, body, hdr)
	if err != nil {
		t.Fatal(err)
	}
	return resp, b
}

func decode(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("не JSON: %v: %s", err, b)
	}
}

func sidCookie(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	return nil
}

func TestMeCreatesSessionWithSafeCookie(t *testing.T) {
	srv, _ := newSessionServer(t, false)
	c := browser(t)
	resp, body := call(t, c, http.MethodGet, srv.URL+"/api/me", "", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("код %d, Cache-Control %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	ck := sidCookie(resp)
	if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" ||
		ck.MaxAge != 180*24*3600 || ck.Secure || len(ck.Value) != 43 {
		t.Fatalf("cookie: %+v", ck)
	}
	var me map[string]any
	decode(t, body, &me)
	if me["created"] != true || me["display_name"] == "" || me["level"] != float64(1) || me["onboarded"] != false {
		t.Fatalf("профиль: %v", me)
	}
	settings, _ := me["settings"].(map[string]any)
	if settings["theme"] != "system" || settings["exam_date"] != nil {
		t.Fatalf("настройки: %v", settings)
	}

	// повторный вход с cookie — та же сессия, cookie продлевается
	resp2, body2 := call(t, c, http.MethodGet, srv.URL+"/api/me", "", nil)
	var me2 map[string]any
	decode(t, body2, &me2)
	if me2["created"] != false || me2["display_name"] != me["display_name"] {
		t.Fatalf("повторный вход: %v", me2)
	}
	if ck2 := sidCookie(resp2); ck2 == nil || ck2.Value != ck.Value || ck2.MaxAge != ck.MaxAge {
		t.Fatalf("cookie должна продлеваться с тем же значением: %+v", ck2)
	}
}

func TestSecureCookieOnlyBehindTrustedProxyOrTLS(t *testing.T) {
	cases := []struct {
		name       string
		trustProxy bool
		header     map[string]string
		secure     bool
	}{
		{"за доверенным прокси по https", true, map[string]string{"X-Forwarded-Proto": "https"}, true},
		{"за прокси по http", true, map[string]string{"X-Forwarded-Proto": "http"}, false},
		{"заголовок без доверия к прокси подделывается", false, map[string]string{"X-Forwarded-Proto": "https"}, false},
	}
	for _, c := range cases {
		srv, _ := newSessionServer(t, c.trustProxy)
		resp, _ := call(t, &http.Client{}, http.MethodGet, srv.URL+"/api/me", "", c.header)
		if ck := sidCookie(resp); ck == nil || ck.Secure != c.secure {
			t.Errorf("%s: cookie %+v, ожидали Secure=%v", c.name, ck, c.secure)
		}
	}
	// прямое TLS-соединение
	tlsSrv := httptest.NewTLSServer(NewRouter(testLogger(&bytes.Buffer{}), Deps{Sessions: session.New(memstore.New())}))
	defer tlsSrv.Close()
	resp, _ := call(t, tlsSrv.Client(), http.MethodGet, tlsSrv.URL+"/api/me", "", nil)
	if ck := sidCookie(resp); ck == nil || !ck.Secure {
		t.Errorf("по TLS cookie должна быть Secure: %+v", ck)
	}
}

func TestRequireSession(t *testing.T) {
	srv, _ := newSessionServer(t, false)
	for name, hdr := range map[string]map[string]string{
		"без cookie":     nil,
		"мусор в cookie": {"Cookie": "sid=not-a-token"},
		"чужой токен":    {"Cookie": "sid=" + strings.Repeat("A", 43)},
	} {
		resp, body := call(t, &http.Client{}, http.MethodPost, srv.URL+"/api/session/recovery-key", "", hdr)
		var e respond.Error
		decode(t, body, &e)
		if resp.StatusCode != 401 || e.Code != "no_session" {
			t.Errorf("%s: код %d, тело %s", name, resp.StatusCode, body)
		}
	}
}

func TestTransferProgressByRecoveryKey(t *testing.T) {
	srv, logs := newSessionServer(t, false)
	a, b := browser(t), browser(t)

	_, body := call(t, a, http.MethodGet, srv.URL+"/api/me", "", nil)
	var meA map[string]any
	decode(t, body, &meA)

	resp, body := call(t, a, http.MethodPost, srv.URL+"/api/session/recovery-key", "", nil)
	var k struct {
		Key      string `json:"key"`
		IssuedAt string `json:"issued_at"`
	}
	decode(t, body, &k)
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" || len(k.Key) != 19 || k.IssuedAt == "" {
		t.Fatalf("ключ: код %d, %s", resp.StatusCode, body)
	}

	// второй браузер вводит ключ, написанный от руки (строчные буквы, пробелы вместо дефисов)
	typed := strings.ToLower(strings.ReplaceAll(k.Key, "-", " "))
	resp, body = call(t, b, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"`+typed+`"}`, nil)
	var meB map[string]any
	decode(t, body, &meB)
	if resp.StatusCode != 200 || meB["display_name"] != meA["display_name"] || meB["created"] != false {
		t.Fatalf("restore: код %d, %s", resp.StatusCode, body)
	}
	ckB := sidCookie(resp)
	if ckB == nil || !ckB.HttpOnly {
		t.Fatalf("cookie после restore: %+v", ckB)
	}
	// теперь второй браузер — полноценный пользователь той же сессии
	_, body = call(t, b, http.MethodGet, srv.URL+"/api/me", "", nil)
	var meB2 map[string]any
	decode(t, body, &meB2)
	if meB2["display_name"] != meA["display_name"] || meB2["created"] != false {
		t.Fatalf("второй браузер не в той же сессии: %v", meB2)
	}
	// первый браузер тоже остался в сессии
	_, body = call(t, a, http.MethodGet, srv.URL+"/api/me", "", nil)
	var meA2 map[string]any
	decode(t, body, &meA2)
	if meA2["display_name"] != meA["display_name"] {
		t.Fatalf("первый браузер потерял сессию: %v", meA2)
	}

	if strings.Contains(logs.String(), k.Key) || strings.Contains(logs.String(), ckB.Value) {
		t.Fatalf("ключ или токен попали в лог: %s", logs.String())
	}
}

func TestRestoreWrongKeyAndBruteForceLimit(t *testing.T) {
	srv, _ := newSessionServer(t, false)
	c := browser(t)
	for i := 0; i < restoreFails; i++ {
		resp, body := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZ`+strconv.Itoa(i)+`"}`, nil)
		var e respond.Error
		decode(t, body, &e)
		if resp.StatusCode != 404 || e.Code != "invalid_key" || sidCookie(resp) != nil {
			t.Fatalf("попытка %d: код %d, %s, cookie %v", i, resp.StatusCode, body, sidCookie(resp))
		}
	}
	resp, body := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZ9"}`, nil)
	var e respond.Error
	decode(t, body, &e)
	retry, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
	if resp.StatusCode != 429 || e.Code != "rate_limited" || retry < 1 || retry > 900 {
		t.Fatalf("после лимита: код %d, %s, Retry-After %q", resp.StatusCode, body, resp.Header.Get("Retry-After"))
	}
}

func TestRestoreLimitIsPerClientAddressBehindProxy(t *testing.T) {
	srv, _ := newSessionServer(t, true)
	c := &http.Client{}
	bad := `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZZ"}`
	for i := 0; i < restoreFails; i++ {
		call(t, c, http.MethodPost, srv.URL+"/api/session/restore", bad, map[string]string{"X-Forwarded-For": "198.51.100.1"})
	}
	resp, _ := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", bad, map[string]string{"X-Forwarded-For": "198.51.100.1"})
	if resp.StatusCode != 429 {
		t.Fatalf("тот же адрес должен быть заблокирован, код %d", resp.StatusCode)
	}
	resp, _ = call(t, c, http.MethodPost, srv.URL+"/api/session/restore", bad, map[string]string{"X-Forwarded-For": "198.51.100.2"})
	if resp.StatusCode != 404 {
		t.Fatalf("другой адрес не должен страдать, код %d", resp.StatusCode)
	}
	// левые адреса слева в цепочке подставляет клиент: считается последний, дописанный прокси
	resp, _ = call(t, c, http.MethodPost, srv.URL+"/api/session/restore", bad, map[string]string{"X-Forwarded-For": "203.0.113.9, 198.51.100.1"})
	if resp.StatusCode != 429 {
		t.Fatalf("подделка адреса слева не должна обходить лимит, код %d", resp.StatusCode)
	}
}

func TestForwardedForIgnoredWithoutTrustedProxy(t *testing.T) {
	srv, _ := newSessionServer(t, false)
	c := &http.Client{}
	for i := 0; i < restoreFails; i++ {
		call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZZ"}`, map[string]string{"X-Forwarded-For": "10.0.0." + strconv.Itoa(i)})
	}
	resp, _ := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZZ"}`, map[string]string{"X-Forwarded-For": "10.0.0.99"})
	if resp.StatusCode != 429 {
		t.Fatalf("без доверия к прокси заголовок подделывается, код %d", resp.StatusCode)
	}
}

func TestRestoreBadBodies(t *testing.T) {
	srv, _ := newSessionServer(t, false)
	c := &http.Client{}
	for name, body := range map[string]string{"не JSON": "ключ", "пустое тело": ""} {
		resp, b := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", body, map[string]string{"Content-Type": "application/json"})
		var e respond.Error
		decode(t, b, &e)
		if resp.StatusCode != 400 || e.Code != "bad_request" {
			t.Errorf("%s: код %d, %s", name, resp.StatusCode, b)
		}
	}
	resp, b := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"`+strings.Repeat("A", 5000)+`"}`, nil)
	var e respond.Error
	decode(t, b, &e)
	if resp.StatusCode != 413 || e.Code != "payload_too_large" {
		t.Errorf("большое тело: код %d, %s", resp.StatusCode, b)
	}
}

func TestNewSessionsAreRateLimited(t *testing.T) {
	srv, _ := newSessionServer(t, false)
	for i := 0; i < newSessionsMax; i++ {
		resp, _ := call(t, &http.Client{}, http.MethodGet, srv.URL+"/api/me", "", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("сессия %d: код %d", i, resp.StatusCode)
		}
	}
	resp, body := call(t, &http.Client{}, http.MethodGet, srv.URL+"/api/me", "", nil)
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" || sidCookie(resp) != nil {
		t.Fatalf("сессия сверх лимита: код %d, %s", resp.StatusCode, body)
	}
	// кто уже вошёл, лимитом не затрагивается
	c := browser(t)
	srv2, _ := newSessionServer(t, false)
	call(t, c, http.MethodGet, srv2.URL+"/api/me", "", nil)
	for i := 0; i < 30; i++ {
		if resp, _ := call(t, c, http.MethodGet, srv2.URL+"/api/me", "", nil); resp.StatusCode != 200 {
			t.Fatalf("повторный вход %d: код %d", i, resp.StatusCode)
		}
	}
}

func TestSessionRoutesMethods(t *testing.T) {
	srv, _ := newSessionServer(t, false)
	for path, method := range map[string]string{"/api/me": http.MethodPost, "/api/session/restore": http.MethodGet, "/api/session/recovery-key": http.MethodGet} {
		resp, _ := call(t, &http.Client{}, method, srv.URL+path, "", nil)
		if resp.StatusCode != 405 || resp.Header.Get("Allow") == "" {
			t.Errorf("%s %s: код %d", method, path, resp.StatusCode)
		}
	}
}

func TestLimiterWindow(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	l := NewLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	if _, ok := l.Take("a"); !ok {
		t.Fatal("первый должен пройти")
	}
	now = now.Add(20 * time.Second)
	if _, ok := l.Take("a"); !ok {
		t.Fatal("второй должен пройти")
	}
	retry, ok := l.Take("a")
	if ok || retry != 40*time.Second {
		t.Fatalf("третий отклоняется до освобождения первого места: %v %v", retry, ok)
	}
	if _, ok := l.Take("b"); !ok {
		t.Fatal("другой ключ не зависит от первого")
	}
	now = now.Add(41 * time.Second) // первое событие вышло из окна
	if _, ok := l.Take("a"); !ok {
		t.Fatal("после окна снова можно")
	}
}

func TestLimiterReserveAndRelease(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	rel1, _, ok1 := l.Reserve("k")
	_, _, ok2 := l.Reserve("k")
	if !ok1 || !ok2 {
		t.Fatal("два места должны занять")
	}
	if _, retry, ok := l.Reserve("k"); ok || retry <= 0 {
		t.Fatalf("третье место занять нельзя: %v %v", retry, ok)
	}
	rel1()
	rel1() // повторный возврат не освобождает чужое место
	if _, _, ok := l.Reserve("k"); !ok {
		t.Fatal("после возврата место освободилось")
	}
	if _, _, ok := l.Reserve("k"); ok {
		t.Fatal("повторный release не должен был освободить второе место")
	}
}

func TestLimiterReserveIsAtomicUnderConcurrency(t *testing.T) {
	l := NewLimiter(5, time.Minute)
	var wg sync.WaitGroup
	var granted atomic.Int32
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, ok := l.Reserve("one-address"); ok {
				granted.Add(1)
			}
		}()
	}
	wg.Wait()
	if granted.Load() != 5 {
		t.Fatalf("при лимите 5 прошло %d из 200 параллельных", granted.Load())
	}
}

func TestLimiterForgetsIdleKeys(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	l := NewLimiter(5, time.Minute)
	l.now = func() time.Time { return now }
	l.Take("ушёл")
	now = now.Add(2 * time.Minute)
	for i := 0; i < 1024; i++ { // периодическая чистка срабатывает раз в 1024 обращения
		l.Take("активный")
		now = now.Add(2 * time.Minute)
	}
	l.mu.Lock()
	_, left := l.hits["ушёл"]
	l.mu.Unlock()
	if left {
		t.Fatal("ключ, который больше не обращается, должен исчезнуть из журнала")
	}
}

// slowStore имитирует хранилище с задержкой, как Postgres: именно в это окно проходили параллельные запросы.
type slowStore struct {
	session.Store
	delay time.Duration
	calls atomic.Int32
}

func (s *slowStore) SessionByRecoveryKey(ctx context.Context, h session.KeyHash) (session.Session, error) {
	s.calls.Add(1)
	time.Sleep(s.delay)
	return s.Store.SessionByRecoveryKey(ctx, h)
}

func TestRestoreLimitHoldsUnderConcurrency(t *testing.T) {
	st := &slowStore{Store: memstore.New(), delay: 20 * time.Millisecond}
	srv, _ := newServerWith(t, false, st, nil)
	const n = 100
	var wg sync.WaitGroup
	var notFound, limited, other atomic.Int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _, err := doRequest(&http.Client{}, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZZ"}`, nil)
			if err != nil {
				t.Error(err) // из горутины только Error: Fatal остановил бы её молча
				other.Add(1)
				return
			}
			switch resp.StatusCode {
			case 404:
				notFound.Add(1)
			case 429:
				limited.Add(1)
			default:
				other.Add(1)
			}
		}()
	}
	wg.Wait()
	if notFound.Load() != restoreFails || limited.Load() != n-restoreFails || other.Load() != 0 {
		t.Fatalf("из %d параллельных: 404=%d, 429=%d, прочих=%d; ожидали ровно %d проверок ключа",
			n, notFound.Load(), limited.Load(), other.Load(), restoreFails)
	}
	if st.calls.Load() != restoreFails {
		t.Fatalf("в хранилище дошло %d запросов вместо %d", st.calls.Load(), restoreFails)
	}
}

func TestRestoreRequiresJSONContentType(t *testing.T) {
	srv, _ := newSessionServer(t, false)
	a := browser(t)
	call(t, a, http.MethodGet, srv.URL+"/api/me", "", nil)
	_, body := call(t, a, http.MethodPost, srv.URL+"/api/session/recovery-key", "", nil)
	var k struct{ Key string }
	decode(t, body, &k)

	// форма с чужого сайта (enctype=text/plain) шлёт валидный JSON, но не application/json
	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x"} {
		resp, b := call(t, &http.Client{}, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"`+k.Key+`","x":"="}`, map[string]string{"Content-Type": ct})
		var e respond.Error
		decode(t, b, &e)
		if resp.StatusCode != http.StatusUnsupportedMediaType || e.Code != "unsupported_media_type" || sidCookie(resp) != nil {
			t.Errorf("%s: код %d, cookie %v, тело %s", ct, resp.StatusCode, sidCookie(resp), b)
		}
	}
	resp, _ := call(t, &http.Client{}, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"`+k.Key+`"}`, nil)
	if resp.StatusCode != 200 || sidCookie(resp) == nil {
		t.Fatalf("с application/json ключ должен подходить: код %d", resp.StatusCode)
	}
}

func TestSuccessfulAndMalformedRestoresDoNotUseUpLimit(t *testing.T) {
	srv, _ := newSessionServer(t, false)
	a := browser(t)
	call(t, a, http.MethodGet, srv.URL+"/api/me", "", nil)
	_, body := call(t, a, http.MethodPost, srv.URL+"/api/session/recovery-key", "", nil)
	var k struct{ Key string }
	decode(t, body, &k)
	c := &http.Client{}
	for i := 0; i < 3*restoreFails; i++ {
		if resp, _ := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"`+k.Key+`"}`, nil); resp.StatusCode != 200 {
			t.Fatalf("удачный restore %d: код %d", i, resp.StatusCode)
		}
		if resp, _ := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", "не JSON", nil); resp.StatusCode != 400 {
			t.Fatalf("мусорное тело %d: код %d", i, resp.StatusCode)
		}
	}
	if resp, _ := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZZ"}`, nil); resp.StatusCode != 404 {
		t.Fatalf("после удачных попыток неверный ключ должен давать 404, код %d", resp.StatusCode)
	}
}

func TestGlobalRestoreLimitStopsDistributedGuessing(t *testing.T) {
	srv, _ := newServerWith(t, true, memstore.New(), func(a *Sessions) { a.restoreAll = NewLimiter(3, time.Minute) })
	c := &http.Client{}
	for i := 0; i < 3; i++ {
		resp, _ := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZZ"}`, map[string]string{"X-Forwarded-For": "203.0.113." + strconv.Itoa(i+1)})
		if resp.StatusCode != 404 {
			t.Fatalf("попытка %d: код %d", i, resp.StatusCode)
		}
	}
	resp, _ := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZZ"}`, map[string]string{"X-Forwarded-For": "203.0.113.200"})
	if resp.StatusCode != 429 {
		t.Fatalf("с нового адреса сверх общего лимита: код %d", resp.StatusCode)
	}
}

func TestIPv6LimitedByPrefix(t *testing.T) {
	srv, _ := newSessionServer(t, true)
	c := &http.Client{}
	bad := `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZZ"}`
	for i := 0; i < restoreFails; i++ { // разные адреса одной подсети /64
		call(t, c, http.MethodPost, srv.URL+"/api/session/restore", bad, map[string]string{"X-Forwarded-For": "2001:db8:0:0::" + strconv.Itoa(i+1)})
	}
	same, _ := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", bad, map[string]string{"X-Forwarded-For": "2001:db8:0:0:ffff::1"})
	other, _ := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", bad, map[string]string{"X-Forwarded-For": "2001:db8:0:1::1"})
	if same.StatusCode != 429 || other.StatusCode != 404 {
		t.Fatalf("та же /64: %d (ждали 429), другая /64: %d (ждали 404)", same.StatusCode, other.StatusCode)
	}
}

func TestForwardedForUsesLastValueOfLastHeaderLine(t *testing.T) {
	srv, _ := newSessionServer(t, true)
	bad := `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZZ"}`
	send := func(lines ...string) int {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/session/restore", strings.NewReader(bad))
		req.Header.Set("Content-Type", "application/json")
		for _, l := range lines {
			req.Header.Add("X-Forwarded-For", l)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for i := 0; i < restoreFails; i++ {
		// подделка в первой строке, настоящий адрес (дописанный прокси) в последней
		send("198.51.100."+strconv.Itoa(i+10), "203.0.113.7")
	}
	if code := send("198.51.100.99", "203.0.113.7"); code != 429 {
		t.Fatalf("подделка в первой строке заголовка обошла лимит: код %d", code)
	}
}

func TestWarnsOnceWhenProxyHeadersArriveWithoutTrustProxy(t *testing.T) {
	srv, logs := newSessionServer(t, false)
	for i := 0; i < 3; i++ {
		call(t, &http.Client{}, http.MethodGet, srv.URL+"/api/me", "", map[string]string{"X-Forwarded-For": "203.0.113.1"})
	}
	if n := strings.Count(logs.String(), "TRUST_PROXY выключен"); n != 1 {
		t.Fatalf("предупреждение должно быть ровно одно, получили %d: %s", n, logs.String())
	}

	srv2, logs2 := newSessionServer(t, false)
	call(t, &http.Client{}, http.MethodGet, srv2.URL+"/api/me", "", nil)
	srv3, logs3 := newSessionServer(t, true)
	call(t, &http.Client{}, http.MethodGet, srv3.URL+"/api/me", "", map[string]string{"X-Forwarded-For": "203.0.113.1"})
	if strings.Contains(logs2.String(), "TRUST_PROXY") || strings.Contains(logs3.String(), "TRUST_PROXY") {
		t.Fatal("без заголовков прокси и при включённом TRUST_PROXY предупреждения быть не должно")
	}
}
