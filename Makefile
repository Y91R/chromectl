GOLANGCI_LINT_VERSION ?= v2.12.2
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.DEFAULT_GOAL := help

.PHONY: help tools build test test-e2e red lint fix guard verify-unit verify clean

help: ## Показать эту справку
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  %-12s %s\n", $$1, $$2}'

## — Сборка —

build: ## Собрать build/chromectl
	@go build -ldflags "-X main.version=$(VERSION)" -o build/chromectl ./cmd/chromectl

## — Проверки —

lint: ## Линтер Go, включая границы пакетов (depguard)
	@golangci-lint run ./...

fix: ## Автоисправления линтера
	@golangci-lint run --fix ./...

guard: ## Правила, которые не выражаются импортами
	@./scripts/guard.sh

test: ## Модульные тесты (без браузера)
	@go test ./... -count=1

test-e2e: ## e2e с реальным Chrome (CHROME_PATH или стандартный путь)
	@# Пустой зелёный e2e хуже упавшего: он подтверждал бы то, чего никто не проверял.
	@test -d internal/e2e || { echo "e2e-тестов ещё нет: появятся в Э1 (docs/plans/etap-1-brauzer-i-vkladki.md)"; exit 1; }
	@go test -tags e2e ./internal/e2e/... -count=1

red: ## Шаг red: тест обязан упасть — make red RUN=TestVersion_JSON [PKG=./cmd/...]
	@./scripts/red.sh '$(RUN)' '$(PKG)'

## — Гейты —

verify-unit: lint guard test ## Быстрый цикл без браузера; готовность этапа им не подтверждается

verify: verify-unit test-e2e ## Единственный признак «готово»

tools: ## Установить инструменты разработки
	@go mod download
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

clean: ## Удалить артефакты сборки
	@rm -rf build coverage.out coverage.html
