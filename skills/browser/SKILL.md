---
name: browser
description: Управление Chrome через CLI chromectl — запустить браузер, открыть страницу, снять снапшот с uid, кликнуть, заполнить форму, нажать клавиши, выполнить JS, дождаться текста, снять скриншот. Используй, когда нужно посмотреть на страницу или story, проверить вёрстку скриншотом, пройти сценарий в браузере. По запросу «открой страницу», «сними скриншот», «заполни форму», «/chromectl:browser».
---

# Браузер через chromectl

Резидентный процесс один — сам Chrome с debug-портом. Каждая команда `chromectl`
подключается к нему, делает одно действие и завершается. Почему так —
[ADR-0009](https://github.com/Y91R/chromectl/blob/main/docs/adr/0009-cli-bez-demona.md).

## Перед началом

- Бинарь: `chromectl` приходит из плагина и уже в PATH. Первый вызов собирает его из
  исходников — нужен Go 1.26+ и запуск с `dangerouslyDisableSandbox: true` (песочница
  не пускает запись в каталог плагина). В репозитории chromectl — `make build` и
  `./build/chromectl`.
- **Chrome не запускается в песочнице Bash** (Seatbelt): команды `chromectl`, которые
  трогают браузер, выполняй с `dangerouslyDisableSandbox: true`. Без браузера
  (`version`, `--help`) — в песочнице.
- Порт по умолчанию 9222 (`--port`). Нужен отдельный браузер — другой `--port`: у
  каждого порта свой state и свой профиль.

## Порядок работы

1. `chromectl browser status` — запущен ли браузер; нет — `browser start --headless`.
2. `chromectl pages new <url>` — открыть страницу (станет выбранной) или
   `navigate <url>` — в выбранной вкладке.
3. `chromectl snapshot` — дерево страницы с uid. **Снапшот предпочтительнее
   скриншота**: он дешевле и даёт uid для действий.
4. Действия по uid: `click`, `fill`, `fill-form`, `press`… Добавь `--snapshot`, чтобы
   сразу получить обновлённое дерево.
5. Проверка: `eval`, `wait-for`, `snapshot`; вёрстка — `screenshot -o x.png` и Read.
6. `chromectl browser stop`, когда браузер больше не нужен.

uid живёт, пока жив документ: после перезагрузки или перехода на другую страницу
команда ответит «снапшот устарел» — сними `snapshot` заново. Узел того же документа
в новом снапшоте сохраняет свой uid.

## Команды

| Команда | Что делает |
|---|---|
| `browser start [--headless] [--profile DIR]` | запустить Chrome с debug-портом |
| `browser stop` / `browser status` | закрыть Chrome / проверить |
| `pages list` | вкладки, `*` — выбранная |
| `pages new <url> [--background]` | открыть вкладку, дождаться загрузки, выбрать |
| `pages select <id>` / `pages close <id>` | выбрать / закрыть (последнюю нельзя) |
| `navigate <url>` / `--back` / `--forward` / `--reload [--ignore-cache]` | переход, `--init-script` на эту навигацию |
| `snapshot [--verbose] [-o файл]` | дерево доступности с uid, включая cross-origin iframe |
| `screenshot [-o файл] [--full-page] [--uid uid] [--format] [--quality]` | скриншот страницы или элемента |
| `click <uid> [--dbl]` | клик мышью |
| `hover <uid>` | навести курсор |
| `drag <from-uid> <to-uid>` | перетащить, в том числе HTML5 drag-and-drop |
| `fill <uid> <значение>` | текст в поле; вариант select по тексту; чекбокс/радио — `true`/`false` |
| `fill-form <uid=значение>...` | несколько полей одной командой — предпочтительнее серии `fill` |
| `type <текст> [--submit Enter]` | печать в элемент с фокусом (сначала `click` по полю) |
| `press <клавиша>` | `Enter`, `Control+A`, `Control+Shift+R`, `Control++` |
| `upload <uid> <файл>...` | файл в input[type=file] или через окно выбора |
| `eval <функция> [--arg uid]... [-o файл]` | JS на странице, результат — JSON в stdout |
| `wait-for <текст>... [--timeout 30s]` | дождаться любого из текстов |
| `dialog accept\|dismiss [--text]` | ответ на диалог для следующей команды |
| `emulate [--viewport WxH[xDPR][,mobile][,touch][,landscape]] [--color-scheme dark\|light\|auto] [--network Offline\|Slow 3G\|Fast 3G\|Slow 4G\|Fast 4G\|none] [--cpu 1..20] [--user-agent] [--geolocation lat,lon] [--headers JSON] [--show]` | эмуляция выбранной вкладки; меняются только переданные флаги, сброс — `""`, `auto`, `none`, `1` |
| `resize <ширина> <высота>` | размер страницы (окна вкладки) |
| `console list [--types error,warn] [--page-size N] [--page-idx N]` | сообщения консоли, включая залогированные до вызова |
| `console get <msgid>` | сообщение целиком: аргументы и стек |
| `network list [--types fetch,xhr] [--page-size N] [--page-idx N]` | запросы последнего `--capture`; без захвата — из Performance API, без заголовков и тел |
| `network get <reqid> [--request-file F] [--response-file F]` | заголовки и тела запроса из последнего захвата |
| `perf trace [--reload=false] [--duration 5s] [-o trace.json.gz]` | трейс одной командой: LCP, FCP, TTFB, CLS, длинные задачи; файл открывается в DevTools |
| `perf insight <trace-file> LCPBreakdown\|LayoutShifts\|LongTasks` | разбор сохранённого трейса, браузер не нужен |
| `heap snapshot -o файл` | снимок кучи `.heapsnapshot` для вкладки Memory DevTools |
| `audit lighthouse [--device desktop\|mobile] [--output-dir D]` | Lighthouse 13.4.1 через npx: accessibility, seo, best-practices, agentic-browsing; нужен Node.js |

Глобальные флаги: `--port`, `--page <id>` (вкладка вместо выбранной на один вызов),
`--json` (машиночитаемый вывод).

Примеры:

```bash
chromectl fill-form 3_4=Иван 3_5=true 3_6=Казань
chromectl eval "(el) => el.getBoundingClientRect().width" --arg 3_7
chromectl eval "() => [...document.querySelectorAll('h2')].map(h => h.textContent)"
```

## Диалоги

Команда закрывает `alert`/`confirm`/`prompt`, открывшиеся во время её работы, **до
выхода** и сообщает их текст (`диалог confirm: … → accept`; у `eval` — в stderr,
чтобы stdout оставался JSON). По умолчанию — accept. `--dialog dismiss` или
`--dialog <текст для prompt>` — на один вызов; `chromectl dialog dismiss` — заранее
для следующей команды.

Если команда пишет «вкладка … не отвечает — возможно, её блокирует диалог» —
выполни `chromectl navigate --reload`: перезагрузка снимает блокировку.

## Чего нет без демона

- `--init-script` у `navigate` действует только в рамках этой навигации.
- Истории сети до вызова нет. Чтобы увидеть заголовки и тела, повтори действие с
  `--capture` (`navigate`, `click`, `fill`, `press`…), затем `network list` и
  `network get <reqid>`. Каждый захват заменяет прежний; `Authorization` и cookie
  маскируются (`--unredacted` — показать), тело хранится до 1 МБ, бинарное — только
  размер. Запросы внутри cross-origin iframe не захватываются.
- Консоль видна целиком, включая сообщения до вызова: Chrome хранит буфер сам.
- `screenshot --uid` внутри cross-origin iframe не поддерживается — снимай страницу.
- `wait-for` ищет текст в главном документе, не внутри cross-origin iframe.
- Состояние — в `~/.cache/chromectl/<port>/state.json` (`CHROMECTL_STATE_DIR`
  переопределяет каталог). Файл руками не правь.

## Смежные скиллы

- `/chromectl:browser-a11y` — аудит доступности: Lighthouse, семантика, фокус, контраст.
- `/chromectl:browser-lcp` — разбор LCP: трейс, LCP-элемент, водопад, оптимизация.
- `/chromectl:browser-troubleshooting` — когда chromectl не стартует, не подключается или
  зависает.

## Ошибки

| Сообщение | Что делать |
|---|---|
| «браузер не запущен, выполните chromectl browser start» | `browser start` |
| «браузер уже запущен: порт …, pid …» | работай с ним или `browser stop` |
| «порт … уже занят» | другой `--port` |
| «выбранная вкладка … закрыта» | `pages list`, затем `pages select <id>` |
| «снапшот устарел, выполните chromectl snapshot» | `snapshot`, взять новый uid |
| «uid … не найден в последнем снапшоте» | uid опечатан или из другой вкладки — `snapshot` |
| «переход на …: net::ERR_…» | адрес недоступен — проверь, запущен ли сервер |
| «исключение в функции: …» | ошибка в JS, переданном в `eval` |
| «исполняемый файл Chrome не найден» | задать `CHROME_PATH` |
