package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

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

// browser — клиент со своей банкой cookie: один «браузер».
func browser(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func call(t *testing.T, c *http.Client, method, url, body string, hdr map[string]string) (*http.Response, []byte) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
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
		var e errorBody
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
	for i := 0; i < 5; i++ {
		resp, body := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZ`+strconv.Itoa(i)+`"}`, nil)
		var e errorBody
		decode(t, body, &e)
		if resp.StatusCode != 404 || e.Code != "invalid_key" || sidCookie(resp) != nil {
			t.Fatalf("попытка %d: код %d, %s, cookie %v", i, resp.StatusCode, body, sidCookie(resp))
		}
	}
	resp, body := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"ZZZZ-ZZZZ-ZZZZ-ZZZ9"}`, nil)
	var e errorBody
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
	for i := 0; i < 5; i++ {
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
	for i := 0; i < 5; i++ {
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
		resp, b := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", body, nil)
		var e errorBody
		decode(t, b, &e)
		if resp.StatusCode != 400 || e.Code != "bad_request" {
			t.Errorf("%s: код %d, %s", name, resp.StatusCode, b)
		}
	}
	resp, b := call(t, c, http.MethodPost, srv.URL+"/api/session/restore", `{"key":"`+strings.Repeat("A", 5000)+`"}`, nil)
	var e errorBody
	decode(t, b, &e)
	if resp.StatusCode != 413 || e.Code != "payload_too_large" {
		t.Errorf("большое тело: код %d, %s", resp.StatusCode, b)
	}
}

func TestNewSessionsAreRateLimited(t *testing.T) {
	srv, _ := newSessionServer(t, false)
	for i := 0; i < 20; i++ {
		resp, _ := call(t, &http.Client{}, http.MethodGet, srv.URL+"/api/me", "", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("сессия %d: код %d", i, resp.StatusCode)
		}
	}
	resp, body := call(t, &http.Client{}, http.MethodGet, srv.URL+"/api/me", "", nil)
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" || sidCookie(resp) != nil {
		t.Fatalf("21-я сессия: код %d, %s", resp.StatusCode, body)
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
	// Blocked не записывает событие, Hit — записывает
	l2 := NewLimiter(1, time.Minute)
	l2.now = func() time.Time { return now }
	if _, blocked := l2.Blocked("x"); blocked {
		t.Fatal("пока событий нет, блокировки нет")
	}
	l2.Hit("x")
	if retry, blocked := l2.Blocked("x"); !blocked || retry != time.Minute {
		t.Fatalf("после события: %v %v", retry, blocked)
	}
}
