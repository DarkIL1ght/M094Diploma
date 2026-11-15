#!/bin/sh
# Проверка, что копия восстанавливается (S-14): поднимает временный Postgres 18,
# заливает в него дамп и считает таблицы. Рабочую базу не трогает.
#   deploy/restore-check.sh                 # последний дамп из ~/m094diploma-backups
#   deploy/restore-check.sh path/to.dump    # конкретный файл
# Запускать раз в неделю из cron или руками после изменений схемы.
set -eu
dir="${BACKUP_DIR:-$HOME/m094diploma-backups}"
dump="${1:-$(ls -1t "$dir"/m094-*.dump 2>/dev/null | head -n 1)}"
[ -n "$dump" ] && [ -f "$dump" ] || { echo "нет дампа для проверки" >&2; exit 1; }

name="m094-restore-check-$$"
trap 'docker rm -f "$name" >/dev/null 2>&1 || true' EXIT
docker run -d --name "$name" -e POSTGRES_PASSWORD=check -e POSTGRES_DB=check postgres:18 >/dev/null
i=0
until docker exec "$name" pg_isready -U postgres -d check >/dev/null 2>&1; do
	i=$((i + 1))
	[ "$i" -lt 60 ] || { echo "временный Postgres не поднялся" >&2; exit 1; }
	sleep 1
done

docker exec -i "$name" pg_restore -U postgres -d check --no-owner --exit-on-error <"$dump"
tables=$(docker exec "$name" psql -U postgres -d check -tAc \
	"select count(*) from information_schema.tables where table_schema = 'public'")
[ "$tables" -gt 0 ] || { echo "в копии $dump нет таблиц" >&2; exit 1; }
echo "$(date '+%F %T') восстановление ok: $dump, таблиц: $tables"
