import { computed, onUnmounted, reactive, ref, shallowRef } from 'vue'
import {
  McpServer,
  ServerType,
  deleteMcpServer,
  fetchInjectionStatus,
  fetchMcpEndpoint,
  fetchMcpServers,
  fetchMcpRuntimeSnapshot,
  injectMcp,
  removeMcpInjection,
  saveMcpServer,
  setMcpServerEnabled,
  testMcpConnection,
  type McpServerRecord,
  type TestResult,
  type UpstreamStatus,
  type ToolInfo,
} from '../../services/mcp'
import { getErrorMessage } from '../../utils/errors'
import { showToast } from '../../utils/toast'
import { runWithToast } from '../../utils/runWithToast'

export type McpPlatform = 'claude' | 'codex'

export const MCP_PLATFORMS: { id: McpPlatform; label: string }[] = [
  { id: 'claude', label: 'Claude Code' },
  { id: 'codex', label: 'Codex' },
]

export type McpServerForm = {
  id: number
  name: string
  type: ServerType
  command: string
  argsText: string // 空格分隔
  envText: string // 每行 KEY=VALUE
  url: string
  headersText: string // 每行 KEY=VALUE
  enabled: boolean
  timeoutText: string
}

const emptyForm = (): McpServerForm => ({
  id: 0,
  name: '',
  type: ServerType.ServerTypeStdio,
  command: '',
  argsText: '',
  envText: '',
  url: '',
  headersText: '',
  enabled: true,
  timeoutText: '30',
})

const parseKvLines = (text: string): Record<string, string> | undefined => {
  const out: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const idx = trimmed.indexOf('=')
    if (idx <= 0) continue
    out[trimmed.slice(0, idx).trim()] = trimmed.slice(idx + 1).trim()
  }
  return Object.keys(out).length > 0 ? out : undefined
}

const kvToText = (m?: Partial<Record<string, string>>): string =>
  m ? Object.entries(m).map(([k, v]) => `${k}=${v ?? ''}`).join('\n') : ''

export const formToRecord = (form: McpServerForm, position: number): McpServerRecord =>
  new McpServer({
    id: form.id,
    name: form.name.trim(),
    type: form.type,
    command: form.type === ServerType.ServerTypeStdio ? form.command.trim() : '',
    args: form.type === ServerType.ServerTypeStdio ? form.argsText.trim().split(/\s+/).filter(Boolean) : undefined,
    env: form.type === ServerType.ServerTypeStdio ? parseKvLines(form.envText) : undefined,
    url: form.type === ServerType.ServerTypeHTTP ? form.url.trim() : '',
    headers: form.type === ServerType.ServerTypeHTTP ? parseKvLines(form.headersText) : undefined,
    enabled: form.enabled,
    position,
    timeoutSec: parseInt(form.timeoutText, 10) || 30,
  })

export const recordToForm = (record: McpServerRecord): McpServerForm => ({
  id: record.id,
  name: record.name,
  type: record.type,
  command: record.command ?? '',
  argsText: (record.args ?? []).join(' '),
  envText: kvToText(record.env),
  url: record.url ?? '',
  headersText: kvToText(record.headers),
  enabled: record.enabled,
  timeoutText: String(record.timeoutSec || 30),
})

const NAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$/

export const validateServerName = (name: string): boolean =>
  NAME_PATTERN.test(name) && !name.includes('__')

export function useMcpServers(t: (key: string, ...args: unknown[]) => string) {
  const servers = ref<McpServerRecord[]>([])
  const statuses = ref<Record<number, UpstreamStatus>>({})
  // 工具清单只会整体替换，且每项带完整 InputSchema，不需要深层响应式。
  const tools = shallowRef<ToolInfo[]>([])
  let catalogRevision = 0
  let disposed = false
  let runtimeLoadPromise: Promise<void> | null = null
  let queuedRuntimeReload = false
  onUnmounted(() => { disposed = true })
  const endpoint = ref('')
  const loading = ref(false)
  // claude/codex 各自的注入状态；null = 尚未加载
  const injections = reactive<Record<McpPlatform, boolean | null>>({ claude: null, codex: null })
  const injecting = reactive<Record<McpPlatform, boolean>>({ claude: false, codex: false })

  const editorState = reactive({
    open: false,
    form: emptyForm(),
    error: '',
    saving: false,
    testing: false,
    testResult: null as TestResult | null,
  })

  const confirmState = reactive({ open: false, target: null as McpServerRecord | null, busy: false })

  const totalTools = computed(() =>
    Object.values(statuses.value).reduce((sum, s) => sum + (s.toolCount || 0), 0),
  )

  // 状态未变化时保留原对象：1Hz 轮询不应让依赖 statuses 的计算属性每秒重算。
  const applyStatuses = (list: UpstreamStatus[]) => {
    const current = statuses.value
    const unchanged =
      list.length === Object.keys(current).length &&
      list.every((st) => {
        const known = current[st.serverId]
        return known && known.state === st.state && known.toolCount === st.toolCount && known.error === st.error
      })
    if (unchanged) return
    const next: Record<number, UpstreamStatus> = {}
    for (const st of list) next[st.serverId] = st
    statuses.value = next
  }

  // 进行中的请求可能早于刚完成的变更，默认补发一次；只有定时轮询才可以直接跳过。
  const refreshStatuses = (options: { skipIfLoading?: boolean } = {}): Promise<void> => {
    if (disposed) return Promise.resolve()
    if (runtimeLoadPromise) {
      if (!options.skipIfLoading) queuedRuntimeReload = true
      return runtimeLoadPromise
    }
    runtimeLoadPromise = (async () => {
      try {
        do {
          queuedRuntimeReload = false
          const snapshot = await runWithToast(() => fetchMcpRuntimeSnapshot(catalogRevision), {
            fallback: t('components.mcp.messages.statusLoadFailed'),
            silent: true,
          })
          if (!snapshot || disposed) continue
          applyStatuses(snapshot.statuses ?? [])
          if (snapshot.catalogChanged) tools.value = snapshot.tools ?? []
          catalogRevision = snapshot.catalogRevision
        } while (queuedRuntimeReload && !disposed)
      } finally {
        runtimeLoadPromise = null
      }
    })()
    return runtimeLoadPromise
  }

  const loadAll = async () => {
    loading.value = true
    try {
      const [serverList, , ep, claude, codex] = await Promise.all([
        fetchMcpServers(),
        refreshStatuses(),
        fetchMcpEndpoint(),
        fetchInjectionStatus('claude'),
        fetchInjectionStatus('codex'),
      ])
      if (disposed) return
      servers.value = serverList ?? []
      endpoint.value = ep
      injections.claude = claude
      injections.codex = codex
    } catch (error) {
      if (!disposed) showToast(getErrorMessage(error, t('components.mcp.messages.loadFailed')), 'error')
    } finally {
      if (!disposed) loading.value = false
    }
  }

  const validateForm = (form: McpServerForm): string => {
    if (!validateServerName(form.name.trim())) return t('components.mcp.form.errors.name')
    if (form.type === ServerType.ServerTypeStdio && !form.command.trim())
      return t('components.mcp.form.errors.command')
    if (form.type === ServerType.ServerTypeHTTP && !form.url.trim()) return t('components.mcp.form.errors.url')
    return ''
  }

  const openCreate = () => {
    Object.assign(editorState, {
      open: true,
      form: emptyForm(),
      error: '',
      saving: false,
      testing: false,
      testResult: null,
    })
  }

  const openEdit = (record: McpServerRecord) => {
    Object.assign(editorState, {
      open: true,
      form: recordToForm(record),
      error: '',
      saving: false,
      testing: false,
      testResult: null,
    })
  }

  const closeEditor = () => {
    editorState.open = false
  }

  const submitEditor = async (): Promise<McpServerRecord | null> => {
    const error = validateForm(editorState.form)
    if (error) {
      editorState.error = error
      return null
    }
    editorState.saving = true
    editorState.error = ''
    try {
      const editing = servers.value.find((s) => s.id === editorState.form.id)
      const position = editing?.position ?? servers.value.length
      const saved = await saveMcpServer(formToRecord(editorState.form, position))
      const idx = servers.value.findIndex((s) => s.id === saved.id)
      if (idx >= 0) servers.value[idx] = saved
      else servers.value.push(saved)
      editorState.open = false
      showToast(t('components.mcp.messages.saved'), 'success')
      void refreshStatuses()
      return saved
    } catch (err) {
      editorState.error = getErrorMessage(err, t('components.mcp.messages.saveFailed'))
      return null
    } finally {
      editorState.saving = false
    }
  }

  // 保存并测试：先落库关闭弹窗，再对已保存配置做连接探测并以 toast 报告结果。
  const submitEditorAndTest = async (): Promise<void> => {
    const saved = await submitEditor()
    if (!saved) return
    editorState.testing = true
    try {
      const result = await testMcpConnection(saved)
      if (result.success) {
        showToast(t('components.mcp.form.testOk', { count: result.toolCount, ms: result.durationMs }))
      } else {
        showToast(t('components.mcp.form.testErr', { error: result.error ?? '' }), 'error')
      }
    } catch (err) {
      showToast(getErrorMessage(err, t('components.mcp.messages.testFailed')), 'error')
    } finally {
      editorState.testing = false
    }
    void refreshStatuses()
  }

  const runTest = async () => {
    const error = validateForm(editorState.form)
    if (error) {
      editorState.error = error
      return
    }
    editorState.testing = true
    editorState.testResult = null
    editorState.error = ''
    try {
      editorState.testResult = await testMcpConnection(
        formToRecord(editorState.form, 0),
      )
    } catch (err) {
      editorState.testResult = {
        success: false,
        toolCount: 0,
        error: getErrorMessage(err, t('components.mcp.messages.testFailed')),
        durationMs: 0,
      } as TestResult
    } finally {
      editorState.testing = false
    }
  }

  const toggleEnabled = async (record: McpServerRecord) => {
    const next = !record.enabled
    const snapshot = record.enabled
    record.enabled = next // optimistic
    try {
      await setMcpServerEnabled(record.id, next)
      void refreshStatuses()
    } catch (err) {
      record.enabled = snapshot // rollback
      showToast(getErrorMessage(err, t('components.mcp.messages.toggleFailed')), 'error')
    }
  }

  const askDelete = (record: McpServerRecord) => {
    confirmState.target = record
    confirmState.open = true
  }

  const confirmDelete = async () => {
    const target = confirmState.target
    if (!target) return
    confirmState.busy = true
    try {
      await deleteMcpServer(target.id)
      servers.value = servers.value.filter((s) => s.id !== target.id)
      confirmState.open = false
      showToast(t('components.mcp.messages.deleted'), 'success')
      void refreshStatuses()
    } catch (err) {
      showToast(getErrorMessage(err, t('components.mcp.messages.deleteFailed')), 'error')
    } finally {
      confirmState.busy = false
    }
  }

  const toggleInjection = async (platform: McpPlatform) => {
    if (injecting[platform]) return
    injecting[platform] = true
    const currently = injections[platform] === true
    injections[platform] = !currently // optimistic
    try {
      if (currently) {
        await removeMcpInjection(platform)
      } else {
        await injectMcp(platform)
      }
      showToast(
        currently
          ? t('components.mcp.messages.injectionRemoved')
          : t('components.mcp.messages.injected'),
        'success',
      )
    } catch (err) {
      injections[platform] = currently // rollback
      showToast(
        getErrorMessage(err, t('components.mcp.messages.injectionFailed')),
        'error',
      )
    } finally {
      injecting[platform] = false
    }
  }

  return {
    servers,
    statuses,
    tools,
    endpoint,
    loading,
    injections,
    injecting,
    editorState,
    confirmState,
    totalTools,
    loadAll,
    refreshStatuses,
    openCreate,
    openEdit,
    closeEditor,
    submitEditor,
    submitEditorAndTest,
    runTest,
    toggleEnabled,
    askDelete,
    confirmDelete,
    toggleInjection,
  }
}
