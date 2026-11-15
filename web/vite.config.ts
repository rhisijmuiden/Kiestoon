import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: { '/api': 'http://localhost:8080' },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Never inline assets as data: URIs; the portal's Content-Security-Policy only allows 'self'.
    assetsInlineLimit: 0,
    chunkSizeWarningLimit: 900,
  },
})
