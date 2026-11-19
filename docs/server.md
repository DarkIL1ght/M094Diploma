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

## Что дальше

- S-12: TLS и прокси. На этой VM роль nginx из плана выполняет общий Caddy — сертификаты он получает сам. Для `/api/ai/` нужен `flush_interval -1` (аналог `proxy_buffering off`), чтобы SSE шёл без буферизации. Приложению в прод-окружении нужен `TRUST_PROXY=true` (Caddy дописывает `X-Forwarded-For` и `X-Forwarded-Proto`): без него лимиты сессий (B-03) считаются на один адрес Caddy для всех пользователей, а cookie `sid` ставится без `Secure`.
- S-17: `.env` диплома на сервере (chmod 600) и `.env.example` в репо. В прод-`.env` обязательно `TRUST_PROXY=true` (см. S-12).
- S-14: бэкапы `pg_dump` по cron, как у соседних проектов.
