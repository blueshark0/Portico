// 浏览器直连开发（`pnpm dev`）且 portico-server 未运行时，
// 这里通过 setTransport 拦截全部 binding 调用并按方法名返回演示数据。
// 后端可达时 main.ts 改装 HTTP transport，不会进入此分支。
import { objectNames, setTransport } from '@wailsio/runtime'
import { rpcMethodName } from '../transport/httpTransport'
import {
  mockAppLogEntries,
  mockMcpInjection,
  mockMcpServers,
  mockMcpTools,
  mockPreferences,
  mockProfiles,
  mockProxyStatus,
  mockRemoteSnapshotMeta,
  mockRequestLogs,
  mockSessions,
  mockSyncSettings,
  dayKey,
} from './demoData'

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value))

const delay = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

const inRange = (createdAt: string, rangeKey: string): boolean => {
  const today = dayKey(new Date().toISOString())
  const key = dayKey(createdAt)
  if (rangeKey === 'today') return key === today
  const cutoff = new Date(Date.now() - (rangeKey === 'last3days' ? 2 : 6) * 24 * 60 * 60 * 1000)
  return key >= dayKey(cutoff.toISOString())
}

const filterLogs = (
  logs: typeof mockRequestLogs,
  platform: string,
  provider: string,
  rangeKey: string,
  costTier: string,
  httpStatus: string,
) =>
  logs.filter(
    (log) =>
      (!platform || log.platform === platform) &&
      (!provider || log.provider === provider) &&
      inRange(log.created_at, rangeKey || 'last7days') &&
      (!costTier || costTier === 'all' || log.cost_tier === costTier) &&
      (!httpStatus ||
        httpStatus === 'all' ||
        (httpStatus === 'success' ? log.http_code < 400 : log.http_code >= 400)),
  )

const statsOf = (logs: typeof mockRequestLogs) => {
  const ok = logs.filter((l) => l.http_code < 400)
  return {
    total_requests: logs.length,
    input_tokens: logs.reduce((s, l) => s + l.input_tokens, 0),
    output_tokens: logs.reduce((s, l) => s + l.output_tokens, 0),
    reasoning_tokens: logs.reduce((s, l) => s + l.reasoning_tokens, 0),
    cache_create_tokens: 0,
    cache_read_tokens: logs.reduce((s, l) => s + l.cache_read_tokens, 0),
    cost_total: Number(logs.reduce((s, l) => s + l.total_cost, 0).toFixed(6)),
    cost_input: Number(logs.reduce((s, l) => s + l.input_cost, 0).toFixed(6)),
    cost_output: Number(logs.reduce((s, l) => s + l.output_cost, 0).toFixed(6)),
    cost_cache_create: 0,
    cost_cache_read: Number(logs.reduce((s, l) => s + l.cache_read_cost, 0).toFixed(6)),
    failed_requests: logs.length - ok.length,
    cache_hit_requests: logs.filter((l) => l.cache_read_tokens > 0).length,
    avg_duration_sec: logs.length
      ? Number((ok.reduce((s, l) => s + l.duration_sec, 0) / logs.length).toFixed(1))
      : 0,
  }
}

const seriesOf = (logs: typeof mockRequestLogs) => {
  const byDay = new Map<string, typeof logs>()
  logs.forEach((log) => {
    const key = dayKey(log.created_at)
    byDay.set(key, [...(byDay.get(key) ?? []), log])
  })
  return [...byDay.entries()]
    .sort(([a], [b]) => (a < b ? -1 : 1))
    .map(([day, dayLogs]) => {
      const base = statsOf(dayLogs)
      return {
        day,
        total_requests: base.total_requests,
        input_tokens: base.input_tokens,
        output_tokens: base.output_tokens,
        reasoning_tokens: base.reasoning_tokens,
        cache_create_tokens: base.cache_create_tokens,
        cache_read_tokens: base.cache_read_tokens,
        total_cost: base.cost_total,
      }
    })
}

const mockProviderFor = (platform: string, providerID: number) => {
  const profile = mockProfiles.find((item) => item.platform === platform)
  const provider = profile?.providers.find((item) => item.id === providerID)
  if (!profile || !provider) throw new Error('provider not found (demo)')
  return { profile, provider }
}

const syncMockActiveMapping = (provider: any) => {
  provider.modelMappingSchemes ??= []
  const active = provider.modelMappingSchemes.find(
    (scheme: any) => scheme.name === provider.activeModelMappingScheme,
  )
  provider.modelMapping = active ? clone(active.mapping) : {}
}

let catalogRevision = 1
let catalogFingerprint = ''

// "{Facade}.{Method}" → handler；未知方法返回 null（与真实运行时「方法存在但零值」行为接近）。
const handlers: Record<string, (...args: any[]) => any> = {
  // routingfacade
  'RoutingFacade.GetAppPreferences': () => clone(mockPreferences),
  'RoutingFacade.SaveAppPreferences': (preferences: any) => {
    Object.assign(mockPreferences, preferences)
    return clone(mockPreferences)
  },
  'RoutingFacade.GetProfile': (platform: string) => {
    const profile = mockProfiles.find((p) => p.platform === platform)
    return clone(profile ?? { platform, defaultProviderId: null, providers: [] })
  },
  'RoutingFacade.SaveProfile': (profile: any) => {
    const index = mockProfiles.findIndex((p) => p.platform === profile.platform)
    if (index >= 0) mockProfiles[index] = clone(profile)
    return clone(profile)
  },
  'RoutingFacade.SaveModelMappingScheme': (platform: string, providerID: number, originalName: string, scheme: any, activate: boolean) => {
    const { profile, provider } = mockProviderFor(platform, providerID)
    provider.modelMappingSchemes ??= []
    const index = originalName
      ? provider.modelMappingSchemes.findIndex((item: any) => item.name.toLowerCase() === originalName.toLowerCase())
      : -1
    if (originalName && index < 0) throw new Error('mapping scheme not found (demo)')
    if (provider.modelMappingSchemes.some((item: any, itemIndex: number) =>
      itemIndex !== index && item.name.toLowerCase() === scheme.name.trim().toLowerCase())) {
      throw new Error('duplicate mapping scheme name (demo)')
    }
    const savedScheme = { name: scheme.name.trim(), mapping: clone(scheme.mapping ?? {}) }
    if (index < 0) provider.modelMappingSchemes.push(savedScheme)
    else {
      if (provider.activeModelMappingScheme?.toLowerCase() === originalName.toLowerCase()) {
        provider.activeModelMappingScheme = savedScheme.name
      }
      provider.modelMappingSchemes[index] = savedScheme
    }
    if (activate) provider.activeModelMappingScheme = savedScheme.name
    syncMockActiveMapping(provider)
    return clone(profile)
  },
  'RoutingFacade.DeleteModelMappingScheme': (platform: string, providerID: number, name: string) => {
    const { profile, provider } = mockProviderFor(platform, providerID)
    const index = provider.modelMappingSchemes?.findIndex((item: any) => item.name === name) ?? -1
    if (index < 0) throw new Error('mapping scheme not found (demo)')
    if (provider.activeModelMappingScheme === name) throw new Error('active scheme cannot be deleted (demo)')
    provider.modelMappingSchemes?.splice(index, 1)
    syncMockActiveMapping(provider)
    return clone(profile)
  },
  'RoutingFacade.SetActiveModelMappingScheme': (platform: string, providerID: number, name: string) => {
    const { profile, provider } = mockProviderFor(platform, providerID)
    if (name && !provider.modelMappingSchemes?.some((item: any) => item.name === name)) {
      throw new Error('mapping scheme not found (demo)')
    }
    provider.activeModelMappingScheme = name
    syncMockActiveMapping(provider)
    return clone(profile)
  },
  'RoutingFacade.TestProvider': async (provider: any) => {
    await delay(350)
    const reachable = !/deepseek/i.test(provider?.name ?? '')
    return reachable
      ? { success: true, httpStatus: 200, durationMs: 180 + Math.floor(Math.random() * 400) }
      : { success: false, httpStatus: 0, durationMs: 2000, error: 'probe timed out (demo)' }
  },

  // observabilityfacade
  'ObservabilityFacade.ListRequestLogs': (platform: string, provider: string, rangeKey: string, costTier: string, httpStatus: string, page: number, pageSize: number) => {
    const filtered = filterLogs(mockRequestLogs, platform, provider, rangeKey || 'last7days', costTier, httpStatus)
    const p = Math.max(1, page || 1)
    const size = pageSize || 200
    return { items: clone(filtered.slice((p - 1) * size, p * size)), total: filtered.length, page: p, page_size: size }
  },
  'ObservabilityFacade.ExportRequestLogs': (platform: string, provider: string, rangeKey: string, costTier: string, httpStatus: string) =>
    clone(filterLogs(mockRequestLogs, platform, provider, rangeKey || 'last7days', costTier, httpStatus).slice(0, 5000)),
  'ObservabilityFacade.StatsSince': (platform: string, provider: string, rangeKey: string, costTier: string, httpStatus: string) => ({
    ...statsOf(filterLogs(mockRequestLogs, platform, provider, rangeKey || 'last7days', costTier, httpStatus)),
    series: seriesOf(filterLogs(mockRequestLogs, platform, provider, rangeKey || 'last7days', costTier, httpStatus)),
  }),
  'ObservabilityFacade.ListSessionRequestLogs': (platform: string, sessionID: string) =>
    clone(mockRequestLogs.filter((l) => l.platform === platform && l.session_id === sessionID)),
  'ObservabilityFacade.ListProviders': (platform: string) => {
    const names = new Set(mockRequestLogs.filter((l) => !platform || l.platform === platform).map((l) => l.provider))
    return [...names]
  },
  'ObservabilityFacade.ProviderDailyStats': (platform: string) => {
    const names = new Set(mockRequestLogs.filter((l) => !platform || l.platform === platform).map((l) => l.provider))
    return [...names].map((name) => {
      const logs = mockRequestLogs.filter((l) => l.provider === name && (!platform || l.platform === platform))
      const okCount = logs.filter((l) => l.http_code < 400).length
      return {
        provider: name,
        total_requests: logs.length,
        successful_requests: okCount,
        failed_requests: logs.length - okCount,
        success_rate: logs.length ? okCount / logs.length : 0,
        input_tokens: logs.reduce((s, l) => s + l.input_tokens, 0),
        output_tokens: logs.reduce((s, l) => s + l.output_tokens, 0),
        reasoning_tokens: logs.reduce((s, l) => s + l.reasoning_tokens, 0),
        cache_create_tokens: 0,
        cache_read_tokens: logs.reduce((s, l) => s + l.cache_read_tokens, 0),
        cost_total: Number(logs.reduce((s, l) => s + l.total_cost, 0).toFixed(6)),
      }
    })
  },
  'ObservabilityFacade.HeatmapStats': (days: number) => {
    const cutoff = new Date(Date.now() - (days - 1) * 24 * 60 * 60 * 1000)
    const byDay = new Map<string, typeof mockRequestLogs>()
    mockRequestLogs.forEach((log) => {
      const key = dayKey(log.created_at)
      if (key >= dayKey(cutoff.toISOString())) byDay.set(key, [...(byDay.get(key) ?? []), log])
    })
    return [...byDay.entries()]
      .sort(([a], [b]) => (a < b ? -1 : 1))
      .map(([day, logs]) => ({
        day,
        total_requests: logs.length,
        input_tokens: logs.reduce((s, l) => s + l.input_tokens, 0),
        output_tokens: logs.reduce((s, l) => s + l.output_tokens, 0),
        reasoning_tokens: logs.reduce((s, l) => s + l.reasoning_tokens, 0),
        total_cost: Number(logs.reduce((s, l) => s + l.total_cost, 0).toFixed(6)),
      }))
  },

  // sessionfacade
  'SessionFacade.ListRecent': (limit: number) => clone(
    ['claude', 'codex', 'gemini'].flatMap((platform) => mockSessions(platform))
      .sort((a, b) => new Date(b.last_success_at).getTime() - new Date(a.last_success_at).getTime())
      .slice(0, Math.min(limit > 0 ? limit : 5, 50))
      .map(({ platform, session_id, provider_name, last_success_at, created_at }) =>
        ({ platform, session_id, provider_name, last_success_at, created_at })),
  ),
  'SessionFacade.ListByPlatform': (platform: string) => clone(mockSessions(platform)),
  'SessionFacade.Unbind': (platform: string, sessionID: string) => {
    for (let i = mockRequestLogs.length - 1; i >= 0; i--) {
      if (mockRequestLogs[i].platform === platform && mockRequestLogs[i].session_id === sessionID) {
        mockRequestLogs.splice(i, 1)
      }
    }
    return null
  },

  // platformproxyfacade
  'PlatformProxyFacade.GetStatus': (platform: string) => clone(mockProxyStatus[platform] ?? { enabled: false, base_url: '' }),
  'PlatformProxyFacade.Enable': (platform: string) => {
    if (mockProxyStatus[platform]) mockProxyStatus[platform].enabled = true
    return null
  },
  'PlatformProxyFacade.Disable': (platform: string) => {
    if (mockProxyStatus[platform]) mockProxyStatus[platform].enabled = false
    return null
  },

  // mcpgatewayfacade
  'McpGatewayFacade.RuntimeSnapshot': (knownRevision: number) => {
    const enabled = new Set(mockMcpServers.filter((server) => server.enabled).map((server) => server.id))
    const tools = mockMcpTools.filter((tool) => enabled.has(tool.serverId))
    const fingerprint = JSON.stringify(tools)
    if (fingerprint !== catalogFingerprint) {
      catalogFingerprint = fingerprint
      catalogRevision += 1
    }
    const catalogChanged = knownRevision !== catalogRevision
    return {
      statuses: handlers['McpGatewayFacade.Statuses'](), catalogRevision, catalogChanged,
      ...(catalogChanged ? { tools: clone(tools) } : {}),
    }
  },
  'McpGatewayFacade.List': () => clone(mockMcpServers),
  'McpGatewayFacade.Save': (server: any) => {
    if (server.id) {
      const index = mockMcpServers.findIndex((s) => s.id === server.id)
      if (index >= 0) mockMcpServers[index] = clone(server)
    } else {
      server.id = Math.max(0, ...mockMcpServers.map((s) => s.id)) + 1
      mockMcpServers.push(clone(server))
    }
    return clone(server)
  },
  'McpGatewayFacade.Delete': (id: number) => {
    const index = mockMcpServers.findIndex((s) => s.id === id)
    if (index >= 0) mockMcpServers.splice(index, 1)
    return null
  },
  'McpGatewayFacade.SetEnabled': (id: number, enabled: boolean) => {
    const server = mockMcpServers.find((s) => s.id === id)
    if (server) server.enabled = enabled
    return null
  },
  'McpGatewayFacade.TestConnection': async (server: any) => {
    await delay(400)
    return server?.enabled === false
      ? { success: false, toolCount: 0, error: 'server disabled (demo)', durationMs: 10 }
      : { success: true, toolCount: 5, durationMs: 320 }
  },
  'McpGatewayFacade.Statuses': () =>
    mockMcpServers.map((server) => ({
      serverId: server.id,
      state: server.enabled ? 'connected' : 'disabled',
      toolCount: server.enabled
        ? mockMcpTools.filter((tool) => tool.serverId === server.id).length
        : 0,
    })),
  'McpGatewayFacade.Endpoint': () => 'http://127.0.0.1:18100/mcp',
  'McpGatewayFacade.ListTools': () => clone(mockMcpTools),
  'McpGatewayFacade.GetInjectionStatus': (platform: string) => mockMcpInjection[platform] ?? false,
  'McpGatewayFacade.Inject': (platform: string) => {
    mockMcpInjection[platform] = true
    return null
  },
  'McpGatewayFacade.RemoveInjection': (platform: string) => {
    mockMcpInjection[platform] = false
    return null
  },

  // configsyncfacade
  'ConfigSyncFacade.GetSyncSettings': () => clone(mockSyncSettings),
  'ConfigSyncFacade.SaveSyncSettings': (settings: any) => {
    Object.assign(mockSyncSettings, settings)
    return clone(mockSyncSettings)
  },
  'ConfigSyncFacade.TestConnection': async () => {
    await delay(300)
    return null
  },
  'ConfigSyncFacade.GetRemoteSnapshotMeta': () => clone(mockRemoteSnapshotMeta),
  'ConfigSyncFacade.PullSnapshot': () => ({
    backend: {
      appPreferences: clone(mockPreferences),
      claudeProfile: clone(mockProfiles[0]),
      codexProfile: clone(mockProfiles[1]),
      geminiProfile: clone(mockProfiles[2]),
    },
    ui: { theme: 'systemdefault', locale: 'zh' },
  }),
  'ConfigSyncFacade.PushSnapshot': () => ({
    exists: true,
    updatedAt: new Date().toISOString(),
    deviceName: 'demo-device',
    appVersion: '1.0.0',
    schemaVersion: 1,
  }),
  'ConfigSyncFacade.CreateLocalRollback': () => 'rollback-20261105-demo',
  'ConfigSyncFacade.ApplySnapshot': () => null,

  // applogfacade
  'AppLogFacade.TailAppLog': (offset: number, maxLines: number, level: string, keyword: string) => {
    let entries = [...mockAppLogEntries]
    if (level) entries = entries.filter((e) => e.level === level)
    if (keyword) entries = entries.filter((e) => e.message.includes(keyword))
    entries = entries.slice(-(maxLines || 500))
    const text = mockAppLogEntries.map((e) => `${e.time} ${e.level} ${e.message}`).join('\n').length
    return { entries: clone(entries), next_offset: text, file_size: text, reset: offset === 0, truncated: false }
  },

  // hotkeyfacade（当前前端未消费，占位）
  'HotkeyFacade.Close': () => [],
  'HotkeyFacade.GetHotkeys': () => [],
  'HotkeyFacade.UpHotkey': () => null,
}

export const installMockTransport = () => {
  setTransport({
    call: async (objectID: number, _method: number, _windowName: string, args: any) => {
      await delay(60)
      if (objectID !== objectNames.Call) return null
      const handler = args?.methodName ? handlers[rpcMethodName(args.methodName)] : undefined
      if (!handler) return null
      return handler(...(args?.args ?? []))
    },
  })
}
