# План: исправление замечаний по зрелости AI-first (P0+P1+P2)

## Контекст

Анализ зрелости шаблона как AI-first среды показал: контрактное ядро и гардрейлы
зрелые (L3, местами L4), но автономность агента упирается в неполные петли
самопроверки и в то, что часть правил — рекомендательные. Цель — закрыть замечания
всех трёх приоритетов: повысить воспроизводимость и гигиену (P0), достроить петли
самопроверки и принуждение правил (P1), расширить покрытие тестами/безопасностью и
связность знаний (P2). Объём — все три тира; пин Go-инструментов — через go.mod
tool-директивы (Go 1.25 в `go.mod.tmpl`).

**Нюанс по пину:** tool-директивы применяем к `oapi-codegen` и `sqlc`. `golangci-lint`
как go.mod-tool тянет огромный граф зависимостей и конфликтует с графом приложения —
его пиним отдельно (точная версия в `make tools`), не как go.mod-tool.

---

## P0 — воспроизводимость и гигиена

### 1. Пин Go-инструментов (go.mod tool-директивы)
- `go.mod.tmpl`: добавить блок `tool (...)` с `oapi-codegen` и `sqlc` + соответствующие
  `require`-строки с точными версиями (резолвятся при реализации). go.sum наполняется
  `go mod tidy` на init.
- `Makefile`: `generate-api` → `go tool oapi-codegen --config oapi-config.yaml ...`;
  `generate-db` → `go tool sqlc generate`. Таргет `tools` переориентировать:
  `go mod download` (тянет tool-deps) + установка пиннутого `golangci-lint` точной
  версией + напоминание про `cd front && npm ci`.
- `lint`: оставить `golangci-lint run ./...` (бинарь из `make tools`, пиннутая версия),
  НЕ go-tool.
- `init.sh`: заменить упоминание `make tools`/`go install` на `go mod download`
  (tool-deps ставятся автоматически), golangci-lint — пиннутой версией.

### 2. Чистка `.claude/settings.local.json`
- Удалить устаревшие строки 14–15 (разрешения под временный `.probe-design.md`) —
  они больше не нужны (`design:lint` и генерация идут через локальный bin `design.md`).
- При необходимости добавить `Bash(make verify*)`. Прочее — оставить.

### 3. `AGENTS.md` + навигация + документирование гардрейлов
- Новый `AGENTS.md` (корень, русский, кратко): карта навигации «вопрос про X → читай Y /
  запусти скилл Z»; «definition of done» = `make verify` зелёный; перечень гардрейлов
  (хук `protect-gen.sh`, permissions-whitelist, `generate-check`) — чтобы агент знал о
  них заранее; запрещённые действия (правка `gen/`, ручной правки токенов, hardcode hex).
- `CLAUDE.md` и `README.md`: одна строка-указатель на `AGENTS.md`.

---

## P1 — петли самопроверки и принуждение правил

### 4. `make verify` — единый гейт
- Новый таргет `verify: generate-check test lint` (design-lint уже внутри
  generate-check), + `design-guard` (см. п.5). Добавить в `.PHONY`.
- Зафиксировать как «готово» в `AGENTS.md`, `README.md` и в финальном шаге всех скиллов
  (заменить разрозненные `make test && make lint && make generate-check` на `make verify`).

### 5. Принуждение «tokens-only / без hex» в компонентах
- `Makefile` таргет `design-guard`: падает, если в `front/src/components/**/*.vue`
  встречается сырой hex-цвет. Реализация — grep:
  `! grep -rERn '#[0-9a-fA-F]{3,8}' front/src/components --include=*.vue` (компоненты
  собираются только на токен-утилитах). Включить в `verify`.

### 6. Интеграционная петля для `store` (testcontainers-go)
- Добавить test-зависимости `github.com/testcontainers/testcontainers-go` +
  `.../modules/postgres`. Новый `internal/store/store_integration_test.go` с
  `//go:build integration`: поднимает эфемерный `postgres:16-alpine`, применяет миграции
  (`internal/db/migrations`), проверяет `Store.Ping` и образцовый запрос.
- `Makefile`: `test-integration: ## требует Docker` → `go test -tags=integration ./...`.
- Сослаться на паттерн в скиллах `/add-migration`, `/add-endpoint` как канонический для
  store-слоя.

---

## P2 — покрытие, безопасность, связность

### 7. Фронт-юниты (Vitest)
- devDeps: `vitest`, `@vue/test-utils`, `happy-dom`; `vitest.config.ts`; скрипт
  `"test": "vitest run"` в `front/package.json`.
- Примеры: `front/src/components/kit/Button.spec.ts` (слот, классы variant, disabled),
  `Input.spec.ts` (эмит `update:modelValue`).

### 8. Визуальная регрессия
- `@storybook/test-runner` + playwright; baseline-снимки для `Kit/Button`, `Kit/Card`,
  `Example/ExampleCard`. Скрипт `"test-storybook"` — требует поднятого Storybook и
  браузеров. Документировать как гейт визуальных изменений.

### 9. Раздел «Безопасность» в `CLAUDE.md`
- Кратко и предметно: валидация входа (уже есть `OapiRequestValidator` —
  зафиксировать), CORS (`cfg.CORSOrigins` в `main.go`), секреты только из env (`.env`
  не коммитим, `Read(.env*)` запрещён), точка подключения auth-middleware, не логировать
  секреты.

### 10. Связка ADR↔plans + расширение маркеров
- Перекрёстные ссылки: в `docs/adr/0001..0003` добавить «Связано: план `docs/plans/...`»;
  в планах — указатель на итоговый ADR. Обновить `docs/adr/README.md` (конвенция
  plan↔ADR).
- Расширить `// ADR:`-маркеры на файлы решений: `cmd/server/main.go` (middleware/монолит),
  `oapi-config.yaml` (strict-server → ADR-0001), `front/scripts/gen-theme.mjs` (ADR-0002,
  сверить), kit-файлы (добавить ADR-0003 к существующему DESIGN.md-комменту). Единый
  формат маркера `ADR: docs/adr/NNNN-...`.

---

## Критичные файлы

- Пин/сборка: `go.mod.tmpl`, `Makefile`, `init.sh`
- Гигиена/навигация: `.claude/settings.local.json`, `AGENTS.md` (новый), `CLAUDE.md`, `README.md`
- Гейты: `Makefile` (`verify`, `design-guard`, `test-integration`)
- Тесты: `internal/store/store_integration_test.go` (новый), `front/vitest.config.ts` (новый),
  `front/src/components/kit/*.spec.ts` (новые), `front/package.json`
- Знания: `docs/adr/0001..0003`, `docs/adr/README.md`, `docs/plans/*`, маркеры в
  `cmd/server/main.go`/`oapi-config.yaml`/`front/scripts/gen-theme.mjs`/kit

## Порядок реализации

1. P0 целиком (быстро, разблокирует детерминизм): пин инструментов → `make verify`
   каркас → чистка permissions → `AGENTS.md`.
2. P1: `design-guard` → интеграционный тест store.
3. P2: vitest → безопасность/маркеры/связки → визуальная регрессия.

## Проверка end-to-end

1. `go mod tidy` (в инстансе) — tool-блок резолвится, `go tool oapi-codegen --version`
   и `go tool sqlc version` работают; `make generate` идёт через `go tool`.
2. `make verify` — зелёный: generate-check + test + lint + design-guard. Намеренно
   вставить hex в `kit/Button.vue` → `design-guard` краснеет; убрать.
3. `make test-integration` (с Docker) — поднимается postgres-контейнер, `Store.Ping` ок.
4. `cd front && npm ci && npm run test` (vitest) — юниты kit зелёные; `npm run build`,
   `npm run build-storybook` проходят.
5. `.claude/settings.local.json` — нет ссылок на `.probe-design.md`.
6. `grep -rn "ADR:" .` — маркеры покрывают ключевые файлы решений; `docs/adr` и планы
   взаимно слинкованы.

## Риски и митигации

- **golangci-lint в go.mod-tools** раздувает граф/конфликтует → пиним отдельно (бинарь
  в `make tools`), не как tool-директиву.
- **testcontainers/visual-regression требуют Docker/браузеры** →
  `//go:build integration`; локально опциональны, основной `make verify` их не требует.
- **Объём P2 большой** → порядок реализации поэтапный; каждый пункт самодостаточен и
  мёржится независимо.
- **Версии инструментов резолвятся при реализации** (не в плане) — брать актуальные
  стабильные теги, фиксировать точно.
