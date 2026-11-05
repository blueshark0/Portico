export const providerTabs = [
  { id: 'claude', label: 'Claude Code' },
  { id: 'codex', label: 'Codex' },
  { id: 'gemini', label: 'Gemini' },
] as const

export type ProviderTab = (typeof providerTabs)[number]['id']

export const providerTabIds = providerTabs.map(({ id }) => id) as ProviderTab[]

export const parseProviderTab = (
  value: unknown,
  fallback: ProviderTab = providerTabs[0].id,
): ProviderTab => {
  const raw = typeof value === 'string' ? value : ''
  if ((providerTabIds as readonly string[]).includes(raw)) {
    return raw as ProviderTab
  }
  return fallback
}
