package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestClaudeStream(t *testing.T) {
	srv := newFakeServer(t, http.StatusOK, `event: message_start
data: {"type":"message_start","message":{"model":"claude-haiku-5-5"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: ping
data: {"type":"ping"}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Привет, "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"мир"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}

event: message_stop
data: {"type":"message_stop"}

`)
	c := NewClaude(Config{BaseURL: srv.URL + "/", APIKey: testKey, HTTPClient: srv.Client()})
	chunks, err := collect(c.Stream(context.Background(), Request{
		System:    "Ты — репетитор.",
		Messages:  []Message{{Role: RoleUser, Text: "Привет"}, {Role: RoleAssistant, Text: "Здравствуй"}, {Role: RoleUser, Text: "Как дела?"}},
		MaxTokens: 500,
	}))
	if err != nil || strings.Join(chunks, "|") != "Привет, |мир" {
		t.Fatalf("chunks = %q, err = %v", chunks, err)
	}

	if srv.got.Method != http.MethodPost || srv.got.URL.Path != "/v1/messages" {
		t.Errorf("запрос: %s %s", srv.got.Method, srv.got.URL.Path)
	}
	h := srv.got.Header
	if h.Get("x-api-key") != testKey || h.Get("anthropic-version") != "2023-06-01" || h.Get("Content-Type") != "application/json" {
		t.Errorf("заголовки: %v", h)
	}
	var body struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Stream    bool   `json:"stream"`
		System    string `json:"system"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(srv.gotBody), &body); err != nil {
		t.Fatal(err)
	}
	if body.Model != "claude-haiku-5-5" || body.MaxTokens != 500 || !body.Stream || body.System != "Ты — репетитор." ||
		len(body.Messages) != 3 || body.Messages[1].Role != "assistant" || body.Messages[2].Content != "Как дела?" {
		t.Errorf("тело: %s", srv.gotBody)
	}
}

func TestClaudeDefaults(t *testing.T) {
	srv := newFakeServer(t, http.StatusOK, "")
	_, _ = collect(NewClaude(Config{BaseURL: srv.URL, Model: "claude-sonnet-5-5", HTTPClient: srv.Client()}).Stream(context.Background(), hello))
	var body map[string]any
	_ = json.Unmarshal([]byte(srv.gotBody), &body)
	if body["model"] != "claude-sonnet-5-5" || body["max_tokens"] != float64(defaultMaxTokens) {
		t.Errorf("тело: %s", srv.gotBody)
	}
	if _, ok := body["system"]; ok {
		t.Errorf("пустой system попал в запрос: %s", srv.gotBody)
	}
	if got := NewClaude(Config{}).base; got != "https://api.anthropic.com" {
		t.Errorf("base по умолчанию = %q", got)
	}
}

func TestClaudeStreamErrorEvent(t *testing.T) {
	srv := newFakeServer(t, http.StatusOK, "event: content_block_delta\n"+
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"привет"}}`+"\n\n"+
		"event: error\n"+`data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`+"\n\n")
	chunks, err := collect(NewClaude(Config{BaseURL: srv.URL, HTTPClient: srv.Client()}).Stream(context.Background(), hello))
	if err == nil || !strings.Contains(err.Error(), "overloaded_error") || !strings.HasPrefix(err.Error(), "claude:") {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(chunks, "") != "привет" {
		t.Errorf("куски = %v", chunks)
	}
}
