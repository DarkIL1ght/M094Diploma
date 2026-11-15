package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/url"

	"github.com/DarkIL1ght/M094Diploma/internal/app/tutor"
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
func (g *Gemini) Stream(ctx context.Context, req tutor.Request) iter.Seq2[string, error] {
	var body geminiRequest
	for _, m := range req.Messages {
		role := "user"
		if m.Role == tutor.RoleAssistant {
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
	endpoint := g.base + "/v1beta/models/" + url.PathEscape(g.model) + ":streamGenerateContent?alt=sse"
	return g.stream(ctx, endpoint, map[string]string{"x-goog-api-key": g.key}, body, decodeGemini)
}

// geminiFinish переводит finishReason в ошибку неполного ответа; nil — ответ закончен нормально.
// Всё, кроме STOP и MAX_TOKENS (SAFETY, RECITATION, BLOCKLIST, OTHER…), — остановка фильтром.
func geminiFinish(reason string) error {
	switch reason {
	case "STOP":
		return nil
	case "MAX_TOKENS":
		return fmt.Errorf("%w: %s", tutor.ErrTruncated, reason)
	}
	return fmt.Errorf("%w: %s", tutor.ErrBlocked, reason)
}

// decodeGemini разбирает SSE Gemini: в каждом событии — частичный ответ, текст в
// candidates[0].content.parts[].text; finishReason отмечает конец ответа и его причину.
func decodeGemini(body io.Reader, emit func(string) bool) error {
	finished := false
	var finishErr error
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
			return false, fmt.Errorf("%w: запрос заблокирован: %s", tutor.ErrBlocked, ev.PromptFeedback.BlockReason)
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
			finished, finishErr = true, geminiFinish(cand.FinishReason)
		}
		return true, nil
	})
	switch {
	case err != nil:
		return err
	case !finished:
		return tutor.ErrCutOff
	}
	return finishErr
}
