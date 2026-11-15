package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"

	"github.com/DarkIL1ght/M094Diploma/internal/app/tutor"
)

// Ollama — адаптер к локальному серверу Ollama.
type Ollama struct{ client }

// NewOllama создаёт адаптер; по умолчанию http://localhost:11434 и модель gemma3:4b.
func NewOllama(c Config) *Ollama {
	return &Ollama{newClient("ollama", c, "http://localhost:11434", "gemma3:4b")}
}

type ollamaOptions struct {
	NumPredict int `json:"num_predict,omitempty"`
}

type ollamaRequest struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  *ollamaOptions `json:"options,omitempty"`
}

// Stream отправляет запрос в /api/chat и отдаёт текст ответа по мере генерации.
// Системный промпт у Ollama — первая реплика с ролью system.
func (o *Ollama) Stream(ctx context.Context, req tutor.Request) iter.Seq2[string, error] {
	body := ollamaRequest{Model: o.model, Stream: true}
	if req.System != "" {
		body.Messages = append(body.Messages, chatMessage{Role: "system", Content: req.System})
	}
	body.Messages = append(body.Messages, chatMessages(req.Messages)...)
	if req.MaxTokens > 0 {
		body.Options = &ollamaOptions{NumPredict: req.MaxTokens}
	}
	return o.stream(ctx, o.base+"/api/chat", nil, body, decodeOllama)
}

// decodeOllama разбирает NDJSON Ollama: строка {message:{content}, done}; done=true — конец ответа
// (done_reason "length" — обрезан по num_predict), {error} — сбой посреди ответа.
func decodeOllama(body io.Reader, emit func(string) bool) error {
	sc := newScanner(body)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var ev struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done       bool   `json:"done"`
			DoneReason string `json:"done_reason"`
			Error      string `json:"error"`
		}
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			return fmt.Errorf("разбор строки: %w", err)
		}
		if ev.Error != "" {
			return errors.New("ошибка в потоке: " + ev.Error)
		}
		if ev.Message.Content != "" && !emit(ev.Message.Content) {
			return nil
		}
		if ev.Done {
			if ev.DoneReason == "length" {
				return fmt.Errorf("%w: %s", tutor.ErrTruncated, ev.DoneReason)
			}
			return nil
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return tutor.ErrCutOff
}
