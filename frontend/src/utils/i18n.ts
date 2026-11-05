// src/i18n.ts
import { createI18n } from 'vue-i18n'
import { Locale, availableLocales, loadLocaleMessages } from '../locales'

const defaultLocale: Locale = 'zh'
const LOCALE_STORAGE_KEY = 'portico-locale'
const LEGACY_LOCALE_STORAGE_KEY = 'code-switch-locale'
const isBrowser = typeof window !== 'undefined'

export const i18n = createI18n({
  legacy: false, // 使用 Composition API
  locale: defaultLocale, // 默认语言
  fallbackLocale: 'en',
  messages: {},
})

const isLocale = (value: unknown): value is Locale => {
  return typeof value === 'string' && availableLocales.includes(value as Locale)
}

export function getStoredLocale(): Locale {
  if (!isBrowser) {
    return defaultLocale
  }
  const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY)
    ?? window.localStorage.getItem(LEGACY_LOCALE_STORAGE_KEY)
  return isLocale(stored) ? stored : defaultLocale
}

export async function setupI18n(locale: Locale) {
  const messages = await loadLocaleMessages(locale)
  i18n.global.setLocaleMessage(locale, messages)
  i18n.global.locale.value = locale
  if (isBrowser) {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, locale)
  }
}
