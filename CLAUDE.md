# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> Краткая карта для агента (навигация, definition of done, гардрейлы) — в [AGENTS.md](AGENTS.md).

## Назначение проекта

`chromectl` — Go-CLI и плагин Claude Code со скиллами для управления Chrome по CDP.

- **Пользователи:** AI-агенты (Claude Code) через скиллы проекта и разработчики из
  терминала.
- **Внешние системы:** Chrome/Chromium (debug-порт, CDP); Node.js — только для
  `audit lighthouse`.

ADR: `docs/adr/0009-cli-bez-demona.md`.

## Модель работы

- **Резидентный процесс один — сам Chrome.** `chromectl browser start` запускает его
  с `--remote-debugging-port` и отдельным `--user-data-dir`, `browser stop` закрывает.
- **Одна команда — одно подключение.** Команда подключается к порту, делает одно
  действие, отсоединяется от вкладки (не закрывая её) и завершается. Горутин и
  подписок, переживающих команду, не бывает.
- **Состояние — в файле**, не в памяти: `~/.cache/chromectl/<port>/state.json`
  (каталог `0700`, файлы `0600`). Формат едет только вперёд: поле `version`, новое
  поле добавляется опциональным, чтение старого state не ломается.
- **Чего нет без демона:** истории сети до вызова (только `--capture` в рамках одной
  команды), эмуляции между командами (применяется заново при каждом подключении),
  раздельных `start`/`stop` трейса. Подробности — ADR-0009.

## Текущее состояние

Реестр этапов — `docs/plans/README.md`, выполнены все Э0–Э6. Команды: `browser`,
`pages`, `navigate`, `screenshot`, `snapshot`, `click`, `hover`, `drag`, `fill`,
`fill-form`, `type`, `press`, `upload`, `eval`, `wait-for`, `dialog`, `emulate`,
`resize`, `console`, `network` (детали — через `--capture`), `perf`, `heap`,
`audit lighthouse` (нужен Node.js). Плагин `chromectl` (`.claude-plugin/`), скиллы:
`/chromectl:browser`, `/chromectl:browser-a11y`, `/chromectl:browser-lcp`,
`/chromectl:browser-troubleshooting`; локально — `claude --plugin-dir .`. Пакеты без пометки уже есть в коде, с
пометкой — появятся в указанном этапе (зонтичный план
`docs/plans/dapper-tinkering-raccoon.md`).

```
cmd/chromectl/             cobra-команды: флаги, таймауты, вывод (output.go)
internal/browser/          поиск Chrome, запуск detached, готовность по /json/version, stop
internal/cdp/              тонкий CDP-клиент: websocket, ответы по id, подписки на события
internal/state/            state.json: pid, вкладка, маршруты uid, политика диалога, эмуляция по вкладкам
internal/snapshot/         AX-дерево по всем фреймам (включая cross-origin) → текст с uid
internal/emulation/        разбор и применение эмуляции, пресеты сети
internal/command/session/  подключение к вкладке, эмуляция, страж диалогов, ожидание после действия, uid → узел
internal/command/pages/    браузер, вкладки, навигация, скриншот
internal/command/input/    click, hover, drag, fill, type, press, upload
internal/command/script/   snapshot, eval, wait-for, dialog
internal/command/emulate/  emulate, resize
internal/console/          сообщения консоли из событий CDP
internal/netlog/           захваченные запросы: маскирование, лимиты тел, файл захвата
internal/command/inspect/  console list/get, network list/get
internal/perf/             метрики трейса: LCP, FCP, TTFB, CLS, длинные задачи, разборы
internal/command/diagnose/ perf trace/insight, heap snapshot, audit lighthouse
internal/e2e/              e2e с реальным Chrome и httptest-фикстурами (-tags e2e)
skills/browser*/           скиллы плагина: browser, browser-a11y, browser-lcp, browser-troubleshooting
bin/chromectl              обёртка плагина: собирает build/chromectl из исходников при первом вызове
.claude-plugin/            манифесты плагина и маркетплейса
```

**Chrome не запускается в песочнице агента** (Seatbelt): `make test-e2e`, `make verify`
и команды `chromectl`, которые трогают браузер, выполняются вне песочницы.

## Порядок работы

1. **План.** Крупная задача режется на этапы-срезы в `docs/plans/` (скилл `/new-plan`).
   После каждого этапа есть что показать. Критерий готовности — наблюдаемый результат,
   формально — зелёный `make verify`. Вопрос, на который исполнитель не может
   ответить, — шлагбаум в реестре. Разошлась реализация — правится шапка плана.
2. **Допущения о Chrome проверяются до кода.** Утверждение «CDP сделает X» — гипотеза,
   пока не выполнено на реальном Chrome. Такие места проверяет спайк в начале этапа,
   результат пишется в ADR.
3. **Тест до реализации.** Заглушка сигнатуры → красный тест (`make red RUN=...`) →
   реализация → зелёный → диверсия. Ожидание — литералом из плана этапа или
   результатов спайка, не из кода. Процедура — `/tdd`,
   ADR: `docs/adr/0008-testy-kak-specifikaciya.md`.
4. **ADR на нетривиальное решение** (`/new-adr`): `docs/adr/NNNN-zagolovok.md`, место в
   коде помечается `// ADR: docs/adr/NNNN-....md — короткое почему`.
5. **Отложенное удаление** — строка в `docs/adr/DEPRECATIONS.md` и маркер
   `// DEPRECATED: docs/adr/DEPRECATIONS.md — удалить после <проверяемое условие>`.

## Команды

`make help` печатает полный список.

```bash
make build          # build/chromectl (версия из git describe)
make test           # unit-тесты, без браузера
make test-e2e       # e2e с реальным Chrome (CHROME_PATH или стандартный путь); до Э1 падает
make red RUN=TestX  # шаг red: упасть, если тест зелёный, не запустился или упал не от ассерта
make lint           # golangci-lint, включая depguard
make fix            # автоисправления линтера
make guard          # context.TODO(), panic в internal
make verify-unit    # lint + guard + test — быстрый цикл, готовность им не подтверждается
make verify         # verify-unit + test-e2e — единственный признак «готово»
make tools          # golangci-lint нужной версии
```

## Принципы Go-кода

- **Ошибки** — `fmt.Errorf("<что делали>: %w", err)` наверх. Ожидаемые исходы —
  sentinel-ошибки, разбираются через `errors.Is`/`errors.As`. В `cmd/chromectl` ошибка
  превращается в сообщение в stderr и ненулевой код выхода; stdout при ошибке пуст.
  `panic` в `internal` не используется (`make guard`).
- **Контекст и таймауты.** `context.Context` — первым аргументом во всём, что ходит в
  Chrome. Каждый CDP-вызов ограничен таймаутом: мёртвый порт не должен вешать команду.
  `context.Background()` — только в `main` и тестах, `context.TODO()` — нигде.
- **Конкурентность.** Горутина живёт не дольше команды, у неё есть владелец и способ
  остановки. Фоновых процессов CLI не оставляет.
- **Вкладку закрывает только `pages close`.** Любая другая команда отсоединяется
  через `Target.detachFromTarget`; контексты `chromedp` с `WithTargetID` не
  используются — их отмена закрывает вкладку (ADR-0009).
- **Секреты.** Захваченные заголовки `Authorization`, `Cookie`, `Set-Cookie`,
  `Proxy-Authorization` маскируются по умолчанию; тела и заголовки в лог не пишутся.
- **Пакеты и имена.** Пакет — зона ответственности: `utils`, `common`, `helpers`,
  `base` не заводятся. Экспортируется минимум.
- **Время наружу — в UTC.**

## Тесты

- Unit: чистые функции (формат снапшота по фикстуре AX-дерева, разбор флагов
  эмуляции, метрики трейса) — golden-файлы и литералы в `testdata/`.
- e2e (`-tags e2e`, `internal/e2e/`): headless Chrome на свободном порту, фикстуры
  через `httptest`, каждая команда — отдельный процесс собранного бинаря. Проверяется
  код выхода, stdout/stderr и отсутствие побочных эффектов (число вкладок в
  `/json/list`, права файлов state).
- Без `time.Sleep`: ожидание — по событию или опросом с дедлайном. `t.Parallel()` там,
  где нет общего браузера.
- Поведение без контракта, зафиксированное как есть, — `// CHARACTERIZATION:`.

## Гардрейлы

- `depguard` (`make lint`): `internal/cdp` не знает про `internal/command`,
  `internal/output` и cobra; `internal/**` не импортирует `cmd`.
- `make guard` (`scripts/guard.sh`): `context.TODO()`, `panic` в `internal`.
- `make red` (`scripts/red.sh`): красный от ассерта, а не от компилятора.
- `TestEveryCommandHasE2ECase` (`cmd/chromectl`, в `make test`): у каждой команды cobra
  есть кейс с её полным путём аргументов в e2e или unit-тестах CLI.
- `TestSkillTableMatchesCommandTree` (`cmd/chromectl`, в `make test`): таблица команд
  скилла `skills/browser` совпадает с деревом cobra — новая команда требует строки в скилле.
- `TestPluginManifests` (`cmd/chromectl`) и `TestPluginWrapper_*` (e2e): манифесты
  плагина указывают на корень, скиллы на месте, обёртка собирает бинарь при первом вызове и переиспользует его.
- `TestSoak_FiftyCommandsLeaveNoTabsSessionsOrProcesses` (e2e): 50 команд подряд не
  меняют набор вкладок и не оставляют процессов CLI.
- `make test-e2e` падает, если e2e нет или Chrome не найден, — пустой зелёный не бывает.
- `.claude/settings.json`: запрещены `git commit`, `git push`, `rm -rf`, чтение `.env*`.
- Подробно, с «зачем» каждого, — `docs/flou-ogranicheniy-i-specifikaciy.md`.

## Конвенции

- Go-модуль: `github.com/Y91R/chromectl`.
- Все `.md` файлы пишутся на русском. Исключение — `README.md`: он на английском, русская
  версия — `README.ru.md`; правятся вместе.
