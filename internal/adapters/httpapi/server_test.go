package httpapi

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func startServer(t *testing.T, h http.Handler, grace time.Duration) (addr string, stop context.CancelFunc, done <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	res := make(chan error, 1)
	go func() { res <- Serve(ctx, NewServer(h), ln, grace, testLogger(&bytes.Buffer{})) }()
	t.Cleanup(cancel)
	return ln.Addr().String(), cancel, res
}

func TestServeShutsDownGracefully(t *testing.T) {
	started := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, "готово")
	})
	addr, stop, done := startServer(t, slow, 5*time.Second)

	got := make(chan string, 1)
	go func() {
		resp, err := http.Get("http://" + addr)
		if err != nil {
			got <- "ошибка: " + err.Error()
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		got <- string(b)
	}()
	<-started
	stop() // сигнал остановки приходит, пока запрос ещё выполняется

	if body := <-got; body != "готово" {
		t.Fatalf("текущий запрос должен дожить до конца, получили %q", body)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("штатная остановка вернула ошибку: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve не вернулся после остановки")
	}
	if _, err := http.Get("http://" + addr); err == nil {
		t.Fatal("после остановки новые соединения не принимаются")
	}
}

func TestServeAbortsRequestsAfterGrace(t *testing.T) {
	started := make(chan struct{})
	stuck := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done() // висит, пока соединение не оборвут
	})
	addr, stop, done := startServer(t, stuck, 100*time.Millisecond)
	go http.Get("http://" + addr) //nolint:errcheck // клиент нужен только чтобы занять обработчик
	<-started
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("остановка по таймауту должна быть штатной: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("зависший запрос не дал серверу остановиться")
	}
}

func TestServeReturnsListenerError(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	ln.Close() // Serve на закрытом слушателе падает сразу
	err := Serve(context.Background(), NewServer(http.NotFoundHandler()), ln, time.Second, testLogger(&bytes.Buffer{}))
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
}
