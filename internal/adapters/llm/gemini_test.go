package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestGeminiStream(t *testing.T) {
	srv := newFakeServer(t, http.StatusOK,
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"думаю","thought":true},{"text":"Привет, "}]}}]}`+"\n\n"+
			`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"мир"}]},"finishReason":"STOP"}]}`+"\n\n")
	g := NewGemini(Config{BaseURL: srv.URL, APIKey: testKey, HTTPClient: srv.Client()})
	chunks, err := collect(g.Stream(context.Background(), Request{
		System:    "Ты — репетитор.",
		Messages:  []Message{{Role: RoleUser, Text: "Привет"}, {Role: RoleAssistant, Text: "Здравствуй"}},
		MaxTokens: 300,
	}))
	if err != nil || strings.Join(chunks, "|") != "Привет, |мир" {
		t.Fatalf("chunks = %q, err = %v", chunks, err)
	}

	if srv.got.URL.Path != "/v1beta/models/gemini-2.5-flash:streamGenerateContent" || srv.got.URL.Query().Get("alt") != "sse" {
		t.Errorf("URL: %s", srv.got.URL)
	}
	if strings.Contains(srv.got.URL.String(), testKey) || srv.got.Header.Get("x-goog-api-key") != testKey {
		t.Errorf("ключ должен идти заголовком, не в URL: %s %v", srv.got.URL, srv.got.Header)
	}
	type part struct {
		Text string `json:"text"`
	}
	var body struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []part `json:"parts"`
		} `json:"contents"`
		SystemInstruction struct {
			Parts []part `json:"parts"`
		} `json:"systemInstruction"`
		GenerationConfig struct {
			MaxOutputTokens int `json:"maxOutputTokens"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal([]byte(srv.gotBody), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Contents) != 2 || body.Contents[0].Role != "user" || body.Contents[1].Role != "model" ||
		body.Contents[1].Parts[0].Text != "Здравствуй" || len(body.SystemInstruction.Parts) != 1 ||
		body.SystemInstruction.Parts[0].Text != "Ты — репетитор." || body.GenerationConfig.MaxOutputTokens != 300 {
		t.Errorf("тело: %s", srv.gotBody)
	}
}

func TestGeminiDefaults(t *testing.T) {
	srv := newFakeServer(t, http.StatusOK, "")
	_, _ = collect(NewGemini(Config{BaseURL: srv.URL, Model: "gemini-3-pro", HTTPClient: srv.Client()}).Stream(context.Background(), hello))
	if srv.got.URL.Path != "/v1beta/models/gemini-3-pro:streamGenerateContent" {
		t.Errorf("URL: %s", srv.got.URL)
	}
	if strings.Contains(srv.gotBody, "systemInstruction") || strings.Contains(srv.gotBody, "maxOutputTokens") {
		t.Errorf("пустые поля попали в запрос: %s", srv.gotBody)
	}
	if got := NewGemini(Config{}).base; got != "https://generativelanguage.googleapis.com" {
		t.Errorf("base по умолчанию = %q", got)
	}
}

func TestGeminiStreamErrors(t *testing.T) {
	first := `data: {"candidates":[{"content":{"parts":[{"text":"привет"}]}}]}` + "\n\n"
	for name, tc := range map[string]struct{ body, want string }{
		"ошибка в потоке":     {first + `data: {"error":{"code":503,"message":"The model is overloaded"}}` + "\n\n", "overloaded"},
		"запрос заблокирован": {first + `data: {"promptFeedback":{"blockReason":"SAFETY"}}` + "\n\n", "SAFETY"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newFakeServer(t, http.StatusOK, tc.body)
			chunks, err := collect(NewGemini(Config{BaseURL: srv.URL, HTTPClient: srv.Client()}).Stream(context.Background(), hello))
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.HasPrefix(err.Error(), "gemini:") {
				t.Fatalf("err = %v", err)
			}
			if strings.Join(chunks, "") != "привет" {
				t.Errorf("куски = %v", chunks)
			}
		})
	}
}
