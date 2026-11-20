-- S-03. Учебный контент: темы, разделы (уроки), вопросы с вариантами и общий материал к группам вопросов.
-- Пишет только загрузчик (S-05): upsert по ext_id, строку обновляет, только если изменился sum_hash.
-- Внутренние ключи — bigint: на них ссылается прогресс пользователей. Наружу (API) отдаётся ext_id.
-- Каталоги предметов, достижений и косметики — в коде и content/*.json, в базе только их коды.

-- +goose Up
create domain subject_code as text
    check (value in ('english', 'tgo', 'algorithms', 'databases'));

create table topics (
    id       bigint       generated always as identity primary key,
    ext_id   text         not null unique,
    subject  subject_code not null,
    title    text         not null,
    position integer      not null check (position >= 1),
    sum_hash text         not null,
    -- цель составного внешнего ключа: предмет вопроса совпадает с предметом его темы
    unique (id, subject)
);

-- темы предмета по порядку (GET /api/subjects/{subject}/topics)
create index topics_subject_position_idx on topics (subject, position);

create table sections (
    id       bigint  generated always as identity primary key,
    ext_id   text    not null unique,
    topic_id bigint  not null references topics on delete restrict,
    title    text    not null,
    position integer not null check (position >= 1),
    body_md  text    not null,
    sum_hash text    not null
);

-- разделы темы по порядку: оглавление, соседние разделы
create index sections_topic_position_idx on sections (topic_id, position);

-- Общий текст или таблица к группе вопросов; вопросы группы идут в тесте подряд.
create table shared_materials (
    id       bigint generated always as identity primary key,
    ext_id   text   not null unique,
    body_md  text   not null,
    sum_hash text   not null
);

create table questions (
    id                 bigint       generated always as identity primary key,
    ext_id             text         not null unique,
    subject            subject_code not null,
    -- у английского и ТГО вопрос может быть без темы
    topic_id           bigint,
    kind               text         not null check (kind in ('single', 'multiple')),
    text_md            text         not null,
    code_lang          text,
    code_source        text,
    shared_material_id bigint       references shared_materials on delete restrict,
    explanation_md     text         not null,
    -- раздел урока для кнопки «Открыть урок»; урок убрали — ссылка просто пропадает
    section_id         bigint       references sections on delete set null,
    -- текст аудирования (английский): браузер озвучивает его сам
    listen_text        text,
    listen_lang        text,
    sum_hash           text         not null,
    -- цель составного ключа из kt_tasks: блок КТ совпадает с предметом вопроса
    unique (id, subject),
    foreign key (topic_id, subject) references topics (id, subject) on delete restrict,
    check ((code_lang is null) = (code_source is null)),
    check ((listen_text is null) = (listen_lang is null))
);

-- подбор вопросов теста по теме и смешанного теста по предметам
create index questions_topic_id_idx on questions (topic_id);
create index questions_subject_idx on questions (subject);
create index questions_shared_material_id_idx on questions (shared_material_id);
create index questions_section_id_idx on questions (section_id);

-- Варианты ответа — часть вопроса: заменяются и удаляются вместе с ним.
create table question_options (
    question_id bigint   not null references questions on delete cascade,
    ext_id      text     not null,
    position    smallint not null check (position >= 1),
    text_md     text     not null,
    is_correct  boolean  not null,
    primary key (question_id, ext_id),
    unique (question_id, position)
);

-- +goose Down
drop table question_options;
drop table questions;
drop table shared_materials;
drop table sections;
drop table topics;
drop domain subject_code;
