import { Browser } from '@wailsio/runtime'

// 用系统浏览器打开外部链接。桌面窗口里 window.open 不会离开 WebView，
// 必须经 Wails 运行时；纯浏览器预览（无运行时）时退回 window.open。
export const openExternal = async (url: string): Promise<void> => {
  try {
    await Browser.OpenURL(url)
  } catch {
    window.open(url, '_blank', 'noopener,noreferrer')
  }
}
