import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: {
      // changeOrigin must stay false: the Manager rejects requests whose
      // Origin host differs from Host (CSRF check), and the browser sends
      // Origin http://127.0.0.1:5173. The string shorthand would rewrite Host
      // to :8080 and every login, logout and settings change would get 403.
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: false },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
  },
})
