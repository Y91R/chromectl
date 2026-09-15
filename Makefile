GOLANGCI_LINT_VERSION ?= v2.12.2

-include .env
export

.DEFAULT_GOAL := help

.PHONY: help tools generate generate-api generate-db generate-mocks generate-client generate-design \
        design-lint generate-check design-guard guard test-guard lint fix \
        red test test-cover test-migrations verify verify-full \
        build build-front build-back run \
        migrate-create migrate-up migrate-down migrate-version \
        docker-up docker-down docker-db docker-logs clean

help: ## Показать эту справку
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  %-18s %s\n", $$1, $$2}'

## — Генерация —

generate: generate-api generate-db generate-mocks generate-client generate-design ## Перегенерировать весь код из контрактов

generate-api: ## OpenAPI → gen/api (strict-server)
	@go tool oapi-codegen --config oapi-config.yaml docs/api/openapi.yaml

generate-db: ## SQL → gen/db (sqlc)
	@go tool sqlc generate

generate-mocks: ## Интерфейсы домена → gen/mocks (mockery)
	@# Каталог чистится целиком: mockery не удаляет моки переименованных или
	@# удалённых интерфейсов, и они годами живут в gen/, ломая generate-check.
	@rm -rf gen/mocks
	@go tool mockery >/dev/null

generate-client: ## OpenAPI → TS-типы фронта
	@cd front && npm run generate:api

generate-design: ## DESIGN.md → CSS-токены Tailwind v4
	@cd front && npm run generate:theme

generate-check: generate design-lint ## Упасть, если генерат разошёлся с контрактами
	@test -z "$$(git status --porcelain gen/ front/src/gen/)" || { \
		git --no-pager diff gen/ front/src/gen/; \
		git status --porcelain gen/ front/src/gen/; \
		echo "генерат разошёлся с контрактами: выполните 'make generate' и закоммитьте gen/ и front/src/gen/"; \
		exit 1; }

## — Проверки —

lint: ## Линтер Go, включая границы слоёв (depguard)
	@golangci-lint run ./...

fix: ## Автоисправления линтера
	@golangci-lint run --fix ./...

guard: ## Правила, которые не выражаются импортами
	@./scripts/guard.sh

test-guard: ## Тест как спецификация: код ответа без кейса, операция без теста
	@./scripts/test-guard.sh

design-lint: ## Валидация контракта дизайна DESIGN.md
	@cd front && npm run design:lint

design-guard: ## Запрет сырого hex-цвета в компонентах (UI — только на токенах)
	@if grep -rEn '#[0-9a-fA-F]{3,8}' front/src/design-system front/src/components --include='*.vue'; then \
		echo "хардкод hex в компоненте: используй токен-утилиты (bg-surface, text-primary, …), см. DESIGN.md"; exit 1; \
	fi

test: ## Модульные тесты (без докера и сети)
	@go test ./... -count=1

red: ## Шаг red: тест обязан упасть — make red RUN=TestCreateItem_409 [PKG=./internal/...]
	@./scripts/red.sh '$(RUN)' '$(PKG)'

test-cover: ## Покрытие по internal/
	@go test ./... -count=1 -coverprofile=coverage.out -coverpkg=./internal/...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "отчёт: coverage.html"

test-migrations: ## up → down → up на живой БД
	@go run ./cmd/migrator up
	@go run ./cmd/migrator down
	@go run ./cmd/migrator up

## — Гейты —

verify: generate-check lint guard test-guard test design-guard ## Единственный признак «готово»

verify-full: verify test-migrations ## Полная проверка (нужны докер и сеть)

## — Сборка и запуск —

build: build-front build-back ## Монолит: фронт в front/public + бинари

build-front: ## Собрать SPA в front/public (раздаётся бэкендом с диска)
	@cd front && npm run build

build-back: ## Собрать бинари бэкенда
	@go build -o bin/server ./cmd/server
	@go build -o bin/migrator ./cmd/migrator

run: ## Запустить сервис (http :8080, служебный :8081)
	@go run ./cmd/server

## — База данных —

migrate-create: ## Создать миграцию: make migrate-create name=add_items_status
	@test -n "$(name)" || { echo "укажите name=<snake_case>"; exit 1; }
	@# Номер берётся от максимального существующего, а не от количества файлов:
	@# при удалённой или пропущенной миграции счёт по количеству попал бы
	@# в уже занятый номер и затёр закоммиченную пару.
	@last=$$(ls internal/db/migrations/*.up.sql 2>/dev/null | sed 's#.*/##' | cut -d_ -f1 | sort -n | tail -1); \
	next=$$(printf "%06d" $$(( 10#$${last:-0} + 1 ))); \
	touch internal/db/migrations/$${next}_$(name).up.sql internal/db/migrations/$${next}_$(name).down.sql; \
	echo "созданы internal/db/migrations/$${next}_$(name).{up,down}.sql"

migrate-up: ## Накатить миграции
	@go run ./cmd/migrator up

migrate-down: ## Откатить одну миграцию
	@go run ./cmd/migrator down

migrate-version: ## Текущая версия схемы
	@go run ./cmd/migrator version

## — Инфраструктура —

docker-up: ## Поднять весь стек
	@docker compose up -d --build

docker-down: ## Погасить стек
	@docker compose down

docker-db: ## Поднять только Postgres
	@docker compose -f docker-compose.dev.yml up -d postgres

docker-logs: ## Логи стека
	@docker compose logs -f

tools: ## Установить инструменты разработки
	@go mod download
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "Фронт: cd front && npm ci"

clean: ## Удалить артефакты сборки
	@rm -rf bin coverage.out coverage.html gen/api gen/db gen/mocks front/public front/src/gen/theme.css
