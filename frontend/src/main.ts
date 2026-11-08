import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import './styles/theme.css'
import './styles/cs.css'
import { getStoredLocale, i18n, setupI18n } from './utils/i18n'
import { initTheme } from './utils/ThemeManager'
import router from './router/index'
import { authRequired, consumeTokenFromURL, probeSession } from './services/auth'

initTheme()
const isMac = navigator.userAgent.includes('Mac')
if (isMac) {
  document.documentElement.classList.add('mac')
}

// 后端是 portico-server 的管理 API（桌面窗口与浏览器都从它加载页面）。
// 仅在开发模式（`pnpm dev`）下，若 /api/health 不可达（未启动后端、Vite 代理报错）
// 则回退到演示数据；也可用 VITE_PORTICO_MOCK=1 强制使用演示数据。
const hasBackend = async (): Promise<boolean> => {
  if (!import.meta.env.DEV) return true
  if (import.meta.env.VITE_PORTICO_MOCK === '1') return false
  try {
    const res = await fetch('/api/health', { cache: 'no-store' })
    return res.ok && (res.headers.get('content-type') ?? '').includes('application/json')
  } catch {
    return false
  }
}

async function bootstrap() {
  if (await hasBackend()) {
    const { installHttpTransport } = await import('./transport/httpTransport')
    installHttpTransport()
    consumeTokenFromURL()
    try {
      authRequired.value = !(await probeSession())
    } catch (error) {
      // 后端暂时不可用时照常挂载，由各页面的调用错误提示用户。
      console.error('probe session failed', error)
    }
  } else {
    const { installMockTransport } = await import('./mocks/installMockTransport')
    installMockTransport()
  }
  await setupI18n(getStoredLocale())
  createApp(App).use(router).use(i18n).mount('#app')
}
bootstrap()
