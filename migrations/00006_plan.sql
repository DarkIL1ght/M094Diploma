-- S-03. План до КТ (B-12): задачи по дням от сегодня до sessions.exam_date. Пересчёт плана заменяет
-- невыполненные задачи, выполненные остаются.

-- +goose Up
create table plan_tasks (
    id         uuid        primary key default uuidv7(),
    session_id text        not null references sessions on delete cascade,
    day        date        not null,
    position   smallint    not null check (position >= 1),
    kind       text        not null check (kind in ('lesson', 'test', 'review', 'kt')),
    title      text        not null,
    -- куда вести по нажатию: ext_id раздела или темы; у practice и kt идентификатора нет
    ref_type   text        check (ref_type in ('section', 'topic', 'practice', 'kt')),
    ref_id     text,
    done_at    timestamptz,
    check (ref_id is null or ref_type is not null),
    -- план по дням (GET /api/plan) идёт по этому индексу; проверка в конце транзакции, чтобы пересчёт плана
    -- мог переставлять задачи без временных номеров
    unique (session_id, day, position) deferrable initially deferred
);

-- +goose Down
drop table plan_tasks;
