---
# Контракт дизайн-системы (формат google-labs design.md). Источник правды токенов.
# Правится руками; CSS генерируется: make generate → front/src/gen/theme.css.
# Валидация: make design-lint. Ссылки {colors.*} резолвятся; в typography — литералы.
colors:
  bg: "#fafafa"            # neutral-50  — фон страницы
  surface: "#ffffff"       # белые карточки/шапки
  surface-muted: "#f5f5f5" # neutral-100 — бейджи/чипы
  border: "#e5e5e5"        # neutral-200 — границы/разделители
  text: "#171717"          # neutral-900 — основной текст
  text-muted: "#737373"    # neutral-500 — вторичный текст
  text-soft: "#404040"     # neutral-700 — текст на чипах
  primary: "#3b82f6"       # blue-500    — акцент, CTA, ссылки
  primary-hover: "#2563eb" # blue-600    — ховер CTA
  on-primary: "#ffffff"    # текст на акценте
typography:
  body:
    fontFamily: "system-ui, 'Segoe UI', Roboto, sans-serif"
    fontSize: "0.875rem"
    fontWeight: "400"
    lineHeight: "1.25rem"
  heading:
    fontFamily: "system-ui, 'Segoe UI', Roboto, sans-serif"
    fontSize: "0.875rem"
    fontWeight: "500"
    lineHeight: "1.25rem"
  caption:
    fontFamily: "system-ui, 'Segoe UI', Roboto, sans-serif"
    fontSize: "0.75rem"
    fontWeight: "400"
    lineHeight: "1rem"
rounded:
  sm: "0.5rem"   # кнопки
  md: "0.75rem"  # карточки
  full: "9999px" # чипы
spacing:
  card: "1rem"    # внутренний отступ карточки
  tight: "0.75rem"
components:  # справочные — в CSS-классы не разворачиваются; компонент собирается в .vue
  card:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.md}"
  badge:
    backgroundColor: "{colors.surface-muted}"
    textColor: "{colors.text-soft}"
    typography: "{typography.caption}"
    rounded: "{rounded.full}"
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    typography: "{typography.heading}"
    rounded: "{rounded.sm}"
  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text}"
    typography: "{typography.heading}"
    rounded: "{rounded.sm}"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
---

# Дизайн-система chrome_skill

<Overview/Brand: общий тон, референсы, характер интерфейса — заполнить при
инициализации.> База шаблона: сдержанный светлый интерфейс, нейтральная палитра,
единственный синий акцент для действий. Проза описывает intent (narrative over
metrics); точные значения — во frontmatter, ссылки вида `{colors.primary}`.

Источник правды — этот файл. Токены генерируются в `front/src/gen/theme.css`
(`make generate`), который импортирует `front/src/style.css`. Сгенерированный файл
руками не редактируется.

## Цвета

Нейтральная шкала (`bg` → `surface` → `border` → `text`) несёт всю структуру.
`primary` — единственный насыщенный цвет, только для CTA, ссылок и активных
состояний. Вторичный текст — `text-muted`, текст на чипах — `text-soft`.

## Типографика

Один системный шрифт (`system-ui`). Иерархия задаётся весом, не размером:
`heading` (500) против `body` (400). `caption` — для чипов и подписей.

## Layout & Spacing

Плотность средняя: внутренний отступ карточки — `{spacing.card}`, плотные зоны —
`{spacing.tight}`. Базовая сетка — Tailwind spacing.

## Elevation & Depth

Без теней. Глубина создаётся только границей `{colors.border}` (1px) и сменой фона
`bg` → `surface`. Z-иерархия — порядком в потоке, не тенями.

## Shapes

Скругления: чипы — `{rounded.full}`, карточки — `{rounded.md}`, кнопки —
`{rounded.sm}`. Острых углов в интерфейсе не используем.

## Дизайн-система

**Дизайн всегда содержит дизайн-систему** — `front/src/design-system/`. Это реализация
описанных здесь токенов в виде Vue-компонентов, разбитых на слои. Любой UI собирается
из системы, а не из голых утилит и не из хардкода. Слои (от атомов к структуре):

### Примитивы (`design-system/primitives/`)

Атомарные UI-элементы — реализация секции `components` контракта:

- **Button** — `primary` (фон `primary`, текст `on-primary`, ховер `primary-hover`) и
  `secondary` (фон `surface`, граница `border`); скругление `sm`.
- **Input** — фон `surface`, граница `border`, текст `text`, плейсхолдер `text-muted`,
  фокус-кольцо `primary`; скругление `sm`.
- **Card** — поверхность `surface`, граница `border`, скругление `md`; слоты header/footer.
- **Badge** — чип `muted` (`surface-muted`/`text-soft`) и `primary`; скругление `full`.
- **Checkbox** — нативный чекбокс, акцент `primary`, подпись `text`.
- **Select** — нативный селект в стиле `Input`.
- **Spinner** — индикатор загрузки: кольцо `border` с активным сектором `primary`.

### Паттерны (`design-system/patterns/`)

Композиции примитивов под повторяющиеся задачи:

- **FormField** — обёртка поля: подпись `text`, слот для контрола (Input/Select),
  подсказка `text-muted` или ошибка `primary`; вертикальный ритм `{spacing.tight}`.

### Layout (`design-system/layout/`)

Структурные примитивы на токенах `spacing` (без цвета):

- **Stack** — флекс-стопка (`col`/`row`) с отступом `{spacing.tight}`/`{spacing.card}`.
- **Container** — центрированная колонка контента с горизонтальным отступом `{spacing.card}`.

Токены `components` справочные: компонент собирается в `.vue` из Tailwind-утилит
сгенерированных токенов (`bg-surface`, `text-primary`, `rounded-md`, `gap-tight`), а не
из готовых CSS-классов. Новый компонент заводится в нужный слой системы; бизнес-компоненты
композятся из системы (образец — `front/src/components/ExampleCard.vue`). Процедура —
скилл `/add-component`.

## Do's and Don'ts

- ✅ Цвета, радиусы, типографику брать утилитами от сгенерированных токенов.
- ❌ Не хардкодить hex и произвольные значения в компонентах.
- ❌ Не плодить новые акцентные цвета — один `primary`.
- ❌ Не редактировать `front/src/gen/theme.css` руками — менять `DESIGN.md`.
