-- S-03. Пробный КТ (B-08): вариант по формату направления из content/exam_config.json и серверный таймер.
-- Формат (блоки, баллы, пороги, время) в базу не копируется: он в конфиге, а попытка хранит только свой вариант,
-- срок и набранные баллы.

-- +goose Up
create table kt_attempts (
    id          uuid        primary key default uuidv7(),
    session_id  text        not null references sessions on delete cascade,
    track       text        not null check (track in ('research', 'profile')),
    -- expired: время вышло, попытку завершил сервер, ответы сохранены
    status      text        not null default 'active' check (status in ('active', 'finished', 'expired')),
    started_at  timestamptz not null,
    -- серверный таймер: оставшееся время = deadline_at - now()
    deadline_at timestamptz not null,
    -- итог; max_points — из конфига на момент попытки, чтобы история не менялась вместе с форматом
    finished_at timestamptz,
    total       integer     check (total >= 0),
    max_points  integer     not null check (max_points > 0),
    passed      boolean,
    xp_awarded  integer     check (xp_awarded >= 0),
    check (deadline_at > started_at),
    check (finished_at >= started_at),
    check (total <= max_points),
    check (case status
               when 'active' then num_nonnulls(finished_at, total, passed, xp_awarded) = 0
               else num_nulls(finished_at, total, passed, xp_awarded) = 0
           end)
);

-- Одна незавершённая попытка на сессию: вторая — 409 attempt_active.
create unique index kt_attempts_one_active_idx on kt_attempts (session_id) where status = 'active';
-- история попыток от новых к старым и «предыдущая попытка этого направления»
create index kt_attempts_history_idx on kt_attempts (session_id, track, started_at desc);

-- Задания варианта. position — сквозной номер, блок — предмет вопроса (составной внешний ключ это проверяет).
create table kt_tasks (
    attempt_id  uuid         not null references kt_attempts on delete cascade,
    position    smallint     not null check (position >= 1),
    block       subject_code not null,
    question_id bigint       not null,
    selected    text[]       not null default '{}',
    flagged     boolean      not null default false,
    -- прослушивания аудирования; лимит (rules.listen_max_plays) проверяет приложение
    plays       smallint     not null default 0 check (plays >= 0),
    -- баллы за задание, заполняются при завершении (в «Базах данных» 0, 1 или 2)
    points      smallint     check (points >= 0),
    primary key (attempt_id, position),
    unique (attempt_id, question_id),
    foreign key (question_id, block) references questions (id, subject) on delete restrict
);

create index kt_tasks_question_id_idx on kt_tasks (question_id);

-- +goose Down
drop table kt_tasks;
drop table kt_attempts;
