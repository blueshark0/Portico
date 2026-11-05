<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ChevronRight } from 'lucide-vue-next'
import { providerTabs } from '../../constants/platforms'
import { formatBeijingDateTime } from '../../utils/dateTime'
import { formatUsdPrecise } from '../../utils/currency'
import { useSessionTrace } from './useSessionTrace'

const { t } = useI18n()
const router = useRouter()
const {
  fullMetrics,
  latestRequest,
  load,
  loadFailed,
  loading,
  logs,
  oldestRequest,
  platform,
  range,
  referenceDay,
  scopeMetrics,
  scopedLogs,
  selectedRequest,
  selectedRequestId,
  sessionId,
  sourceRequest,
  status,
  visibleLogs,
} = useSessionTrace()

const platformLabel = computed(() => providerTabs.find((tab) => tab.id === platform.value)?.label ?? platform.value)
const fullTime = (value?: string) => value ? formatBeijingDateTime(value).replaceAll('-', '/') : '—'
const timelineTime = (value: string) => formatBeijingDateTime(value).slice(5).replace('-', '/')
const shortDay = (day: string) => day ? day.slice(5).replace('-', '/') : '—'
const successRate = computed(() => scopeMetrics.value.requests > 0 ? scopeMetrics.value.successful / scopeMetrics.value.requests * 100 : null)
const failureRate = computed(() => scopeMetrics.value.requests > 0 ? scopeMetrics.value.failed / scopeMetrics.value.requests * 100 : null)
const cacheShare = computed(() => scopeMetrics.value.requests > 0 ? scopeMetrics.value.cacheHits / scopeMetrics.value.requests * 100 : null)
const percent = (value: number | null) => value === null ? '—' : `${value.toFixed(1)}%`
const duration = (value: number) => value > 0 ? `${value >= 10 ? value.toFixed(1) : value.toFixed(2)} s` : '—'

const metrics = computed(() => [
  {
    key: 'requests',
    label: t(range.value === 'day' ? 'sessionTrace.metrics.dayRequests' : 'sessionTrace.metrics.totalRequests'),
    value: scopeMetrics.value.requests.toLocaleString(),
    hint: t('sessionTrace.metrics.requestHint', { success: scopeMetrics.value.successful, failed: scopeMetrics.value.failed }),
    tone: '',
  },
  {
    key: 'success',
    label: t('sessionTrace.metrics.successRate'),
    value: percent(successRate.value),
    hint: t('sessionTrace.metrics.ratio', { part: scopeMetrics.value.successful, total: scopeMetrics.value.requests }),
    tone: 'success',
  },
  {
    key: 'failed',
    label: t('sessionTrace.metrics.failureRate'),
    value: percent(failureRate.value),
    hint: t('sessionTrace.metrics.ratio', { part: scopeMetrics.value.failed, total: scopeMetrics.value.requests }),
    tone: 'danger',
  },
  {
    key: 'cache',
    label: t('sessionTrace.metrics.cacheShare'),
    value: percent(cacheShare.value),
    hint: t('sessionTrace.metrics.cacheHint', { hits: scopeMetrics.value.cacheHits, total: scopeMetrics.value.requests }),
    tone: 'reasoning',
  },
])

const coverageStart = computed(() => shortDay(oldestRequest.value ? formatBeijingDateTime(oldestRequest.value.created_at).slice(0, 10) : ''))
const coverageEnd = computed(() => shortDay(latestRequest.value ? formatBeijingDateTime(latestRequest.value.created_at).slice(0, 10) : ''))
const rangeSummary = computed(() => {
  if (range.value === 'day') {
    return t('sessionTrace.range.daySummary', { date: shortDay(referenceDay.value), count: scopeMetrics.value.requests })
  }
  return t('sessionTrace.range.fullSummary', {
    count: fullMetrics.value.requests,
    start: coverageStart.value,
    end: coverageEnd.value,
  })
})
const timelineSubtitle = computed(() => range.value === 'day'
  ? t('sessionTrace.timeline.daySubtitle', { date: shortDay(referenceDay.value) })
  : t('sessionTrace.timeline.fullSubtitle', { start: coverageStart.value, end: coverageEnd.value }))
const usageItems = computed(() => [
  { key: 'input', label: t('sessionTrace.usage.input'), value: scopeMetrics.value.inputTokens },
  { key: 'output', label: t('sessionTrace.usage.output'), value: scopeMetrics.value.outputTokens },
  { key: 'cacheRead', label: t('sessionTrace.usage.cacheRead'), value: scopeMetrics.value.cacheReadTokens },
  { key: 'cacheCreate', label: t('sessionTrace.usage.cacheCreate'), value: scopeMetrics.value.cacheCreateTokens },
])
const selectedIsSuccess = computed(() => !!selectedRequest.value && selectedRequest.value.http_code >= 200 && selectedRequest.value.http_code < 300)
const statusOptions = computed(() => [
  { value: 'all', label: t('sessionTrace.filters.all') },
  { value: 'success', label: t('sessionTrace.filters.success') },
  { value: 'failed', label: t('sessionTrace.filters.failed') },
])

const backToRequests = () => void router.push({ path: '/logs', query: { view: 'requests' } })
</script>

<template>
  <div class="cs-page trace-page">
    <div class="cs-page-head">
      <div>
        <h1 class="cs-page-title">{{ t('sessionTrace.title') }}</h1>
        <p class="cs-page-sub">{{ t('sessionTrace.subtitle') }}</p>
      </div>
      <div class="cs-page-actions">
        <button type="button" class="cs-btn trace-back" @click="backToRequests">{{ t('sessionTrace.back') }}</button>
      </div>
    </div>

    <div v-if="logs.length === 0" class="cs-card trace-state">
      <p>{{ loading ? t('sessionTrace.loading') : loadFailed ? t('sessionTrace.loadFailed') : t('sessionTrace.empty') }}</p>
      <button v-if="loadFailed" type="button" class="cs-btn" @click="load">{{ t('sessionTrace.retry') }}</button>
    </div>

    <template v-else>
      <section class="cs-card trace-identity" :aria-label="t('sessionTrace.title')">
        <div class="trace-identity-main">
          <div class="trace-identity-title">
            <span class="cs-pill cs-pill--success trace-recorded">{{ t('sessionTrace.recorded') }}</span>
            <h2 :title="sessionId">{{ platformLabel }} · {{ t('sessionTrace.sessionLabel') }} {{ sessionId }}</h2>
          </div>
          <p class="trace-identity-meta">
            {{ t('sessionTrace.identity', {
              start: fullTime(oldestRequest?.created_at),
              last: fullTime(latestRequest?.created_at),
              provider: latestRequest?.provider || '—',
              model: latestRequest?.model || '—',
            }) }}
          </p>
        </div>
        <div class="trace-full-summary">
          <div class="trace-full-line">
            <div>
              <p>{{ t('sessionTrace.fullLifecycle') }}</p>
              <strong>{{ t('sessionTrace.requestCount', { count: fullMetrics.requests }) }}</strong>
            </div>
            <span>{{ t('sessionTrace.fullOutcome', { success: fullMetrics.successful, failed: fullMetrics.failed }) }}</span>
          </div>
          <span v-if="sourceRequest" class="cs-pill cs-pill--brand trace-source-entry">
            {{ t('sessionTrace.enteredFrom', { time: fullTime(sourceRequest.created_at).slice(5) }) }}
          </span>
        </div>
      </section>

      <div class="cs-card trace-range-bar">
        <span class="trace-range-label">{{ t('sessionTrace.range.label') }}</span>
        <button type="button" class="trace-range-tab trace-range-tab--day" :class="{ active: range === 'day' }" :aria-pressed="range === 'day'" @click="range = 'day'; status = 'all'">
          {{ t('sessionTrace.range.day', { date: shortDay(referenceDay) }) }}
        </button>
        <button type="button" class="trace-range-tab trace-range-tab--full" :class="{ active: range === 'full' }" :aria-pressed="range === 'full'" @click="range = 'full'; status = 'all'">
          {{ t('sessionTrace.range.full') }}
        </button>
        <span class="trace-range-summary">{{ rangeSummary }}</span>
      </div>

      <div class="trace-metrics">
        <article v-for="metric in metrics" :key="metric.key" class="cs-card trace-metric">
          <p class="trace-metric-label">{{ metric.label }}</p>
          <p class="trace-metric-value" :class="`trace-metric-value--${metric.tone}`">{{ metric.value }}</p>
          <p class="trace-metric-hint">{{ metric.hint }}</p>
        </article>
      </div>

      <div class="trace-content">
        <section class="cs-card trace-timeline" aria-labelledby="trace-timeline-title">
          <div class="trace-timeline-head">
            <div>
              <h2 id="trace-timeline-title">{{ t('sessionTrace.timeline.title') }}</h2>
              <p>{{ timelineSubtitle }}</p>
            </div>
            <select v-model="status" class="cs-select trace-status-filter" :aria-label="t('sessionTrace.filters.label')">
              <option v-for="option in statusOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
            </select>
          </div>
          <div class="trace-table-head trace-timeline-row">
            <span>{{ t('sessionTrace.columns.time') }}</span>
            <span>{{ t('sessionTrace.columns.model') }}</span>
            <span>{{ t('sessionTrace.columns.status') }}</span>
            <span>{{ t('sessionTrace.columns.ttft') }}</span>
            <span>{{ t('sessionTrace.columns.duration') }}</span>
            <span>{{ t('sessionTrace.columns.cost') }}</span>
            <span />
          </div>
          <div class="trace-request-list">
            <button
              v-for="log in visibleLogs"
              :key="log.id"
              type="button"
              class="trace-timeline-row trace-request-row"
              :class="{ active: selectedRequestId === log.id }"
              @click="selectedRequestId = log.id"
            >
              <span class="trace-time cs-mono" :class="{ 'trace-time--failure': log.http_code >= 400 }">
                <span class="trace-timeline-dot" />
                {{ timelineTime(log.created_at) }}
              </span>
              <span class="trace-model" :title="log.model">{{ log.model || '—' }}</span>
              <span><span class="cs-pill trace-status" :class="log.http_code >= 200 && log.http_code < 300 ? 'cs-pill--success' : 'cs-pill--danger'">{{ log.http_code }}</span></span>
              <span class="cs-mono trace-ttft">{{ duration(log.ttft_sec) }}</span>
              <span class="cs-mono">{{ duration(log.duration_sec) }}</span>
              <span class="cs-mono trace-request-cost">{{ log.has_pricing ? formatUsdPrecise(log.total_cost) : '—' }}</span>
              <ChevronRight v-if="selectedRequestId === log.id" :size="17" :stroke-width="2" class="trace-row-chevron" />
              <span v-else />
            </button>
            <div v-if="visibleLogs.length === 0" class="cs-empty">{{ t('sessionTrace.timeline.empty') }}</div>
          </div>
          <p class="trace-timeline-footer">{{ t('sessionTrace.timeline.footer', { filtered: visibleLogs.length, total: scopedLogs.length }) }}</p>
        </section>

        <aside class="trace-side">
          <section class="cs-card trace-usage" aria-labelledby="trace-usage-title">
            <h2 id="trace-usage-title">{{ t('sessionTrace.usage.title') }}</h2>
            <p class="trace-usage-scope">{{ t(range === 'day' ? 'sessionTrace.usage.day' : 'sessionTrace.usage.full') }}</p>
            <div class="trace-usage-grid">
              <div v-for="item in usageItems" :key="item.key">
                <p>{{ item.label }}</p>
                <strong class="cs-mono">{{ item.value.toLocaleString() }}</strong>
              </div>
            </div>
            <div class="trace-total-cost">
              <span>{{ t('sessionTrace.usage.totalCost') }}</span>
              <strong>{{ formatUsdPrecise(scopeMetrics.totalCost) }}</strong>
            </div>
          </section>

          <section v-if="selectedRequest" class="cs-card trace-source" aria-labelledby="trace-source-title">
            <div class="trace-source-head">
              <h2 id="trace-source-title">{{ t('sessionTrace.source.title') }}</h2>
              <span class="cs-pill" :class="selectedIsSuccess ? 'cs-pill--success' : 'cs-pill--danger'">
                {{ t(selectedIsSuccess ? 'sessionTrace.source.success' : 'sessionTrace.source.failure', { code: selectedRequest.http_code }) }}
              </span>
            </div>
            <p class="trace-source-id cs-mono">{{ t('sessionTrace.source.id', { id: selectedRequest.id }) }}</p>
            <div class="trace-source-metrics">
              <div><p>{{ t('sessionTrace.source.cost') }}</p><strong>{{ selectedRequest.has_pricing ? formatUsdPrecise(selectedRequest.total_cost) : '—' }}</strong></div>
              <div><p>{{ t('sessionTrace.source.ttft') }}</p><strong class="trace-source-ttft">{{ duration(selectedRequest.ttft_sec) }}</strong></div>
              <div><p>{{ t('sessionTrace.source.duration') }}</p><strong>{{ duration(selectedRequest.duration_sec) }}</strong></div>
            </div>
            <p class="trace-source-note">{{ t('sessionTrace.source.note') }}</p>
          </section>
        </aside>
      </div>
      <p class="trace-note">{{ t('sessionTrace.note') }}</p>
    </template>
  </div>
</template>

<style scoped>
.trace-page {
  gap: 0;
  padding-right: 19px;
  padding-bottom: 20px;
}

.trace-page .cs-page-head {
  margin-bottom: 17px;
}

.trace-page .cs-page-sub {
  margin-top: 4px;
  font-size: 12px;
  line-height: 17px;
}

.trace-page .cs-page-actions {
  padding-top: 4px;
}

.trace-back {
  width: 154px;
  height: 34px;
}

.trace-state {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 14px;
  min-height: 280px;
  color: var(--cs-text-secondary);
  font-size: 12px;
}

.trace-state p {
  margin: 0;
}

.trace-identity {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  height: 104px;
  padding: 13px 15px;
  border-radius: 12px;
}

.trace-identity-main {
  min-width: 0;
}

.trace-identity-title {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 23px;
}

.trace-recorded {
  width: 68px;
  height: 23px;
  flex-shrink: 0;
  padding: 0;
  border-radius: 6px;
}

.trace-identity-title h2 {
  overflow: hidden;
  margin: 0;
  color: var(--cs-text);
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trace-identity-meta {
  overflow: hidden;
  margin: 13px 0 0 2px;
  color: var(--cs-text-secondary);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trace-full-summary {
  width: 450px;
  flex-shrink: 0;
}

.trace-full-line {
  display: flex;
  align-items: center;
  gap: 24px;
}

.trace-full-line p {
  margin: 0 0 2px;
  color: var(--cs-text-secondary);
  font-size: 10px;
}

.trace-full-line strong {
  color: var(--cs-text);
  font-size: 20px;
}

.trace-full-line > span {
  margin-top: 19px;
  color: var(--cs-text-secondary);
  font-size: 11px;
  white-space: nowrap;
}

.trace-source-entry {
  width: 250px;
  height: 23px;
  margin-top: 11px;
  border-radius: 6px;
}

.trace-range-bar {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 42px;
  margin-top: 10px;
  padding: 4px 13px;
  border-radius: 10px;
}

.trace-range-label {
  margin-right: 33px;
  color: var(--cs-text-secondary);
  font-size: 11px;
  font-weight: 500;
}

.trace-range-tab {
  height: 32px;
  padding: 0 12px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: var(--cs-surface);
  color: var(--cs-text-secondary);
  font: 500 11px var(--cs-font);
  cursor: pointer;
  white-space: nowrap;
}

.trace-range-tab--day {
  width: 190px;
}

.trace-range-tab--full {
  width: 140px;
}

.trace-range-tab.active {
  border-color: var(--cs-brand-tint-border);
  background: var(--cs-brand-tint);
  color: var(--cs-brand);
}

.trace-range-summary {
  overflow: hidden;
  margin-left: auto;
  color: var(--cs-text-secondary);
  font-size: 11px;
  text-align: right;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trace-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 20px;
  height: 93px;
  margin-top: 13px;
}

.trace-metric {
  position: relative;
  height: 93px;
  padding: 10px 13px;
  border-radius: 12px;
}

.trace-metric p {
  margin: 0;
}

.trace-metric-label {
  color: var(--cs-text-secondary);
  font-size: 11px;
}

.trace-metric-value {
  margin-top: 4px !important;
  color: var(--cs-text);
  font-size: 24px;
  font-weight: 700;
  line-height: 32px;
}

.trace-metric-value--success {
  color: var(--cs-success);
}

.trace-metric-value--danger {
  color: var(--cs-danger);
}

.trace-metric-value--reasoning {
  color: var(--cs-metric-reasoning);
}

.trace-metric-hint {
  position: absolute;
  right: 14px;
  bottom: 12px;
  color: var(--cs-text-secondary);
  font-size: 10px;
}

.trace-content {
  display: grid;
  grid-template-columns: minmax(0, 810px) minmax(320px, 400px);
  gap: 20px;
  height: 398px;
  margin-top: 15px;
}

.trace-timeline {
  display: flex;
  flex-direction: column;
  height: 398px;
  overflow: hidden;
  border-radius: 12px;
}

.trace-timeline-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  flex-shrink: 0;
  gap: 12px;
  height: 44px;
  margin: 14px 17px 0;
}

.trace-timeline-head h2 {
  margin: 0;
  color: var(--cs-text);
  font-size: 15px;
  line-height: 18px;
}

.trace-timeline-head p {
  margin: 6px 0 0;
  color: var(--cs-text-secondary);
  font-size: 10px;
}

.trace-status-filter {
  width: 189px;
  min-height: 28px;
  height: 28px;
  padding-top: 4px;
  padding-bottom: 4px;
  font-size: 10px;
}

.trace-timeline-row {
  display: grid;
  grid-template-columns: 136px 105px 82px 96px 118px 126px 18px;
  column-gap: 12px;
  align-items: center;
  min-width: 781px;
  padding: 0 8px 0 20px;
  font-size: 10px;
}

.trace-table-head {
  flex-shrink: 0;
  height: 46px;
  margin: 0 17px;
  border-bottom: 1px solid var(--cs-border);
  color: var(--cs-text-secondary);
  font-weight: 500;
}

.trace-table-head span:first-child {
  padding-left: 9px;
}

.trace-request-list {
  flex: 1;
  min-height: 0;
  margin: 9px 13px 0;
  overflow: auto;
}

.trace-request-row {
  width: 100%;
  height: 34px;
  margin-bottom: 5px;
  border: 1px solid transparent;
  border-radius: 7px;
  background: var(--cs-surface);
  color: var(--cs-text);
  font-family: var(--cs-font);
  text-align: left;
  cursor: pointer;
}

.trace-request-row:nth-child(odd):not(.active) {
  background: var(--cs-field);
}

.trace-request-row.active {
  border-color: var(--cs-brand-tint-border);
  background: var(--cs-brand-tint);
}

.trace-time {
  position: relative;
  white-space: nowrap;
}

.trace-timeline-dot {
  position: absolute;
  left: -12px;
  top: 50%;
  width: 8px;
  height: 8px;
  margin-top: -4px;
  border-radius: 4px;
  background: var(--cs-success);
}

.trace-request-row.active .trace-timeline-dot {
  background: var(--cs-brand);
}

.trace-time--failure .trace-timeline-dot {
  background: var(--cs-danger);
}

.trace-request-row.active .trace-time,
.trace-request-row.active .trace-request-cost {
  color: var(--cs-brand);
  font-weight: 500;
}

.trace-model {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trace-status {
  width: 52px;
  height: 23px;
  padding: 0;
  border-radius: 6px;
}

.trace-ttft {
  color: var(--cs-text-secondary);
}

.trace-request-cost {
  font-variant-numeric: tabular-nums;
}

.trace-row-chevron {
  color: var(--cs-brand);
}

.trace-timeline-footer {
  display: flex;
  align-items: center;
  flex-shrink: 0;
  height: 43px;
  margin: 0 17px;
  color: var(--cs-text-secondary);
  font-size: 10px;
}

.trace-side {
  display: flex;
  flex-direction: column;
  gap: 14px;
  height: 398px;
}

.trace-usage {
  height: 235px;
  flex-shrink: 0;
  padding: 12px 15px;
  border-radius: 12px;
}

.trace-usage h2 {
  margin: 0;
  color: var(--cs-text);
  font-size: 14px;
}

.trace-usage-scope {
  margin: 6px 0 0;
  color: var(--cs-text-secondary);
  font-size: 10px;
}

.trace-usage-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 20px;
  margin-top: 14px;
}

.trace-usage-grid p {
  margin: 0 0 4px;
  color: var(--cs-text-secondary);
  font-size: 10px;
}

.trace-usage-grid strong {
  color: var(--cs-text);
  font-size: 14px;
  font-weight: 500;
}

.trace-total-cost {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 48px;
  margin-top: 12px;
  border-top: 1px solid var(--cs-border);
  color: var(--cs-text-secondary);
  font-size: 11px;
}

.trace-total-cost strong {
  color: var(--cs-text);
  font-size: 20px;
}

.trace-source {
  height: 149px;
  flex-shrink: 0;
  padding: 11px 14px;
  border-radius: 12px;
}

.trace-source-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.trace-source-head h2 {
  margin: 0;
  color: var(--cs-text);
  font-size: 13px;
}

.trace-source-head .cs-pill {
  height: 23px;
  border-radius: 6px;
}

.trace-source-id {
  margin: 7px 0 0;
  color: var(--cs-text-secondary);
  font-size: 11px;
}

.trace-source-metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-top: 13px;
}

.trace-source-metrics p {
  margin: 0 0 6px;
  color: var(--cs-text-secondary);
  font-size: 10px;
}

.trace-source-metrics strong {
  color: var(--cs-text);
  font-size: 13px;
  font-weight: 500;
}

.trace-source-metrics .trace-source-ttft {
  color: var(--cs-brand);
}

.trace-source-note {
  margin: 10px 0 0;
  color: var(--cs-text-secondary);
  font-size: 9px;
}

.trace-note {
  margin: 20px 0 0;
  color: var(--cs-text-secondary);
  font-size: 10px;
  line-height: 14px;
}

@media (max-width: 1100px) {
  .trace-full-summary {
    width: 330px;
  }

  .trace-identity-meta {
    white-space: normal;
  }

  .trace-content {
    grid-template-columns: minmax(0, 1fr);
    height: auto;
  }

  .trace-side {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    height: auto;
  }

  .trace-usage,
  .trace-source {
    height: 235px;
  }
}

@media (max-width: 800px) {
  .trace-full-summary,
  .trace-range-summary {
    display: none;
  }

  .trace-metrics {
    height: auto;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .trace-side {
    grid-template-columns: minmax(0, 1fr);
  }

  .trace-source {
    height: 170px;
  }
}
</style>
