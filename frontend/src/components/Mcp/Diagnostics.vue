<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ArrowRight, Search, Server } from 'lucide-vue-next'
import {
  ServerType,
  fetchInjectionStatus,
  fetchMcpEndpoint,
  fetchMcpServers,
  fetchMcpStatuses,
  fetchMcpTools,
  testMcpConnection,
  type McpServerRecord,
  type ToolInfo,
  type UpstreamStatus,
} from '../../services/mcp'
import { getErrorMessage } from '../../utils/errors'
import { showToast } from '../../utils/toast'
import { runWithToast } from '../../utils/runWithToast'

type View = 'gateway' | 'upstream' | 'tools'
type Perspective = 'upstream' | 'downstream'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const servers = ref<McpServerRecord[]>([])
const statuses = ref<UpstreamStatus[]>([])
const tools = ref<ToolInfo[]>([])
const endpoint = ref('')
const injections = ref<Record<string, boolean>>({ claude: false, codex: false })
// 单上游重新诊断结果（会话内保留）：serverId → 探测结果
const probes = ref<Record<number, { latencyMs: number; toolCount: number; at: number; ok: boolean }>>({})
const probingId = ref(0)
const rediagnosing = ref(false)

const view = computed<View>(() => {
  const v = route.query.view
  return v === 'upstream' || v === 'tools' ? v : 'gateway'
})

const perspective = computed<Perspective>(() =>
  route.query.perspective === 'upstream' ? 'upstream' : 'downstream',
)

const selectedServerId = computed(() => Number(route.query.server) || 0)

const selectedServer = computed(
  () => servers.value.find((s) => s.id === selectedServerId.value) ?? servers.value[0] ?? null,
)

const statusMap = computed(() => {
  const map: Record<number, UpstreamStatus> = {}
  for (const status of statuses.value) map[status.serverId] = status
  return map
})

const statusOf = (server: McpServerRecord) => {
  if (!server.enabled) return { tone: 'warning', label: t('mcpDiag.status.disabled') }
  const state = statusMap.value[server.id]?.state
  if (state === 'connected') return { tone: 'success', label: t('mcpDiag.status.connected') }
  if (state === 'failed') return { tone: 'danger', label: t('mcpDiag.status.failed') }
  return { tone: 'muted', label: t('mcpDiag.status.connecting') }
}

const toolsOf = (serverId: number) => tools.value.filter((tool) => tool.serverId === serverId)

const totalTools = computed(() => tools.value.length)
const injectedCount = computed(() => Object.values(injections.value).filter(Boolean).length)
const connectedCount = computed(
  () => statuses.value.filter((s) => s.state === 'connected').length,
)

const loadAll = async () => {
  const [serverList, statusList, toolList, ep, claude, codex] = await Promise.all([
    fetchMcpServers(),
    fetchMcpStatuses(),
    runWithToast(() => fetchMcpTools(), { fallback: t('mcpDiag.messages.loadFailed'), silent: true }),
    fetchMcpEndpoint(),
    fetchInjectionStatus('claude'),
    fetchInjectionStatus('codex'),
  ])
  servers.value = serverList ?? []
  statuses.value = statusList ?? []
  tools.value = toolList ?? []
  endpoint.value = ep
  injections.value = { claude, codex }
}

onMounted(() => {
  void loadAll()
})

const go = (query: Record<string, string>) => {
  void router.push({ path: '/mcp/diagnostics', query })
}

const copyEndpoint = async () => {
  try {
    await navigator.clipboard.writeText(endpoint.value)
    showToast(t('mcpDiag.messages.copied'))
  } catch (error) {
    showToast(getErrorMessage(error, t('mcpDiag.messages.copyFailed')), 'error')
  }
}

// 网关级重新诊断：刷新状态与工具目录。
const rediagnoseGateway = async () => {
  if (rediagnosing.value) return
  rediagnosing.value = true
  try {
    const [statusList, toolList] = await Promise.all([fetchMcpStatuses(), fetchMcpTools()])
    statuses.value = statusList ?? []
    tools.value = toolList ?? []
    showToast(t('mcpDiag.messages.rediagnosed'))
  } catch (error) {
    showToast(getErrorMessage(error, t('mcpDiag.messages.loadFailed')), 'error')
  } finally {
    rediagnosing.value = false
  }
}

// 上游级重新诊断：对该上游做一次真实连接探测（延迟 + 工具数）。
const rediagnoseUpstream = async (server: McpServerRecord) => {
  if (probingId.value) return
  probingId.value = server.id
  try {
    const result = await testMcpConnection(server)
    probes.value = {
      ...probes.value,
      [server.id]: {
        latencyMs: result.durationMs,
        toolCount: result.toolCount,
        at: Date.now(),
        ok: result.success,
      },
    }
    if (!result.success) {
      showToast(t('mcpDiag.messages.testErr', { error: result.error ?? '' }), 'error')
    }
  } catch (error) {
    showToast(getErrorMessage(error, t('mcpDiag.messages.testFailed')), 'error')
  } finally {
    probingId.value = 0
  }
}

// ---- 工具视图 ----
const toolKeyword = ref('')
const toolSource = ref('')
const selectedTool = ref<ToolInfo | null>(null)

const toolSources = computed(() => [...new Set(tools.value.map((tool) => tool.serverName))])

const filteredTools = computed(() => {
  const query = toolKeyword.value.trim().toLowerCase()
  return tools.value.filter((tool) => {
    if (toolSource.value && tool.serverName !== toolSource.value) return false
    if (!query) return true
    return `${tool.publicName} ${tool.upstreamName} ${tool.description}`.toLowerCase().includes(query)
  })
})

const visibleTool = computed(
  () => selectedTool.value ?? filteredTools.value[0] ?? null,
)

const formatSchema = (raw: string) => {
  if (!raw) return ''
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

const formatTime = (at: number) => {
  const d = new Date(at)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

const serverCommand = (server: McpServerRecord) => {
  if (server.type === ServerType.ServerTypeStdio) {
    return [server.command, ...(server.args ?? [])].filter(Boolean).join(' ')
  }
  return server.url ?? ''
}
</script>

<template>
  <div class="cs-page">
    <!-- ================= 网关视图 ================= -->
    <template v-if="view === 'gateway'">
      <div class="cs-page-head">
        <div>
          <h1 class="cs-page-title">{{ t('mcpDiag.gateway.title') }}</h1>
          <p class="cs-page-sub">{{ t('mcpDiag.gateway.subtitle') }}</p>
        </div>
        <div class="cs-page-actions">
          <button type="button" class="cs-btn" @click="go({ view: 'tools' })">
            {{ t('mcpDiag.gateway.toTools') }}
          </button>
        </div>
      </div>

      <div class="cs-card cs-card-pad diag-gw-head">
        <div>
          <h2 class="cs-card-title">{{ t('mcpDiag.gateway.cardTitle') }}</h2>
          <div class="diag-gw-line">
            <span class="cs-pill" :class="connectedCount > 0 ? 'cs-pill--success' : 'cs-pill--muted'">
              {{ connectedCount > 0 ? t('mcpDiag.gateway.running') : t('mcpDiag.gateway.idle') }}
            </span>
            <span class="cs-mono diag-gw-addr">Streamable HTTP · {{ endpoint.replace(/^https?:\/\//, '').replace(/\/mcp$/, '') }}</span>
            <span class="cs-mono diag-gw-path">/mcp</span>
          </div>
        </div>
        <div class="diag-gw-actions">
          <button type="button" class="cs-btn" @click="copyEndpoint">{{ t('mcpDiag.gateway.copyEndpoint') }}</button>
          <button type="button" class="cs-btn cs-btn--primary" :disabled="rediagnosing" @click="rediagnoseGateway">
            {{ rediagnosing ? t('mcpDiag.gateway.rediagnosing') : t('mcpDiag.gateway.rediagnose') }}
          </button>
        </div>
      </div>

      <div class="diag-metrics">
        <div class="cs-card diag-metric">
          <p class="diag-metric-label">{{ t('mcpDiag.gateway.metrics.clients') }}</p>
          <p class="diag-metric-value">{{ t('mcpDiag.gateway.metrics.clientsValue', { count: injectedCount }) }}</p>
          <p class="diag-metric-sub">{{ injections.claude ? 'Claude Code' : '' }}{{ injections.claude && injections.codex ? ' · ' : '' }}{{ injections.codex ? 'Codex' : '' }}</p>
        </div>
        <div class="cs-card diag-metric">
          <p class="diag-metric-label">{{ t('mcpDiag.gateway.metrics.tools') }}</p>
          <p class="diag-metric-value cs-mono">{{ totalTools }}</p>
          <p class="diag-metric-sub">{{ t('mcpDiag.gateway.metrics.toolsSub', { count: connectedCount }) }}</p>
        </div>
        <div class="cs-card diag-metric">
          <p class="diag-metric-label">{{ t('mcpDiag.gateway.metrics.conflicts') }}</p>
          <p class="diag-metric-value diag-metric-value--ok cs-mono">0</p>
          <p class="diag-metric-sub">{{ t('mcpDiag.gateway.metrics.conflictsSub') }}</p>
        </div>
        <div class="cs-card diag-metric">
          <p class="diag-metric-label">{{ t('mcpDiag.gateway.metrics.upstreams') }}</p>
          <p class="diag-metric-value cs-mono">{{ servers.length }}</p>
          <p class="diag-metric-sub">{{ t('mcpDiag.gateway.metrics.upstreamsSub', { count: servers.filter(s => s.enabled).length }) }}</p>
        </div>
      </div>

      <div class="diag-gw-grid">
        <div class="cs-card cs-card-pad">
          <h2 class="cs-card-title">{{ t('mcpDiag.gateway.detailTitle') }}</h2>
          <dl class="diag-kv">
            <div class="diag-kv-row"><dt>{{ t('mcpDiag.gateway.kv.listen') }}</dt><dd class="cs-mono">{{ endpoint.replace(/^https?:\/\//, '').replace(/\/mcp$/, '') }}</dd></div>
            <div class="diag-kv-row"><dt>{{ t('mcpDiag.gateway.kv.endpoint') }}</dt><dd class="cs-mono">{{ endpoint }}</dd></div>
            <div class="diag-kv-row"><dt>{{ t('mcpDiag.gateway.kv.transport') }}</dt><dd>Streamable HTTP</dd></div>
            <div class="diag-kv-row"><dt>{{ t('mcpDiag.gateway.kv.protocol') }}</dt><dd>2025-11-25</dd></div>
            <div class="diag-kv-row"><dt>{{ t('mcpDiag.gateway.kv.upstreams') }}</dt><dd>{{ servers.map(s => s.name).join(' · ') || '—' }}</dd></div>
            <div class="diag-kv-row"><dt>{{ t('mcpDiag.gateway.kv.naming') }}</dt><dd class="cs-mono">{server}__{tool}</dd></div>
            <div class="diag-kv-row"><dt>{{ t('mcpDiag.gateway.kv.conflict') }}</dt><dd>{{ t('mcpDiag.gateway.kv.conflictValue') }}</dd></div>
            <div class="diag-kv-row diag-kv-row--clients">
              <dt>{{ t('mcpDiag.gateway.kv.clients') }}</dt>
              <dd>
                <span
                  v-for="platform in (['claude', 'codex'] as const)"
                  :key="platform"
                  class="diag-client"
                >
                  <span>{{ platform === 'claude' ? 'Claude Code' : 'Codex' }}</span>
                  <span class="cs-pill" :class="injections[platform] ? 'cs-pill--success' : 'cs-pill--muted'">
                    {{ injections[platform] ? t('mcpDiag.gateway.injected') : t('mcpDiag.gateway.notInjected') }}
                  </span>
                </span>
              </dd>
            </div>
          </dl>
        </div>

        <div class="cs-card cs-card-pad">
          <h2 class="cs-card-title">{{ t('mcpDiag.gateway.toolsTitle') }}</h2>
          <p class="cs-section-sub">{{ t('mcpDiag.gateway.toolsSub') }}</p>
          <div class="diag-tools-head">
            <span>{{ t('mcpDiag.gateway.colName') }}</span>
            <span>{{ t('mcpDiag.gateway.colSource') }}</span>
            <span>{{ t('mcpDiag.gateway.colSchema') }}</span>
          </div>
          <div v-if="tools.length === 0" class="cs-empty">{{ t('mcpDiag.gateway.toolsEmpty') }}</div>
          <div v-for="tool in tools.slice(0, 12)" :key="tool.publicName" class="diag-tools-row">
            <span class="cs-mono diag-tools-name">{{ tool.publicName }}</span>
            <span class="cs-mono diag-tools-source">{{ tool.serverName }} / {{ tool.upstreamName }}</span>
            <span class="cs-pill cs-pill--success">{{ t('mcpDiag.gateway.published') }}</span>
          </div>
          <button v-if="tools.length > 0" type="button" class="diag-tools-more" @click="go({ view: 'tools' })">
            {{ t('mcpDiag.gateway.allTools', { count: totalTools }) }}
            <ArrowRight :size="12" :stroke-width="2" />
          </button>
        </div>
      </div>
    </template>

    <!-- ================= 上游服务视图 ================= -->
    <template v-else-if="view === 'upstream'">
      <div class="cs-page-head">
        <div>
          <h1 class="cs-page-title">{{ t('mcpDiag.upstream.title') }}</h1>
          <p class="cs-page-sub">{{ t('mcpDiag.upstream.subtitle') }}</p>
        </div>
        <div class="cs-page-actions">
          <select
            class="cs-select diag-upstream-picker"
            :value="selectedServer?.id ?? ''"
            @change="go({ view: 'upstream', server: ($event.target as HTMLSelectElement).value })"
          >
            <option v-for="server in servers" :key="server.id" :value="String(server.id)">{{ server.name }}</option>
          </select>
          <button type="button" class="cs-btn" @click="go({})">{{ t('mcpDiag.upstream.backToGateway') }}</button>
        </div>
      </div>

      <template v-if="selectedServer">
        <div class="cs-card cs-card-pad diag-up-head">
          <span class="diag-up-icon"><Server :size="22" :stroke-width="2" /></span>
          <div class="diag-up-title">
            <h2 class="diag-up-name cs-mono">{{ selectedServer.name }}</h2>
            <p class="diag-up-cmd cs-mono">{{ serverCommand(selectedServer) }}</p>
          </div>
          <div class="diag-up-side">
            <span class="cs-pill" :class="`cs-pill--${statusOf(selectedServer).tone}`">{{ statusOf(selectedServer).label }}</span>
            <span v-if="probes[selectedServer.id]" class="diag-up-probe">
              {{ t('mcpDiag.upstream.lastProbe', { time: formatTime(probes[selectedServer.id].at) }) }}
            </span>
            <button
              type="button"
              class="cs-btn"
              :disabled="probingId === selectedServer.id"
              @click="rediagnoseUpstream(selectedServer)"
            >
              {{ probingId === selectedServer.id ? t('mcpDiag.upstream.probing') : t('mcpDiag.upstream.rediagnose') }}
            </button>
            <button type="button" class="cs-btn cs-btn--primary" @click="go({ view: 'tools', perspective: 'upstream' })">
              {{ t('mcpDiag.upstream.toolList') }}
            </button>
          </div>
        </div>

        <div class="diag-metrics">
          <div class="cs-card diag-metric">
            <p class="diag-metric-label">{{ t('mcpDiag.upstream.metrics.connection') }}</p>
            <p class="diag-metric-value" :class="`diag-metric-value--${statusOf(selectedServer).tone}`">
              {{ statusOf(selectedServer).label }}
            </p>
            <p class="diag-metric-sub">{{ statusMap[selectedServer.id]?.error || t('mcpDiag.upstream.metrics.connectionOk') }}</p>
          </div>
          <div class="cs-card diag-metric">
            <p class="diag-metric-label">{{ t('mcpDiag.upstream.metrics.latency') }}</p>
            <p class="diag-metric-value cs-mono">
              {{ probes[selectedServer.id] ? `${probes[selectedServer.id].latencyMs} ms` : '—' }}
            </p>
            <p class="diag-metric-sub">{{ t('mcpDiag.upstream.metrics.latencySub') }}</p>
          </div>
          <div class="cs-card diag-metric">
            <p class="diag-metric-label">{{ t('mcpDiag.upstream.metrics.tools') }}</p>
            <p class="diag-metric-value cs-mono">{{ statusMap[selectedServer.id]?.toolCount ?? 0 }}</p>
            <p class="diag-metric-sub">{{ t('mcpDiag.upstream.metrics.toolsSub', { count: toolsOf(selectedServer.id).length }) }}</p>
          </div>
          <div class="cs-card diag-metric">
            <p class="diag-metric-label">{{ t('mcpDiag.upstream.metrics.transport') }}</p>
            <p class="diag-metric-value diag-metric-value--sm">{{ selectedServer.type === ServerType.ServerTypeStdio ? 'stdio' : 'Streamable HTTP' }}</p>
            <p class="diag-metric-sub">{{ selectedServer.type === ServerType.ServerTypeStdio ? t('mcpDiag.upstream.metrics.local') : t('mcpDiag.upstream.metrics.remote') }}</p>
          </div>
        </div>

        <div class="diag-gw-grid">
          <div class="cs-card cs-card-pad">
            <h2 class="cs-card-title">{{ t('mcpDiag.upstream.detailTitle') }}</h2>
            <dl class="diag-kv">
              <div class="diag-kv-row"><dt>{{ t('mcpDiag.upstream.kv.transport') }}</dt><dd>{{ selectedServer.type === ServerType.ServerTypeStdio ? 'stdio' : 'Streamable HTTP' }}</dd></div>
              <div class="diag-kv-row"><dt>{{ selectedServer.type === ServerType.ServerTypeStdio ? t('mcpDiag.upstream.kv.command') : t('mcpDiag.upstream.kv.url') }}</dt><dd class="cs-mono">{{ serverCommand(selectedServer) }}</dd></div>
              <div class="diag-kv-row"><dt>{{ t('mcpDiag.upstream.kv.timeout') }}</dt><dd>{{ selectedServer.timeoutSec || 30 }} s</dd></div>
              <div class="diag-kv-row"><dt>{{ t('mcpDiag.upstream.kv.enabled') }}</dt><dd>{{ selectedServer.enabled ? t('mcpDiag.upstream.kv.enabledYes') : t('mcpDiag.upstream.kv.enabledNo') }}</dd></div>
              <div class="diag-kv-row"><dt>{{ t('mcpDiag.upstream.kv.lastError') }}</dt><dd class="diag-kv-error">{{ statusMap[selectedServer.id]?.error || t('mcpDiag.upstream.kv.noError') }}</dd></div>
            </dl>
          </div>

          <div class="cs-card cs-card-pad">
            <div class="diag-cap-head">
              <h2 class="cs-card-title">{{ t('mcpDiag.upstream.capTitle') }}</h2>
              <span class="cs-pill" :class="statusOf(selectedServer).tone === 'danger' ? 'cs-pill--danger' : 'cs-pill--success'">
                {{ statusOf(selectedServer).tone === 'danger' ? t('mcpDiag.upstream.capBlocked') : t('mcpDiag.upstream.capOk') }}
              </span>
            </div>
            <dl class="diag-kv">
              <div class="diag-kv-row"><dt class="cs-mono">Tools</dt><dd>{{ t('mcpDiag.upstream.capTools', { count: toolsOf(selectedServer.id).length }) }}</dd></div>
              <div class="diag-kv-row"><dt class="cs-mono">Resources</dt><dd>{{ t('mcpDiag.upstream.capNotCollected') }}</dd></div>
              <div class="diag-kv-row"><dt class="cs-mono">Prompts</dt><dd>{{ t('mcpDiag.upstream.capNotCollected') }}</dd></div>
              <div class="diag-kv-row"><dt class="cs-mono">Logging</dt><dd>{{ t('mcpDiag.upstream.capNotCollected') }}</dd></div>
              <div class="diag-kv-row"><dt class="cs-mono">Roots</dt><dd>{{ t('mcpDiag.upstream.capNotCollected') }}</dd></div>
            </dl>
            <h3 class="diag-events-title">{{ t('mcpDiag.upstream.eventsTitle') }}</h3>
            <span v-if="probes[selectedServer.id]?.ok" class="cs-pill cs-pill--success">
              {{ t('mcpDiag.upstream.eventProbeOk', { time: formatTime(probes[selectedServer.id].at) }) }}
            </span>
            <span v-else-if="statusOf(selectedServer).tone === 'success'" class="cs-pill cs-pill--success">
              {{ t('mcpDiag.upstream.eventConnected') }}
            </span>
            <span v-else class="cs-pill cs-pill--muted">{{ t('mcpDiag.upstream.eventNone') }}</span>
          </div>
        </div>
      </template>
      <div v-else class="cs-card cs-card-pad">
        <div class="cs-empty">{{ t('mcpDiag.upstream.empty') }}</div>
      </div>
    </template>

    <!-- ================= 工具视图 ================= -->
    <template v-else>
      <div class="cs-page-head">
        <div>
          <h1 class="cs-page-title">{{ t('mcpDiag.tools.title') }}</h1>
          <p class="cs-page-sub">{{ t('mcpDiag.tools.subtitle') }}</p>
        </div>
        <div class="cs-page-actions">
          <div class="cs-seg">
            <button type="button" :class="{ active: perspective === 'upstream' }" @click="go({ view: 'tools', perspective: 'upstream' })">
              {{ t('mcpDiag.tools.upstream') }}
            </button>
            <button type="button" :class="{ active: perspective === 'downstream' }" @click="go({ view: 'tools', perspective: 'downstream' })">
              {{ t('mcpDiag.tools.downstream') }}
            </button>
          </div>
          <button type="button" class="cs-btn" @click="go({})">{{ t('mcpDiag.tools.backToGateway') }}</button>
        </div>
      </div>

      <div class="cs-filterbar">
        <span class="diag-tool-search-wrap">
          <Search :size="13" :stroke-width="2" class="diag-tool-search-icon" />
          <input v-model="toolKeyword" class="diag-tool-search" :placeholder="t('mcpDiag.tools.searchPlaceholder')" />
        </span>
        <select v-model="toolSource" class="cs-select diag-tool-source">
          <option value="">{{ t('mcpDiag.tools.allSources') }}</option>
          <option v-for="source in toolSources" :key="source" :value="source">{{ source }}</option>
        </select>
        <span class="cs-filter-count">{{ t('mcpDiag.tools.count', { count: filteredTools.length }) }}</span>
      </div>

      <div class="diag-tools-grid">
        <div class="cs-card cs-card-pad diag-tools-list">
          <h2 class="cs-card-title">
            {{ perspective === 'upstream' ? t('mcpDiag.tools.upstreamList', { count: filteredTools.length }) : t('mcpDiag.tools.downstreamList', { count: filteredTools.length }) }}
          </h2>
          <div v-if="filteredTools.length === 0" class="cs-empty">{{ t('mcpDiag.tools.empty') }}</div>
          <button
            v-for="tool in filteredTools"
            :key="tool.publicName"
            type="button"
            class="diag-tool-item"
            :class="{ selected: visibleTool?.publicName === tool.publicName }"
            @click="selectedTool = tool"
          >
            <span class="cs-mono diag-tool-item-name">
              {{ perspective === 'upstream' ? tool.upstreamName : tool.publicName }}
            </span>
            <span class="diag-tool-item-desc">{{ tool.description || tool.serverName }}</span>
            <span v-if="visibleTool?.publicName === tool.publicName" class="cs-pill cs-pill--brand">
              {{ t('mcpDiag.tools.selected') }}
            </span>
          </button>
        </div>

        <div v-if="visibleTool" class="cs-card cs-card-pad diag-tool-detail">
          <div class="diag-tool-detail-head">
            <div>
              <h2 class="diag-tool-name cs-mono">
                {{ perspective === 'upstream' ? visibleTool.upstreamName : visibleTool.publicName }}
              </h2>
              <p class="cs-section-sub">{{ visibleTool.description || t('mcpDiag.tools.noDescription') }}</p>
            </div>
            <span class="cs-pill cs-pill--success">{{ t('mcpDiag.tools.available') }}</span>
          </div>
          <hr class="cs-divider" />
          <h3 class="diag-tool-section">{{ t('mcpDiag.tools.infoTitle') }}</h3>
          <dl class="diag-kv diag-kv--2col">
            <div class="diag-kv-row"><dt>{{ t('mcpDiag.tools.kv.upstreamName') }}</dt><dd class="cs-mono">{{ visibleTool.upstreamName }}</dd></div>
            <div class="diag-kv-row"><dt>{{ t('mcpDiag.tools.kv.publicName') }}</dt><dd class="cs-mono">{{ visibleTool.publicName }}</dd></div>
            <div class="diag-kv-row"><dt>{{ t('mcpDiag.tools.kv.server') }}</dt><dd class="cs-mono">{{ visibleTool.serverName }}</dd></div>
            <div class="diag-kv-row">
              <dt>{{ t('mcpDiag.tools.kv.rw') }}</dt>
              <dd>{{ visibleTool.readOnly ? t('mcpDiag.tools.kv.readOnly') : t('mcpDiag.tools.kv.writable') }}{{ visibleTool.destructive ? ` · ${t('mcpDiag.tools.kv.destructive')}` : '' }}</dd>
            </div>
          </dl>
          <hr class="cs-divider" />
          <h3 class="diag-tool-section">{{ t('mcpDiag.tools.schemaTitle') }}</h3>
          <pre class="cs-code-box diag-schema">{{ formatSchema(visibleTool.inputSchema) || t('mcpDiag.tools.noSchema') }}</pre>
        </div>
        <div v-else class="cs-card cs-card-pad">
          <div class="cs-empty">{{ t('mcpDiag.tools.empty') }}</div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.diag-gw-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.diag-gw-line {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 12px;
}

.diag-gw-addr {
  font-size: 12px;
  font-weight: 500;
  color: var(--cs-text);
}

.diag-gw-path {
  font-size: 11px;
  color: var(--cs-text-secondary);
}

.diag-gw-actions {
  display: flex;
  gap: 10px;
  flex-shrink: 0;
}

.diag-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 18px;
}

.diag-metric {
  padding: 11px 14px;
  min-height: 88px;
}

.diag-metric-label {
  margin: 0;
  font-size: 11px;
  color: var(--cs-text-secondary);
}

.diag-metric-value {
  margin: 8px 0 0;
  font-size: 22px;
  font-weight: 700;
  color: var(--cs-text);
}

.diag-metric-value--sm {
  font-size: 15px;
  padding-top: 4px;
}

.diag-metric-value--ok,
.diag-metric-value--success {
  color: var(--cs-success);
}

.diag-metric-value--danger {
  color: var(--cs-danger);
}

.diag-metric-value--warning {
  color: var(--cs-warning);
}

.diag-metric-sub {
  margin: 8px 0 0;
  font-size: 10px;
  color: var(--cs-text-secondary);
}

.diag-gw-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.6fr);
  gap: 18px;
  align-items: start;
}

.diag-kv {
  margin: 14px 0 0;
  display: flex;
  flex-direction: column;
}

.diag-kv-row {
  display: grid;
  grid-template-columns: 122px minmax(0, 1fr);
  gap: 12px;
  padding: 9px 0;
  border-bottom: 1px solid var(--cs-border);
  font-size: 10px;
}

.diag-kv-row:last-child {
  border-bottom: none;
}

.diag-kv-row dt {
  color: var(--cs-text-secondary);
}

.diag-kv-row dd {
  margin: 0;
  color: var(--cs-text);
  font-weight: 500;
  word-break: break-all;
  user-select: text;
}

.diag-kv--2col .diag-kv-row {
  grid-template-columns: 140px minmax(0, 1fr);
}

.diag-kv-error {
  color: var(--cs-danger) !important;
}

.diag-kv-row--clients dd {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.diag-client {
  display: flex;
  align-items: center;
  gap: 10px;
}

.diag-tools-head,
.diag-tools-row {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr) 70px;
  gap: 12px;
  align-items: center;
  padding: 8px 10px;
}

.diag-tools-head {
  background: var(--cs-field);
  border-radius: 6px;
  font-size: 10px;
  font-weight: 500;
  color: var(--cs-text-secondary);
  margin-top: 12px;
}

.diag-tools-row {
  border-bottom: 1px solid var(--cs-border);
  font-size: 10px;
}

.diag-tools-name,
.diag-tools-source {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.diag-tools-source {
  color: var(--cs-text-secondary);
}

.diag-tools-more {
  margin-top: 10px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  color: var(--cs-brand);
  font-size: 10px;
  font-weight: 500;
  font-family: inherit;
  cursor: pointer;
  padding: 0;
}

.diag-up-head {
  display: flex;
  align-items: center;
  gap: 16px;
}

.diag-up-icon {
  width: 50px;
  height: 50px;
  border-radius: 12px;
  background: var(--cs-brand-tint);
  color: var(--cs-brand);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.diag-up-title {
  min-width: 0;
  flex: 1;
}

.diag-up-name {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: var(--cs-text);
}

.diag-up-cmd {
  margin: 6px 0 0;
  font-size: 11px;
  color: var(--cs-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.diag-up-side {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.diag-up-probe {
  font-size: 11px;
  color: var(--cs-text-secondary);
}

.diag-upstream-picker {
  width: auto;
  min-width: 160px;
}

.diag-cap-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.diag-events-title {
  margin: 18px 0 10px;
  font-size: 12px;
  font-weight: 700;
  color: var(--cs-text);
}

.diag-tool-search-wrap {
  flex: 1;
  position: relative;
  display: flex;
  align-items: center;
}

.diag-tool-search-icon {
  position: absolute;
  left: 0;
  color: var(--cs-text-secondary);
  pointer-events: none;
}

.diag-tool-search {
  width: 100%;
  border: none;
  background: transparent;
  outline: none;
  font-size: 11px;
  color: var(--cs-text);
  font-family: inherit;
  padding-left: 20px;
}

.diag-tool-search::placeholder {
  color: var(--cs-text-secondary);
}

.diag-tool-source {
  width: auto;
  min-width: 150px;
  min-height: 32px;
  border-color: transparent;
  background-color: transparent;
}

.diag-tools-grid {
  display: grid;
  grid-template-columns: minmax(260px, 330px) minmax(0, 1fr);
  gap: 18px;
  align-items: start;
}

.diag-tools-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 70vh;
  overflow-y: auto;
}

.diag-tool-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 9px 11px;
  border: 1px solid var(--cs-border);
  border-radius: 8px;
  background: var(--cs-surface);
  font-family: inherit;
  text-align: left;
  cursor: pointer;
  position: relative;
}

.diag-tool-item.selected {
  background: var(--cs-brand-tint);
  border-color: var(--cs-brand-tint-border);
}

.diag-tool-item-name {
  font-size: 11px;
  font-weight: 500;
  color: var(--cs-text);
  word-break: break-all;
}

.diag-tool-item.selected .diag-tool-item-name {
  color: var(--cs-brand);
}

.diag-tool-item-desc {
  font-size: 10px;
  color: var(--cs-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.diag-tool-item .cs-pill {
  position: absolute;
  top: 8px;
  right: 8px;
}

.diag-tool-detail {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.diag-tool-detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.diag-tool-name {
  margin: 0;
  font-size: 16px;
  font-weight: 500;
  color: var(--cs-text);
  word-break: break-all;
}

.diag-tool-section {
  margin: 6px 0 0;
  font-size: 13px;
  font-weight: 700;
  color: var(--cs-text);
}

.diag-schema {
  max-height: 300px;
  margin: 0;
}

@media (max-width: 1100px) {
  .diag-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .diag-gw-grid,
  .diag-tools-grid {
    grid-template-columns: 1fr;
  }
}
</style>
