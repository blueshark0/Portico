import {
  fetchAppPreferences,
  saveAppPreferences,
  fetchRouteProfile,
  saveRouteProfile,
  type RouteProfile,
} from './routing'

export type AppSettings = {
  show_heatmap: boolean
  show_home_title: boolean
  proxy_enabled: boolean
  proxy_url: string
  default_claude_provider: string     // Claude 默认供应商名称
  default_codex_provider: string      // Codex 默认供应商名称
  default_gemini_provider: string     // Gemini 默认供应商名称
}

const DEFAULT_SETTINGS: AppSettings = {
  show_heatmap: true,
  show_home_title: true,
  proxy_enabled: false,
  proxy_url: '',
  default_claude_provider: '',
  default_codex_provider: '',
  default_gemini_provider: '',
}

export const fetchAppSettings = async (): Promise<AppSettings> => {
  const [preferences, claudeProfile, codexProfile, geminiProfile] = await Promise.all([
    fetchAppPreferences(),
    fetchRouteProfile('claude'),
    fetchRouteProfile('codex'),
    fetchRouteProfile('gemini'),
  ])
  return {
    show_heatmap: preferences?.show_heatmap ?? DEFAULT_SETTINGS.show_heatmap,
    show_home_title: preferences?.show_home_title ?? DEFAULT_SETTINGS.show_home_title,
    proxy_enabled: preferences?.proxy_enabled ?? DEFAULT_SETTINGS.proxy_enabled,
    proxy_url: preferences?.proxy_url ?? DEFAULT_SETTINGS.proxy_url,
    default_claude_provider: defaultProviderName(claudeProfile),
    default_codex_provider: defaultProviderName(codexProfile),
    default_gemini_provider: defaultProviderName(geminiProfile),
  }
}

export const saveAppSettings = async (settings: AppSettings): Promise<AppSettings> => {
  await saveAppPreferences({
    show_heatmap: settings.show_heatmap,
    show_home_title: settings.show_home_title,
    proxy_enabled: settings.proxy_enabled,
    proxy_url: settings.proxy_url,
  })
  await Promise.all([
    applyDefaultProvider('claude', settings.default_claude_provider),
    applyDefaultProvider('codex', settings.default_codex_provider),
    applyDefaultProvider('gemini', settings.default_gemini_provider),
  ])
  return fetchAppSettings()
}

const defaultProviderName = (profile?: RouteProfile | null): string => {
  if (!profile || profile.defaultProviderId == null) return ''
  const provider = profile.providers?.find((item) => item.id === profile.defaultProviderId)
  return provider?.name ?? ''
}

const applyDefaultProvider = async (platform: string, providerName: string): Promise<void> => {
  const profile = await fetchRouteProfile(platform)
  const provider = providerName
    ? profile.providers?.find((item) => item.name === providerName)
    : null
  const nextDefaultProviderId = provider?.id ?? null
  if ((profile.defaultProviderId ?? null) === nextDefaultProviderId) {
    return
  }
  await saveRouteProfile({
    ...profile,
    defaultProviderId: nextDefaultProviderId,
  })
}
