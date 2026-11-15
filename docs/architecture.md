# Архитектура

Сайт подготовки к КТ в магистратуру: Go-бэкенд и фронтенд без сборки. Документ обновляется вместе с кодом.

## Схема

```mermaid
flowchart LR
    browser[Браузер] --> httpapi
    subgraph adapters
        httpapi["httpapi<br/>(роутер — B-02)"]
        llm["llm<br/>Claude · Gemini · Ollama"]
        storage["хранилище (Postgres)"]
    end
    subgraph app[app — сценарии]
        tutor["tutor<br/>порт Provider"]
    end
    subgraph domain[domain — сущности и правила]
        entities[...]
    end
    httpapi --> tutor
    tutor --> entities
    llm -. реализует tutor.Provider .-> tutor
    llm --> providers
    providers[(API провайдеров ИИ)]
```

## Слои

Зависимости направлены внутрь: adapters → app → domain.

| Слой | Пакеты | Ответственность |
|---|---|---|
| domain | `internal/domain/...` | сущности и правила предметной области, без I/O |
| app | `internal/app/...` | сценарии; объявляет маленькие интерфейсы того, что ему нужно снаружи |
| | `internal/app/tutor` | ИИ-репетитор: пока порт к модели — `Request`, `Provider`, ошибки неполного ответа `ErrCutOff`, `ErrTruncated`, `ErrBlocked`; сценарии — B-14 |
| adapters | `internal/adapters/llm` | реализации `tutor.Provider`: потоковые ответы Claude, Gemini, Ollama (B-13) |
| | `internal/adapters/httpapi/...` | HTTP: роутер и сервер (B-02) |
| | хранилище | будет в S-03 |
| запуск | `cmd/...` | сборка зависимостей через конструкторы, конфиг — будет в B-02 |

## Поток запроса

Будет в B-02.

## Где данные

Будет в S-03.

## Ключевые решения

- **LLM через стандартную библиотеку, без SDK.** Три протокола (SSE Anthropic, SSE Gemini, NDJSON Ollama) —
  это сотня строк разбора; SDK добавил бы зависимости и разные модели ошибок. Адрес и `*http.Client`
  передаются в конструктор, поэтому адаптеры тестируются на `httptest.Server`.
- **Интерфейс провайдера объявляет потребитель (B-14).** Порт `tutor.Provider` со
  `Stream(ctx, tutor.Request) iter.Seq2[string, error]` и типы запроса лежат в `internal/app/tutor`,
  адаптеры `llm` их импортируют — зависимость направлена внутрь. HTTP-ошибка провайдера (`llm.StatusError`)
  остаётся в адаптере: статус — деталь транспорта.
- **Неполный ответ — ошибка после текста.** Обрыв потока, лимит длины и остановка фильтром приходят
  последним элементом потока (`ErrCutOff`, `ErrTruncated`, `ErrBlocked`), уже отданный текст не теряется.
