---
name: add-component
description: Добавить компонент в дизайн-систему или собрать бизнес-компонент из неё — на токенах DESIGN.md, со story
---

# Новый компонент

Дизайн всегда содержит дизайн-систему (`front/src/design-system/`) из слоёв:
`primitives/` (атомы), `patterns/` (композиции примитивов), `layout/` (структура). UI
собирается из системы, а не из хардкода. Компоненты — только на сгенерированных токенах
(`bg-surface`, `text-primary`, `rounded-md`, `gap-tight`), хардкод hex запрещён.

Сперва выбери слой:
- **primitives** — атомарный UI-элемент (кнопка, поле, чип).
- **patterns** — композиция примитивов под повторяющуюся задачу (поле формы с подписью).
- **layout** — структурный примитив на токенах spacing, без цвета (стопка, контейнер).

## Новый компонент дизайн-системы

1. **Токены**: если нужны цвета/радиусы/типографика/отступы, которых нет — сперва
   опиши их в `DESIGN.md` и прогенери (`/update-design`). Иначе — пропусти шаг.
2. **Контракт**: для примитива добавь его в `DESIGN.md` — секция `components`
   (frontmatter, валидные под-токены: backgroundColor, textColor, typography, rounded,
   padding, size, height, width) и проза `## Дизайн-система`. Паттерн/layout описываются
   только прозой в соответствующем подразделе.
3. **Реализация**: `front/src/design-system/<слой>/<Name>.vue` — Vue 3
   `<script setup lang="ts">`, типизированные `defineProps`, слоты где уместно. Только
   Tailwind-утилиты токенов. В шапке —
   `<!-- ADR: docs/adr/0004-dizayn-sistema-sloi.md — <слой> дизайн-системы на токенах DESIGN.md -->`.
   Паттерн композится из примитивов (`import { Input } from '../primitives'`).
4. **Реэкспорт**: добавь в `front/src/design-system/<слой>/index.ts` (корневой
   `index.ts` реэкспортит слои автоматически через `export *`).
5. **Story**: `front/src/design-system/<слой>/<Name>.stories.ts` —
   `title: 'Design System/<Слой>/<Name>'` (например `Design System/Primitives/Button`),
   `tags: ['autodocs']`, варианты состояний в отдельных stories.
6. **Визуальная проверка**: `cd front && npm run storybook -- --no-open`, открой через
   chrome-devtools MCP
   `http://localhost:6006/iframe.html?id=design-system-<слой>-<name>--<story>&viewMode=story`,
   `take_screenshot`, сверь варианты. Проверь панель a11y (addon-a11y).
7. **Сборка и проверка**: `cd front && npm run build` (vue-tsc + vite) и `npm run test`
   (Vitest); затем `make verify` — `design-guard` следит, что в компоненте нет
   хардкода hex (только токен-утилиты).

## Бизнес-компонент

Собирается из дизайн-системы (`import { Card, Button } from '../design-system'`), а не из
голых утилит. Живёт в `front/src/components/`. Story рядом с компонентом, та же
визуальная проверка. Образец — `front/src/components/ExampleCard.vue`.
