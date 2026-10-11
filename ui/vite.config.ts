import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  plugins: [vue(), VitePWA({
    registerType: 'prompt',
    manifest: {
      name: 'wslpp — WSL Port Proxy', short_name: 'wslpp',
      description: 'View and manage TCP/UDP port forwarding from Windows to WSL',
      display: 'standalone', start_url: '/', theme_color: '#176b54', background_color: '#f5f6f8',
      icons: [
        { src: '/icon.svg', sizes: 'any', type: 'image/svg+xml', purpose: 'any' },
        { src: '/pwa-192.png', sizes: '192x192', type: 'image/png' },
        { src: '/pwa-512.png', sizes: '512x512', type: 'image/png' }
      ]
    },
    workbox: { globPatterns: ['**/*.{js,css,svg,png}'], navigateFallback: null, runtimeCaching: [] }
  })],
  server: { proxy: { '/api': { target: 'http://127.0.0.1:47831', changeOrigin: true } } },
  build: { outDir: '../internal/web/static', emptyOutDir: true }
})
