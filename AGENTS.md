# AGENTS.md — как агенту работать в этом репозитории

Короткая карта для AI-агента. Подробности — в `CLAUDE.md`. Все `.md` на русском, кроме
`README.md` (английский; русская версия — `README.ru.md`).

## Definition of done

Задача готова, когда зелено `make verify` (= `lint` + `guard` + `test` + `test-e2e`).
`make verify-unit` — только для быстрого цикла, готовность им не подтверждается.
Зелёные тесты засчитываются, только если каждый из них был красным до реализации:
тест, написанный после кода, подтверждает код, а не требование.

## Куда идти по типу задачи

| Вопрос / задача | Источник правды / куда идти | Скилл |
|---|---|---|
| Что делаем и в каком порядке | `docs/plans/README.md` (реестр этапов) | `/new-plan` |
| Открыть страницу, снять скриншот, переключить вкладку | `chromectl` (вне песочницы — Chrome в ней не стартует) | `/chromectl:browser` |
| Проверить доступность страницы | Lighthouse, снапшот, сниппеты `eval` | `/chromectl:browser-a11y` |
| Разобрать медленную загрузку, LCP | `perf trace`, `perf insight`, `network --capture` | `/chromectl:browser-lcp` |
| chromectl не стартует, не подключается, зависает | таблица ошибок, state и лог Chrome | `/chromectl:browser-troubleshooting` |
| Плагин: манифесты, обёртка бинаря, скиллы | `.claude-plugin/`, `.codex-plugin/`, `.agents/plugins/`, `kimi.plugin.json`, `bin/chromectl`, `skills/` (установка агентов — в README) | — |
| Почему CLI без демона, что теряется | `docs/adr/0009-cli-bez-demona.md` | — |
| Написать тест на поведение, починить баг | `make red` (red → green → диверсия) | `/tdd` |
| Как писать Go-код (ошибки, контекст, вкладки, секреты) | `CLAUDE.md` → «Принципы Go-кода» | — |
| Зафиксировать решение | `docs/adr/` (+ маркеры `// ADR:` в коде) | `/new-adr` |
| Что удалить позже | `docs/adr/DEPRECATIONS.md` | — |
| Команды | `make help` | — |

## Codex и Kimi Code

Codex и Kimi Code читают этот файл; подробные правила Go и тестов — в [CLAUDE.md](CLAUDE.md).
Плагин устанавливается через `codex plugin marketplace add <абсолютный путь репозитория>`
и `codex plugin add chromectl@chromectl`, затем нужна новая сессия.
В Kimi Code — `/plugins install <абсолютный путь репозитория>`, `/reload`,
`/skill:chromectl:browser`. После изменения исходников переустановите плагин.
Скиллы общие для всех агентов: [browser](skills/browser/SKILL.md),
[browser-a11y](skills/browser-a11y/SKILL.md), [browser-lcp](skills/browser-lcp/SKILL.md),
[browser-troubleshooting](skills/browser-troubleshooting/SKILL.md).
В Codex и Kimi Code обёртку вызывают по абсолютному пути относительно загруженного скилла.
В Codex Chrome, `make test-e2e` и `make verify` — через `exec_command` с
`sandbox_permissions: "require_escalated"` и `justification`.
В Kimi Code — Bash со штатными разрешениями среды.

## Гардрейлы (знай заранее)

Подробно, с механикой и «зачем» каждого, — `docs/flou-ogranicheniy-i-specifikaciy.md`.
Как работают скиллы, вызов стороннего ревьюера (codex) и сведение двух ревью в один
список — `docs/skilly-storonniy-agent-i-konsensus-revyu.md`.

- **Вкладку закрывает только `pages close`.** Контексты `chromedp` с `WithTargetID` не
  используются: их отмена закрывает вкладку.
- **`depguard`** (в `make lint`): `internal/cdp` не знает про команды, вывод и cobra;
  `internal` не импортирует `cmd`.
- **`make guard`** ловит `context.TODO()` и `panic` в `internal`.
- **`make red RUN=<тест>`** валит сборку, если тест зелёный до реализации, не
  запустился или упал не от ассерта.
- **`make test-e2e`** падает без e2e-тестов и без Chrome — молчаливого пропуска нет.
- **permissions** (`.claude/settings.json`): запрещены `git push`, `git commit`,
  `rm -rf`, чтение `.env*`. Коммитит человек.

## Обязательный порядок изменения

1. план этапа или его шапка отражает, что делаем;
2. допущение о поведении Chrome — сначала спайк на реальном браузере;
3. объявляем сигнатуры заглушками (`errors.New("не реализовано")`), чтобы код собирался;
4. пишем тест на каждый исход, включая ошибочные, — ожидания из плана, литералом;
   `make red RUN=<тест>` подтверждает осмысленный красный;
5. реализуем;
6. ломаем реализацию намеренно и проверяем, что падает именно нужный кейс;
7. ADR, если решение неочевидно;
8. `make verify`.
