<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Sparkles, Command, ChevronRight } from 'lucide-vue-next'
import { useDashboard, type DashboardClient } from './useDashboard'
import { useQuickSwitch } from '../../composables/useQuickSwitch'
import { parseDateTime } from '../../utils/dateTime'
import { shortSessionId } from '../../utils/sessionId'
import brandClaude from '../../assets/icons/brand-claude.svg'
import brandCodex from '../../assets/icons/brand-codex.svg'
import brandGemini from '../../assets/icons/brand-gemini.svg'

const { t } = useI18n()
const router = useRouter()
const { openQuickSwitch } = useQuickSwitch()
const {
  clients,
  recentSessions,
  mcpEndpoint,
  mcpHealth,
  enabledServerCount,
  totalToolCount,
  proxyBusy,
  toggleClientProxy,
} = useDashboard()

const CLIENT_ICONS: Record<DashboardClient['platform'], string> = {
  claude: brandClaude,
  codex: brandCodex,
  gemini: brandGemini,
}

const formatSessionTime = (value: unknown) => {
  const date = parseDateTime(value)
  if (!date) return ''
  const now = new Date()
  const sameDay = date.toDateString() === now.toDateString()
  const hhmm = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
  if (sameDay) return t('dashboard.recent.todayAt', { time: hhmm })
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' }).format(date)
}

const goToMcp = () => {
  void router.push('/mcp')
}

const goToSessions = () => {
  void router.push('/sessions')
}

const copyEndpoint = async () => {
  try {
    await navigator.clipboard.writeText(mcpEndpoint.value)
  } catch (error) {
    console.error('copy endpoint failed', error)
  }
}
</script>

<template>
  <div class="cs-page">
    <div class="cs-page-head">
      <div>
        <h1 class="cs-page-title">{{ t('dashboard.title') }}</h1>
        <p class="cs-page-sub">{{ t('dashboard.subtitle') }}</p>
      </div>
    </div>

    <button type="button" class="dash-quick" @click="openQuickSwitch()">
      <span class="dash-quick-label">
        <Sparkles :size="14" :stroke-width="2" class="dash-quick-sparkle" />
        {{ t('dashboard.quickSwitch.label') }}
      </span>
      <span class="dash-quick-kbd">
        <Command :size="12" :stroke-width="2" />
        <span>K</span>
      </span>
      <span class="dash-quick-hint">{{ t('dashboard.quickSwitch.hint') }}</span>
      <span class="cs-btn dash-quick-btn">{{ t('dashboard.quickSwitch.open') }}</span>
    </button>

    <section>
      <h2 class="cs-section-title">{{ t('dashboard.clients.title') }}</h2>
      <p class="cs-section-sub">{{ t('dashboard.clients.subtitle') }}</p>
      <div class="dash-clients">
        <div v-for="client in clients" :key="client.platform" class="cs-card dash-client">
          <div class="dash-client-head">
            <span class="dash-client-icon">
              <img :src="CLIENT_ICONS[client.platform]" :alt="client.label" class="dash-client-icon-img" />
            </span>
            <span class="dash-client-name">{{ client.label }}</span>
            <span
              class="cs-pill"
              :class="client.proxyEnabled ? 'cs-pill--success' : 'cs-pill--muted'"
            >
              {{ client.proxyEnabled ? t('dashboard.clients.applied') : t('dashboard.clients.notApplied') }}
            </span>
          </div>
          <p class="dash-client-label">{{ t('dashboard.clients.currentProvider') }}</p>
          <p class="dash-client-provider">{{ client.providerName || t('dashboard.clients.none') }}</p>
          <p class="dash-client-model cs-mono">{{ client.model || '—' }}</p>
          <hr class="cs-divider" />
          <div class="dash-client-foot">
            <label class="dash-client-relay">
              <span class="cs-switch cs-switch--sm">
                <input
                  type="checkbox"
                  :checked="client.proxyEnabled"
                  :disabled="proxyBusy[client.platform]"
                  @change="toggleClientProxy(client.platform, !client.proxyEnabled, t('dashboard.clients.toggleFailed'))"
                />
                <span></span>
              </span>
              <span class="dash-client-relay-label">{{ t('dashboard.clients.relay') }}</span>
            </label>
            <button type="button" class="dash-client-switch" @click="openQuickSwitch(client.platform)">
              {{ t('dashboard.clients.switch') }}
              <ChevronRight :size="12" :stroke-width="2" />
            </button>
          </div>
        </div>
      </div>
    </section>

    <section>
      <h2 class="cs-section-title">{{ t('dashboard.frequent.title') }}</h2>
      <div class="dash-frequent">
        <div class="cs-card dash-recent">
          <div v-if="recentSessions.length === 0" class="cs-empty">
            {{ t('dashboard.recent.empty') }}
          </div>
          <template v-else>
            <button
              v-for="(session, index) in recentSessions"
              :key="`${session.platform}-${session.sessionId}`"
              type="button"
              class="dash-recent-row"
              @click="goToSessions"
            >
              <span class="dash-recent-name">{{ session.providerName }} · {{ session.platformLabel }}</span>
              <span class="dash-recent-meta cs-mono" :title="session.sessionId">{{ shortSessionId(session.sessionId) }}</span>
              <span class="dash-recent-time">{{ formatSessionTime(session.lastSuccessAt) }}</span>
              <hr v-if="index < recentSessions.length - 1" class="cs-divider" />
            </button>
          </template>
        </div>

        <div class="cs-card cs-card-pad dash-local">
          <div class="dash-local-head">
            <h3 class="cs-card-title">{{ t('dashboard.local.title') }}</h3>
            <span
              class="cs-pill"
              :class="{
                'cs-pill--success': mcpHealth === 'ok',
                'cs-pill--danger': mcpHealth === 'error',
                'cs-pill--muted': mcpHealth === 'idle',
              }"
            >
              {{
                mcpHealth === 'ok'
                  ? t('dashboard.local.healthy')
                  : mcpHealth === 'error'
                    ? t('dashboard.local.degraded')
                    : t('dashboard.local.idle')
              }}
            </span>
          </div>
          <p class="dash-local-label">{{ t('dashboard.local.endpoint') }}</p>
          <button type="button" class="dash-local-endpoint cs-mono" :title="t('dashboard.local.copy')" @click="copyEndpoint">
            {{ mcpEndpoint }}
          </button>
          <p class="dash-local-label">{{ t('dashboard.local.mcpServices') }}</p>
          <p class="dash-local-count">
            {{ t('dashboard.local.summary', { servers: enabledServerCount, tools: totalToolCount }) }}
          </p>
          <div class="dash-local-actions">
            <button type="button" class="cs-btn" @click="goToMcp">
              {{ t('dashboard.local.manage') }}
            </button>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.dash-quick {
  display: flex;
  align-items: center;
  gap: 14px;
  min-height: 49px;
  padding: 6px 6px 6px 16px;
  background: var(--cs-surface);
  border: 1px solid var(--cs-border);
  border-radius: 11px;
  cursor: pointer;
  font-family: inherit;
  text-align: left;
  transition: border-color 0.15s ease;
}

.dash-quick:hover {
  border-color: var(--cs-brand-tint-border);
}

.dash-quick-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;
  color: var(--cs-text);
}

.dash-quick-sparkle {
  color: var(--cs-brand);
}

.dash-quick-kbd {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  font-weight: 500;
  color: var(--cs-text-secondary);
}

.dash-quick-hint {
  font-size: 11px;
  color: var(--cs-text-secondary);
}

.dash-quick-btn {
  margin-left: auto;
  pointer-events: none;
}

.dash-clients {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
  margin-top: 14px;
}

.dash-client {
  padding: 16px 17px 12px;
  display: flex;
  flex-direction: column;
}

.dash-client-head {
  display: flex;
  align-items: center;
  gap: 11px;
}

.dash-client-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: var(--cs-field);
  border: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.dash-client-icon-img {
  width: 24px;
  height: 24px;
  display: block;
}

.dash-client-name {
  font-size: 14px;
  font-weight: 700;
  color: var(--cs-text);
}

.dash-client-head .cs-pill {
  margin-left: auto;
}

.dash-client-label {
  margin: 16px 0 0;
  font-size: 10px;
  color: var(--cs-text-secondary);
}

.dash-client-provider {
  margin: 6px 0 0;
  font-size: 13px;
  font-weight: 500;
  color: var(--cs-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dash-client-model {
  margin: 8px 0 12px;
  font-size: 11px;
  color: var(--cs-text-secondary);
}

.dash-client-switch {
  align-self: flex-end;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  border: none;
  background: transparent;
  color: var(--cs-brand);
  font-size: 10px;
  font-weight: 500;
  font-family: inherit;
  cursor: pointer;
  padding: 8px 0 2px;
}

.dash-client-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 4px;
}

.dash-client-relay {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.dash-client-relay-label {
  font-size: 10px;
  color: var(--cs-text-secondary);
}

.cs-switch--sm {
  transform: scale(0.8);
  transform-origin: left center;
}

.dash-frequent {
  display: grid;
  grid-template-columns: minmax(0, 1.9fr) minmax(0, 1fr);
  gap: 18px;
  margin-top: 14px;
}

.dash-recent {
  padding: 6px 0;
  min-height: 200px;
}

.dash-recent-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  column-gap: 16px;
  row-gap: 2px;
  width: 100%;
  padding: 12px 19px;
  border: none;
  background: transparent;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
}

.dash-recent-row:hover {
  background: var(--cs-row-hover);
}

.dash-recent-row .cs-divider {
  grid-column: 1 / -1;
}

.dash-recent-name {
  font-size: 12px;
  font-weight: 500;
  color: var(--cs-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dash-recent-meta {
  font-size: 10px;
  color: var(--cs-text-secondary);
  grid-column: 1;
}

.dash-recent-time {
  grid-column: 2 / 4;
  grid-row: 1 / 3;
  font-size: 10px;
  color: var(--cs-text-secondary);
  align-self: center;
}

.dash-local {
  display: flex;
  flex-direction: column;
}

.dash-local-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.dash-local-label {
  margin: 18px 0 0;
  font-size: 10px;
  color: var(--cs-text-secondary);
}

.dash-local-endpoint {
  margin-top: 8px;
  padding: 12px 10px;
  background: var(--cs-field);
  border: 1px solid var(--cs-border);
  border-radius: var(--cs-r-field);
  font-size: 11px;
  color: var(--cs-text);
  text-align: left;
  cursor: copy;
  font-family: var(--cs-font-mono);
}

.dash-local-count {
  margin: 8px 0 0;
  font-size: 13px;
  font-weight: 500;
  color: var(--cs-text);
}

.dash-local-actions {
  margin-top: auto;
  padding-top: 18px;
  display: flex;
  justify-content: flex-end;
}

@media (max-width: 1100px) {
  .dash-clients {
    grid-template-columns: 1fr;
  }

  .dash-frequent {
    grid-template-columns: 1fr;
  }
}
</style>
