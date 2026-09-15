import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// Юнит-тесты компонентов (поведение: пропсы, классы токенов, эмиты v-model).
// Tailwind-плагин не нужен — классы проверяются как строки.
export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'happy-dom',
    globals: true,
    include: ['src/**/*.spec.ts'],
  },
})
