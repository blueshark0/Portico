import type { ModelMappingScheme, ProviderRecord } from '../services/routing'

export const schemeMapping = (scheme?: ModelMappingScheme | null): Record<string, string> =>
  Object.fromEntries(
    Object.entries(scheme?.mapping ?? {}).map(([key, value]) => [key, value ?? '']),
  )

export const schemesOf = (provider?: ProviderRecord | null): ModelMappingScheme[] =>
  provider?.modelMappingSchemes ?? []

export const activeSchemeOf = (provider?: ProviderRecord | null): ModelMappingScheme | null => {
  const activeName = provider?.activeModelMappingScheme ?? ''
  return schemesOf(provider).find((scheme) => scheme.name === activeName) ?? null
}

export const activeMappingOf = (provider?: ProviderRecord | null): Record<string, string> =>
  schemeMapping(activeSchemeOf(provider))

export const mappingManagerPath = (platform: string, providerID: number) =>
  `/providers/${encodeURIComponent(platform)}/${providerID}/mapping`

export const mappingCreatePath = (platform: string, providerID: number, copyFrom = '') => {
  const query = copyFrom ? `?copy=${encodeURIComponent(copyFrom)}` : ''
  return `${mappingManagerPath(platform, providerID)}/new${query}`
}

export const mappingEditPath = (platform: string, providerID: number, name: string) =>
  `${mappingManagerPath(platform, providerID)}/edit?scheme=${encodeURIComponent(name)}`

export const firstMappingPair = (scheme: ModelMappingScheme) => {
  const entries = Object.entries(schemeMapping(scheme)).sort(([left], [right]) => left.localeCompare(right))
  return entries.find(([source, target]) => source !== target) ?? entries[0] ?? null
}
