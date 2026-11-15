// Package tutor — сценарии ИИ-репетитора. Пока здесь только порт к языковой модели:
// тип запроса и интерфейс Provider, который реализуют адаптеры internal/adapters/llm.
// Сами сценарии появятся в B-14.
package tutor

import (
	"context"
	"errors"
	"iter"
)

// Role — автор реплики в истории диалога.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message — одна реплика истории.
type Message struct {
	Role Role
	Text string
}

// Request — запрос к модели, общий для всех провайдеров.
type Request struct {
	System    string // системный промпт; пусто — без него
	Messages  []Message
	MaxTokens int // ≤ 0 — по умолчанию провайдера
}

// Provider — языковая модель, отвечающая потоком. Stream отдаёт куски текста;
// ошибка, если есть, приходит последней, уже после отданного текста.
type Provider interface {
	Name() string // имя провайдера для логов
	Stream(ctx context.Context, req Request) iter.Seq2[string, error]
}

// Ошибки, после которых ответ модели неполон. Отданный до них текст остаётся у потребителя:
// ему решать, показать ли его с пометкой.
var (
	ErrCutOff    = errors.New("поток ответа оборвался")           // соединение закрылось до конца ответа
	ErrTruncated = errors.New("ответ обрезан по лимиту длины")    // кончились MaxTokens или контекст модели
	ErrBlocked   = errors.New("ответ остановлен фильтром модели") // отказ модели или фильтр безопасности
)
