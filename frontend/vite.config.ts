import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

const apiTarget = 'http://127.0.0.1:6006'
const publicOrigin =
  process.env.VITE_PUBLIC_ORIGIN ??
  'https://uu400116-8dae-6907077e.westd.seetacloud.com:8443'

const proxy = {
  '/api': { target: apiTarget, changeOrigin: true },
  '/action': { target: apiTarget, changeOrigin: true },
  '/ws': { target: apiTarget, ws: true, changeOrigin: true },
}

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: {
    host: '0.0.0.0',
    port: 6008,
    strictPort: true,
    allowedHosts: true,
    origin: publicOrigin,
    hmr: {
      protocol: 'wss',
      host: 'uu400116-8dae-6907077e.westd.seetacloud.com',
      clientPort: 8443,
    },
    proxy,
  },
  preview: {
    host: '0.0.0.0',
    port: 6008,
    strictPort: true,
    allowedHosts: true,
    proxy,
  },
})
