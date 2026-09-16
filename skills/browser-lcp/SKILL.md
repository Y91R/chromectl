---
name: browser-lcp
description: Отладка и оптимизация Largest Contentful Paint (LCP) через chromectl — трейс загрузки, разбор LCP, поиск LCP-элемента, водопад сети, типовые ошибки разметки, проверка под медленной сетью и CPU. Используй, когда страница медленно загружается, спрашивают про LCP, Core Web Vitals (CWV), скорость появления главного контента или героя-картинки. По запросу «разбери LCP», «почему долго грузится», «/chromectl:browser-lcp». Работает поверх скилла /chromectl:browser.
---

# LCP через chromectl

Команды — из скилла `/chromectl:browser` (Chrome только вне песочницы Bash). Сниппеты — в
[references/lcp-snippets.md](references/lcp-snippets.md), стратегии —
[references/optimization-strategies.md](references/optimization-strategies.md).

## Что такое LCP

Время от начала навигации до отрисовки самого крупного изображения или текстового блока
в видимой области.

- **Хорошо**: до 2.5 с
- **Нужно улучшить**: 2.5–4.0 с
- **Плохо**: больше 4.0 с

На большинстве мобильных страниц LCP-элемент — картинка.

## Из чего складывается LCP

| Часть | Идеальная доля | Что измеряет |
|---|---|---|
| **TTFB** | ~40% | начало навигации → первый байт HTML |
| **Задержка загрузки ресурса** | <10% | TTFB → браузер начал грузить LCP-ресурс |
| **Длительность загрузки ресурса** | ~40% | скачивание LCP-ресурса |
| **Задержка отрисовки** | <10% | ресурс скачан → элемент отрисован |

Обе «задержки» должны стремиться к нулю — если одна из них велика, начинай с неё.
Частая ошибка — сжать картинку (длительность загрузки), когда узкое место — задержка
отрисовки: сэкономленное время просто переходит в неё.

## Порядок работы

### 1. Трейс загрузки

```bash
chromectl navigate <url>
chromectl perf trace -o /tmp/trace.json.gz
```

Команда перезагружает страницу под записью и печатает LCP, FCP, TTFB, CLS и длинные
задачи. Трейс открывается в DevTools (Performance → Load profile).

### 2. Разбор LCP

```bash
chromectl perf insight /tmp/trace.json.gz LCPBreakdown
chromectl perf insight /tmp/trace.json.gz LongTasks
```

`LCPBreakdown` делит LCP на TTFB и «от первого байта до отрисовки» и называет элемент.
Задержку и длительность загрузки ресурса отдельно он не показывает — их даёт шаг 4.
Длинные задачи — частая причина задержки отрисовки.

### 3. LCP-элемент

`chromectl eval "<сниппет «LCP-элемент»>"` — тег, ресурс (`url`) и сырые тайминги. Пустой
`url` — LCP текстовый, грузить нечего.

### 4. Водопад сети

```bash
chromectl navigate <url> --capture
chromectl network list --types image,font,document,stylesheet,script
chromectl network get <reqid>
```

Время старта и длительность LCP-ресурса даёт Performance API:
`chromectl eval "<сниппет «Тайминг LCP-ресурса»>"`.

- **Старт** намного позже документа и первых ресурсов — задержка загрузки ресурса.
- **Длительность** большая — файл тяжёлый или сервер медленный.

### 5. Типовые ошибки разметки

`chromectl eval "<сниппет «Типовые ошибки»>"` — lazy-картинки в первом экране, крупные
картинки без `fetchpriority`, блокирующие скрипты в `<head>`.

## Оптимизация

Сначала узкое место из шагов 2–4, потом правки из
[references/optimization-strategies.md](references/optimization-strategies.md).

## Проверка правок и эмуляция

- Повтори `chromectl perf trace -o /tmp/after.json.gz` и сравни разбор: узкая часть
  должна уменьшиться.
- Лабораторные цифры отличаются от реальных. Проверь под ограничениями:

  ```bash
  chromectl emulate --network "Fast 3G" --cpu 4
  chromectl perf trace -o /tmp/slow.json.gz
  chromectl emulate --network none --cpu 1
  ```
