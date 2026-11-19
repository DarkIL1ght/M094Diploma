package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthURL(t *testing.T) {
	cases := map[string]string{
		":8080":          "http://127.0.0.1:8080/healthz",
		"0.0.0.0:9000":   "http://127.0.0.1:9000/healthz",
		"[::]:9000":      "http://127.0.0.1:9000/healthz",
		"127.0.0.1:8081": "http://127.0.0.1:8081/healthz",
		"[::1]:8082":     "http://[::1]:8082/healthz",
		"example:80":     "http://example:80/healthz",
	}
	for addr, want := range cases {
		got, err := healthURL(addr)
		if err != nil || got != want {
			t.Errorf("healthURL(%q) = %q, %v; want %q", addr, got, err, want)
		}
	}
	if _, err := healthURL("8080"); err == nil {
		t.Error("адрес без порта должен давать ошибку")
	}
}

func TestCheckHealthAgainstServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	addr := srv.Listener.Addr().String()
	if err := CheckHealth(addr); err != nil {
		t.Fatalf("живой сервер: %v", err)
	}
	srv.Close()
	if err := CheckHealth(addr); err == nil {
		t.Fatal("остановленный сервер должен давать ошибку")
	}
}

func TestCheckHealthRejectsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "плохо", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	if err := CheckHealth(srv.Listener.Addr().String()); err == nil {
		t.Fatal("ответ 503 должен считаться неудачей")
	}
}
