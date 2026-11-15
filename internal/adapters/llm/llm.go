// Package llm — адаптеры к языковым моделям: Claude (Anthropic API), Gemini API и Ollama.
// Только стандартная библиотека, без SDK. Каждый адаптер отдаёт ответ потоком кусков текста:
//
//	for chunk, err := range provider.Stream(ctx, req) {
//		if err != nil {
//			return err // поток закончен: ошибка приходит последней
//		}
//		send(chunk)
//	}
//
// Адаптеры реализуют порт tutor.Provider: тип запроса и ошибки неполного ответа объявляет потребитель
// (internal/app/tutor), здесь — только протоколы провайдеров.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"

	"github.com/DarkIL1ght/M094Diploma/internal/app/tutor"
)

// Config — настройки адаптера. Пустые поля заменяются значениями по умолчанию провайдера.
type Config struct {
	BaseURL string
	APIKey  string // у Ollama не нужен
	Model   string
	// HTTPClient — nil означает http.DefaultClient: у него нет таймаута, запрос ограничивает только ctx.
	// Таймаут на весь запрос (Client.Timeout) оборвёт длинный ответ — лучше ResponseHeaderTimeout в Transport.
	HTTPClient *http.Client
}

const (
	defaultMaxTokens = 4096
	maxErrorBody     = 512 // столько байт тела ответа с ошибкой попадает в StatusError
)

// StatusError — провайдер ответил не-2xx. Body — начало тела ответа, обрезанное до maxErrorBody.
// Тип остаётся в адаптере: HTTP-статус — деталь транспорта, сценариям он не нужен
// (понятное сообщение по статусу при надобности строит HTTP-слой через errors.As).
type StatusError struct {
	Provider string
	Status   int
	Body     string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", e.Provider, e.Status, e.Body)
}

// chatMessage — реплика в формате {role, content}: так её ждут Anthropic и Ollama.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func chatMessages(msgs []tutor.Message) []chatMessage {
	out := make([]chatMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, chatMessage{Role: string(m.Role), Content: m.Text})
	}
	return out
}

// client — общая часть адаптеров: адрес, ключ, модель и HTTP-клиент.
type client struct {
	name, base, key, model string
	hc                     *http.Client
}

func newClient(name string, c Config, defaultBase, defaultModel string) client {
	cl := client{name: name, base: strings.TrimRight(c.BaseURL, "/"), key: c.APIKey, model: c.Model, hc: c.HTTPClient}
	if cl.base == "" {
		cl.base = defaultBase
	}
	if cl.model == "" {
		cl.model = defaultModel
	}
	if cl.hc == nil {
		cl.hc = http.DefaultClient
	}
	return cl
}

// Name — имя провайдера для логов.
func (c client) Name() string { return c.name }

// decoder достаёт куски текста из тела ответа и отдаёт их в emit. emit возвращает false,
// когда потребитель больше не читает, — тогда decoder должен сразу вернуться.
type decoder func(body io.Reader, emit func(string) bool) error

// stream отправляет запрос и превращает разбор ответа в итератор. Ошибка, если есть, приходит последней.
func (c client) stream(ctx context.Context, url string, headers map[string]string, body any, decode decoder) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		resp, err := c.post(ctx, url, headers, body)
		if err != nil {
			yield("", err)
			return
		}
		defer resp.Body.Close()
		stopped := false
		err = decode(resp.Body, func(s string) bool {
			stopped = !yield(s, nil)
			return !stopped
		})
		if err == nil || stopped {
			return
		}
		if ctx.Err() != nil {
			err = ctx.Err() // оборванное отменой чтение — это отмена, а не сбой провайдера
		}
		yield("", fmt.Errorf("%s: %w", c.name, err))
	}
}

func (c client) post(ctx context.Context, url string, headers map[string]string, body any) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", c.name, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", c.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, fmt.Errorf("%s: %w", c.name, err) // ключ идёт заголовком, в URL и тексте ошибки его нет
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		return nil, &StatusError{Provider: c.name, Status: resp.StatusCode, Body: readErrorBody(resp.Body)}
	}
	return resp, nil
}

// readErrorBody читает начало тела ответа с ошибкой, не разрезая UTF-8 посередине символа.
func readErrorBody(r io.Reader) string {
	raw, _ := io.ReadAll(io.LimitReader(r, maxErrorBody+1))
	if len(raw) <= maxErrorBody {
		return strings.TrimSpace(string(raw))
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(raw[:maxErrorBody]), "")) + "…"
}

// readSSE разбирает поток Server-Sent Events и отдаёт fn данные каждого события
// (несколько строк data: склеиваются через \n). fn возвращает false, чтобы прекратить чтение.
func readSSE(r io.Reader, fn func(data []byte) (bool, error)) error {
	sc := newScanner(r)
	var data []byte
	pending := false
	flush := func() (bool, error) {
		if !pending {
			return true, nil
		}
		more, err := fn(data)
		data, pending = data[:0], false
		return more, err
	}
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			if more, err := flush(); !more || err != nil {
				return err
			}
			continue
		}
		if v, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			if pending {
				data = append(data, '\n')
			}
			data = append(data, bytes.TrimPrefix(v, []byte(" "))...)
			pending = true
		}
		// event:, id:, retry: и комментарии (":…") не нужны: тип события есть в самих данных.
	}
	if err := sc.Err(); err != nil {
		return err
	}
	_, err := flush()
	return err
}

// newScanner читает построчно; строка события может быть длинной, поэтому буфер до 1 МБ.
func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	return sc
}
