# 0002. DESIGN.md как контракт дизайна (формат google-labs design.md)

- **Статус:** принято
- **Дата:** 2026-06-18
- **Связано:** `DESIGN.md`, `front/scripts/gen-theme.mjs`, `front/src/style.css`,
  `Makefile` (`generate-design`, `design-lint`)

## Контекст

Дизайн-токены жили руками в `front/src/style.css` (`@theme` Tailwind v4), а `DESIGN.md`
был прозой с плейсхолдерами. Не было контракта, валидации и связи дизайна с кодом —
это выбивалось из spec-first идентичности шаблона, где API и БД описываются контрактом,
а код генерируется и руками не правится.

## Решение

`DESIGN.md` переведён в формат [google-labs design.md](https://github.com/google-labs-code/design.md):
YAML-frontmatter с токенами (`colors`/`typography`/`rounded`/`spacing`/`components`) +
проза с intent. Это единый источник правды. `make generate` (под-таргет
`generate-design`) экспортит токены в `front/src/gen/theme.css` (`@theme`) официальным
CLI `@google/design.md` (`export --format css-tailwind`), `style.css` импортирует
сгенерированное. `make design-lint` валидирует контракт. Сгенерированный файл под
защитой хука `protect-gen.sh` и `make generate-check` (как и `gen/api`).

## Почему именно так

- **CLI как движок экспорта** (а не свой генератор): `css-tailwind` — целевой формат
  инструмента, эмитит готовый Tailwind-v4 `@theme`; CLI уже умеет разрешать ссылки и
  считать контраст. Свой генератор дублировал бы эту логику и добавил кода в шаблон.
  Тонкий wrapper `gen-theme.mjs` только оркеструет CLI, добавляет шапку-маркер и
  снимает кавычки у `--font-*` (особенность alpha-CLI: оборачивает font-family целиком,
  что ломает многосемейный стек и keyword `system-ui`).
- **Версия пиньётся** (`@google/design.md@0.3.0`, точная, в `devDependencies`): alpha,
  схема меняется между версиями; пин даёт воспроизводимость, дрейф вывода ловит
  `generate-check`.
- **`components`-токены справочные**, в CSS-классы не разворачиваются: компоненты
  собираются в `.vue` из Tailwind-утилит токенов; второй конкурирующий источник стилей
  не вводим.

## Последствия

Плюс: токены под контрактом и валидацией, дизайн-дрейф ловит CI, проза-intent рядом с
точными значениями, смена токена в `DESIGN.md` меняет вид компонентов без правки `.vue`.
Минус: зависимость от alpha-инструмента (митигирована пином версии и тонким wrapper'ом,
который при необходимости заменяется на собственный парсер без смены контракта);
поведение CLI с composite-typography и `components` проверяется при апгрейде версии.
Альтернатива (собственный генератор YAML→CSS) задокументирована как fallback.

## Где в коде

- `front/scripts/gen-theme.mjs` — wrapper над CLI, помечен `ADR:` в шапке.
- `front/src/style.css` — импорт `./gen/theme.css`, помечен `ADR:`.
- `Makefile` — таргеты `generate-design`, `design-lint`.
