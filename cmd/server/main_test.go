package main

import (
	"bytes"
	"net"
	"strings"
	"testing"
)

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

func TestMigrateNeedsDatabaseAndKnownAction(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"migrate", "up"}, func(string) string { return "" }, &out, &errb); code != 2 ||
		!strings.Contains(errb.String(), "DATABASE_URL") {
		t.Fatalf("без DATABASE_URL: код %d, stderr %q", code, errb.String())
	}
	errb.Reset()
	env := func(k string) string {
		if k == "DATABASE_URL" {
			return "postgres://localhost:1/none"
		}
		return ""
	}
	for _, args := range [][]string{{"migrate"}, {"migrate", "sideways"}, {"migrate", "up", "extra"}} {
		if code := run(args, env, &out, &errb); code != 2 || !strings.Contains(errb.String(), "up, down или status") {
			t.Fatalf("%v: код %d, stderr %q", args, code, errb.String())
		}
		errb.Reset()
	}
}
