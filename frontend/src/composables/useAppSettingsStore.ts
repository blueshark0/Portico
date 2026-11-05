import { computed, ref } from 'vue'
import type { ProviderTab } from '../constants/platforms'
import { fetchAppSettings, saveAppSettings, type AppSettings } from '../services/appSettings'

const DEFAULT_APP_SETTINGS: AppSettings = {
  show_heatmap: true,
  show_home_title: true,
  proxy_enabled: false,
  proxy_url: '',
  default_claude_provider: '',
  default_codex_provider: '',
  default_gemini_provider: '',
}

const appSettings = ref<AppSettings>({ ...DEFAULT_APP_SETTINGS })
const loading = ref(false)
const saving = ref(false)
const initialized = ref(false)

let loadPromise: Promise<AppSettings> | null = null

type DefaultProviderKey =
  | 'default_claude_provider'
  | 'default_codex_provider'
  | 'default_gemini_provider'

const normalizeAppSettings = (settings?: Partial<AppSettings> | null): AppSettings => ({
  show_heatmap: settings?.show_heatmap ?? DEFAULT_APP_SETTINGS.show_heatmap,
  show_home_title: settings?.show_home_title ?? DEFAULT_APP_SETTINGS.show_home_title,
  proxy_enabled: settings?.proxy_enabled ?? DEFAULT_APP_SETTINGS.proxy_enabled,
  proxy_url: settings?.proxy_url ?? DEFAULT_APP_SETTINGS.proxy_url,
  default_claude_provider:
    settings?.default_claude_provider ?? DEFAULT_APP_SETTINGS.default_claude_provider,
  default_codex_provider:
    settings?.default_codex_provider ?? DEFAULT_APP_SETTINGS.default_codex_provider,
  default_gemini_provider:
    settings?.default_gemini_provider ?? DEFAULT_APP_SETTINGS.default_gemini_provider,
})

const cloneAppSettings = (settings: AppSettings): AppSettings => ({
  ...normalizeAppSettings(settings),
})

const getDefaultProviderKey = (tab: ProviderTab): DefaultProviderKey => {
  if (tab === 'claude') return 'default_claude_provider'
  if (tab === 'codex') return 'default_codex_provider'
  return 'default_gemini_provider'
}

const loadSettings = async (force = false): Promise<AppSettings> => {
  if (loadPromise) {
    return loadPromise
  }
  if (initialized.value && !force) {
    return cloneAppSettings(appSettings.value)
  }

  const previous = cloneAppSettings(appSettings.value)
  loadPromise = (async () => {
    loading.value = true
    try {
      const settings = normalizeAppSettings(await fetchAppSettings())
      appSettings.value = settings
      initialized.value = true
      return cloneAppSettings(settings)
    } catch (error) {
      appSettings.value = initialized.value ? previous : cloneAppSettings(DEFAULT_APP_SETTINGS)
      throw error
    } finally {
      loading.value = false
      loadPromise = null
    }
  })()

  return loadPromise
}

const updateSettings = async (partial: Partial<AppSettings>): Promise<AppSettings> => {
  const previous = cloneAppSettings(appSettings.value)
  const nextSettings = normalizeAppSettings({
    ...appSettings.value,
    ...partial,
  })

  appSettings.value = nextSettings
  saving.value = true
  try {
    const saved = normalizeAppSettings(await saveAppSettings(nextSettings))
    appSettings.value = saved
    initialized.value = true
    return cloneAppSettings(saved)
  } catch (error) {
    appSettings.value = previous
    throw error
  } finally {
    saving.value = false
  }
}

export const useAppSettingsStore = () => {
  const showHeatmap = computed(() => appSettings.value.show_heatmap)
  const showHomeTitle = computed(() => appSettings.value.show_home_title)
  const proxyEnabled = computed({
    get: () => appSettings.value.proxy_enabled,
    set: (val: boolean) => { appSettings.value.proxy_enabled = val },
  })
  const proxyURL = computed(() => appSettings.value.proxy_url)

  const getDefaultProviderName = (tab: ProviderTab) => {
    return appSettings.value[getDefaultProviderKey(tab)]
  }

  return {
    appSettings,
    getDefaultProviderName,
    initialized,
    loadSettings,
    loading,
    proxyEnabled,
    proxyURL,
    saving,
    showHeatmap,
    showHomeTitle,
    updateSettings,
  }
}
