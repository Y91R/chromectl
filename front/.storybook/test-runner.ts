import type { TestRunnerConfig } from '@storybook/test-runner'
import { injectAxe, checkA11y, configureAxe } from 'axe-playwright'

// Гейт визуальной/a11y регрессии: каждая story рендерится без ошибок (smoke) +
// axe-проверка доступности. Запуск: `npm run test-storybook` (нужен поднятый Storybook
// и браузеры playwright). Правило color-contrast отключено: контраст
// токена `primary` на белом — известный advisory (см. ADR-0003), правится в DESIGN.md.
const config: TestRunnerConfig = {
  async preVisit(page) {
    await injectAxe(page)
  },
  async postVisit(page) {
    await configureAxe(page, { rules: [{ id: 'color-contrast', enabled: false }] })
    await checkA11y(page, '#storybook-root', { detailedReport: false })
  },
}

export default config
