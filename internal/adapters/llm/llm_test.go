package llm

import (
	"context"
	"errors"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

const testKey = "secret-key-123"

// collect склеивает куски потока; останавливается на первой ошибке.
func collect(seq iter.Seq2[string, error]) ([]string, error) {
	var chunks []string
	for s, err := range seq {
		if err != nil {
			return chunks, err
		}
		chunks = append(chunks, s)
	}
	return chunks, nil
}

// fakeServer отвечает телом body с заданным статусом и запоминает запрос.
type fakeServer struct {
	*httptest.Server
	got     *http.Request
	gotBody string
}

func newFakeServer(t *testing.T, status int, body string) *fakeServer {
	t.Helper()
	f := &fakeServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		f.got, f.gotBody = r.Clone(context.Background()), string(raw)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(f.Close)
	return f
}

// streamer — общий для всех адаптеров метод потока (интерфейс самого потребителя появится в B-14).
type streamer interface {
	Name() string
	Stream(context.Context, Request) iter.Seq2[string, error]
}

// provider — то, что общие тесты знают о каждом адаптере: как его создать и как выглядит
// один кусок текста в его протоколе.
type provider struct {
	name  string
	new   func(Config) streamer
	chunk string // одно событие потока с текстом «привет»
}

var providers = []provider{
	{"claude", func(c Config) streamer {
		return NewClaude(c)
	}, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"привет\"}}\n\n"},
	{"gemini", func(c Config) streamer {
		return NewGemini(c)
	}, "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"привет\"}]}}]}\n\n"},
	{"ollama", func(c Config) streamer {
		return NewOllama(c)
	}, `{"message":{"role":"assistant","content":"привет"},"done":false}` + "\n"},
}

var hello = Request{Messages: []Message{{Role: RoleUser, Text: "привет"}}}

func TestName(t *testing.T) {
	for _, p := range providers {
		if got := p.new(Config{}).Name(); got != p.name {
			t.Errorf("Name() = %q, want %q", got, p.name)
		}
	}
}

func TestStatusError(t *testing.T) {
	long := strings.Repeat("ошибка ", 200) // длиннее лимита, кириллица — проверка обрезки по рунам
	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			srv := newFakeServer(t, http.StatusTooManyRequests, long)
			chunks, err := collect(p.new(Config{BaseURL: srv.URL, APIKey: testKey, HTTPClient: srv.Client()}).Stream(context.Background(), hello))
			var se *StatusError
			if !errors.As(err, &se) {
				t.Fatalf("err = %v, want *StatusError", err)
			}
			if se.Status != http.StatusTooManyRequests || se.Provider != p.name {
				t.Errorf("StatusError = %+v", se)
			}
			if len(se.Body) > maxErrorBody+len("…") || !utf8.ValidString(se.Body) || !strings.HasSuffix(se.Body, "…") {
				t.Errorf("тело не обрезано аккуратно: %d байт, %q", len(se.Body), se.Body[len(se.Body)-10:])
			}
			if msg := err.Error(); !strings.Contains(msg, "429") || strings.Contains(msg, testKey) {
				t.Errorf("текст ошибки: %q", msg)
			}
			if len(chunks) != 0 {
				t.Errorf("куски до ошибки: %v", chunks)
			}
		})
	}
}

func TestCutOffStream(t *testing.T) {
	// Поток закрылся без признака конца ответа — это ошибка, а не тихо урезанный текст.
	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			srv := newFakeServer(t, http.StatusOK, p.chunk)
			chunks, err := collect(p.new(Config{BaseURL: srv.URL, HTTPClient: srv.Client()}).Stream(context.Background(), hello))
			if !errors.Is(err, ErrCutOff) {
				t.Fatalf("err = %v, want ErrCutOff", err)
			}
			if strings.Join(chunks, "") != "привет" {
				t.Errorf("куски = %v", chunks)
			}
		})
	}
}

func TestCancelStopsStream(t *testing.T) {
	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, p.chunk)
				w.(http.Flusher).Flush()
				<-r.Context().Done() // ждём, пока клиент не отменит запрос
			}))
			defer srv.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var chunks []string
			var err error
			for s, e := range p.new(Config{BaseURL: srv.URL, HTTPClient: srv.Client()}).Stream(ctx, hello) {
				if e != nil {
					err = e
					break
				}
				chunks = append(chunks, s)
				cancel()
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
			if len(chunks) != 1 {
				t.Errorf("куски = %v", chunks)
			}
		})
	}
}

func TestConsumerBreak(t *testing.T) {
	// Потребитель вышел из цикла после первого куска: итератор не должен звать yield повторно (это паника).
	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			srv := newFakeServer(t, http.StatusOK, p.chunk+p.chunk)
			n := 0
			for range p.new(Config{BaseURL: srv.URL, HTTPClient: srv.Client()}).Stream(context.Background(), hello) {
				n++
				break
			}
			if n != 1 {
				t.Errorf("итераций = %d", n)
			}
		})
	}
}

func TestReadSSE(t *testing.T) {
	in := ": комментарий\nevent: a\ndata: {\"x\":1}\n\ndata: строка1\ndata: строка2\r\n\r\ndata: без пустой строки в конце"
	var got []string
	err := readSSE(strings.NewReader(in), func(data []byte) (bool, error) {
		got = append(got, string(data))
		return true, nil
	})
	want := []string{`{"x":1}`, "строка1\nстрока2", "без пустой строки в конце"}
	if err != nil || strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, err %v", got, err)
	}
}
