# S-02. Команды разработки. Нужны Docker с compose v2 и make
# (Windows: make из Git for Windows SDK, `winget install ezwinports.make` или WSL).
.PHONY: help up down reset logs ps psql test lint fmt run build migrate-up migrate-down migrate-status

DB_PORT ?= 5433
DB_URL  := postgres://m094:m094@localhost:$(DB_PORT)/m094?sslmode=disable

# app и mock поднимаются, только когда появились их файлы (Dockerfile — B-02, api/openapi.yaml — B-01)
PROFILES := $(if $(wildcard Dockerfile),--profile app) $(if $(wildcard api/openapi.yaml),--profile mock)
COMPOSE  := docker compose $(PROFILES)

help: ## список команд
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-14s %s\n", $$1, $$2}'

up: ## поднять всё: база, приложение (если есть), мок API (если есть)
	$(COMPOSE) up -d --build --wait
	@echo "база:  $(DB_URL)"
	@$(if $(wildcard Dockerfile),echo "сайт:  http://localhost:8080",true)
	@$(if $(wildcard api/openapi.yaml),echo "мок:   http://localhost:4010",true)

down: ## остановить (данные базы сохраняются)
	$(COMPOSE) down

reset: ## остановить и стереть базу
	$(COMPOSE) down -v

logs: ## логи всех сервисов
	$(COMPOSE) logs -f --tail=100

ps: ## что запущено
	$(COMPOSE) ps

psql: ## консоль базы
	docker compose exec db psql -U m094 -d m094

test: ## Go-тесты с локальной базой (make up до этого)
	TEST_DATABASE_URL='$(DB_URL)' go test -race -count=1 ./...

migrate-up: ## применить новые миграции к локальной базе (make up до этого)
	DATABASE_URL='$(DB_URL)' go run ./cmd/server migrate up

migrate-down: ## откатить последнюю миграцию
	DATABASE_URL='$(DB_URL)' go run ./cmd/server migrate down

migrate-status: ## какие миграции применены
	DATABASE_URL='$(DB_URL)' go run ./cmd/server migrate status

run: ## сервер на :8080 без Docker (логи для чтения глазами)
	LOG_FORMAT=text go run ./cmd/server

build: ## собрать бинарник в bin/server
	go build -trimpath -o bin/server ./cmd/server

lint: ## как в CI: gofmt, vet, eslint
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...
	npx --yes eslint@9.39.1 web

fmt: ## отформатировать Go
	gofmt -w .
