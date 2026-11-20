-- S-03. ИИ-наставник (B-14): история чатов, суточный лимит запросов и кэш объяснений ошибок.
-- Контекст чата и ссылки на уроки хранятся как ext_id без внешних ключей: это ссылки на контент из текста
-- ответа, а не связь, которую должна защищать база.

-- +goose Up
create table ai_chats (
    id                 uuid        primary key default uuidv7(),
    session_id         text        not null references sessions on delete cascade,
    title              text        not null check (char_length(title) between 1 and 80),
    -- необязательный контекст: раздел урока или вопрос (и тест или попытка КТ, где он был)
    context_type       text        check (context_type in ('section', 'question')),
    context_id         text,
    context_attempt_id text,
    created_at         timestamptz not null,
    updated_at         timestamptz not null,
    check ((context_type is null) = (context_id is null)),
    check (context_attempt_id is null or context_type = 'question'),
    check (updated_at >= created_at)
);

-- история чатов от новых к старым с подгрузкой (before)
create index ai_chats_session_updated_idx on ai_chats (session_id, updated_at desc);

create table ai_messages (
    id                 uuid        primary key default uuidv7(),
    chat_id            uuid        not null references ai_chats on delete cascade,
    role               text        not null check (role in ('user', 'assistant')),
    text_md            text        not null,
    -- поток оборвался или остановлен: текст неполный
    interrupted        boolean     not null default false,
    lesson_section_ids text[]      not null default '{}',
    created_at         timestamptz not null
);

create index ai_messages_chat_created_idx on ai_messages (chat_id, created_at);

-- Запросы к модели за сутки: остаток лимита (GET /api/ai/limits).
create table ai_usage_daily (
    session_id text    not null references sessions on delete cascade,
    day        date    not null,
    requests   integer not null default 0 check (requests >= 0),
    primary key (session_id, day)
);

-- Кэш «Объяснить ошибку»: одно объяснение на вопрос и набор выбранных вариантов, общее для всех пользователей.
-- sum_hash вопроса входит в ключ: после правки вопроса старое объяснение больше не находится.
create table ai_explanations (
    question_id        bigint      not null references questions on delete cascade,
    question_sum_hash  text        not null,
    -- выбранные варианты, отсортированные по ext_id
    selected           text[]      not null,
    text_md            text        not null,
    lesson_section_ids text[]      not null default '{}',
    created_at         timestamptz not null,
    primary key (question_id, question_sum_hash, selected)
);

-- +goose Down
drop table ai_explanations;
drop table ai_usage_daily;
drop table ai_messages;
drop table ai_chats;
