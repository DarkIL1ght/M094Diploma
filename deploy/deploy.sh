#!/bin/sh
# S-13. Выкатка окружения на сервере: тянет образ, перезапускает и ждёт healthy, иначе выходит с ошибкой.
# Запускает CI по SSH (.github/workflows/deploy.yml); руками — для отката на предыдущий тег:
#   ~/m094diploma/deploy/deploy.sh --backup v1.2.0        # prod: сначала копия базы (deploy/backup.sh)
#   ~/m094diploma-staging/deploy/deploy.sh sha-1a2b3c4     # staging
# Каталог окружения — родитель deploy/: в нём compose.yml и .env (docs/server.md).
set -eu
cd "$(dirname "$0")/.."
backup=no
if [ "${1:-}" = --backup ]; then
	backup=yes
	shift
fi
tag="${1:?нужен тег образа: sha-<коммит>, main, vX.Y.Z или latest}"
[ -f .env ] || { echo "нет $PWD/.env — создай из .env.example (docs/server.md)" >&2; exit 1; }
# Тег из аргумента важнее IMAGE_TAG из .env: compose берёт переменную окружения раньше файла.
export IMAGE_TAG="$tag"
compose() { docker compose -f compose.yml --env-file .env "$@"; }

compose pull --quiet
# Копия до выкатки: если новая версия испортит данные, есть состояние прямо перед ней. На первой выкатке базы ещё нет.
if [ "$backup" = yes ]; then
	if [ -n "$(compose ps --status running --quiet db)" ]; then
		deploy/backup.sh
	else
		echo "база ещё не запущена — копию пропускаю"
	fi
fi
# TODO S-03: применить миграции до запуска новой версии app, когда в образе появится способ (goose), например:
#   compose up -d --wait db
#   compose run --rm app migrate up
if ! compose up -d --wait --wait-timeout 180 --remove-orphans; then
	compose ps
	compose logs --no-color --tail=100 app
	echo "выкатка $tag не поднялась до healthy" >&2
	exit 1
fi
echo "$(date '+%F %T') выкачено: $(basename "$PWD") $tag"
