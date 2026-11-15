package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
)

// Claude — адаптер к Anthropic Messages API.
type Claude struct{ client }

// NewClaude создаёт адаптер; по умолчанию https://api.anthropic.com и модель claude-haiku-5-5.
func NewClaude(c Config) *Claude {
	return &Claude{newClient("claude", c, "https://api.anthropic.com", "claude-haiku-5-5")}
}

type claudeRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Stream    bool          `json:"stream"`
	System    string        `json:"system,omitempty"`
	Messages  []chatMessage `json:"messages"`
}

// Stream отправляет запрос в /v1/messages и отдаёт текст ответа по мере генерации.
func (c *Claude) Stream(ctx context.Context, req Request) iter.Seq2[string, error] {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	body := claudeRequest{Model: c.model, MaxTokens: maxTokens, Stream: true, System: req.System, Messages: chatMessages(req.Messages)}
	headers := map[string]string{"x-api-key": c.key, "anthropic-version": "2023-06-01"}
	return c.stream(ctx, c.base+"/v1/messages", headers, body, decodeClaude)
}

// decodeClaude разбирает SSE Anthropic: текст — в content_block_delta с delta.type=text_delta,
// конец ответа — message_stop, сбой посреди ответа — событие error.
func decodeClaude(body io.Reader, emit func(string) bool) error {
	finished := false
	err := readSSE(body, func(data []byte) (bool, error) {
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &ev); err != nil {
			return false, fmt.Errorf("разбор события: %w", err)
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				return emit(ev.Delta.Text), nil
			}
		case "message_stop":
			finished = true
		case "error":
			return false, fmt.Errorf("ошибка в потоке: %s: %s", ev.Error.Type, ev.Error.Message)
		}
		return true, nil
	})
	if err == nil && !finished {
		return ErrCutOff
	}
	return err
}
