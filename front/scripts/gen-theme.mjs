// Генерирует front/src/gen/theme.css из контракта DESIGN.md.
// Движок — официальный CLI @google/design.md (export --format css-tailwind даёт
// готовый @theme Tailwind v4). Wrapper добавляет шапку-маркер и чинит одну особенность
// alpha-CLI: значения font-family он оборачивает в кавычки целиком, что ломает
// многосемейный стек и keyword system-ui — снимаем кавычки у --font-<name>.
// ADR: docs/adr/0002-design-md-kak-kontrakt-dizayna.md
import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'

const bin = resolve('node_modules/.bin/design.md')

let css
try {
  css = execFileSync(bin, ['export', '--format', 'css-tailwind', '../DESIGN.md'], {
    encoding: 'utf8',
  })
} catch (e) {
  console.error('design.md export не выполнился:', e.message)
  process.exit(1)
}

if (!css.includes('@theme')) {
  console.error('design.md export не вернул @theme — проверь DESIGN.md (make design-lint)')
  process.exit(1)
}

css = css.replace(/^(\s*--font-[\w-]+:\s*)"(.*)";$/gm, '$1$2;')

const header =
  '/* СГЕНЕРИРОВАНО из DESIGN.md командой `make generate`. Не редактировать руками. */\n' +
  '/* Источник: DESIGN.md  •  ADR: docs/adr/0002-design-md-kak-kontrakt-dizayna.md */\n\n'

mkdirSync('src/gen', { recursive: true })
writeFileSync('src/gen/theme.css', header + css)
console.log('front/src/gen/theme.css обновлён из DESIGN.md')
