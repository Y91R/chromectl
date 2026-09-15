# chrome_skill

Шаблон Go-сервиса: spec-first бэкенд (Echo + oapi-codegen + sqlc) и story-first фронт
(Vue 3 + Vite + Tailwind + Storybook). Этот README описывает **как вести задачу** в
проекте. Детали архитектуры — в [CLAUDE.md](CLAUDE.md), дизайн-система — в
[DESIGN.md](DESIGN.md), карта для AI-агента — в [AGENTS.md](AGENTS.md).

## Быстрый старт

```bash
make tools          # один раз: oapi-codegen, sqlc, mockery, golangci-lint
cp .env.example .env
make docker-db      # поднять Postgres
make migrate-up     # применить миграции (go run ./cmd/migrator up)
make generate       # кодогенерация по контрактам
make run            # API :8080, служебный порт :8081 (/livez, /readyz)
cd front && npm install && npm run dev   # фронт на :5173 (proxy /api → :8080)
```

**Монолит.** Бэкенд раздаёт собранный фронт с диска: `make build` собирает SPA в
`front/public`, и сервер отдаёт и API, и UI на одном порту (отдельный веб-сервер для
статики не нужен). Фронт читается с диска при запросе — его можно пересобрать без
пересборки бинаря. Для разработки UI удобнее `npm run dev` с проксированием на бэкенд.

```bash
make build          # монолит: фронт (front/public) + бинари → bin/server, bin/migrator
make build-front    # пересобрать только SPA → front/public
make build-back     # только бинари бэкенда
```

## Главный принцип: сначала контракт, потом код

**Никакой реализации без спецификации.** Контракт — единственный источник правды,
код из него генерируется и не пишется руками. Это касается каждого слоя:

| Что меняешь | Контракт (правишь руками) | Генерация | Реализация (пишешь руками) |
|---|---|---|---|
| API     | `docs/api/openapi.yaml`        | `make generate` → `gen/api`, `front/src/gen` | `internal/httptransport` |
| Домен   | инвариант в `entity`, сценарий в `usecase` | `make generate` → `gen/mocks` | `internal/service` |
| БД      | миграция + `internal/db/queries/*.sql` | `make generate` → `gen/db` | `internal/repository` |
| Дизайн  | `DESIGN.md` (токены, формат design.md) | `make generate` → `front/src/gen/theme.css` | дизайн-система в `design-system/`, UI из неё |
| UI      | story на моковых данных        | —          | компонент рядом со story в `front/` |

`gen/` и `front/src/gen/` руками не редактируются — PreToolUse-хук блокирует правку,
а `make generate-check` (и CI) ловит рассинхрон со спекой.

## Флоу работы над задачей

Любая задача проходит один и тот же цикл. Шаги не пропускаются.

1. **Понять и зафиксировать «что».** Опиши контракт: эндпоинт в `openapi.yaml`,
   миграцию + SQL-запросы, или story компонента. Неочевидную форму API/схему —
   согласуй до реализации.
2. **Сгенерировать.** `make generate` — появятся типы и интерфейсы в `gen/`.
3. **Реализовать по слоям.** Правило — в `internal/service/entity` (инвариант нельзя
   обойти мимо конструктора), сценарий — в `internal/service/usecase/<операция>`
   (зависимости объявлены в его `contracts.go`), доступ к данным — в
   `internal/repository` (возвращает сущности, ошибки драйвера переводит в доменные),
   маппинг и коды ответов — в `internal/httptransport/schemas`
   (см. [архитектуру](CLAUDE.md#архитектура)). Домен не импортирует `gen/`, echo и pgx —
   это проверяет `depguard`.
4. **Зафиксировать «почему» (ADR).** Если по ходу принято нетривиальное решение
   (форма API, схема данных, граница слоёв, компромисс) — заведи
   `docs/adr/NNNN-zagolovok.md` по шаблону `docs/adr/0000-template.md`. Спека отвечает
   на «что», ADR — на «почему именно так». Места в коде, реализующие решение, помечай
   `// ADR: docs/adr/NNNN-....md — короткое почему` (ищется `grep -rn "ADR:" internal/`).
5. **Специфицировать тестом — до реализации.** Тест задаёт поведение так же, как
   спека задаёт форму: заглушка сигнатуры → красный тест (`make red RUN=...`) →
   реализация → диверсия. Ожидание берётся из контракта, а не из кода: написанный
   после реализации тест закрепляет её баги как контракт. Эндпоинт готов, когда на
   каждый код ответа из спеки есть кейс (образец —
   `internal/httptransport/handlers_test.go`), операция — когда покрыт каждый её исход
   (`internal/service/usecase/createitem/usecase_test.go`); наличие проверяет
   `make test-guard`.
   Моки генерирует mockery по `ports` и `contracts.go`, руками они не пишутся.
6. **Проверить.** `make verify` (generate-check + lint + guard + test-guard + test + design-guard) —
   всё зелёное, сгенерированный код закоммичен вместе с контрактом, который его породил.

Процедуры оформлены скиллами Claude Code:
- `/add-endpoint` — добавить API-эндпоинт по всему циклу выше;
- `/add-usecase` — добавить доменную операцию (сущность, инвариант, usecase, тест);
- `/tdd` — цикл red-green-refactor: тест как спецификация, диверсия, регрессия на баг;
- `/add-migration` — добавить миграцию БД и типизированные запросы;
- `/new-adr` — зафиксировать нетривиальное решение;
- `/update-design` — изменить дизайн-систему через контракт DESIGN.md (токены → генерация → проверка);
- `/add-component` — добавить UI-примитив в дизайн-кит или собрать компонент из кита;
- `/init-project` — первичная инициализация (заполнение CLAUDE.md, DESIGN.md).

## Обратная совместимость: прод едет только вперёд

Откат — это деплой новой версии вперёд, а не возврат старой. Поэтому каждое изменение
обязано быть обратно совместимым с уже работающим прод-кодом и существующими данными.

- **Expand/contract.** Сначала вводим новое, не ломая старое (новое поле опционально,
  колонка nullable/с дефолтом, новая ручка рядом со старой), переключаем чтение/запись.
  Удаление старого — **отдельным шагом позже**, когда от него уже ничто не зависит.
- **Breaking-изменения только отдельным шагом.** Удаление/переименование поля, колонки,
  ручки и несовместимая смена контракта не делаются в том же изменении, что вводит
  замену. Для БД — отдельная миграция `drop`/`rename` после того, как код перестал
  использовать старое.
- **Отложенные удаления — в реестр.** Что подлежит удалению позже, фиксируется в
  [`docs/adr/DEPRECATIONS.md`](docs/adr/DEPRECATIONS.md) (что удалить, когда станет
  можно, зачем). В коде — маркер `// DEPRECATED: docs/adr/DEPRECATIONS.md — удалить
  после <условие>` с проверяемым условием.

## Где что лежит

```
docs/api/openapi.yaml   контракт REST API (источник правды)
DESIGN.md               контракт дизайн-системы (токены, формат design.md)
docs/adr/               ADR («почему») + DEPRECATIONS.md (что снести позже)
docs/flou-ogranicheniy-i-specifikaciy.md  карта ограничений: что проверяется, как и зачем
docs/skilly-storonniy-agent-i-konsensus-revyu.md  скиллы (проектные и конвейер СТ → планы → код), ревьюер codex, консенсус
docs/plans/             комплект планов: реестр этапов + план на этап
internal/service/       домен: entity (инварианты), usecase (операции), ports, errors
internal/repository/    адаптеры БД: сущности наружу, доменные ошибки, txmanager
internal/httptransport/ реализация контракта + schemas/ (мапперы и коды ответов)
internal/server/        httpserver (API + статика SPA) и debugserver (/livez, /readyz)
internal/app/           сборка графа зависимостей и остановка в порядке LIFO
internal/config/        конфиг из env (в т.ч. STATIC_DIR — папка статики фронта)
cmd/server/             процесс сервиса; cmd/migrator/ — миграции (golang-migrate)
gen/                    сгенерированный код — не редактировать
front/                  Vue + Vite + Tailwind + Storybook (story рядом с компонентом)
front/src/design-system/ дизайн-система на токенах: primitives/patterns/layout + story
front/src/gen/theme.css токены Tailwind из DESIGN.md — не редактировать
front/static/           статические исходники Vite (favicon) — в репозитории
front/public/           выход сборки SPA, бэкенд раздаёт его с диска (не коммитится)
```

Полный список команд — раздел «Команды» в [CLAUDE.md](CLAUDE.md).
Все `.md` в проекте — на русском.
