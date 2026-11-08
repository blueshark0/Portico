import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 开发时把 /api 代理到本机 portico-server（PORTICO_ADMIN_URL 可覆盖）；
// 后端未运行时 main.ts 探测失败，自动回退到演示数据。
const adminURL = process.env.PORTICO_ADMIN_URL || 'http://127.0.0.1:18101'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  server: {
    port: parseInt(process.env.VITE_PORT || '9245'),
    strictPort: true,
    proxy: {
      '/api': { target: adminURL, changeOrigin: true },
    },
  },
})
