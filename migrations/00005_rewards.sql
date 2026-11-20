-- S-03. Мотивация: XP, серия, задания дня и сундук (B-09), достижения, косметика и уведомления (B-10),
-- лига недели (B-11) — рейтинг по xp_daily за неделю, отдельной таблицы у неё нет.
-- Уровень считается из XP формулой в коде, каталоги достижений, заданий и косметики — тоже в коде.

-- +goose Up
-- Счётчики пользователя для GET /api/me. Инвариант: xp = sum(xp_daily.xp), приложение меняет их в одной транзакции.
create table progress (
    session_id     text    primary key references sessions on delete cascade,
    xp             bigint  not null default 0 check (xp >= 0),
    streak_days    integer not null default 0 check (streak_days >= 0),
    best_streak    integer not null default 0 check (best_streak >= streak_days),
    -- последний день, засчитанный в серию: по нему видно, продолжается она или прервалась
    last_active_on date
);

-- XP и минуты занятий по дням: графики профиля, цель дня, лига недели.
create table xp_daily (
    session_id text    not null references sessions on delete cascade,
    day        date    not null,
    xp         integer not null default 0 check (xp >= 0),
    minutes    integer not null default 0 check (minutes >= 0),
    primary key (session_id, day)
);

-- лига: сумма XP всех пользователей за дни недели
create index xp_daily_day_idx on xp_daily (day) include (session_id, xp);

-- Задания дня выбираются при первом запросе дня и дальше не меняются.
create table daily_tasks (
    session_id text        not null references sessions on delete cascade,
    day        date        not null,
    position   smallint    not null check (position >= 1),
    -- код задания из каталога в коде, например solve_questions
    kind       text        not null,
    target     integer     not null check (target >= 1),
    progress   integer     not null default 0 check (progress >= 0),
    xp         integer     not null check (xp >= 0),
    done_at    timestamptz,
    primary key (session_id, day, position),
    unique (session_id, day, kind)
);

-- Открытый сундук дня. Нет строки — сундук закрыт или готов (это видно по заданиям дня).
create table daily_chests (
    session_id  text        not null references sessions on delete cascade,
    day         date        not null,
    opened_at   timestamptz not null,
    xp          integer     not null check (xp >= 0),
    cosmetic_id text,
    primary key (session_id, day)
);

-- Полученные уровни достижений I–V. Нет строки — достижение не получено; прогресс считается из статистики.
create table achievements (
    session_id     text        not null references sessions on delete cascade,
    achievement_id text        not null,
    level          smallint    not null check (level between 1 and 5),
    unlocked_at    timestamptz not null,
    updated_at     timestamptz not null,
    primary key (session_id, achievement_id)
);

-- Открытая косметика (из сундука, за достижение или уровень).
create table cosmetic_unlocks (
    session_id  text        not null references sessions on delete cascade,
    cosmetic_id text        not null,
    unlocked_at timestamptz not null,
    primary key (session_id, cosmetic_id)
);

-- Награды, ещё не показанные пользователю (тосты и окно достижения).
create table notifications (
    id                uuid        primary key default uuidv7(),
    session_id        text        not null references sessions on delete cascade,
    type              text        not null
        check (type in ('xp', 'level_up', 'achievement', 'chest_ready', 'cosmetic_unlocked')),
    created_at        timestamptz not null,
    seen_at           timestamptz,
    xp                integer,
    level             integer,
    achievement_id    text,
    achievement_level smallint check (achievement_level between 1 and 5),
    cosmetic_id       text,
    check (type not in ('xp', 'level_up') or xp is not null),
    check (type <> 'level_up' or level is not null),
    check (type <> 'achievement' or (achievement_id is not null and achievement_level is not null)),
    check (type <> 'cosmetic_unlocked' or cosmetic_id is not null)
);

-- внешний ключ на сессию и события сессии по порядку появления (непоказанные — фильтром по seen_at)
create index notifications_session_created_idx on notifications (session_id, created_at);

-- +goose Down
drop table notifications;
drop table cosmetic_unlocks;
drop table achievements;
drop table daily_chests;
drop table daily_tasks;
drop table xp_daily;
drop table progress;
