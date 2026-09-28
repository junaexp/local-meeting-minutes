import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vitest/config'
import tailwindcss from '@tailwindcss/vite'
import { svelteTesting } from '@testing-library/svelte/vite'
import path from 'node:path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [svelte(), tailwindcss(), svelteTesting()],
  resolve: { alias: { $lib: path.resolve('./src/lib') } },
  server: { host: '127.0.0.1', proxy: { '/api': 'http://127.0.0.1:8791' } },
  test: { environment: 'jsdom', include: ['src/**/*.test.ts'] },
})
