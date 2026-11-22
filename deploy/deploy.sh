#!/bin/sh
# S-13. Выкатка окружения на сервере: тянет образ, перезапускает и ждёт healthy. Не поднялось — возвращает
# прежнюю версию и выходит с ошибкой. Запускает CI по SSH (.github/workflows/deploy.yml); руками — для отката:
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
# Имя app в общей сети Caddy выводится из каталога (= compose-проекта), поэтому у staging и prod оно не совпадёт.
export APP_ALIAS="$(basename "$PWD")-app"
compose() { docker compose -f compose.yml --env-file .env "$@"; }

# Версия, на которую вернёмся, если новая не поднимется. Пусто — это первая выкатка.
prev=
cid=$(compose ps --quiet m094-app)
if [ -n "$cid" ]; then
	prev=$(docker inspect --format '{{.Config.Image}}' "$cid")
	prev=${prev##*:}
fi

compose pull --quiet
# Копия до выкатки: если новая версия испортит данные, есть состояние прямо перед ней. На первой выкатке базы ещё нет.
if [ "$backup" = yes ]; then
	if [ -n "$(compose ps --status running --quiet db)" ]; then
		deploy/backup.sh
	else
		echo "база ещё не запущена — копию пропускаю"
	fi
fi
# Миграции (S-03) — новым образом до запуска новой версии app. Не применились — выкатку не начинаем.
# Возврат прежней версии ниже схему базы не откатывает: миграции должны быть совместимы с предыдущей версией app.
compose up -d --wait db
if ! compose run --rm --no-deps m094-app migrate up; then
	echo "миграции для $tag не применились — выкатку не начинаю" >&2
	exit 1
fi
# Контент (S-05) — из seed в образе, после миграций: одна транзакция, при ошибке база остаётся как была.
if ! compose run --rm --no-deps m094-app seed; then
	echo "контент для $tag не загрузился — выкатку не начинаю" >&2
	exit 1
fi
if ! compose up -d --wait --wait-timeout 180 --remove-orphans; then
	compose ps
	compose logs --no-color --tail=100 m094-app
	echo "выкатка $tag не поднялась до healthy" >&2
	if [ -n "$prev" ] && [ "$prev" != "$tag" ]; then
		echo "возвращаю прежнюю версию $prev" >&2
		export IMAGE_TAG="$prev"
		compose up -d --wait --wait-timeout 180 || echo "прежняя версия $prev тоже не поднялась" >&2
	fi
	exit 1
fi
echo "$(date '+%F %T') выкачено: $(basename "$PWD") $tag"
