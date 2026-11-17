package main

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHealthcheckAgainstServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	if err := healthcheck(srv.Listener.Addr().String()); err != nil {
		t.Fatalf("живой сервер: %v", err)
	}
	addr := srv.Listener.Addr().String()
	srv.Close()
	if err := healthcheck(addr); err == nil {
		t.Fatal("остановленный сервер должен давать ошибку")
	}
}

func TestRunRejectsBadConfigAndCommand(t *testing.T) {
	env := func(k string) string {
		if k == "LOG_LEVEL" {
			return "loud"
		}
		return ""
	}
	var out, errb bytes.Buffer
	if code := run(nil, env, &out, &errb); code != 2 || !strings.Contains(errb.String(), "LOG_LEVEL") {
		t.Fatalf("код %d, stderr %q", code, errb.String())
	}
	errb.Reset()
	if code := run([]string{"bogus"}, func(string) string { return "" }, &out, &errb); code != 2 {
		t.Fatalf("неизвестная команда: код %d", code)
	}
}

func TestRunFailsWhenAddressBusy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	env := func(k string) string {
		if k == "ADDR" {
			return ln.Addr().String()
		}
		return ""
	}
	var out, errb bytes.Buffer
	if code := run(nil, env, &out, &errb); code != 1 || !strings.Contains(out.String(), "не удалось занять адрес") {
		t.Fatalf("код %d, лог %q", code, out.String())
	}
}
