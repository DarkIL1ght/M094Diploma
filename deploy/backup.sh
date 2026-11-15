#!/bin/sh
# Ежедневная копия базы (S-14). Запуск из cron на сервере:
#   15 4 * * * $HOME/m094diploma/deploy/backup.sh >>$HOME/m094diploma-backups.log 2>&1
# Копии — в $BACKUP_DIR (по умолчанию ~/m094diploma-backups), формат pg_dump custom, хранятся $KEEP_DAYS дней.
# Compose-файл и env берутся из COMPOSE_FILE / ENV_FILE (по умолчанию compose.prod.yml и .env на сервере).
set -eu
cd "$(dirname "$0")/.."
compose_file="${COMPOSE_FILE:-compose.prod.yml}"
env_file="${ENV_FILE:-.env}"
dir="${BACKUP_DIR:-$HOME/m094diploma-backups}"
keep="${KEEP_DAYS:-14}"
mkdir -p "$dir"
out="$dir/m094-$(date +%F).dump"
# Пишем во временный файл: оборванный дамп не должен выглядеть как готовая копия.
docker compose -f "$compose_file" --env-file "$env_file" exec -T db \
	sh -c 'pg_dump -U "$POSTGRES_USER" --format=custom "$POSTGRES_DB"' >"$out.tmp"
mv "$out.tmp" "$out"
find "$dir" -name 'm094-*.dump' -mtime +"$keep" -delete
echo "$(date '+%F %T') ok $out $(wc -c <"$out") байт"
