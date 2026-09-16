---
name: browser-troubleshooting
description: Диагностика, когда chromectl не запускает Chrome, не подключается к нему или команда зависает — песочница, занятый порт, устаревший state, заблокированная диалогом вкладка, устаревший снапшот, Lighthouse без Node.js. Используй, когда команда chromectl падает с ошибкой или не отвечает. По запросу «chromectl не работает», «браузер не стартует», «/chromectl:browser-troubleshooting».
---

# Диагностика chromectl

Действуй по шагам: сначала сверь сообщение с таблицей, потом — проверки ниже. Устройство
CLI — [ADR-0009](https://github.com/Y91R/chromectl/blob/main/docs/adr/0009-cli-bez-demona.md).

## 1. Сообщение об ошибке

| Сообщение | Причина | Что делать |
|---|---|---|
| «chromectl: для первого запуска нужен Go 1.26+» | бинарь плагина собирается из исходников, а Go нет в PATH | установить Go 1.26+ (https://go.dev/dl) или `go install github.com/Y91R/chromectl/cmd/chromectl@latest` на другой машине с Go |
| `operation not permitted` при первом вызове `chromectl` (в том числе `version`) | сборка бинаря пишет в каталог плагина, песочница Bash это запрещает | повторить с `dangerouslyDisableSandbox: true` — дальше бинарь уже собран |
| `Operation not permitted`, `Failed to create a ProcessSingleton`, `debug-порт … не открылся` при запуске из агента | Chrome не запускается в песочнице Bash (Seatbelt) | повторить команду с `dangerouslyDisableSandbox: true`; настройки песочницы — `/sandbox` |
| «браузер не запущен, выполните chromectl browser start» | нет state для этого `--port` или Chrome закрыли мимо CLI | `chromectl browser start`; проверь, что `--port` тот же |
| «браузер уже запущен: порт …, pid …» | state этого порта жив | работать с ним или `chromectl browser stop` |
| «порт … уже занят» | на порту другой процесс, свой Chrome туда не встанет | другой `--port`; кто занял — `lsof -iTCP:<порт> -sTCP:LISTEN` |
| «исполняемый файл Chrome не найден» | Chrome не в стандартном месте | `CHROME_PATH=/путь/к/chrome` |
| «процесс Chrome завершился при старте …» | профиль занят другим Chrome или Chrome падает | лог в `…/profile/chromectl-chrome.log`; другой `--profile` |
| «вкладка … не отвечает — возможно, её блокирует диалог» | диалог, оставшийся от другой сессии, блокирует страницу | `chromectl navigate --reload` |
| «выбранная вкладка … закрыта» | вкладку закрыли | `chromectl pages list`, затем `pages select <id>` |
| «снапшот устарел, выполните chromectl snapshot» | документ перезагружен или сменился | `chromectl snapshot`, новый uid |
| «uid … не найден в последнем снапшоте» | опечатка или uid другой вкладки | `chromectl snapshot` на нужной вкладке |
| «нет захваченных запросов» | `network get` без `--capture` | повторить действие с `--capture` |
| «для Lighthouse нужен Node.js (npx) в PATH» | нет Node.js | установить Node.js; первый запуск скачивает `lighthouse@13.4.1` — нужна сеть |
| «таймаут ответа», `context deadline exceeded` | страница занята (долгий JS, диалог) или Chrome завис | `chromectl browser status`; `navigate --reload`; в крайнем случае `browser stop` и `start` |

## 2. Состояние браузера

```bash
chromectl browser status              # запущен ли, pid, число вкладок
chromectl pages list                  # вкладки, * — выбранная
curl -s --noproxy '*' http://127.0.0.1:9222/json/version   # отвечает ли debug-порт
```

- `status` говорит «не запущен», а Chrome на порту жив — state удалён или другой
  `CHROMECTL_STATE_DIR`. `browser stop` на нём не сработает: закрой Chrome по pid и
  запусти заново.
- Порт отвечает чужим `Browser` — это не наш Chrome (см. «порт уже занят»).

## 3. Где лежит state и лог

- State: `~/.cache/chromectl/<port>/state.json` (или `$CHROMECTL_STATE_DIR/<port>/`).
  Руками не правь; при безнадёжной путанице — `browser stop`, затем удали каталог порта.
- Профиль и лог Chrome: `<state>/<port>/profile/chromectl-chrome.log`.
- Захват сети: `<state>/<port>/network/<вкладка>.jsonl`.

## 4. Изолировать проблему

1. Отдельный браузер на другом порту и с чистым state:

   ```bash
   CHROMECTL_STATE_DIR=$(mktemp -d) chromectl --port 9333 browser start --headless
   CHROMECTL_STATE_DIR=<тот же каталог> chromectl --port 9333 pages new https://example.com
   ```

2. Воспроизводится на `https://example.com` — дело в окружении (Chrome, песочница,
   порт). Не воспроизводится — в самой странице (долгий JS, диалоги, iframe).
3. Для cross-origin iframe есть ограничения: `screenshot --uid`, `wait-for` и захват
   сети внутрь них не заглядывают.

## 5. Если не помогло

- Проверь версию: `chromectl version`. В репозитории chromectl бинарь свежий после
  `make build`, запускать `./build/chromectl`.
- Прогони e2e на этой машине: `make test-e2e` (вне песочницы) — если e2e зелёные,
  проблема в конкретной странице или окружении команды.
