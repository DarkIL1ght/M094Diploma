package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestOllamaStream(t *testing.T) {
	srv := newFakeServer(t, http.StatusOK, `{"message":{"role":"assistant","content":"Привет, "},"done":false}
{"message":{"role":"assistant","content":"мир"},"done":false}
{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}
`)
	o := NewOllama(Config{BaseURL: srv.URL, HTTPClient: srv.Client()})
	chunks, err := collect(o.Stream(context.Background(), Request{
		System:    "Ты — репетитор.",
		Messages:  []Message{{Role: RoleUser, Text: "Привет"}},
		MaxTokens: 200,
	}))
	if err != nil || strings.Join(chunks, "|") != "Привет, |мир" {
		t.Fatalf("chunks = %q, err = %v", chunks, err)
	}

	if srv.got.Method != http.MethodPost || srv.got.URL.Path != "/api/chat" {
		t.Errorf("запрос: %s %s", srv.got.Method, srv.got.URL.Path)
	}
	var body struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Options struct {
			NumPredict int `json:"num_predict"`
		} `json:"options"`
	}
	if err := json.Unmarshal([]byte(srv.gotBody), &body); err != nil {
		t.Fatal(err)
	}
	if body.Model != "gemma3:4b" || !body.Stream || len(body.Messages) != 2 ||
		body.Messages[0].Role != "system" || body.Messages[0].Content != "Ты — репетитор." ||
		body.Messages[1].Role != "user" || body.Options.NumPredict != 200 {
		t.Errorf("тело: %s", srv.gotBody)
	}
}

func TestOllamaDefaults(t *testing.T) {
	if got := NewOllama(Config{}).base; got != "http://localhost:11434" {
		t.Errorf("base по умолчанию = %q", got)
	}
	srv := newFakeServer(t, http.StatusOK, "")
	_, _ = collect(NewOllama(Config{BaseURL: srv.URL, Model: "qwen3:8b", HTTPClient: srv.Client()}).Stream(context.Background(), hello))
	var body struct {
		Model    string            `json:"model"`
		Messages []json.RawMessage `json:"messages"`
	}
	_ = json.Unmarshal([]byte(srv.gotBody), &body)
	if body.Model != "qwen3:8b" || len(body.Messages) != 1 || strings.Contains(srv.gotBody, "num_predict") {
		t.Errorf("тело: %s", srv.gotBody)
	}
}

func TestOllamaStreamError(t *testing.T) {
	srv := newFakeServer(t, http.StatusOK, `{"message":{"content":"привет"},"done":false}
{"error":"model runner has unexpectedly stopped"}
`)
	chunks, err := collect(NewOllama(Config{BaseURL: srv.URL, HTTPClient: srv.Client()}).Stream(context.Background(), hello))
	if err == nil || !strings.Contains(err.Error(), "unexpectedly stopped") || !strings.HasPrefix(err.Error(), "ollama:") {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(chunks, "") != "привет" {
		t.Errorf("куски = %v", chunks)
	}
}
