<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronRight, Search } from 'lucide-vue-next'
import { providerTabs, type ProviderTab } from '../../constants/platforms'
import {
  fetchPlatformSessions,
  unbindSession,
  type SessionBinding,
} from '../../services/sessions'
import { parseDateTime } from '../../utils/dateTime'
import { getErrorMessage } from '../../utils/errors'
import { formatTokenCount } from '../../utils/number'
import { displaySessionId } from '../../utils/sessionId'
import { formatUsd } from '../../utils/currency'
import { showToast } from '../../utils/toast'

type SessionRow = SessionBinding & { platformLabel: string }

const { t } = useI18n()

const tabs = computed(() => [
  { id: 'all' as const, label: t('sessionsPage.filters.all') },
  ...providerTabs.map((tab) => ({ id: tab.id, label: tab.label })),
])

const filter = ref<'all' | ProviderTab>('all')
const keyword = ref('')
const rows = ref<SessionRow[]>([])
const loading = ref(false)
const menuFor = ref('')

const loadSessions = async () => {
  loading.value = true
  try {
    const targets =
      filter.value === 'all' ? providerTabs.map((tab) => tab.id) : [filter.value]
    const collected: SessionRow[] = []
    await Promise.all(
      targets.map(async (platform) => {
        const label = providerTabs.find((tab) => tab.id === platform)?.label ?? platform
        const bindings = await fetchPlatformSessions(platform)
        for (const binding of bindings ?? []) {
          collected.push({ ...binding, platformLabel: label })
        }
      }),
    )
    rows.value = collected.sort((a, b) => {
      const ta = parseDateTime(a.last_success_at)?.getTime() ?? 0
      const tb = parseDateTime(b.last_success_at)?.getTime() ?? 0
      return tb - ta
    })
  } catch (error) {
    console.error('Failed to load sessions', error)
    rows.value = []
  } finally {
    loading.value = false
  }
}

const visibleRows = computed(() => {
  const query = keyword.value.trim().toLowerCase()
  if (!query) return rows.value
  return rows.value.filter((row) =>
    `${row.session_id} ${row.provider_name} ${row.platformLabel} ${(row.models ?? []).join(' ')}`
      .toLowerCase()
      .includes(query),
  )
})

const switchFilter = (id: 'all' | ProviderTab) => {
  if (filter.value === id) return
  filter.value = id
  void loadSessions()
}

const formatTime = (value: unknown) => {
  const date = parseDateTime(value)
  if (!date) return t('sessionsPage.dateUnknown')
  const diffMinutes = Math.floor((Date.now() - date.getTime()) / 60000)
  if (diffMinutes < 1) return t('sessionsPage.justNow')
  if (diffMinutes < 60) return t('sessionsPage.minutesAgo', { count: diffMinutes })
  if (diffMinutes < 24 * 60) return t('sessionsPage.hoursAgo', { count: Math.floor(diffMinutes / 60) })
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

const modelsLabel = (row: SessionRow) => {
  const models = row.models ?? []
  if (models.length === 0) return ''
  if (models.length === 1) return models[0]
  return `${models[0]} +${models.length - 1}`
}

const usageLabel = (row: SessionRow) =>
  t('sessionsPage.usage', {
    count: row.requests,
    input: formatTokenCount(row.input_tokens),
    output: formatTokenCount(row.output_tokens),
    cost: formatUsd(row.total_cost),
  })

const toggleMenu = (key: string) => {
  menuFor.value = menuFor.value === key ? '' : key
}

const closeMenu = () => {
  menuFor.value = ''
}

const unbind = async (row: SessionRow) => {
  closeMenu()
  try {
    await unbindSession(row.platform, row.session_id)
    rows.value = rows.value.filter(
      (item) => !(item.platform === row.platform && item.session_id === row.session_id),
    )
    showToast(t('sessionsPage.messages.unbound'))
  } catch (error) {
    showToast(getErrorMessage(error, t('sessionsPage.messages.unbindFailed')), 'error')
  }
}

onMounted(() => {
  void loadSessions()
  window.addEventListener('click', closeMenu)
})

onUnmounted(() => {
  window.removeEventListener('click', closeMenu)
})
</script>

<template>
  <div class="cs-page">
    <div class="cs-page-head">
      <div>
        <h1 class="cs-page-title">{{ t('sessionsPage.title') }}</h1>
        <p class="cs-page-sub">{{ t('sessionsPage.subtitle') }}</p>
      </div>
    </div>

    <div class="cs-card cs-card-pad sess-panel">
      <div class="cs-filterbar cs-filterbar--inset">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          type="button"
          class="cs-tab"
          :class="{ active: filter === tab.id }"
          @click="switchFilter(tab.id)"
        >
          {{ tab.label }}
        </button>
        <span class="sess-search-wrap">
          <Search :size="13" :stroke-width="2" class="sess-search-icon" />
          <input
            v-model="keyword"
            class="sess-search"
            :placeholder="t('sessionsPage.searchPlaceholder')"
          />
        </span>
      </div>

      <div v-if="visibleRows.length === 0 && !loading" class="cs-empty">
        {{ t('sessionsPage.empty') }}
      </div>

      <div
        v-for="row in visibleRows"
        :key="`${row.platform}:${row.session_id}`"
        class="sess-row"
      >
        <div class="sess-main">
          <span class="sess-id cs-mono" :title="displaySessionId(row.session_id)">{{ displaySessionId(row.session_id) }}</span>
          <span class="sess-sub">
            {{ row.platformLabel }} · {{ row.provider_name }}<template v-if="modelsLabel(row)"> · {{ modelsLabel(row) }}</template>
          </span>
          <span v-if="row.requests > 0" class="sess-usage">{{ usageLabel(row) }}</span>
          <span v-else class="sess-usage sess-usage--empty">{{ t('sessionsPage.noUsage') }}</span>
        </div>
        <span class="sess-time">{{ formatTime(row.last_success_at) }}</span>
        <span class="cs-menu-anchor" @click.stop>
          <button type="button" class="sess-chevron" @click="toggleMenu(`${row.platform}:${row.session_id}`)">
            <ChevronRight :size="15" :stroke-width="2" />
          </button>
          <div v-if="menuFor === `${row.platform}:${row.session_id}`" class="cs-menu">
            <button type="button" class="cs-menu-item cs-menu-item--danger" @click="unbind(row)">
              {{ t('sessionsPage.unbind') }}
            </button>
          </div>
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.sess-panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.sess-search-wrap {
  margin-left: auto;
  position: relative;
  display: flex;
  align-items: center;
}

.sess-search-icon {
  position: absolute;
  left: 0;
  color: var(--cs-text-secondary);
  pointer-events: none;
}

.sess-search {
  border: none;
  background: transparent;
  font-size: 11px;
  color: var(--cs-text);
  font-family: inherit;
  outline: none;
  min-width: 180px;
  padding-left: 20px;
}

.sess-search::placeholder {
  color: var(--cs-text-secondary);
}

.sess-row {
  display: flex;
  align-items: center;
  gap: 16px;
  min-height: 80px;
  padding: 12px 17px;
  border: 1px solid var(--cs-border);
  border-radius: var(--cs-r-row);
  background: var(--cs-surface);
}

.sess-main {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

.sess-id {
  font-size: 13px;
  font-weight: 500;
  color: var(--cs-text);
  overflow-wrap: anywhere;
}

.sess-sub {
  font-size: 10px;
  color: var(--cs-text-secondary);
}

.sess-usage {
  font-size: 10px;
  color: var(--cs-text);
}

.sess-usage--empty {
  color: var(--cs-text-secondary);
}

.sess-time {
  font-size: 10px;
  color: var(--cs-text-secondary);
  white-space: nowrap;
}

.sess-chevron {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  color: var(--cs-text-secondary);
  cursor: pointer;
  padding: 2px 8px;
  border-radius: 6px;
}

.sess-chevron:hover {
  background: var(--cs-row-hover);
}
</style>
