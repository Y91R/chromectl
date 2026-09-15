---
name: init-project
description: Инициализация проекта — собрать всю информацию о проекте и сформировать CLAUDE.md в режиме планирования
model: opus
allowed-tools: Read Grep Glob Bash EnterPlanMode ExitPlanMode AskUserQuestion Edit Write
---

# Инициализация проекта

Собери полную картину проекта и подготовь/обнови `CLAUDE.md`. Вся аналитика и план — строго в режиме планирования.

## Порядок работы

1. **Сразу вызови `EnterPlanMode`** (схема доступна через ToolSearch). Никаких правок файлов до одобрения плана.

2. **Исследуй проект** (только чтение):
   - структура репозитория: `git ls-files`, ключевые директории;
   - `go.mod` — модуль, версия Go, зависимости;
   - `Makefile` — все команды сборки, тестов, генерации, миграций;
   - `docs/api/openapi.yaml` — контракт API, эндпоинты;
   - `cmd/` — точки входа (`server`, `migrator`); `internal/` — слои: `app` (сборка
     графа), `server/` (echo и служебный порт), `httptransport/` (+ `schemas/`),
     `service/` (домен: `entity`, `usecase/<операция>`, `ports`, `errors`),
     `repository/`, `config`, `observability`;
   - миграции БД, `internal/db/queries/` (sqlc), `docker-compose*.yml`, `.env.example`;
   - `front/` — фронт со встроенным Storybook; `DESIGN.md` — контракт дизайн-системы
     (формат google-labs design.md: frontmatter-токены + проза), токены генерируются
     в `front/src/gen/theme.css`; `front/src/design-system/` — обязательная дизайн-система
     (слои primitives/patterns/layout на токенах, UI собирается из неё);
   - текущий `CLAUDE.md` — найди плейсхолдеры (`chrome_skill`, `github.com/your-org/chrome_skill`, «заполнить при инициализации») и устаревшие места.

3. **Уточни у пользователя** через `AskUserQuestion` то, что нельзя вывести из кода: название и назначение сервиса, кто пользователи, какие внешние системы задействованы.

4. **Представь план** через `ExitPlanMode`: что именно изменится в `CLAUDE.md` (заполнение плейсхолдеров, актуализация команд и архитектуры) и в `DESIGN.md` (проза Overview/Brand, корректировка стартовой палитры/типографики под проект).

5. **После одобрения** внеси изменения. Правила:
   - `CLAUDE.md` пишется на русском, кратко и по делу;
   - команды в нём должны быть реально рабочими (сверены с Makefile);
   - не выдумывать то, чего в коде нет; не описывать очевидное;
   - сохранить разделы Spec-first, флоу UI и конвенции, если они актуальны;
   - `DESIGN.md` — контракт: правится frontmatter-токены и проза, затем `make design-lint`
     и `make generate-design` (обновит `front/src/gen/theme.css`); сгенерированный файл
     руками не трогать.