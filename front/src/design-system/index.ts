// Дизайн-система — единая точка импорта всех слоёв.
// Слои: primitives (атомы) → patterns (композиции) → layout (структура).
// Все собраны на токенах из DESIGN.md (front/src/gen/theme.css).
// ADR: docs/adr/0004-dizayn-sistema-sloi.md
export * from './primitives'
export * from './patterns'
export * from './layout'
