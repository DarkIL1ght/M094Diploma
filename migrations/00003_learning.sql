-- S-03. Учёба и тесты: отметки «Изучено» (B-05), тесты по теме, работа над ошибками и смешанный тест (B-06),
-- счётчики ответов по вопросам для списка ошибок и «карты дыр» (B-07).
-- Всё, что принадлежит пользователю, удаляется вместе с сессией (on delete cascade).

-- +goose Up
-- Строка остаётся и после снятия отметки: так видно, что XP за раздел уже начислен (начисляется один раз).
create table section_marks (
    session_id       text        not null references sessions on delete cascade,
    section_id       bigint      not null references sections on delete cascade,
    studied          boolean     not null,
    first_studied_at timestamptz not null,
    updated_at       timestamptz not null,
    primary key (session_id, section_id)
);

create index section_marks_section_id_idx on section_marks (section_id);

create table test_attempts (
    id             uuid        primary key default uuidv7(),
    session_id     text        not null references sessions on delete cascade,
    mode           text        not null check (mode in ('topic', 'mistakes', 'mixed')),
    topic_id       bigint      references topics on delete restrict,
    title          text        not null,
    status         text        not null default 'active' check (status in ('active', 'finished')),
    started_at     timestamptz not null,
    -- итог (POST /api/tests/{id}/finish): повторный вызов отдаёт сохранённое
    finished_at    timestamptz,
    correct        integer     check (correct >= 0),
    xp_awarded     integer     check (xp_awarded >= 0),
    mistakes_added integer     check (mistakes_added >= 0),
    check ((mode = 'topic') = (topic_id is not null)),
    check (case status
               when 'active' then num_nonnulls(finished_at, correct, xp_awarded, mistakes_added) = 0
               else num_nulls(finished_at, correct, xp_awarded, mistakes_added) = 0
           end),
    check (finished_at >= started_at)
);

-- Не больше одного незавершённого теста одного режима и темы: POST /api/tests возвращает его, а не создаёт новый.
-- nulls not distinct — у режимов mistakes и mixed темы нет, и NULL должен считаться одинаковым.
create unique index test_attempts_one_active_idx on test_attempts (session_id, mode, topic_id) nulls not distinct
    where status = 'active';
-- внешний ключ на сессию (каскад при очистке) и тесты пользователя от новых к старым
create index test_attempts_session_started_idx on test_attempts (session_id, started_at desc);
create index test_attempts_topic_id_idx on test_attempts (topic_id);

-- Вопросы теста в порядке показа и выбранные ответы. Вопрос из истории тестов удалить нельзя (restrict).
create table test_questions (
    attempt_id  uuid     not null references test_attempts on delete cascade,
    position    smallint not null check (position >= 1),
    question_id bigint   not null references questions on delete restrict,
    -- ext_id выбранных вариантов; пустой массив — ответа нет
    selected    text[]   not null default '{}',
    -- итог по вопросу: null, пока тест идёт; при завершении true или false (без ответа — false)
    is_correct  boolean,
    primary key (attempt_id, position),
    unique (attempt_id, question_id)
);

create index test_questions_question_id_idx on test_questions (question_id);

-- Счётчики ответов пользователя по вопросу. Ошибок = answered - correct.
-- Вопрос в списке ошибок, пока ошибки есть и correct_streak меньше required_streak (2, задаёт приложение).
-- Точность темы для «карты дыр» — sum(correct) / sum(answered) по вопросам темы.
create table question_progress (
    session_id       text        not null references sessions on delete cascade,
    question_id      bigint      not null references questions on delete cascade,
    answered         integer     not null check (answered >= 1),
    correct          integer     not null check (correct between 0 and answered),
    correct_streak   integer     not null check (correct_streak between 0 and correct),
    last_answered_at timestamptz not null,
    primary key (session_id, question_id)
);

create index question_progress_question_id_idx on question_progress (question_id);

-- +goose Down
drop table question_progress;
drop table test_questions;
drop table test_attempts;
drop table section_marks;
