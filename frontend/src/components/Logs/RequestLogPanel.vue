<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { providerTabs } from '../../constants/platforms'
import { formatBeijingDateTime } from '../../utils/dateTime'
import { formatUsdPrecise } from '../../utils/currency'
import type { LogCostTier, RequestLog } from '../../services/logs'
import { useLogsDashboard } from './useLogsDashboard'

const { t } = useI18n()
const router = useRouter()
const {
  filters,
  providerOptions,
  rangeOptions,
  httpStatusOptions,
  summaryStats,
  pagedLogs,
  loading,
  exporting,
  page,
  pageSize,
  pageSizeOptions,
  totalPages,
  totalLogs,
  applyFilters,
  manualRefresh,
  nextPage,
  prevPage,
  updatePageSize,
  exportCsv,
} = useLogsDashboard({ t })

const platformOptions = computed(() => [
  { value: '', label: t('logsPage.filters.allPlatforms') },
  ...providerTabs.map((tab) => ({ value: tab.id as string, label: tab.label })),
])
const costTierOptions = computed<Array<{ value: LogCostTier; label: string }>>(() => [
  { value: 'all', label: t('logsPage.filters.costTiers.all') },
  { value: 'low', label: t('logsPage.filters.costTiers.low') },
  { value: 'medium', label: t('logsPage.filters.costTiers.medium') },
  { value: 'high', label: t('logsPage.filters.costTiers.high') },
])

const summaryMetrics = computed(() => {
  const stats = summaryStats.value
  const count = (value: number | undefined) => stats ? (value ?? 0).toLocaleString() : '—'
  return [
    {
      key: 'cost',
      label: t('logsPage.filteredSummary.cost'),
      value: stats ? formatUsdPrecise(stats.cost_total) : '—',
    },
    { key: 'requests', label: t('logsPage.filteredSummary.requests'), value: count(stats?.total_requests) },
    {
      key: 'total',
      label: t('logsPage.filteredSummary.totalTokens'),
      value: count(stats ? stats.input_tokens + stats.output_tokens : undefined),
    },
    { key: 'input', label: t('logsPage.filteredSummary.inputTokens'), value: count(stats?.input_tokens) },
    { key: 'output', label: t('logsPage.filteredSummary.outputTokens'), value: count(stats?.output_tokens) },
    { key: 'cacheRead', label: t('logsPage.filteredSummary.cacheRead'), value: count(stats?.cache_read_tokens) },
    { key: 'cacheCreate', label: t('logsPage.filteredSummary.cacheCreate'), value: count(stats?.cache_create_tokens) },
    { key: 'reasoning', label: t('logsPage.filteredSummary.reasoning'), value: count(stats?.reasoning_tokens) },
  ]
})

const platformLabel = (platform: string) => providerTabs.find((tab) => tab.id === platform)?.label ?? platform
const formatTime = (value: string) => formatBeijingDateTime(value)
const formatDuration = (value: number) => value > 0 ? `${value >= 10 ? value.toFixed(1) : value.toFixed(2)} s` : '—'
const formatCost = (log: RequestLog) => log.has_pricing ? formatUsdPrecise(log.total_cost) : '—'
const isSuccess = (log: RequestLog) => log.http_code >= 200 && log.http_code < 300
const pageStart = computed(() => totalLogs.value === 0 ? 0 : (page.value - 1) * pageSize.value + 1)
const pageEnd = computed(() => totalLogs.value === 0 ? 0 : Math.min(totalLogs.value, pageStart.value + pagedLogs.value.length - 1))

const traceSession = (log: RequestLog) => {
  if (!log.session_id) return
  void router.push({
    path: '/logs/session',
    query: {
      platform: log.platform,
      session: log.session_id,
      request: String(log.id),
    },
  })
}
</script>

<template>
  <div class="logs-request-view">
    <section class="cs-card logs-request-panel" aria-labelledby="request-log-title">
      <div class="logs-request-head">
        <div>
          <h2 id="request-log-title" class="logs-request-title">{{ t('logsPage.recent.title') }}</h2>
          <p>{{ t('logsPage.recent.subtitle') }}</p>
        </div>
        <div class="logs-request-actions">
          <span class="cs-pill cs-pill--brand">{{ t('logsPage.pager.total', { total: totalLogs.toLocaleString() }) }}</span>
          <button type="button" class="cs-btn logs-export" :disabled="totalLogs === 0 || exporting" @click="exportCsv">
            {{ exporting ? t('logsPage.actions.exporting') : t('logsPage.actions.export') }}
          </button>
        </div>
      </div>

      <div class="logs-request-filters">
        <div class="logs-filter-primary">
          <select v-model="filters.platform" class="cs-select" :aria-label="t('logsPage.filters.platform')" @change="applyFilters">
            <option v-for="option in platformOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
          </select>
          <select v-model="filters.provider" class="cs-select" :aria-label="t('logsPage.filters.provider')" @change="applyFilters">
            <option value="">{{ t('logsPage.filters.allProviders') }}</option>
            <option v-for="provider in providerOptions" :key="provider" :value="provider">{{ provider }}</option>
          </select>
          <select v-model="filters.httpStatus" class="cs-select" :aria-label="t('logsPage.filters.status')" @change="applyFilters">
            <option v-for="option in httpStatusOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
          </select>
          <select v-model="filters.rangeKey" class="cs-select" :aria-label="t('logsPage.filters.range')" @change="applyFilters">
            <option v-for="option in rangeOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
          </select>
          <select v-model.number="pageSize" class="cs-select" :aria-label="t('logsPage.filters.pageSize')" @change="updatePageSize">
            <option v-for="option in pageSizeOptions" :key="option.value" :value="option.value">
              {{ t('logsPage.filters.pageSizeOption', { count: option.value }) }}
            </option>
          </select>
          <span class="logs-filter-spacer" />
          <button type="button" class="cs-btn logs-refresh" :disabled="loading" @click="manualRefresh">
            {{ t('logsPage.actions.refresh') }}
          </button>
        </div>
        <div class="logs-cost-filter">
          <span class="logs-cost-label">{{ t('logsPage.filters.costTier') }}</span>
          <button
            v-for="option in costTierOptions"
            :key="option.value"
            type="button"
            class="logs-cost-option"
            :class="{ active: filters.costTier === option.value, 'logs-cost-option--all': option.value === 'all' }"
            :aria-pressed="filters.costTier === option.value"
            @click="filters.costTier = option.value; applyFilters()"
          >
            {{ option.label }}
          </button>
          <span class="logs-cost-hint">{{ t('logsPage.filters.costHint') }}</span>
        </div>
      </div>

      <section
        class="logs-filter-summary"
        :class="{ 'logs-filter-summary--loading': loading }"
        aria-labelledby="filtered-summary-title"
      >
        <div class="logs-summary-scope">
          <h3 id="filtered-summary-title">{{ t('logsPage.filteredSummary.title') }}</h3>
          <p>{{ t('logsPage.filteredSummary.subtitle') }}</p>
        </div>
        <span class="logs-summary-divider" aria-hidden="true" />
        <div v-for="metric in summaryMetrics" :key="metric.key" class="logs-summary-metric">
          <p class="logs-summary-label">{{ metric.label }}</p>
          <p
            class="logs-summary-value cs-mono"
            :class="{ 'logs-summary-value--brand': metric.key === 'cost' }"
            :title="metric.value"
          >
            {{ metric.value }}
          </p>
        </div>
      </section>

      <div class="logs-table-scroll" :class="{ 'logs-table-scroll--loading': loading }">
        <div class="logs-rows" role="table" :aria-label="t('logsPage.recent.title')">
          <div class="logs-request-row logs-request-row--head" role="row">
            <span role="columnheader">{{ t('logsPage.columns.time') }}</span>
            <span role="columnheader">{{ t('logsPage.columns.client') }}</span>
            <span role="columnheader">{{ t('logsPage.columns.provider') }}</span>
            <span role="columnheader">{{ t('logsPage.columns.model') }}</span>
            <span class="logs-token-heading" role="columnheader">
              {{ t('logsPage.columns.tokens') }}
              <span class="logs-token-info" tabindex="0" :aria-label="t('logsPage.recent.tokenHint')" :title="t('logsPage.recent.tokenHint')">i</span>
            </span>
            <span role="columnheader">{{ t('logsPage.columns.cost') }}</span>
            <span role="columnheader">{{ t('logsPage.columns.ttft') }}</span>
            <span role="columnheader">{{ t('logsPage.columns.duration') }}</span>
            <span role="columnheader">{{ t('logsPage.columns.status') }}</span>
            <span role="columnheader">{{ t('logsPage.columns.actions') }}</span>
          </div>

          <div v-if="pagedLogs.length === 0" class="cs-empty logs-request-empty">
            {{ loading ? t('logsPage.loading') : t('logsPage.empty') }}
          </div>
          <div
            v-for="(log, index) in pagedLogs"
            :key="log.id"
            :data-request-id="log.id"
            class="logs-request-row logs-request-row--data"
            :class="{ 'logs-request-row--alt': index % 2 === 0 }"
            role="row"
          >
            <span class="logs-cell-time cs-mono" role="cell" :title="formatTime(log.created_at)">{{ formatTime(log.created_at) }}</span>
            <span class="logs-cell-ellipsis" role="cell">{{ platformLabel(log.platform) }}</span>
            <span class="logs-cell-ellipsis" role="cell" :title="log.provider">{{ log.provider || '—' }}</span>
            <span class="logs-cell-ellipsis" role="cell" :title="log.model">{{ log.model || '—' }}</span>
            <span class="logs-token-cell" role="cell">
              <span class="logs-token-pair">
                <span class="logs-token-label">{{ t('logsPage.tokens.inputShort') }}</span>
                <span class="logs-token-value cs-mono" :title="log.input_tokens.toLocaleString()">{{ log.input_tokens.toLocaleString() }}</span>
              </span>
              <span class="logs-token-pair">
                <span class="logs-token-label">{{ t('logsPage.tokens.outputShort') }}</span>
                <span class="logs-token-value cs-mono" :title="log.output_tokens.toLocaleString()">{{ log.output_tokens.toLocaleString() }}</span>
              </span>
              <span class="logs-token-pair logs-token-pair--cache">
                <span class="logs-token-cache-label">{{ t('logsPage.tokens.cacheReadShort') }}</span>
                <span class="logs-token-value cs-mono" :title="log.cache_read_tokens.toLocaleString()">{{ log.cache_read_tokens.toLocaleString() }}</span>
              </span>
              <span class="logs-token-pair logs-token-pair--cache">
                <span class="logs-token-cache-label">{{ t('logsPage.tokens.cacheWriteShort') }}</span>
                <span class="logs-token-value cs-mono" :title="log.cache_create_tokens.toLocaleString()">{{ log.cache_create_tokens.toLocaleString() }}</span>
              </span>
            </span>
            <span class="logs-cell-cost cs-mono" role="cell">{{ formatCost(log) }}</span>
            <span class="cs-mono" :class="{ 'logs-cell-muted': !log.ttft_sec }" role="cell" :title="t('logsPage.columns.ttftHint')">{{ formatDuration(log.ttft_sec) }}</span>
            <span class="cs-mono" role="cell">{{ formatDuration(log.duration_sec) }}</span>
            <span role="cell">
              <span class="cs-pill logs-status-pill" :class="isSuccess(log) ? 'cs-pill--success' : 'cs-pill--danger'" :title="`HTTP ${log.http_code}`">
                {{ isSuccess(log) ? t('logsPage.status.success') : t('logsPage.status.failure') }}
              </span>
            </span>
            <span role="cell">
              <button
                type="button"
                class="cs-btn logs-trace-button"
                :disabled="!log.session_id"
                :title="log.session_id ? t('logsPage.actions.traceTitle') : t('logsPage.actions.traceUnavailable')"
                @click="traceSession(log)"
              >
                {{ t('logsPage.actions.trace') }} ↗
              </button>
            </span>
          </div>
        </div>
      </div>

      <div class="logs-request-pager">
        <span class="logs-pager-info">{{ t('logsPage.pager.range', { start: pageStart, end: pageEnd, total: totalLogs.toLocaleString() }) }}</span>
        <button type="button" class="cs-btn logs-page-prev" :disabled="page <= 1 || loading" @click="prevPage">
          {{ t('logsPage.pager.prev') }}
        </button>
        <span class="cs-pill cs-pill--brand logs-page-current">{{ t('logsPage.pager.current', { page, total: totalPages }) }}</span>
        <button type="button" class="cs-btn logs-page-next" :disabled="page >= totalPages || loading" @click="nextPage">
          {{ t('logsPage.pager.next') }}
        </button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.logs-request-view {
  margin-top: 23px;
}

.logs-request-panel {
  display: flex;
  flex-direction: column;
  height: 649px;
  overflow: hidden;
  border-radius: 13px;
}

.logs-request-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  flex-shrink: 0;
  gap: 12px;
  height: 55px;
  margin: 16px 17px 0;
}

.logs-request-title {
  margin: 1px 0 0;
  color: var(--cs-text);
  font-size: 16px;
  line-height: 20px;
}

.logs-request-head p {
  margin: 5px 0 0;
  color: var(--cs-text-secondary);
  font-size: 11px;
}

.logs-request-actions {
  display: flex;
  align-items: center;
  gap: 18px;
  padding-top: 1px;
}

.logs-export {
  width: 116px;
  height: 34px;
}

.logs-request-filters {
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  gap: 6px;
  height: 90px;
  margin: 0 17px;
  padding: 9px;
  border-radius: 10px;
  background: var(--cs-field);
}

.logs-filter-primary {
  display: grid;
  grid-template-columns: 192px 196px 162px 178px 171px minmax(0, 1fr) 113px;
  gap: 10px;
  height: 36px;
}

.logs-filter-primary .cs-select {
  min-width: 0;
  min-height: 36px;
  height: 36px;
  background-color: var(--cs-surface);
}

.logs-refresh {
  width: 113px;
  height: 34px;
  align-self: center;
}

.logs-cost-filter {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 30px;
  min-width: 0;
}

.logs-cost-label {
  margin: 0 33px 0 4px;
  color: var(--cs-text-secondary);
  font-size: 11px;
  font-weight: 500;
  white-space: nowrap;
}

.logs-cost-option {
  width: 62px;
  height: 30px;
  padding: 0;
  border: 1px solid var(--cs-border);
  border-radius: 8px;
  background: var(--cs-surface);
  color: var(--cs-text);
  font: 500 11px var(--cs-font);
  cursor: pointer;
}

.logs-cost-option--all {
  width: 75px;
}

.logs-cost-option.active {
  border-color: var(--cs-brand-tint-border);
  background: var(--cs-brand-tint);
  color: var(--cs-brand);
}

.logs-cost-hint {
  margin-left: 38px;
  color: var(--cs-text-secondary);
  font-size: 10px;
  white-space: nowrap;
}

.logs-filter-summary {
  display: flex;
  align-items: center;
  flex-shrink: 0;
  gap: 15px;
  height: 56px;
  margin: 10px 17px 0;
  padding: 0 13px;
  overflow: hidden;
  border: 1px solid var(--cs-border);
  border-radius: 10px;
  background: var(--cs-field);
  transition: opacity 0.15s ease;
}

.logs-filter-summary--loading {
  opacity: 0.65;
}

.logs-summary-scope {
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  gap: 3px;
  width: 150px;
}

.logs-summary-scope h3,
.logs-summary-scope p,
.logs-summary-metric p {
  margin: 0;
}

.logs-summary-scope h3 {
  color: var(--cs-text);
  font-size: 12px;
  line-height: 16px;
}

.logs-summary-scope p {
  color: var(--cs-text-secondary);
  font-size: 10px;
  line-height: 14px;
  white-space: nowrap;
}

.logs-summary-divider {
  flex-shrink: 0;
  width: 1px;
  height: 29px;
  background: var(--cs-border);
}

.logs-summary-metric {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.logs-summary-label {
  overflow: hidden;
  color: var(--cs-text-secondary);
  font-size: 10px;
  font-weight: 500;
  line-height: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.logs-summary-value {
  overflow: hidden;
  color: var(--cs-text);
  font-size: 15px;
  font-weight: 400;
  line-height: 19px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.logs-summary-value--brand {
  color: var(--cs-brand);
}

.logs-table-scroll {
  flex: 1;
  min-height: 0;
  margin: 8px 17px 0;
  overflow: auto;
  transition: opacity 0.15s ease;
}

.logs-table-scroll--loading {
  opacity: 0.6;
}

.logs-rows {
  min-width: 1168px;
}

.logs-request-row {
  display: grid;
  grid-template-columns: 140px 94px 75px 93px 263px 82px 77px 76px 61px minmax(80px, 1fr);
  column-gap: 12px;
  align-items: center;
  min-width: 1168px;
  padding: 0 7px 0 12px;
  color: var(--cs-text);
  font-size: 10px;
}

.logs-request-row--head {
  position: sticky;
  top: 0;
  z-index: 3;
  height: 42px;
  margin-bottom: 10px;
  border-radius: 7px;
  background: var(--cs-field);
  color: var(--cs-text-secondary);
  font-size: 11px;
  font-weight: 500;
}

.logs-request-row--data {
  height: 44px;
  margin-bottom: 5px;
  border-radius: 8px;
}

.logs-request-row--alt {
  background: var(--cs-field);
}

.logs-cell-time {
  white-space: nowrap;
}

.logs-cell-ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.logs-token-heading {
  display: flex;
  align-items: center;
  gap: 17px;
  padding-left: 12px;
}

.logs-token-info {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 25px;
  height: 25px;
  border: 1px solid var(--cs-border);
  border-radius: 50%;
  background: var(--cs-surface);
  color: var(--cs-brand);
  font: 500 11px var(--cs-font-mono);
  outline: none;
}

.logs-token-info:focus-visible {
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--cs-brand) 15%, transparent);
}

.logs-token-cell {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  grid-template-rows: 18px 18px;
  column-gap: 8px;
  align-items: center;
  height: 38px;
}

.logs-token-pair {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  white-space: nowrap;
}

.logs-token-label {
  color: var(--cs-text-secondary);
  font-weight: 500;
}

.logs-token-cache-label {
  color: #0d9cb9;
  font-size: 9px;
  font-weight: 500;
}

.logs-token-pair--cache {
  color: var(--cs-text-secondary);
  font-size: 9px;
}

.logs-token-value {
  overflow: hidden;
  text-overflow: ellipsis;
  font-variant-numeric: tabular-nums;
}

.logs-cell-cost {
  color: var(--cs-brand);
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}

.logs-cell-muted {
  color: var(--cs-text-secondary);
}

.logs-status-pill {
  width: 51px;
  height: 22px;
  padding: 0;
  border-radius: 6px;
}

.logs-trace-button {
  width: 100%;
  max-width: 106px;
  height: 29px;
  padding: 0;
  border-radius: 7px;
  color: var(--cs-brand);
  font-size: 10px;
}

.logs-request-empty {
  min-height: 180px;
}

.logs-request-pager {
  display: flex;
  align-items: center;
  flex-shrink: 0;
  gap: 10px;
  height: 46px;
  margin: 0 17px 16px;
  border-top: 1px solid var(--cs-border);
}

.logs-pager-info {
  margin-right: auto;
  color: var(--cs-text-secondary);
  font-size: 11px;
}

.logs-page-prev {
  width: 76px;
  height: 34px;
}

.logs-page-next {
  width: 106px;
  height: 34px;
}

.logs-page-current {
  height: 22px;
  border-radius: 6px;
}

@media (max-width: 1300px) {
  .logs-filter-primary {
    grid-template-columns: repeat(5, minmax(100px, 1fr)) 113px;
  }

  .logs-filter-spacer {
    display: none;
  }

  .logs-cost-hint {
    display: none;
  }

  .logs-filter-summary {
    gap: 10px;
  }

  .logs-summary-scope {
    width: 130px;
  }

  .logs-summary-value {
    font-size: 13px;
  }
}

@media (max-width: 980px) {
  .logs-request-panel {
    height: 758px;
  }

  .logs-request-filters {
    height: 132px;
  }

  .logs-filter-primary {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    height: 78px;
    row-gap: 6px;
  }

  .logs-refresh {
    width: 100%;
  }

  .logs-cost-filter {
    margin-top: 6px;
  }

  .logs-request-actions .cs-pill {
    display: none;
  }
}
</style>
