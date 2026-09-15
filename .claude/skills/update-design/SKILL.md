---
name: update-design
description: Изменить дизайн-систему через контракт DESIGN.md — токены, генерация CSS, визуальная проверка
---

# Изменение дизайн-системы

`DESIGN.md` — контракт дизайна (как `openapi.yaml` — контракт API). Формат
google-labs design.md: frontmatter с токенами + проза с intent. CSS-токены
генерируются, `front/src/gen/theme.css` руками не редактируется.

Строго по шагам:

1. **Контракт**: правь `DESIGN.md` — frontmatter (`colors`/`typography`/`rounded`/
   `spacing`) и прозу (Overview, Цвета, Типографика и т.д.). Ссылки на цвета —
   `{colors.primary}` (резолвятся); в `typography` пиши литералы (ссылки там не
   резолвятся). `components` — справочные, в CSS-утилиты не разворачиваются.
2. **Линт**: `make design-lint` — broken-ref, контраст WCAG, структура. Ошибки чини
   в контракте; предупреждения (unused-color, контраст) — на усмотрение.
3. **Генерация**: `make generate` (или `make generate-design`) → обновится
   `front/src/gen/theme.css` с `@theme` Tailwind v4. Файл не трогать руками.
4. **Визуальная проверка**: `cd front && npm run storybook -- --no-open`, открой story
   через browser MCP (chrome-devtools): `navigate_page` на
   `http://localhost:6006/iframe.html?id=example-examplecard--default&viewMode=story`,
   `take_screenshot`, сверь, что компоненты применили новые токены (`bg-surface`,
   `text-primary`, `rounded-md` и т.п.).
5. **ADR (если решение нетривиально)**: смена философии палитры/типографики/движка
   токенов — `docs/adr/NNNN-...md` по шаблону `docs/adr/0000-template.md`; маркер
   `// ADR:` / `/* ADR: */` в рукописном коде (`style.css`, `front/scripts/gen-theme.mjs`).
6. **Проверка**: `make verify` — `theme.css` синхронен с `DESIGN.md`, компоненты на
   токенах (design-guard), всё закоммичено вместе с изменением контракта.

Готово, когда `DESIGN.md`, сгенерированный `theme.css` и скриншот story согласованы.
