// 浏览器直连开发（`pnpm dev`）时 Wails 运行时不可用，
// 这里通过 setTransport 拦截全部 binding 调用并按 methodID 返回演示数据。
// `wails3 task dev` 走真实运行时，不会进入此分支（见 main.ts 的探测）。
import { setTransport } from '@wailsio/runtime'
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

// methodID → handler；未知 ID 返回 null（与真实运行时「方法存在但零值」行为接近）。
const handlers: Record<number, (...args: any[]) => any> = {
  // routingfacade
  3055782468: () => clone(mockPreferences),
  2684023907: (preferences: any) => {
    Object.assign(mockPreferences, preferences)
    return clone(mockPreferences)
  },
  1587903684: (platform: string) => {
    const profile = mockProfiles.find((p) => p.platform === platform)
    return clone(profile ?? { platform, defaultProviderId: null, providers: [] })
  },
  1086826521: (profile: any) => {
    const index = mockProfiles.findIndex((p) => p.platform === profile.platform)
    if (index >= 0) mockProfiles[index] = clone(profile)
    return clone(profile)
  },
  3032235800: async (provider: any) => {
    await delay(350)
    const reachable = !/deepseek/i.test(provider?.name ?? '')
    return reachable
      ? { success: true, httpStatus: 200, durationMs: 180 + Math.floor(Math.random() * 400) }
      : { success: false, httpStatus: 0, durationMs: 2000, error: 'probe timed out (demo)' }
  },

  // observabilityfacade
  1692049370: (platform: string, provider: string, rangeKey: string, costTier: string, httpStatus: string, page: number, pageSize: number) => {
    const filtered = filterLogs(mockRequestLogs, platform, provider, rangeKey || 'last7days', costTier, httpStatus)
    const p = Math.max(1, page || 1)
    const size = pageSize || 200
    return { items: clone(filtered.slice((p - 1) * size, p * size)), total: filtered.length, page: p, page_size: size }
  },
  2012320427: (platform: string, provider: string, rangeKey: string, costTier: string, httpStatus: string) => ({
    ...statsOf(filterLogs(mockRequestLogs, platform, provider, rangeKey || 'last7days', costTier, httpStatus)),
    series: seriesOf(filterLogs(mockRequestLogs, platform, provider, rangeKey || 'last7days', costTier, httpStatus)),
  }),
  2260435430: (platform: string, sessionID: string) =>
    clone(mockRequestLogs.filter((l) => l.platform === platform && l.session_id === sessionID)),
  3709587354: (platform: string) => {
    const names = new Set(mockRequestLogs.filter((l) => !platform || l.platform === platform).map((l) => l.provider))
    return [...names]
  },
  720220581: (platform: string) => {
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
  1068250303: (days: number) => {
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
  2856418829: (platform: string) => clone(mockSessions(platform)),
  2191426179: (platform: string, sessionID: string) => {
    for (let i = mockRequestLogs.length - 1; i >= 0; i--) {
      if (mockRequestLogs[i].platform === platform && mockRequestLogs[i].session_id === sessionID) {
        mockRequestLogs.splice(i, 1)
      }
    }
    return null
  },

  // platformproxyfacade
  3973939786: (platform: string) => clone(mockProxyStatus[platform] ?? { enabled: false, base_url: '' }),
  3899534107: (platform: string) => {
    if (mockProxyStatus[platform]) mockProxyStatus[platform].enabled = true
    return null
  },
  471175048: (platform: string) => {
    if (mockProxyStatus[platform]) mockProxyStatus[platform].enabled = false
    return null
  },

  // mcpgatewayfacade
  787164027: () => clone(mockMcpServers),
  1788497038: (server: any) => {
    if (server.id) {
      const index = mockMcpServers.findIndex((s) => s.id === server.id)
      if (index >= 0) mockMcpServers[index] = clone(server)
    } else {
      server.id = Math.max(0, ...mockMcpServers.map((s) => s.id)) + 1
      mockMcpServers.push(clone(server))
    }
    return clone(server)
  },
  2806829936: (id: number) => {
    const index = mockMcpServers.findIndex((s) => s.id === id)
    if (index >= 0) mockMcpServers.splice(index, 1)
    return null
  },
  4039365050: (id: number, enabled: boolean) => {
    const server = mockMcpServers.find((s) => s.id === id)
    if (server) server.enabled = enabled
    return null
  },
  2395227903: async (server: any) => {
    await delay(400)
    return server?.enabled === false
      ? { success: false, toolCount: 0, error: 'server disabled (demo)', durationMs: 10 }
      : { success: true, toolCount: 5, durationMs: 320 }
  },
  2403049257: () =>
    mockMcpServers.map((server) => ({
      serverId: server.id,
      state: server.enabled ? 'connected' : 'disabled',
      toolCount: server.enabled
        ? mockMcpTools.filter((tool) => tool.serverId === server.id).length
        : 0,
    })),
  3609087734: () => 'http://127.0.0.1:18100/mcp',
  308868850: () => clone(mockMcpTools),
  314695828: (platform: string) => mockMcpInjection[platform] ?? false,
  420971030: (platform: string) => {
    mockMcpInjection[platform] = true
    return null
  },
  2053537476: (platform: string) => {
    mockMcpInjection[platform] = false
    return null
  },

  // configsyncfacade
  3381139004: () => clone(mockSyncSettings),
  1278112865: (settings: any) => {
    Object.assign(mockSyncSettings, settings)
    return clone(mockSyncSettings)
  },
  1558873298: async () => {
    await delay(300)
    return null
  },
  1408576833: () => clone(mockRemoteSnapshotMeta),
  3147351989: () => ({
    backend: {
      appPreferences: clone(mockPreferences),
      claudeProfile: clone(mockProfiles[0]),
      codexProfile: clone(mockProfiles[1]),
      geminiProfile: clone(mockProfiles[2]),
    },
    ui: { theme: 'systemdefault', locale: 'zh' },
  }),
  938748122: () => ({
    exists: true,
    updatedAt: new Date().toISOString(),
    deviceName: 'demo-device',
    appVersion: '1.0.0',
    schemaVersion: 1,
  }),
  1456844065: () => 'rollback-20261105-demo',
  4255137226: () => null,

  // applogfacade
  1766650753: (offset: number, maxLines: number, level: string, keyword: string) => {
    let entries = [...mockAppLogEntries]
    if (level) entries = entries.filter((e) => e.level === level)
    if (keyword) entries = entries.filter((e) => e.message.includes(keyword))
    entries = entries.slice(-(maxLines || 500))
    const text = mockAppLogEntries.map((e) => `${e.time} ${e.level} ${e.message}`).join('\n').length
    return { entries: clone(entries), next_offset: text, file_size: text, reset: offset === 0, truncated: false }
  },

  // hotkeyfacade（当前前端未消费，占位）
  2003331397: () => [],
  4236411700: () => [],
  1765084196: () => null,
}

export const installMockTransport = () => {
  setTransport({
    call: async (objectID: number, _method: number, _windowName: string, args: any) => {
      await delay(60)
      if (objectID === 10) return null // CancelCall
      const methodID = args?.methodID
      const handler = handlers[methodID]
      if (!handler) return null
      return handler(...(args?.args ?? []))
    },
  })
}
