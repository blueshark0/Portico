<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Activity, Ellipsis, Plug, Plus } from 'lucide-vue-next'
import BaseModal from '../common/BaseModal.vue'
import McpEditorModal from './McpEditorModal.vue'
import { useMcpServers } from './useMcpServers'
import {
  ServerType,
  fetchMcpTools,
  testMcpConnection,
  type McpServerRecord,
  type ToolInfo,
} from '../../services/mcp'
import { getErrorMessage } from '../../utils/errors'
import { showToast } from '../../utils/toast'
import { runWithToast } from '../../utils/runWithToast'

const { t } = useI18n()
const router = useRouter()

const {
  servers,
  statuses,
  editorState,
  confirmState,
  loadAll,
  refreshStatuses,
  injections,
  injecting,
  toggleInjection,
  openCreate,
  openEdit,
  closeEditor,
  submitEditor,
  submitEditorAndTest,
  toggleEnabled,
  askDelete,
  confirmDelete,
} = useMcpServers(t)

const tools = ref<ToolInfo[]>([])
const selectedId = ref<number | null>(null)
const menuFor = ref(0)
const testingId = ref(0)

const refreshTools = async () => {
  const list = await runWithToast(() => fetchMcpTools(), {
    fallback: t('mcpPage.messages.toolsLoadFailed'),
    silent: true,
  })
  tools.value = list ?? []
}

let pollTimer: number | undefined
let runtimeRefreshing = false

// 启用上游后连接是异步完成的；持续刷新内存快照，避免状态停留在“连接中”。
const refreshRuntime = async () => {
  if (runtimeRefreshing || document.hidden) return
  runtimeRefreshing = true
  try {
    await refreshStatuses()
    await refreshTools()
  } finally {
    runtimeRefreshing = false
  }
}

onMounted(async () => {
  await loadAll()
  await refreshTools()
  pollTimer = window.setInterval(() => void refreshRuntime(), 1000)
  window.addEventListener('click', closeMenu)
})

onUnmounted(() => {
  if (pollTimer !== undefined) window.clearInterval(pollTimer)
  window.removeEventListener('click', closeMenu)
})

const statusMap = computed(() => statuses.value)

const statusOf = (server: McpServerRecord) => {
  if (!server.enabled) return { tone: 'warning', label: t('mcpPage.status.disabled') }
  const state = statusMap.value[server.id]?.state
  if (state === 'connected') return { tone: 'success', label: t('mcpPage.status.connected') }
  if (state === 'failed') return { tone: 'danger', label: t('mcpPage.status.failed') }
  return { tone: 'muted', label: t('mcpPage.status.connecting') }
}

const toolCountOf = (server: McpServerRecord) => statusMap.value[server.id]?.toolCount ?? 0

const enabledCount = computed(() => servers.value.filter((s) => s.enabled).length)
const totalTools = computed(() =>
  servers.value.reduce((sum, s) => sum + (s.enabled ? toolCountOf(s) : 0), 0),
)

const selectedServer = computed(
  () => servers.value.find((s) => s.id === selectedId.value) ?? servers.value[0] ?? null,
)

const selectedTools = computed(() =>
  selectedServer.value ? tools.value.filter((tool) => tool.serverId === selectedServer.value!.id) : [],
)

const transportLabel = (server: McpServerRecord) =>
  server.type === ServerType.ServerTypeStdio ? 'stdio' : 'Streamable HTTP'

const serverSubtitle = (server: McpServerRecord) =>
  [
    transportLabel(server),
    server.type === ServerType.ServerTypeStdio ? t('mcpPage.localProcess') : t('mcpPage.remoteService'),
    t('mcpPage.toolCount', { count: toolCountOf(server) }),
  ].join(' · ')

const serverCommand = (server: McpServerRecord) => {
  if (server.type === ServerType.ServerTypeStdio) {
    return [server.command, ...(server.args ?? [])].filter(Boolean).join(' ')
  }
  return server.url ?? ''
}

const closeMenu = () => {
  menuFor.value = 0
}

const toggleMenu = (id: number) => {
  menuFor.value = menuFor.value === id ? 0 : id
}

const runTest = async (server: McpServerRecord) => {
  if (testingId.value) return
  testingId.value = server.id
  try {
    const result = await testMcpConnection(server)
    if (result.success) {
      showToast(t('mcpPage.messages.testOk', { count: result.toolCount, ms: result.durationMs }))
    } else {
      showToast(t('mcpPage.messages.testErr', { error: result.error ?? '' }), 'error')
    }
  } catch (error) {
    showToast(getErrorMessage(error, t('mcpPage.messages.testFailed')), 'error')
  } finally {
    testingId.value = 0
  }
}

const openDiagnostics = (server?: McpServerRecord) => {
  if (server) {
    void router.push({ path: '/mcp/diagnostics', query: { view: 'upstream', server: String(server.id) } })
  } else {
    void router.push({ path: '/mcp/diagnostics' })
  }
}

const onSaved = async () => {
  await refreshStatuses()
  await refreshTools()
}

const onSubmitEditor = async () => {
  const saved = await submitEditor()
  if (saved) await onSaved()
}

const onSubmitAndTest = async () => {
  await submitEditorAndTest()
  await onSaved()
}
</script>

<template>
  <div class="cs-page" @click="closeMenu">
    <div class="cs-page-head">
      <div>
        <h1 class="cs-page-title">{{ t('mcpPage.title') }}</h1>
        <p class="cs-page-sub">{{ t('mcpPage.subtitle') }}</p>
      </div>
      <div class="cs-page-actions">
        <button type="button" class="cs-btn" @click="openDiagnostics()">
          {{ t('mcpPage.actions.diagnostics') }}
        </button>
        <button type="button" class="cs-btn cs-btn--primary" @click="openCreate">
          <Plus :size="14" :stroke-width="2" />
          {{ t('mcpPage.actions.create') }}
        </button>
      </div>
    </div>

    <section class="cs-card mcp-cli" aria-labelledby="mcp-cli-title">
      <h2 id="mcp-cli-title" class="cs-card-title">{{ t('mcpPage.cli.title') }}</h2>
      <p class="cs-section-sub">{{ t('mcpPage.cli.subtitle') }}</p>
      <div class="mcp-cli-grid">
        <div
          v-for="platform in (['claude', 'codex'] as const)"
          :key="platform"
          class="mcp-cli-item"
        >
          <span class="mcp-cli-avatar" :class="`mcp-cli-avatar--${platform}`" aria-hidden="true">
            {{ platform === 'claude' ? 'C' : '⌘' }}
          </span>
          <span class="mcp-cli-copy">
            <span class="mcp-cli-name">{{ platform === 'claude' ? 'Claude Code' : 'Codex' }}</span>
            <span class="mcp-cli-description">
              {{ t(injections[platform] ? 'mcpPage.cli.injected' : 'mcpPage.cli.notInjected', { client: platform === 'claude' ? 'Claude Code' : 'Codex' }) }}
            </span>
          </span>
          <label class="cs-switch mcp-cli-switch">
            <input
              type="checkbox"
              :checked="injections[platform] === true"
              :disabled="injections[platform] === null || injecting[platform]"
              :aria-label="t('mcpPage.cli.toggle', { client: platform === 'claude' ? 'Claude Code' : 'Codex' })"
              @change="toggleInjection(platform)"
            />
            <span></span>
          </label>
        </div>
      </div>
      <p class="mcp-cli-help">{{ t('mcpPage.cli.help') }}</p>
    </section>

    <div class="mcp-grid">
      <div class="cs-card cs-card-pad mcp-list">
        <h2 class="cs-card-title">{{ t('mcpPage.list.title') }}</h2>
        <p class="cs-section-sub">
          {{ t('mcpPage.list.summary', { enabled: enabledCount, tools: totalTools }) }}
        </p>

        <div v-if="servers.length === 0" class="cs-empty">{{ t('mcpPage.list.empty') }}</div>

        <button
          v-for="server in servers"
          :key="server.id"
          type="button"
          class="mcp-row"
          :class="{ selected: selectedServer?.id === server.id }"
          @click.stop="selectedId = server.id"
        >
          <span class="mcp-avatar"><Plug :size="16" :stroke-width="2" /></span>
          <span class="mcp-main">
            <span class="mcp-name cs-mono">{{ server.name }}</span>
            <span class="mcp-sub">{{ serverSubtitle(server) }}</span>
          </span>
          <span class="cs-pill" :class="`cs-pill--${statusOf(server).tone}`">
            {{ statusOf(server).label }}
          </span>
          <span class="cs-menu-anchor" @click.stop>
            <button type="button" class="mcp-kebab" @click="toggleMenu(server.id)">
              <Ellipsis :size="16" :stroke-width="2" />
            </button>
            <div v-if="menuFor === server.id" class="cs-menu">
              <button type="button" class="cs-menu-item" @click="openEdit(server); closeMenu()">
                {{ t('mcpPage.menu.edit') }}
              </button>
              <button type="button" class="cs-menu-item" @click="toggleEnabled(server); closeMenu()">
                {{ server.enabled ? t('mcpPage.menu.disable') : t('mcpPage.menu.enable') }}
              </button>
              <button type="button" class="cs-menu-item cs-menu-item--danger" @click="askDelete(server); closeMenu()">
                {{ t('mcpPage.menu.delete') }}
              </button>
            </div>
          </span>
        </button>
      </div>

      <div v-if="selectedServer" class="cs-card cs-card-pad mcp-detail">
        <h2 class="mcp-detail-name cs-mono">{{ selectedServer.name }}</h2>
        <span class="cs-pill" :class="selectedServer.enabled ? 'cs-pill--success' : 'cs-pill--warning'">
          {{ selectedServer.enabled ? t('mcpPage.detail.enabled') : t('mcpPage.detail.disabled') }}
        </span>

        <p class="mcp-detail-label">{{ t('mcpPage.detail.transport') }}</p>
        <p class="mcp-detail-value cs-mono">{{ transportLabel(selectedServer) }}</p>

        <p class="mcp-detail-label">
          {{ selectedServer.type === ServerType.ServerTypeStdio ? t('mcpPage.detail.command') : t('mcpPage.detail.url') }}
        </p>
        <div class="mcp-command cs-mono">{{ serverCommand(selectedServer) }}</div>

        <template v-if="statusMap[selectedServer.id]?.error">
          <p class="mcp-detail-label">{{ t('mcpPage.detail.lastError') }}</p>
          <p class="mcp-detail-error">{{ statusMap[selectedServer.id].error }}</p>
        </template>

        <h3 class="mcp-tools-title">{{ t('mcpPage.detail.toolsTitle') }}</h3>
        <hr class="cs-divider" />
        <div v-if="selectedTools.length === 0" class="mcp-tools-empty">
          {{ t('mcpPage.detail.toolsEmpty') }}
        </div>
        <div v-for="tool in selectedTools.slice(0, 8)" :key="tool.publicName" class="mcp-tool-row">
          <span class="cs-mono">{{ tool.upstreamName }}</span>
        </div>
        <p v-if="selectedTools.length > 8" class="mcp-tools-more">
          {{ t('mcpPage.detail.toolsMore', { count: selectedTools.length - 8 }) }}
        </p>

        <div class="mcp-detail-actions">
          <button type="button" class="cs-btn" @click="openEdit(selectedServer)">
            {{ t('mcpPage.actions.edit') }}
          </button>
          <button
            type="button"
            class="cs-btn cs-btn--primary"
            :disabled="testingId === selectedServer.id"
            @click="runTest(selectedServer)"
          >
            {{ testingId === selectedServer.id ? t('mcpPage.actions.testing') : t('mcpPage.actions.test') }}
          </button>
        </div>
        <button type="button" class="cs-btn cs-btn--tint mcp-diag-btn" @click="openDiagnostics(selectedServer)">
          <Activity :size="14" :stroke-width="2" />
          {{ t('mcpPage.actions.openDiagnostics') }}
        </button>
      </div>
    </div>

    <McpEditorModal
      :state="editorState"
      @close="closeEditor"
      @submit="onSubmitEditor"
      @submit-and-test="onSubmitAndTest"
    />

    <BaseModal
      :open="confirmState.open"
      :title="t('mcpPage.delete.title')"
      width="narrow"
      @close="confirmState.open = false"
    >
      <p class="mcp-delete-body">
        {{ t('mcpPage.delete.body', { name: confirmState.target?.name ?? '' }) }}
      </p>
      <template #footer>
        <button type="button" class="cs-btn" @click="confirmState.open = false">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="cs-btn cs-btn--danger"
          :disabled="confirmState.busy"
          @click="confirmDelete().then(onSaved)"
        >
          {{ t('mcpPage.delete.confirm') }}
        </button>
      </template>
    </BaseModal>
  </div>
</template>

<style scoped>
.mcp-cli {
  padding: 12px 16px 10px;
}

.mcp-cli-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-top: 12px;
}

.mcp-cli-item {
  display: grid;
  grid-template-columns: 27px minmax(0, 1fr) 36px;
  align-items: center;
  gap: 10px;
  min-height: 49px;
  padding: 9px;
  border: 1px solid var(--cs-border);
  border-radius: var(--cs-r-row);
}

.mcp-cli-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 27px;
  height: 27px;
  border-radius: 7px;
  font-size: 15px;
  font-weight: 700;
}

.mcp-cli-avatar--claude {
  background: var(--cs-brand-tint);
  color: var(--cs-metric-reasoning);
}

.mcp-cli-avatar--codex {
  background: var(--cs-field);
  color: var(--cs-text);
}

.mcp-cli-copy {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.mcp-cli-name {
  color: var(--cs-text);
  font-size: 12px;
  font-weight: 500;
}

.mcp-cli-description {
  overflow: hidden;
  color: var(--cs-text-secondary);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mcp-cli-switch {
  width: 36px;
  height: 20px;
}

.mcp-cli-switch span::before {
  width: 14px;
  height: 14px;
}

.mcp-cli-switch input:checked + span::before {
  transform: translateX(16px);
}

.mcp-cli-switch input:focus-visible + span {
  outline: 2px solid var(--cs-brand);
  outline-offset: 2px;
}

.mcp-cli-help {
  margin: 8px 0 0;
  color: var(--cs-text-secondary);
  font-size: 9px;
  line-height: 1.5;
}

.mcp-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.9fr) minmax(340px, 1fr);
  gap: 18px;
  align-items: start;
}

.mcp-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.mcp-list .cs-section-sub {
  margin-bottom: 8px;
}

.mcp-row {
  display: flex;
  align-items: center;
  gap: 14px;
  min-height: 88px;
  padding: 12px 14px;
  border-radius: var(--cs-r-row);
  border: 1px solid var(--cs-border);
  background: var(--cs-surface);
  font-family: inherit;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.12s ease, background 0.12s ease;
}

.mcp-row.selected {
  background: var(--cs-brand-tint);
  border-color: var(--cs-brand-tint-border);
}

.mcp-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--cs-text-secondary);
  flex-shrink: 0;
  width: 20px;
}

.mcp-main {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

.mcp-name {
  font-size: 12px;
  font-weight: 500;
  color: var(--cs-text);
}

.mcp-sub {
  font-size: 10px;
  color: var(--cs-text-secondary);
}

.mcp-kebab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  color: var(--cs-text-secondary);
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 6px;
}

.mcp-kebab:hover {
  background: var(--cs-row-hover);
}

.mcp-detail {
  display: flex;
  flex-direction: column;
  gap: 10px;
  position: sticky;
  top: 0;
}

.mcp-detail-name {
  margin: 0;
  font-size: 17px;
  font-weight: 700;
  color: var(--cs-text);
}

.mcp-detail .cs-pill {
  align-self: flex-start;
}

.mcp-detail-label {
  margin: 10px 0 0;
  font-size: 11px;
  color: var(--cs-text-secondary);
}

.mcp-detail-value {
  margin: 0;
  font-size: 13px;
  font-weight: 500;
  color: var(--cs-text);
}

.mcp-command {
  background: var(--cs-field);
  border: 1px solid var(--cs-border);
  border-radius: var(--cs-r-field);
  padding: 12px 10px;
  font-size: 10px;
  color: var(--cs-text);
  word-break: break-all;
  user-select: text;
}

.mcp-detail-error {
  margin: 0;
  font-size: 11px;
  color: var(--cs-danger);
  word-break: break-all;
}

.mcp-tools-title {
  margin: 14px 0 0;
  font-size: 14px;
  font-weight: 700;
  color: var(--cs-text);
}

.mcp-tools-empty {
  font-size: 11px;
  color: var(--cs-text-secondary);
  padding: 10px 0;
}

.mcp-tool-row {
  padding: 8px 0;
  border-bottom: 1px solid var(--cs-border);
  font-size: 10px;
  font-weight: 500;
  color: var(--cs-text);
}

.mcp-tools-more {
  margin: 8px 0 0;
  font-size: 10px;
  color: var(--cs-text-secondary);
}

.mcp-detail-actions {
  display: flex;
  gap: 9px;
  margin-top: 12px;
}

.mcp-diag-btn {
  margin-top: 4px;
  width: 100%;
}

.mcp-delete-body {
  margin: 0 0 12px;
  font-size: 12px;
  color: var(--cs-text);
  line-height: 1.7;
}

@media (max-width: 1100px) {
  .mcp-cli-grid {
    grid-template-columns: 1fr;
  }

  .mcp-grid {
    grid-template-columns: 1fr;
  }

  .mcp-detail {
    position: static;
  }
}
</style>
