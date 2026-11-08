import { ref } from 'vue'

// 管理 API 的访问令牌。本机访问（桌面窗口、本机浏览器）后端免令牌；
// 远程访问时由登录页写入，或通过 `?token=` 链接（`portico-server token show` 输出）带入。
const TOKEN_STORAGE_KEY = 'portico.adminToken'

/** 为 true 时 App 渲染登录页而不是主界面。 */
export const authRequired = ref(false)

export const getToken = (): string => {
  try {
    return localStorage.getItem(TOKEN_STORAGE_KEY) ?? ''
  } catch {
    return ''
  }
}

export const setToken = (token: string) => {
  localStorage.setItem(TOKEN_STORAGE_KEY, token.trim())
}

export const clearToken = () => {
  localStorage.removeItem(TOKEN_STORAGE_KEY)
}

export const hasStoredToken = (): boolean => getToken() !== ''

export const authHeaders = (token = getToken()): Record<string, string> =>
  token ? { Authorization: `Bearer ${token}` } : {}

/** 收到 401：清除失效令牌并切换到登录页。 */
export const markAuthRequired = () => {
  clearToken()
  authRequired.value = true
}

/**
 * 读取地址栏中的 `?token=`，存入本地后立即从地址栏移除，
 * 避免令牌留在浏览器历史、截图或被复制分享的链接里。
 */
export const consumeTokenFromURL = () => {
  const url = new URL(window.location.href)
  const token = url.searchParams.get('token')
  if (token === null) return
  if (token.trim()) setToken(token)
  url.searchParams.delete('token')
  window.history.replaceState(window.history.state, '', url.toString())
}

/** 用指定令牌探测 /api/session：true 表示可访问，false 表示令牌无效或缺失。 */
export const probeSession = async (token = getToken()): Promise<boolean> => {
  const res = await fetch('/api/session', { headers: authHeaders(token), cache: 'no-store' })
  if (res.status === 401) return false
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return true
}
