import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import './styles/theme.css'
import './styles/cs.css'
import { getStoredLocale, i18n, setupI18n } from './utils/i18n'
import { initTheme } from './utils/ThemeManager'
import router from './router/index'

initTheme()
const isMac = navigator.userAgent.includes('Mac')
if (isMac) {
  document.documentElement.classList.add('mac')
}

// 浏览器直连开发（`pnpm dev`）时没有 Wails 运行时，探测 /wails/runtime 是否存在：
// 真实 Wails 环境（`wails3 task dev`）返回 JSON，纯 Vite 返回 404 HTML —— 此时挂载演示数据。
const hasWailsRuntime = async (): Promise<boolean> => {
  if (!import.meta.env.DEV) return true
  try {
    const res = await fetch(`${window.location.origin}/wails/runtime`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{"object":-1,"method":-1}',
    })
    return res.ok && (res.headers.get('content-type') ?? '').includes('application/json')
  } catch {
    return false
  }
}

async function bootstrap() {
  if (!(await hasWailsRuntime())) {
    const { installMockTransport } = await import('./mocks/installMockTransport')
    installMockTransport()
  }
  await setupI18n(getStoredLocale())
  createApp(App).use(router).use(i18n).mount('#app')
}
bootstrap()
