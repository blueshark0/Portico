import { getErrorMessage } from './errors'
import { showToast } from './toast'

type RunWithToastOptions = {
  /** 无法从 error 提取信息时展示的兜底文案（已翻译）。 */
  fallback: string
  /** 仅 console.error 留痕、不弹 toast（用于后台静默刷新类调用）。 */
  silent?: boolean
}

// Wails bindings 调用的统一错误通道：console.error 留痕 + toast 提示。
// 取代各调用点自写的 try/catch 样板、window.alert 与静默吞错。
export const runWithToast = async <T>(
  fn: () => Promise<T>,
  { fallback, silent = false }: RunWithToastOptions,
): Promise<T | undefined> => {
  try {
    return await fn()
  } catch (error) {
    console.error(fallback, error)
    if (!silent) {
      showToast(getErrorMessage(error, fallback), 'error')
    }
    return undefined
  }
}
