import { computed, reactive, ref } from 'vue'
import { providerTabIds, providerTabs, type ProviderTab } from '../../constants/platforms'
import { fetchRouteProfile, saveRouteProfile, type RouteProfile } from '../../services/routing'
import { testProviderConnection, type ProviderProbeResult } from '../../services/providers'
import type { ProviderRecord } from '../../services/providers'
import { getErrorMessage } from '../../utils/errors'
import { showToast } from '../../utils/toast'
import type { TranslateFn } from '../../types/i18n'
import { formatDateTime, parseDateTime } from '../../utils/dateTime'

export type DirectoryEntry = {
  platform: ProviderTab
  platformLabel: string
  provider: ProviderRecord
}

export type ProviderDirectoryView = 'active' | 'archived'

export const hostOf = (apiUrl: string) => {
  try {
    return new URL(apiUrl).host
  } catch {
    return apiUrl
  }
}

export const modelOf = (entry: DirectoryEntry) => {
  const models = Object.keys(entry.provider.supportedModels ?? {}).filter(
    (key) => entry.provider.supportedModels?.[key] !== false,
  )
  return models[0] ?? ''
}

export const entrySubtitle = (entry: DirectoryEntry) =>
  [entry.platformLabel, modelOf(entry) || hostOf(entry.provider.apiUrl)].filter(Boolean).join(' · ')

export const formatArchivedAt = (value: unknown, includeTime = false) => {
  const parsed = parseDateTime(value)
  if (!parsed) return ''
  const formatted = formatDateTime(parsed)
  return includeTime ? formatted.slice(0, 16) : formatted.slice(0, 10)
}

export type ProviderForm = {
  name: string
  apiUrl: string
  apiKey: string
  officialSite: string
  supportedModels: Record<string, boolean>
  modelMapping: Record<string, string>
  requestBodyReplacements: { path: string; matchValue?: string; value: string }[]
  proxyMode: string
  proxyUrl: string
  insecureSkipVerify: boolean
}

export const emptyProviderForm = (): ProviderForm => ({
  name: '',
  apiUrl: '',
  apiKey: '',
  officialSite: '',
  supportedModels: {},
  modelMapping: {},
  requestBodyReplacements: [],
  proxyMode: '',
  proxyUrl: '',
  insecureSkipVerify: false,
})

export const providerToForm = (provider: ProviderRecord): ProviderForm => ({
  name: provider.name,
  apiUrl: provider.apiUrl,
  apiKey: provider.apiKey,
  officialSite: provider.officialSite ?? '',
  supportedModels: Object.fromEntries(
    Object.entries(provider.supportedModels ?? {}).map(([key, value]) => [key, value ?? false]),
  ),
  modelMapping: Object.fromEntries(
    Object.entries(provider.modelMapping ?? {}).map(([key, value]) => [key, value ?? '']),
  ),
  requestBodyReplacements: (provider.requestBodyReplacements ?? []).map((r) => ({
    path: r.path,
    matchValue: r.matchValue,
    value: r.value,
  })),
  proxyMode: provider.proxyMode ?? '',
  proxyUrl: provider.proxyUrl ?? '',
  insecureSkipVerify: provider.insecureSkipVerify ?? false,
})

export const formToProviderPatch = (form: ProviderForm) => ({
  apiUrl: form.apiUrl.trim(),
  apiKey: form.apiKey.trim(),
  officialSite: form.officialSite.trim(),
  supportedModels: { ...(form.supportedModels ?? {}) },
  modelMapping: { ...(form.modelMapping ?? {}) },
  requestBodyReplacements: (form.requestBodyReplacements ?? [])
    .map((r) => ({
      path: r.path.trim(),
      matchValue: r.matchValue?.trim() || undefined,
      value: r.value.trim(),
    }))
    .filter((r) => r.path && r.value),
  proxyMode: form.proxyMode ?? '',
  proxyUrl: (form.proxyUrl ?? '').trim(),
  insecureSkipVerify: form.insecureSkipVerify ?? false,
})

const PROXY_PREFIXES = ['http://', 'https://', 'socks5://', 'socks5h://']

export const useProviderDirectory = (t: TranslateFn, view: ProviderDirectoryView = 'active') => {
  const profiles = reactive<Record<ProviderTab, RouteProfile | null>>({
    claude: null,
    codex: null,
    gemini: null,
  })
  const loading = ref(false)
  const filter = ref<'all' | ProviderTab>('all')
  const selectedKey = ref('')
  // 连接探测结果只在会话内保留（不落库）：key = `${platform}:${id}`
  const probeResults = reactive<Record<string, ProviderProbeResult & { at: number }>>({})
  const probing = ref('')

  const editorState = reactive({
    open: false,
    editing: false,
    platform: 'claude' as ProviderTab,
    providerId: 0,
    form: emptyProviderForm(),
    errors: { name: '', apiUrl: '', proxyUrl: '' },
    saving: false,
  })

  const advancedState = reactive({
    open: false,
    entryKey: '',
    tab: 'connection' as 'connection' | 'body',
    draft: emptyProviderForm(),
    saving: false,
  })

  const deleteState = reactive({ open: false, entryKey: '', busy: false })
  const copyState = reactive({ open: false, entryKey: '' })
  const archiveState = reactive({ open: false, entryKey: '', busy: false })
  const restoringKey = ref('')

  const entries = computed<DirectoryEntry[]>(() => {
    const list: DirectoryEntry[] = []
    for (const tab of providerTabs) {
      const profile = profiles[tab.id]
      for (const provider of profile?.providers ?? []) {
        list.push({ platform: tab.id, platformLabel: tab.label, provider })
      }
    }
    return list
  })

  const activeEntries = computed(() => entries.value.filter((e) => !e.provider.archived))
  const archivedEntries = computed(() => entries.value.filter((e) => e.provider.archived))
  const viewEntries = computed(() => (view === 'archived' ? archivedEntries.value : activeEntries.value))

  const filteredEntries = computed(() => {
    if (filter.value === 'all') return viewEntries.value
    return viewEntries.value.filter((e) => e.platform === filter.value)
  })

  const entryKey = (entry: DirectoryEntry) => `${entry.platform}:${entry.provider.id}`

  const findEntry = (key: string): DirectoryEntry | null => {
    return entries.value.find((e) => entryKey(e) === key) ?? null
  }

  const selectedEntry = computed(() => {
    const selected = filteredEntries.value.find((entry) => entryKey(entry) === selectedKey.value)
    return selected ?? filteredEntries.value[0] ?? null
  })

  const isDefault = (entry: DirectoryEntry) =>
    profiles[entry.platform]?.defaultProviderId === entry.provider.id

  const loadAll = async () => {
    loading.value = true
    try {
      await Promise.all(
        providerTabIds.map(async (tab) => {
          profiles[tab] = await fetchRouteProfile(tab)
        }),
      )
    } catch (error) {
      showToast(getErrorMessage(error, t('providersPage.messages.loadFailed')), 'error')
    } finally {
      loading.value = false
    }
  }

  // 单平台落库：position 统一重排，保持与旧实现一致。
  const persistPlatform = async (platform: ProviderTab) => {
    const profile = profiles[platform]
    if (!profile) return
    profile.providers = profile.providers.map((p, index) => ({ ...p, position: index + 1 }))
    const saved = await saveRouteProfile(profile)
    profiles[platform] = saved
  }

  const withPlatformMutation = async (
    platform: ProviderTab,
    mutate: (providers: ProviderRecord[]) => boolean | void,
    failKey: string,
  ) => {
    const profile = profiles[platform]
    if (!profile) return false
    const snapshot = JSON.parse(JSON.stringify(profile.providers)) as ProviderRecord[]
    if (mutate(profile.providers) === false) return false
    try {
      await persistPlatform(platform)
      return true
    } catch (error) {
      profile.providers = snapshot
      showToast(getErrorMessage(error, t(failKey)), 'error')
      return false
    }
  }

  // ---- 编辑器 ----

  const openCreate = (platform?: ProviderTab) => {
    editorState.open = true
    editorState.editing = false
    editorState.platform = platform ?? (filter.value === 'all' ? 'claude' : filter.value)
    editorState.providerId = 0
    editorState.form = emptyProviderForm()
    editorState.errors = { name: '', apiUrl: '', proxyUrl: '' }
  }

  const openEdit = (entry: DirectoryEntry) => {
    editorState.open = true
    editorState.editing = true
    editorState.platform = entry.platform
    editorState.providerId = entry.provider.id
    editorState.form = providerToForm(entry.provider)
    editorState.errors = { name: '', apiUrl: '', proxyUrl: '' }
  }

  const validateForm = (): boolean => {
    const form = editorState.form
    const name = form.name.trim()
    editorState.errors.name = ''
    editorState.errors.apiUrl = ''
    editorState.errors.proxyUrl = ''

    let ok = true
    if (!name) {
      editorState.errors.name = t('providersPage.form.errors.nameRequired')
      ok = false
    } else {
      const dup = (profiles[editorState.platform]?.providers ?? []).some(
        (p) =>
          p.name.trim().toLowerCase() === name.toLowerCase() &&
          (!editorState.editing || p.id !== editorState.providerId),
      )
      if (dup) {
        editorState.errors.name = t('providersPage.form.errors.duplicateName')
        ok = false
      }
    }

    try {
      const parsed = new URL(form.apiUrl.trim())
      if (!/^https?:/.test(parsed.protocol)) throw new Error('protocol')
    } catch {
      editorState.errors.apiUrl = t('providersPage.form.errors.invalidUrl')
      ok = false
    }

    if (form.proxyMode === 'custom') {
      const proxyUrl = form.proxyUrl.trim()
      if (!proxyUrl) {
        editorState.errors.proxyUrl = t('providersPage.form.errors.proxyUrlRequired')
        ok = false
      } else if (!PROXY_PREFIXES.some((p) => proxyUrl.startsWith(p))) {
        editorState.errors.proxyUrl = t('providersPage.form.errors.invalidProxyUrl')
        ok = false
      }
    }
    return ok
  }

  const submitEditor = async () => {
    if (!validateForm()) return
    editorState.saving = true
    const platform = editorState.platform
    const patch = formToProviderPatch(editorState.form)
    try {
      const ok = await withPlatformMutation(
        platform,
        (providers) => {
          if (editorState.editing) {
            const index = providers.findIndex((p) => p.id === editorState.providerId)
            if (index < 0) return false
            // 后端约束：已有供应商的 name 不可修改（id 稳定名）。
            providers.splice(index, 1, { ...providers[index], ...patch })
            return true
          }
          const nextId = Math.max(0, ...providers.map((p) => p.id)) + 1
          providers.unshift({
            id: nextId,
            name: editorState.form.name.trim(),
            tint: '',
            accent: '',
            position: 0,
            archived: false,
            ...patch,
          } as ProviderRecord)
          return true
        },
        'providersPage.messages.saveFailed',
      )
      if (ok) {
        editorState.open = false
        showToast(t('providersPage.messages.saved'))
      }
    } finally {
      editorState.saving = false
    }
  }

  // ---- 高级配置：在草稿上编辑，保存时整体写回 ----

  const openAdvanced = (entry: DirectoryEntry, tab: 'connection' | 'body' = 'connection') => {
    advancedState.open = true
    advancedState.entryKey = entryKey(entry)
    advancedState.tab = tab
    advancedState.draft = providerToForm(entry.provider)
  }

  const submitAdvanced = async () => {
    const entry = findEntry(advancedState.entryKey)
    if (!entry) return
    advancedState.saving = true
    const patch = formToProviderPatch(advancedState.draft)
    try {
      const ok = await withPlatformMutation(
        entry.platform,
        (providers) => {
          const index = providers.findIndex((p) => p.id === entry.provider.id)
          if (index < 0) return false
          providers.splice(index, 1, { ...providers[index], ...patch })
          return true
        },
        'providersPage.messages.saveFailed',
      )
      if (ok) {
        advancedState.open = false
        showToast(t('providersPage.messages.saved'))
      }
    } finally {
      advancedState.saving = false
    }
  }

  // ---- 操作 ----

  const setDefault = async (entry: DirectoryEntry) => {
    const profile = profiles[entry.platform]
    if (!profile || isDefault(entry)) return
    const previous = profile.defaultProviderId ?? null
    profile.defaultProviderId = entry.provider.id
    try {
      const saved = await saveRouteProfile(profile)
      profiles[entry.platform] = saved
      showToast(t('providersPage.messages.applied', { name: entry.provider.name }))
    } catch (error) {
      profile.defaultProviderId = previous
      showToast(getErrorMessage(error, t('providersPage.messages.saveFailed')), 'error')
    }
  }

  const testConnection = async (entry: DirectoryEntry) => {
    const key = entryKey(entry)
    if (probing.value) return
    probing.value = key
    try {
      const result = await testProviderConnection(entry.provider)
      probeResults[key] = { ...result, at: Date.now() }
    } catch (error) {
      probeResults[key] = {
        success: false,
        httpStatus: 0,
        durationMs: 0,
        error: getErrorMessage(error, t('providersPage.messages.testFailed')),
        at: Date.now(),
      }
    } finally {
      probing.value = ''
    }
  }

  const probeSummary = (entry: DirectoryEntry | null) => {
    if (!entry) return ''
    const probe = probeResults[entryKey(entry)]
    if (!probe) return t('providersPage.detail.neverTested')
    const time = new Date(probe.at)
    const hhmm = `${String(time.getHours()).padStart(2, '0')}:${String(time.getMinutes()).padStart(2, '0')}`
    if (probe.success) {
      return t('providersPage.detail.testOk', { ms: probe.durationMs, time: hhmm })
    }
    return t('providersPage.detail.testErr', { error: probe.error ?? '', time: hhmm })
  }

  const requestDelete = (entry: DirectoryEntry) => {
    deleteState.open = true
    deleteState.entryKey = entryKey(entry)
  }

  const confirmDelete = async () => {
    const entry = findEntry(deleteState.entryKey)
    if (!entry) return
    deleteState.busy = true
    try {
      const ok = await withPlatformMutation(
        entry.platform,
        (providers) => {
          const index = providers.findIndex((p) => p.id === entry.provider.id)
          if (index < 0) return false
          providers.splice(index, 1)
          return true
        },
        'providersPage.messages.deleteFailed',
      )
      if (ok) {
        deleteState.open = false
        showToast(t('providersPage.messages.deleted'))
      }
    } finally {
      deleteState.busy = false
    }
  }

  const closeArchive = () => {
    if (archiveState.busy) return
    archiveState.open = false
    archiveState.entryKey = ''
  }

  const requestArchive = (entry: DirectoryEntry) => {
    if (entry.provider.archived) return
    if (isDefault(entry)) {
      showToast(t('providersPage.messages.archiveBlockedBecauseDefault'), 'error')
      return
    }
    archiveState.open = true
    archiveState.entryKey = entryKey(entry)
    archiveState.busy = false
  }

  const confirmArchive = async () => {
    const entry = findEntry(archiveState.entryKey)
    if (!entry || entry.provider.archived) {
      archiveState.open = false
      archiveState.entryKey = ''
      return false
    }
    if (isDefault(entry)) {
      showToast(t('providersPage.messages.archiveBlockedBecauseDefault'), 'error')
      archiveState.open = false
      archiveState.entryKey = ''
      return false
    }
    if (archiveState.busy) return false
    archiveState.busy = true
    try {
      const ok = await withPlatformMutation(
        entry.platform,
        (providers) => {
          const target = providers.find((p) => p.id === entry.provider.id)
          if (!target) return false
          target.archived = true
          target.archivedAt = null
          return true
        },
        'providersPage.messages.saveFailed',
      )
      if (ok) {
        archiveState.open = false
        archiveState.entryKey = ''
        showToast(t('providersPage.messages.archived', { name: entry.provider.name }))
      }
      return ok
    } finally {
      archiveState.busy = false
    }
  }

  const restoreProvider = async (entry: DirectoryEntry) => {
    if (!entry.provider.archived || restoringKey.value) return false
    restoringKey.value = entryKey(entry)
    try {
      const ok = await withPlatformMutation(
        entry.platform,
        (providers) => {
          const target = providers.find((p) => p.id === entry.provider.id)
          if (!target) return false
          target.archived = false
          target.archivedAt = null
          return true
        },
        'providersPage.messages.saveFailed',
      )
      if (ok) {
        showToast(t('providersPage.messages.restored', { name: entry.provider.name }))
      }
      return ok
    } finally {
      restoringKey.value = ''
    }
  }

  // ---- 跨 CLI 快速复制 ----

  const requestCopy = (entry: DirectoryEntry) => {
    copyState.open = true
    copyState.entryKey = entryKey(entry)
  }

  // 生成目标平台草稿：预填编辑器（创建模式），名称冲突时追加后缀；
  // 请求体改写仅 claude 平台支持，复制到其他平台时清空。
  const confirmCopy = (targetPlatform: ProviderTab) => {
    const source = findEntry(copyState.entryKey)
    if (!source) return
    const form = providerToForm(source.provider)
    const siblings = profiles[targetPlatform]?.providers ?? []
    let name = form.name.trim()
    if (siblings.some((p) => p.name.trim().toLowerCase() === name.toLowerCase())) {
      name = `${name} (${providerTabs.find((tab) => tab.id === targetPlatform)?.label})`
    }
    form.name = name
    if (targetPlatform !== 'claude') {
      form.requestBodyReplacements = []
    }
    copyState.open = false
    editorState.open = true
    editorState.editing = false
    editorState.platform = targetPlatform
    editorState.providerId = 0
    editorState.form = form
    editorState.errors = { name: '', apiUrl: '', proxyUrl: '' }
  }

  return {
    profiles,
    loading,
    filter,
    probeResults,
    probing,
    editorState,
    advancedState,
    deleteState,
    copyState,
    archiveState,
    restoringKey,
    filteredEntries,
    archivedEntries,
    selectedEntry,
    selectedKey,
    entryKey,
    findEntry,
    isDefault,
    loadAll,
    openCreate,
    openEdit,
    submitEditor,
    openAdvanced,
    submitAdvanced,
    setDefault,
    testConnection,
    probeSummary,
    requestDelete,
    confirmDelete,
    closeArchive,
    requestArchive,
    confirmArchive,
    restoreProvider,
    requestCopy,
    confirmCopy,
  }
}
