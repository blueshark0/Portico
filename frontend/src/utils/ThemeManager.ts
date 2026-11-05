// src/utils/ThemeManager.ts
const THEME_KEY = 'theme'
const SYSTEM_THEME_QUERY = '(prefers-color-scheme: dark)'

let themeInitialized = false

export type ThemeMode = 'light' | 'dark' | 'systemdefault'

export function applyTheme(mode: ThemeMode) {
  let resolvedTheme = mode
  if (mode === 'systemdefault') {
    resolvedTheme = window.matchMedia(SYSTEM_THEME_QUERY).matches ? 'dark' : 'light'
  }

  document.documentElement.classList.remove('dark', 'light')
  document.documentElement.classList.add(resolvedTheme)
}

export function initTheme() {
  if (themeInitialized) {
    return
  }
  themeInitialized = true

  const savedTheme = (localStorage.getItem(THEME_KEY) || 'systemdefault') as ThemeMode
  applyTheme(savedTheme)

  window.matchMedia(SYSTEM_THEME_QUERY).addEventListener('change', () => {
    const current = getCurrentTheme()
    if (current === 'systemdefault') {
      applyTheme('systemdefault')
    }
  })
}

export function setTheme(mode: ThemeMode) {
  localStorage.setItem(THEME_KEY, mode)
  applyTheme(mode)
}

export function getCurrentTheme(): ThemeMode {
  return (localStorage.getItem(THEME_KEY) || 'systemdefault') as ThemeMode
}
