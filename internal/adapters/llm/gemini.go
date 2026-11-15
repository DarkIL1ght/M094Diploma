package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
)

// Gemini — адаптер к Gemini API (generativelanguage.googleapis.com).
type Gemini struct{ client }

// NewGemini создаёт адаптер; по умолчанию модель gemini-2.5-flash.
func NewGemini(c Config) *Gemini {
	return &Gemini{newClient("gemini", c, "https://generativelanguage.googleapis.com", "gemini-2.5-flash")}
}

type geminiPart struct {
	Text    string `json:"text"`
	Thought bool   `json:"thought,omitempty"` // «мысли» thinking-моделей ученику не показываем
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type geminiRequest struct {
	Contents          []geminiContent         `json:"contents"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

// Stream отправляет запрос в :streamGenerateContent?alt=sse и отдаёт текст ответа по мере генерации.
func (g *Gemini) Stream(ctx context.Context, req Request) iter.Seq2[string, error] {
	var body geminiRequest
	for _, m := range req.Messages {
		role := "user"
		if m.Role == RoleAssistant {
			role = "model" // у Gemini ответы модели — роль model
		}
		body.Contents = append(body.Contents, geminiContent{Role: role, Parts: []geminiPart{{Text: m.Text}}})
	}
	if req.System != "" {
		body.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: req.System}}}
	}
	if req.MaxTokens > 0 {
		body.GenerationConfig = &geminiGenerationConfig{MaxOutputTokens: req.MaxTokens}
	}
	url := g.base + "/v1beta/models/" + g.model + ":streamGenerateContent?alt=sse"
	return g.stream(ctx, url, map[string]string{"x-goog-api-key": g.key}, body, decodeGemini)
}

// decodeGemini разбирает SSE Gemini: в каждом событии — частичный ответ, текст в
// candidates[0].content.parts[].text; finishReason отмечает конец ответа.
func decodeGemini(body io.Reader, emit func(string) bool) error {
	finished := false
	err := readSSE(body, func(data []byte) (bool, error) {
		var ev struct {
			Candidates []struct {
				Content      geminiContent `json:"content"`
				FinishReason string        `json:"finishReason"`
			} `json:"candidates"`
			PromptFeedback struct {
				BlockReason string `json:"blockReason"`
			} `json:"promptFeedback"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &ev); err != nil {
			return false, fmt.Errorf("разбор события: %w", err)
		}
		if ev.Error != nil {
			return false, fmt.Errorf("ошибка в потоке: %d: %s", ev.Error.Code, ev.Error.Message)
		}
		if ev.PromptFeedback.BlockReason != "" {
			return false, fmt.Errorf("запрос заблокирован: %s", ev.PromptFeedback.BlockReason)
		}
		if len(ev.Candidates) == 0 {
			return true, nil
		}
		cand := ev.Candidates[0]
		for _, p := range cand.Content.Parts {
			if p.Thought || p.Text == "" {
				continue
			}
			if !emit(p.Text) {
				return false, nil
			}
		}
		if cand.FinishReason != "" {
			finished = true
		}
		return true, nil
	})
	if err == nil && !finished {
		return ErrCutOff
	}
	return err
}
