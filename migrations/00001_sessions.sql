-- S-03. Анонимные сессии (B-03) и настройки пользователя. Сессия — это и есть пользователь: регистрации нет.
-- Хранит всё, что нужно порту session.Store; токены и ключ восстановления — только SHA-256.

-- +goose Up
create table sessions (
    id                 text        primary key,
    display_name       text        not null,
    created_at         timestamptz not null,
    -- действующий ключ восстановления; новый ключ заменяет прежний, поэтому одна пара полей, а не таблица
    recovery_key_hash  bytea       unique check (octet_length(recovery_key_hash) = 32),
    recovery_issued_at timestamptz,
    -- настройки (PATCH /api/me/settings); сброс прогресса их не трогает
    onboarded          boolean     not null default false,
    theme              text        not null default 'system' check (theme in ('light', 'dark', 'system')),
    accent             text        not null default 'accent-1',
    pattern            text        not null default 'pattern-1',
    avatar_frame       text,
    focus_mode         boolean     not null default false,
    exam_date          date,
    daily_goal_type    text        not null default 'minutes' check (daily_goal_type in ('minutes', 'xp')),
    daily_goal_value   integer     not null default 30 check (daily_goal_value between 5 and 600),
    check ((recovery_key_hash is null) = (recovery_issued_at is null))
);

comment on table sessions is 'Анонимный пользователь: сессия, ключ восстановления и настройки';
comment on column sessions.accent is 'Код косметики из каталога в коде; открыт ли он, проверяет приложение';

-- У сессии может быть несколько токенов: каждый браузер, подключённый ключом восстановления, получает свой.
create table session_tokens (
    token_hash   bytea       primary key check (octet_length(token_hash) = 32),
    session_id   text        not null references sessions on delete cascade,
    created_at   timestamptz not null,
    last_seen_at timestamptz not null
);

-- поиск токенов сессии (очистка сессий без токенов, каскадное удаление)
create index session_tokens_session_id_idx on session_tokens (session_id);
-- очистка: токены, которыми не пользовались с момента before
create index session_tokens_last_seen_at_idx on session_tokens (last_seen_at);

-- +goose Down
drop table session_tokens;
drop table sessions;
