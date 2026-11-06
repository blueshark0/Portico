// 浏览器直连开发（`pnpm dev`，无 Wails 运行时）时的演示数据。
// 通过 mockTransport 按 methodID 分发给前端，保证各页面有真实感的数据可看。
// 数据刻意做成「看起来真实」：同一批请求日志派生统计、会话聚合与热力图，互相一致。

export type MockProvider = {
  id: number
  name: string
  apiUrl: string
  apiKey: string
  officialSite: string
  tint: string
  accent: string
  position: number
  supportedModels?: Record<string, boolean>
  modelMapping?: Record<string, string>
  modelMappingSchemes?: { name: string; mapping: Record<string, string> }[]
  activeModelMappingScheme?: string
}

export type MockProfile = {
  platform: string
  defaultProviderId: number | null
  providers: MockProvider[]
}

export type MockRequestLog = {
  id: number
  platform: string
  model: string
  provider: string
  session_id: string
  http_code: number
  input_tokens: number
  output_tokens: number
  cache_create_tokens: number
  cache_read_tokens: number
  reasoning_tokens: number
  is_stream: boolean
  is_fast: boolean
  duration_sec: number
  ttft_sec: number
  created_at: string
  input_cost: number
  output_cost: number
  cache_create_cost: number
  cache_read_cost: number
  ephemeral_5m_cost: number
  ephemeral_1h_cost: number
  total_cost: number
  has_pricing: boolean
  cost_tier: string
}

export type MockMcpServer = {
  id: number
  name: string
  type: string
  command?: string
  args?: string[]
  env?: Record<string, string>
  url?: string
  headers?: Record<string, string>
  enabled: boolean
  position: number
  timeoutSec: number
  createdAt: string
  updatedAt: string
}

const dayMs = 24 * 60 * 60 * 1000
const now = new Date()

// created_at 形如 2026-11-03T08:12:31.000Z（UTC 字符串）
const utcString = (d: Date) =>
  `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}-${String(d.getUTCDate()).padStart(2, '0')}T${String(d.getUTCHours()).padStart(2, '0')}:${String(d.getUTCMinutes()).padStart(2, '0')}:${String(d.getUTCSeconds()).padStart(2, '0')}.000Z`

export const mockProfiles: MockProfile[] = [
  {
    platform: 'claude',
    defaultProviderId: 1,
    providers: [
      {
        id: 1,
        name: 'Kimi Coding',
        apiUrl: 'https://api.moonshot.cn/anthropic',
        apiKey: 'sk-mockKimiCodingKey9f2c71d8',
        officialSite: 'https://platform.moonshot.cn',
        tint: '#356DF3',
        accent: '#356DF3',
        position: 0,
        supportedModels: {
          'claude-sonnet-4-5': true,
          'claude-opus-4-1': true,
          'kimi-k2-thinking': false,
        },
        modelMapping: { 'claude-sonnet-4-5': 'kimi-k2-0905-preview' },
        modelMappingSchemes: [
          {
            name: '日常编码',
            mapping: { 'claude-sonnet-4-5': 'kimi-k2-0905-preview' },
          },
          {
            name: '深度推理',
            mapping: {
              'claude-sonnet-4-5': 'kimi-k2-thinking',
              'claude-opus-4-1': 'kimi-k2-thinking',
            },
          },
        ],
        activeModelMappingScheme: '日常编码',
      },
      {
        id: 2,
        name: 'DeepSeek',
        apiUrl: 'https://api.deepseek.com/anthropic',
        apiKey: 'sk-mockDeepSeekKey41aa07be',
        officialSite: 'https://platform.deepseek.com',
        tint: '#4D6BFE',
        accent: '#4D6BFE',
        position: 1,
        supportedModels: {
          'claude-sonnet-4-5': true,
          'claude-opus-4-1': true,
        },
      },
      {
        id: 3,
        name: 'AnyRouter',
        apiUrl: 'https://api.anyrouter.top/v1',
        apiKey: 'sk-mockAnyRouterKey88d3e0c1',
        officialSite: 'https://anyrouter.top',
        tint: '#7C5CFF',
        accent: '#7C5CFF',
        position: 2,
        supportedModels: {
          'claude-sonnet-4-5': true,
          'claude-opus-4-1': true,
          'claude-haiku-4-5': true,
        },
      },
    ],
  },
  {
    platform: 'codex',
    defaultProviderId: 4,
    providers: [
      {
        id: 4,
        name: 'OpenAI Official',
        apiUrl: 'https://api.openai.com/v1',
        apiKey: 'sk-mockOpenAIKeyc207f51a',
        officialSite: 'https://platform.openai.com',
        tint: '#10A37F',
        accent: '#10A37F',
        position: 0,
        supportedModels: {
          'gpt-5-codex': true,
          'gpt-5': true,
          'o4-mini': true,
        },
      },
      {
        id: 5,
        name: 'AnyRouter',
        apiUrl: 'https://api.anyrouter.top/v1',
        apiKey: 'sk-mockAnyRouterKey88d3e0c1',
        officialSite: 'https://anyrouter.top',
        tint: '#7C5CFF',
        accent: '#7C5CFF',
        position: 1,
        supportedModels: {
          'gpt-5-codex': true,
          'gpt-5': true,
        },
      },
    ],
  },
  {
    platform: 'gemini',
    defaultProviderId: 6,
    providers: [
      {
        id: 6,
        name: 'Google AI Studio',
        apiUrl: 'https://generativelanguage.googleapis.com',
        apiKey: 'AIzaSyMockGoogleKey7d41b209',
        officialSite: 'https://aistudio.google.com',
        tint: '#1B72E8',
        accent: '#1B72E8',
        position: 0,
        supportedModels: {
          'gemini-2.5-pro': true,
          'gemini-2.5-flash': true,
          'gemini-3-pro-preview': true,
        },
      },
      {
        id: 7,
        name: '302.AI',
        apiUrl: 'https://api.302.ai/gemini',
        apiKey: 'sk-mock302Key5e96c2d4',
        officialSite: 'https://302.ai',
        tint: '#FF6B3D',
        accent: '#FF6B3D',
        position: 1,
        supportedModels: {
          'gemini-2.5-pro': true,
          'gemini-2.5-flash': true,
        },
      },
    ],
  },
]

export const mockPreferences = {
  show_heatmap: true,
  show_home_title: true,
  proxy_enabled: false,
  proxy_url: '',
}

export const mockProxyStatus: Record<string, { enabled: boolean; base_url: string }> = {
  claude: { enabled: true, base_url: 'http://127.0.0.1:18100' },
  codex: { enabled: true, base_url: 'http://127.0.0.1:18100' },
  gemini: { enabled: false, base_url: 'http://127.0.0.1:18100' },
}

// ---- 请求日志：确定性伪随机，7 天共 ~120 条 ----

type Rng = () => number
const mulberry32 = (seed: number): Rng => {
  let a = seed >>> 0
  return () => {
    a |= 0
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const pick = <T>(rng: Rng, items: readonly T[]): T => items[Math.floor(rng() * items.length)]

const buildLogs = (): MockRequestLog[] => {
  const rng = mulberry32(20261105)
  const sessions: { platform: string; provider: string; model: string; id: string }[] = [
    { platform: 'claude', provider: 'Kimi Coding', model: 'claude-sonnet-4-5', id: 'a1f42c9e-session-01' },
    { platform: 'claude', provider: 'DeepSeek', model: 'claude-opus-4-1', id: 'b8d31107-session-02' },
    { platform: 'claude', provider: 'Kimi Coding', model: 'claude-sonnet-4-5', id: 'c22019fa-session-03' },
    { platform: 'codex', provider: 'OpenAI Official', model: 'gpt-5-codex', id: 'd14e88b0-session-04' },
    { platform: 'codex', provider: 'AnyRouter', model: 'gpt-5', id: 'e90a63d2-session-05' },
    { platform: 'gemini', provider: 'Google AI Studio', model: 'gemini-2.5-pro', id: 'f3110c44-session-06' },
    { platform: 'gemini', provider: '302.AI', model: 'gemini-2.5-flash', id: '04bb77e9-session-07' },
    { platform: 'gemini', provider: 'Google AI Studio', model: 'gemini-3-pro-preview', id: '12c8aa5f-session-08' },
  ]

  const logs: MockRequestLog[] = []
  let id = 1
  const weights = [3, 4, 6, 7, 9, 11, 14] // 越近越多
  weights.forEach((count, dayOffset) => {
    const day = new Date(now.getTime() - dayOffset * dayMs)
    for (let i = 0; i < count; i++) {
      const s = pick(rng, sessions)
      const at = new Date(day.getTime() - Math.floor(rng() * 9 * 3600 * 1000))
      const input = 800 + Math.floor(rng() * 42000)
      const output = 200 + Math.floor(rng() * 6800)
      const reasoning = s.platform === 'codex' ? Math.floor(rng() * 5200) : 0
      const cacheRead = rng() < 0.45 ? Math.floor(input * (0.3 + rng() * 0.6)) : 0
      const failed = rng() < 0.06
      const inputCost = (input / 1_000_000) * 3
      const outputCost = (output / 1_000_000) * 15
      const cacheCost = (cacheRead / 1_000_000) * 0.3
      const total = inputCost + outputCost + cacheCost + (reasoning / 1_000_000) * 15
      const duration = 2 + rng() * 46
      logs.push({
        id: id++,
        platform: s.platform,
        model: s.model,
        provider: s.provider,
        session_id: s.id,
        http_code: failed ? pick(rng, [401, 429, 500] as const) : 200,
        input_tokens: input,
        output_tokens: output,
        cache_create_tokens: 0,
        cache_read_tokens: cacheRead,
        reasoning_tokens: reasoning,
        is_stream: rng() < 0.85,
        is_fast: rng() < 0.12,
        duration_sec: Number(duration.toFixed(1)),
        ttft_sec: failed ? 0 : Number((0.3 + rng() * 2.4).toFixed(2)),
        created_at: utcString(at),
        input_cost: Number(inputCost.toFixed(6)),
        output_cost: Number(outputCost.toFixed(6)),
        cache_create_cost: 0,
        cache_read_cost: Number(cacheCost.toFixed(6)),
        ephemeral_5m_cost: 0,
        ephemeral_1h_cost: 0,
        total_cost: Number(total.toFixed(6)),
        has_pricing: true,
        cost_tier: total < 0.05 ? 'low' : total < 0.4 ? 'medium' : 'high',
      })
    }
  })
  return logs.sort((a, b) => (a.created_at < b.created_at ? 1 : -1))
}

export const mockRequestLogs: MockRequestLog[] = buildLogs()

const dayKey = (createdAt: string) => createdAt.slice(0, 10)

export const mockSessions = (platform: string) => {
  const groups = new Map<string, MockRequestLog[]>()
  mockRequestLogs
    .filter((log) => log.platform === platform && log.http_code === 200)
    .forEach((log) => {
      const list = groups.get(log.session_id) ?? []
      list.push(log)
      groups.set(log.session_id, list)
    })
  return [...groups.entries()]
    .map(([sessionId, logs]) => ({
      platform,
      session_id: sessionId,
      provider_name: logs[0].provider,
      last_success_at: logs.map((l) => l.created_at).sort().at(-1) ?? logs[0].created_at,
      created_at: logs.map((l) => l.created_at).sort()[0],
      requests: logs.length,
      input_tokens: logs.reduce((s, l) => s + l.input_tokens, 0),
      output_tokens: logs.reduce((s, l) => s + l.output_tokens, 0),
      total_cost: Number(logs.reduce((s, l) => s + l.total_cost, 0).toFixed(6)),
      models: [...new Set(logs.map((l) => l.model))],
    }))
    .sort((a, b) => (a.last_success_at < b.last_success_at ? 1 : -1))
}

export const mockMcpServers: MockMcpServer[] = [
  {
    id: 1,
    name: 'filesystem',
    type: 'stdio',
    command: 'npx',
    args: ['-y', '@modelcontextprotocol/server-filesystem', '/home/mike/projects'],
    env: {},
    enabled: true,
    position: 0,
    timeoutSec: 30,
    createdAt: utcString(new Date(now.getTime() - 30 * dayMs)),
    updatedAt: utcString(new Date(now.getTime() - 2 * dayMs)),
  },
  {
    id: 2,
    name: 'playwright',
    type: 'stdio',
    command: 'npx',
    args: ['-y', '@playwright/mcp@latest', '--headless'],
    env: {},
    enabled: true,
    position: 1,
    timeoutSec: 60,
    createdAt: utcString(new Date(now.getTime() - 28 * dayMs)),
    updatedAt: utcString(new Date(now.getTime() - 5 * dayMs)),
  },
  {
    id: 3,
    name: 'context7',
    type: 'http',
    url: 'https://mcp.context7.com/mcp',
    headers: {},
    enabled: true,
    position: 2,
    timeoutSec: 30,
    createdAt: utcString(new Date(now.getTime() - 20 * dayMs)),
    updatedAt: utcString(new Date(now.getTime() - 20 * dayMs)),
  },
  {
    id: 4,
    name: 'deepwiki',
    type: 'http',
    url: 'https://mcp.deepwiki.com/mcp',
    headers: { Authorization: 'Bearer sk-mockDeepwikiKey' },
    enabled: false,
    position: 3,
    timeoutSec: 30,
    createdAt: utcString(new Date(now.getTime() - 12 * dayMs)),
    updatedAt: utcString(new Date(now.getTime() - 12 * dayMs)),
  },
]

export const mockMcpTools = [
  { serverId: 1, serverName: 'filesystem', publicName: 'filesystem__read_file', upstreamName: 'read_file', description: 'Read a file from the allowed directories', inputSchema: '{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}', readOnly: true, destructive: false },
  { serverId: 1, serverName: 'filesystem', publicName: 'filesystem__write_file', upstreamName: 'write_file', description: 'Write content to a file', inputSchema: '{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}', readOnly: false, destructive: true },
  { serverId: 1, serverName: 'filesystem', publicName: 'filesystem__list_directory', upstreamName: 'list_directory', description: 'List files and directories', inputSchema: '{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}', readOnly: true, destructive: false },
  { serverId: 1, serverName: 'filesystem', publicName: 'filesystem__search_files', upstreamName: 'search_files', description: 'Recursively search for files by pattern', inputSchema: '{"type":"object","properties":{"path":{"type":"string"},"pattern":{"type":"string"}},"required":["path","pattern"]}', readOnly: true, destructive: false },
  { serverId: 2, serverName: 'playwright', publicName: 'playwright__browser_navigate', upstreamName: 'browser_navigate', description: 'Navigate the browser to a URL', inputSchema: '{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}', readOnly: false, destructive: false },
  { serverId: 2, serverName: 'playwright', publicName: 'playwright__browser_snapshot', upstreamName: 'browser_snapshot', description: 'Capture an accessibility snapshot of the page', inputSchema: '{"type":"object","properties":{}}', readOnly: true, destructive: false },
  { serverId: 2, serverName: 'playwright', publicName: 'playwright__browser_click', upstreamName: 'browser_click', description: 'Click an element on the page', inputSchema: '{"type":"object","properties":{"ref":{"type":"string"}},"required":["ref"]}', readOnly: false, destructive: false },
  { serverId: 3, serverName: 'context7', publicName: 'context7__resolve-library-id', upstreamName: 'resolve-library-id', description: 'Resolve a library name to a Context7-compatible library ID', inputSchema: '{"type":"object","properties":{"libraryName":{"type":"string"},"query":{"type":"string"}},"required":["libraryName"]}', readOnly: true, destructive: false },
  { serverId: 3, serverName: 'context7', publicName: 'context7__query-docs', upstreamName: 'query-docs', description: 'Fetch up-to-date documentation for a library', inputSchema: '{"type":"object","properties":{"context7CompatibleLibraryID":{"type":"string"},"topic":{"type":"string"}},"required":["context7CompatibleLibraryID"]}', readOnly: true, destructive: false },
]

export const mockMcpInjection: Record<string, boolean> = {
  claude: true,
  codex: true,
  gemini: false,
}

export const mockSyncSettings = {
  endpoint: '',
  username: '',
  appPassword: '',
  remotePath: '/portico',
  syncPassphrase: '',
}

export const mockRemoteSnapshotMeta = {
  exists: false,
  updatedAt: '0001-01-01T00:00:00.000Z',
  deviceName: '',
  appVersion: '',
  schemaVersion: 0,
}

export const mockAppLogEntries = [
  { time: '2026/11/05 09:31:12', source: 'relay.go:214', level: 'INFO', message: 'POST /v1/messages → Kimi Coding (claude-sonnet-4-5) 200 in 8.2s' },
  { time: '2026/11/05 09:30:44', source: 'relay.go:214', level: 'INFO', message: 'POST /responses → OpenAI Official (gpt-5-codex) 200 in 21.7s' },
  { time: '2026/11/05 09:27:03', source: 'auth.go:88', level: 'WARN', message: 'rejected request with invalid placeholder token (codex)' },
  { time: '2026/11/05 09:12:56', source: 'mcpgateway.go:310', level: 'INFO', message: 'upstream context7 connected, 2 tools registered' },
  { time: '2026/11/05 08:58:19', source: 'sessions.go:141', level: 'INFO', message: 'session b8d31107 re-bound to default provider DeepSeek' },
  { time: '2026/11/05 08:41:02', source: 'platformproxy.go:203', level: 'INFO', message: 'claude proxy enabled → http://127.0.0.1:18100' },
  { time: '2026/11/05 08:40:57', source: 'bootstrap.go:76', level: 'INFO', message: 'relay listening on 127.0.0.1:18100' },
]

export { dayKey, dayMs, utcString }
