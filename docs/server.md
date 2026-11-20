# Сервер

Проверено 2026-11-15 (S-11). Секреты (ключи, пароли, токены) сюда не пишем: они на сервере в `.env` и в GitHub Secrets (S-17).

## VM

| Что | Значение |
|---|---|
| Облако | Oracle Cloud, Always Free, Ampere A1 |
| Архитектура | ARM64 (aarch64) — образы собираем под `linux/arm64` (S-13) |
| Ресурсы | 2 OCPU, 12 ГБ RAM, 48 ГБ диск |
| ОС | Ubuntu 24.04, часовой пояс Asia/Almaty |
| Docker | Docker Engine 29 + Compose v5 |
| Пользователь | `ubuntu` |

VM общая: на ней уже работают другие проекты Дархана, каждый своим compose-проектом. HTTPS для всех отдаёт один общий Caddy (Let's Encrypt), сайты различаются поддоменами `*.130-61-149-209.sslip.io`. Диплом встаёт рядом так же: свой compose-проект `m094diploma` (app + Postgres), наружу ничего не публикует, Caddy проксирует на app по внутренней сети.

## Доступ

- SSH только по ключу: `PasswordAuthentication no`, `PubkeyAuthentication yes`. Ключ — у Дархана; остальным доступ на сервер не нужен, выкатка идёт через CI (S-13).
- Публичный IP пока эфемерный: при пересоздании VM он сменится вместе с адресом sslip.io. Перед защитой стоит закрепить Reserved IP или завести домен.

## Сеть

Порты открыты в двух местах — без любого из них снаружи не достучаться:

1. Security List подсети в консоли Oracle: Ingress TCP 22, 80, 443 от `0.0.0.0/0`.
2. iptables образа (`netfilter-persistent` сохраняет правила):

```
-A INPUT -p tcp -m multiport --dports 80,443 -j ACCEPT   # стоит до REJECT
-A INPUT -p udp --dport 443 -j ACCEPT                    # HTTP/3
-A INPUT -p tcp --dport 22 -m state --state NEW -j ACCEPT
-A INPUT -j REJECT --reject-with icmp-host-prohibited
```

ufw не включать: он конфликтует с правилами образа Oracle.

## Секреты (S-17)

В git секретов нет: пароли и ключи лежат в двух местах — `.env` на сервере и GitHub Secrets. Образец со всеми
переменными и комментариями — `.env.example` в корне репо; `.env` и `.env.*` (кроме образца) в `.gitignore`.
CI-задание «Секреты в git (gitleaks)» проверяет всю историю на каждом PR; ложные срабатывания — в `.gitleaks.toml`.

**`.env` на сервере** — свой у каждого окружения, в каталоге compose-проекта:

| Окружение | Файл |
|---|---|
| prod | `/home/ubuntu/m094diploma/.env` |
| staging | `/home/ubuntu/m094diploma-staging/.env` |

Создаёт Дархан руками из `.env.example`, владелец `ubuntu`, права `600`:

```
install -m 600 /dev/null ~/m094diploma/.env   # пустой файл сразу с нужными правами
nano ~/m094diploma/.env                       # значения с пометкой «сервер» из .env.example
```

CI `.env` не читает и не перезаписывает: при выкатке копируются только `compose.yml` и скрипты из `deploy/`.

**GitHub Secrets** (Settings → Secrets and variables → Actions, уровень репозитория) — для выкатки S-13, заводит Дархан:

| Секрет | Что |
|---|---|
| `SSH_HOST` | адрес VM (сейчас `130.61.149.209`) |
| `SSH_USER` | `ubuntu` |
| `SSH_KEY` | закрытый ключ отдельной пары только для CI (`ssh-keygen -t ed25519 -C m094diploma-ci -N ""`); открытый — строкой в `~/.ssh/authorized_keys` на сервере |
| `SSH_KNOWN_HOSTS` | ключ хоста: вывод `ssh-keyscan -t ed25519 130.61.149.209`, сверенный с `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub` на самом сервере |

Токен для GHCR не нужен: CI пушит образ встроенным `GITHUB_TOKEN`, а сервер тянет публичный пакет без входа.

**Ротация.** Пароль базы: `ALTER USER m094 PASSWORD '…'` в `psql` контейнера db, затем новое значение в `.env`
и `docker compose up -d` (app пересоздастся с новым `DATABASE_URL`). Ключ CI: новая пара, новый открытый ключ
в `authorized_keys`, новый закрытый в `SSH_KEY`, старую строку из `authorized_keys` удалить. Ключ ИИ-провайдера —
выпустить новый в консоли провайдера, заменить в `.env`, `docker compose up -d`, старый отозвать. Если секрет
попал в git — он считается утёкшим: сначала ротация, потом уже чистка истории (если вообще нужна).

## Выкатка (S-13)

Два окружения на одной VM, один `deploy/compose.yml`, разные каталоги и `.env`. Имя compose-проекта = имя каталога,
поэтому контейнеры, сеть и том базы у окружений свои.

| Окружение | Когда выкатывается | Каталог | Compose-проект | Теги образа |
|---|---|---|---|---|
| staging | каждый пуш в `main` после зелёного CI | `~/m094diploma-staging` | `m094diploma-staging` | `sha-<7 знаков коммита>`, `main` |
| prod | тег `vX.Y.Z` на коммите `main` с зелёным CI | `~/m094diploma` | `m094diploma` | `vX.Y.Z`, `latest` |

Как идёт (`.github/workflows/deploy.yml`): образ `ghcr.io/darkil1ght/m094diploma` собирается под `linux/arm64`
(кэш слоёв GitHub Actions) и пушится в GHCR; по SSH на сервер копируются `compose.yml` и скрипты `deploy/`,
затем `deploy/deploy.sh <тег>`: `docker compose pull`, `up -d --wait` — ждёт healthy у db и app (healthcheck образа),
иначе печатает логи app и роняет задание. Перед prod `deploy.sh --backup` сначала делает копию базы `deploy/backup.sh`
(в `~/m094diploma-backups`). Наружу compose ничего не публикует: app подключён к сети общего Caddy (`CADDY_NETWORK`)
под именем `APP_ALIAS`, база — только к своей сети проекта. Миграции базы встанут в `deploy.sh` с S-03 (там TODO).

Выкатить prod: `git tag v1.2.0 <коммит main> && git push origin v1.2.0` — когда CI на этом коммите уже зелёный.

**Откат** — выкатить предыдущий тег: в Actions открыть запуск Deploy нужного тега и нажать Re-run all jobs
(он пересоберёт образ из того коммита и вернёт ему `latest`), или сразу на сервере:

```
~/m094diploma/deploy/deploy.sh --backup v1.1.0          # prod
~/m094diploma-staging/deploy/deploy.sh sha-1a2b3c4      # staging
```

Ручной откат не двигает `latest`/`main` в реестре: следующая выкатка из CI вернёт новую версию.

**Один раз руками** (Дархан):

1. GitHub Secrets `SSH_HOST`, `SSH_USER`, `SSH_KEY`, `SSH_KNOWN_HOSTS` — см. «Секреты». Открытый ключ CI — в `~/.ssh/authorized_keys`.
2. Settings → Environments → `production`: Required reviewers — Дархан (ручное подтверждение выкатки prod),
   Deployment branches and tags — только теги `v*`. `staging` создаётся сам при первой выкатке.
3. На сервере — каталоги и `.env` для обоих окружений (`.env` по «Секретам», `APP_ALIAS` у каждого свой,
   в staging `IMAGE_TAG=main`, в prod `IMAGE_TAG=latest`; `CADDY_NETWORK` — имя сети Caddy из `docker network ls`):
   `mkdir -p ~/m094diploma/deploy ~/m094diploma-staging/deploy`.
4. Пакет GHCR сделать публичным после первой сборки (Packages → m094diploma → Package settings → Change visibility → Public):
   репо открытый, а серверу так не нужен `docker login`. Иначе — `docker login ghcr.io` на сервере с токеном `read:packages`.
5. Caddy: блоки для поддоменов staging и prod с `reverse_proxy m094diploma-staging-app:8080` / `m094diploma-app:8080` — S-12.
6. Cron бэкапов prod (S-14): строка в шапке `deploy/backup.sh`.

## Что дальше

- S-12: TLS и прокси. На этой VM роль nginx из плана выполняет общий Caddy — сертификаты он получает сам. Для `/api/ai/` нужен `flush_interval -1` (аналог `proxy_buffering off`), чтобы SSE шёл без буферизации. Приложению в прод-окружении нужен `TRUST_PROXY=true` (Caddy дописывает `X-Forwarded-For` и `X-Forwarded-Proto`): без него лимиты сессий (B-03) считаются на один адрес Caddy для всех пользователей, а cookie `sid` ставится без `Secure`.
- S-14: бэкапы `pg_dump` по cron, как у соседних проектов.
