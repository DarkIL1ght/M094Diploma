-- B-04. Очистка пустых сессий (Store.DeleteUnused, #93) ищет сессии старше суток по created_at.

-- +goose Up
create index sessions_created_at_idx on sessions (created_at);

-- +goose Down
drop index sessions_created_at_idx;
