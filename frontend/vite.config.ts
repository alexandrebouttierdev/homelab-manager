import { defineConfig } from 'vitest/config'
import { loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig(({ mode }) => ({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: {
      // changeOrigin must stay false: the Manager rejects requests whose
      // Origin host differs from Host (CSRF check), and the browser sends
      // Origin http://127.0.0.1:5173. The string shorthand would rewrite Host
      // to :8080 and every login, logout and settings change would get 403.
      // HOMELAB_DEV_API_TARGET points the proxy at a Manager on another port.
      '/api': {
        target:
          loadEnv(mode, '.', 'HOMELAB_DEV_').HOMELAB_DEV_API_TARGET ?? 'http://127.0.0.1:8080',
        changeOrigin: false,
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
  },
}))
