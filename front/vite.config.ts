import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  // Статические исходники (favicon и т.п.) — в front/static, а build кладётся в
  // front/public, откуда бэкенд раздаёт фронт с диска (без go:embed).
  publicDir: 'static',
  build: {
    outDir: 'public',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
