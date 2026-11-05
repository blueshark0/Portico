import { computed, onMounted, ref, watch } from 'vue'
import { providerTabs, type ProviderTab } from '../../constants/platforms'
import { fetchRouteProfile } from '../../services/routing'
import { disableProxy, enableProxy, fetchProxyStatus } from '../../services/claudeSettings'
import { fetchPlatformSessions, type SessionBinding } from '../../services/sessions'
import {
  fetchInjectionStatus,
  fetchMcpEndpoint,
  fetchMcpServers,
  fetchMcpStatuses,
  type McpServerRecord,
  type UpstreamStatus,
} from '../../services/mcp'
import { getErrorMessage } from '../../utils/errors'
import { showToast } from '../../utils/toast'
import { useQuickSwitch } from '../../composables/useQuickSwitch'

export type DashboardClient = {
  platform: ProviderTab
  label: string
  proxyEnabled: boolean
  providerName: string
  model: string
}

export type RecentSession = {
  platform: ProviderTab
  platformLabel: string
  providerName: string
  sessionId: string
  lastSuccessAt: unknown
}

const modelOf = (supported?: Partial<Record<string, boolean>>) => {
  const models = Object.keys(supported ?? {}).filter((key) => supported?.[key] !== false)
  return models[0] ?? ''
}

export const useDashboard = () => {
  const clients = ref<DashboardClient[]>([])
  const recentSessions = ref<RecentSession[]>([])
  const mcpEndpoint = ref('')
  const mcpServers = ref<McpServerRecord[]>([])
  const mcpStatuses = ref<UpstreamStatus[]>([])
  const mcpInjected = ref<Record<string, boolean>>({})
  const loading = ref(true)
  const proxyBusy = ref<Record<string, boolean>>({})

  // 切换客户端接入状态：写入该 CLI 的本地配置指向本机 relay（幂等）。
  const toggleClientProxy = async (platform: ProviderTab, enabled: boolean, failMessage: string) => {
    if (proxyBusy.value[platform]) return
    proxyBusy.value = { ...proxyBusy.value, [platform]: true }
    try {
      if (enabled) {
        await enableProxy(platform)
      } else {
        await disableProxy(platform)
      }
      clients.value = clients.value.map((client) =>
        client.platform === platform ? { ...client, proxyEnabled: enabled } : client,
      )
    } catch (error) {
      showToast(getErrorMessage(error, failMessage), 'error')
    } finally {
      proxyBusy.value = { ...proxyBusy.value, [platform]: false }
    }
  }

  const enabledServerCount = computed(
    () => mcpServers.value.filter((server) => server.enabled).length,
  )
  const totalToolCount = computed(() =>
    mcpStatuses.value.reduce((sum, status) => sum + (status.toolCount || 0), 0),
  )
  // 网关健康：存在启用服务时，全部已连接为正常；任一失败为异常；无启用服务为未启用
  const mcpHealth = computed<'ok' | 'error' | 'idle'>(() => {
    if (enabledServerCount.value === 0) return 'idle'
    const enabledIds = new Set(
      mcpServers.value.filter((server) => server.enabled).map((server) => server.id),
    )
    const states = mcpStatuses.value.filter((status) => enabledIds.has(status.serverId))
    if (states.some((status) => status.state === 'failed')) return 'error'
    if (states.length > 0 && states.every((status) => status.state === 'connected')) return 'ok'
    return 'idle'
  })

  const load = async () => {
    loading.value = true
    try {
      const [statusList, serverList, endpoint, claudeInjected, codexInjected] = await Promise.all([
        fetchMcpStatuses(),
        fetchMcpServers(),
        fetchMcpEndpoint(),
        fetchInjectionStatus('claude'),
        fetchInjectionStatus('codex'),
      ])
      mcpStatuses.value = statusList ?? []
      mcpServers.value = serverList ?? []
      mcpEndpoint.value = endpoint
      mcpInjected.value = { claude: claudeInjected, codex: codexInjected }

      // 并行拉取各平台数据；用返回值而非 push，保证 clients 顺序恒为 providerTabs 声明顺序。
      const results = await Promise.all(
        providerTabs.map(async (tab) => {
          const [profile, proxyStatus, bindings] = await Promise.all([
            fetchRouteProfile(tab.id),
            fetchProxyStatus(tab.id),
            fetchPlatformSessions(tab.id),
          ])
          const current = profile?.providers?.find((p) => p.id === profile.defaultProviderId)
          const clientRow: DashboardClient = {
            platform: tab.id,
            label: tab.label,
            proxyEnabled: Boolean(proxyStatus?.enabled),
            providerName: current?.name ?? '',
            model: current ? modelOf(current.supportedModels) : '',
          }
          const sessions: RecentSession[] = (bindings ?? []).map((binding) => ({
            platform: tab.id,
            platformLabel: tab.label,
            providerName: binding.provider_name,
            sessionId: binding.session_id,
            lastSuccessAt: binding.last_success_at,
          }))
          return { clientRow, sessions }
        }),
      )
      clients.value = results.map((r) => r.clientRow)
      recentSessions.value = results
        .flatMap((r) => r.sessions)
        .sort((a, b) => {
          const ta = new Date(a.lastSuccessAt as string).getTime() || 0
          const tb = new Date(b.lastSuccessAt as string).getTime() || 0
          return tb - ta
        })
        .slice(0, 5)
    } finally {
      loading.value = false
    }
  }

  // 快捷切换面板改完默认供应商后，就地刷新卡片的当前供应商/模型（不触发整页 loading）。
  const refreshClients = async () => {
    const profiles = await Promise.all(providerTabs.map((tab) => fetchRouteProfile(tab.id)))
    const nameModelByPlatform = new Map(
      providerTabs.map((tab, index) => {
        const profile = profiles[index]
        const current = profile?.providers?.find((p) => p.id === profile.defaultProviderId)
        return [
          tab.id,
          {
            providerName: current?.name ?? '',
            model: current ? modelOf(current.supportedModels) : '',
          },
        ] as const
      }),
    )
    clients.value = clients.value.map((client) => ({
      ...client,
      ...(nameModelByPlatform.get(client.platform) ?? {}),
    }))
  }

  const { profileRevision } = useQuickSwitch()
  watch(profileRevision, () => {
    void refreshClients()
  })

  onMounted(() => {
    void load()
  })

  return {
    clients,
    recentSessions,
    mcpEndpoint,
    mcpServers,
    mcpHealth,
    mcpInjected,
    enabledServerCount,
    totalToolCount,
    loading,
    proxyBusy,
    toggleClientProxy,
    reload: load,
  }
}
