# chromectl

[English](README.md) · **Русский**

CLI и плагин Claude Code для управления Chrome по Chrome DevTools Protocol (CDP): снапшот
дерева доступности с uid, клики и формы, скриншоты, эмуляция устройств, консоль и сеть,
трейс производительности, Lighthouse и снимок кучи.

Резидентный процесс один — сам Chrome с открытым debug-портом. Каждая команда
`chromectl` подключается, делает одно действие, отсоединяется от вкладки и
завершается: ни MCP-сервера, ни демона, ничего не остаётся в фоне. Почему так и что при
этом теряется — [ADR-0009](docs/adr/0009-cli-bez-demona.md).

## Требования

- Chrome или Chromium (нестандартный путь — `CHROME_PATH`)
- Go 1.26+ — плагин собирает бинарь из исходников при первом вызове
- Claude Code 2.1.265+ для плагина (его `bin/` попадает в `PATH`)
- Node.js — только для `chromectl audit lighthouse`

## Установка плагина Claude Code

```
/plugin marketplace add Y91R/chromectl
/plugin install chromectl@chromectl
```

Скиллы плагина:

| Скилл | Для чего |
|---|---|
| `/chromectl:browser` | открыть страницу, снять снапшот и скриншот, кликнуть, заполнить форму, выполнить JS, дождаться текста |
| `/chromectl:browser-a11y` | аудит доступности: Lighthouse, семантика, подписи, фокус, контраст |
| `/chromectl:browser-lcp` | отладка и оптимизация LCP: трейс, LCP-элемент, водопад сети |
| `/chromectl:browser-troubleshooting` | диагностика, когда CLI не запускает Chrome, не подключается или зависает |

**Песочница.** Chrome не запускается в песочнице Bash Claude Code (macOS Seatbelt), а
первый вызов `chromectl` пишет собранный бинарь в каталог плагина. Скиллы велят Claude
выполнять такие команды вне песочницы — Claude Code спросит разрешение. Правила
песочницы — `/sandbox`.

## Установка без плагина

```bash
go install github.com/Y91R/chromectl/cmd/chromectl@latest
```

## Быстрый старт

```bash
chromectl browser start --headless
chromectl pages new https://example.com
chromectl snapshot                 # дерево доступности с uid
chromectl click 1_5 --snapshot     # действие по uid и сразу обновлённое дерево
chromectl screenshot -o page.png
chromectl browser stop
```

| Группа | Команды |
|---|---|
| Браузер и вкладки | `browser start/stop/status`, `pages list/new/select/close`, `navigate` |
| Страница | `snapshot`, `screenshot`, `eval`, `wait-for`, `dialog` |
| Ввод | `click`, `hover`, `drag`, `fill`, `fill-form`, `type`, `press`, `upload` |
| Эмуляция | `emulate` (viewport, тема, сеть, CPU, геолокация…), `resize` |
| Инспекция | `console list/get`, `network list/get` |
| Диагностика | `perf trace`, `perf insight`, `heap snapshot`, `audit lighthouse` |

Глобальные флаги: `--port` (по умолчанию 9222; у каждого порта свой state и профиль),
`--page <id>`, `--json`. Полный справочник со всеми флагами — таблица команд в
[skills/browser/SKILL.md](skills/browser/SKILL.md) и `chromectl <команда> --help`.

## Чего нет без демона

- Истории сети до вызова: повторите действие с `--capture`, затем `network list` /
  `network get`. Чувствительные заголовки маскируются по умолчанию.
- Эмуляция хранится по вкладкам и применяется заново при каждом подключении.
- Трейс снимается одной командой (`perf trace`), без раздельных start/stop.
- Состояние — в `~/.cache/chromectl/<port>/state.json` (`CHROMECTL_STATE_DIR`
  переопределяет каталог).

## Разработка

```bash
make tools        # один раз: golangci-lint
make build        # build/chromectl
make test         # unit-тесты без браузера
make verify       # lint + guard + unit + e2e (нужен Chrome, вне песочницы)
claude --plugin-dir .   # попробовать плагин из этого репозитория
```

Порядок работы и гардрейлы — [CLAUDE.md](CLAUDE.md), карта для агента —
[AGENTS.md](AGENTS.md), планы этапов — [docs/plans](docs/plans/README.md), решения —
[docs/adr](docs/adr/README.md).
