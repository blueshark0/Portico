<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import UsageTrendChart from './UsageTrendChart.vue'
import {
  LOG_RANGE_OPTIONS,
  fetchLogStats,
  type LogRangeKey,
  type LogStats,
} from '../../services/logs'
import { formatUsd } from '../../utils/currency'
import { showToast } from '../../utils/toast'
import { formatOverviewTokenCount } from './usageTrend'

const { t } = useI18n()
const rangeKey = ref<LogRangeKey>('today')
const stats = ref<LogStats | null>(null)
const trendStats = ref<LogStats | null>(null)
const loading = ref(false)
let loadSequence = 0

const rangeOptions = computed(() => LOG_RANGE_OPTIONS.map((value) => ({
  value,
  label: t(`logsPage.filters.ranges.${value}`),
})))
const rangeLabel = computed(() => t(`logsPage.filters.ranges.${rangeKey.value}`))
const todayLabel = computed(() => {
  const now = new Date()
  return `${String(now.getMonth() + 1).padStart(2, '0')}/${String(now.getDate()).padStart(2, '0')}`
})

const load = async () => {
  const sequence = ++loadSequence
  loading.value = true
  try {
    const [current, trend] = await Promise.all([
      fetchLogStats({ rangeKey: rangeKey.value, costTier: 'all', httpStatus: 'all' }),
      fetchLogStats({ rangeKey: 'last7days', costTier: 'all', httpStatus: 'all' }),
    ])
    if (sequence !== loadSequence) return
    stats.value = current ?? null
    trendStats.value = trend ?? null
  } catch (error) {
    console.error('failed to load usage overview', error)
    if (sequence === loadSequence) showToast(t('logsPage.overview.loadFailed'), 'error')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

onMounted(() => void load())
watch(rangeKey, () => void load())

const successRate = computed(() => {
  const total = stats.value?.total_requests ?? 0
  if (total <= 0) return null
  return ((total - (stats.value?.failed_requests ?? 0)) / total) * 100
})
const metrics = computed(() => [
  {
    key: 'cost',
    label: t('logsPage.overview.metrics.cost', { range: rangeLabel.value }),
    value: stats.value ? formatUsd(stats.value.cost_total) : '—',
    hint: t('logsPage.overview.metrics.costHint'),
    tone: 'brand',
  },
  {
    key: 'requests',
    label: t('logsPage.overview.metrics.requests'),
    value: stats.value ? stats.value.total_requests.toLocaleString() : '—',
    hint: t('logsPage.overview.metrics.requestsHint', { range: rangeLabel.value }),
    tone: '',
  },
  {
    key: 'success',
    label: t('logsPage.overview.metrics.successRate'),
    value: successRate.value === null ? '—' : `${successRate.value.toFixed(1)}%`,
    hint: t('logsPage.overview.metrics.successHint', { count: (stats.value?.failed_requests ?? 0).toLocaleString() }),
    tone: 'success',
  },
  {
    key: 'duration',
    label: t('logsPage.overview.metrics.duration'),
    value: stats.value && stats.value.total_requests > 0 ? `${stats.value.avg_duration_sec.toFixed(2)} s` : '—',
    hint: t('logsPage.overview.metrics.durationHint'),
    tone: '',
  },
])

const tokenItems = computed(() => {
  const data = stats.value
  return [
    { key: 'total', label: t('logsPage.overview.tokens.total'), value: (data?.input_tokens ?? 0) + (data?.output_tokens ?? 0), color: 'var(--cs-text)' },
    { key: 'input', label: t('logsPage.overview.tokens.input'), value: data?.input_tokens ?? 0, color: 'var(--cs-metric-input)' },
    { key: 'output', label: t('logsPage.overview.tokens.output'), value: data?.output_tokens ?? 0, color: 'var(--cs-metric-output)' },
    { key: 'reasoning', label: t('logsPage.overview.tokens.reasoning'), value: data?.reasoning_tokens ?? 0, color: 'var(--cs-metric-reasoning)' },
    { key: 'cacheRead', label: t('logsPage.overview.tokens.cacheRead'), value: data?.cache_read_tokens ?? 0, color: 'var(--cs-metric-cache-read)' },
    { key: 'cacheCreate', label: t('logsPage.overview.tokens.cacheCreate'), value: data?.cache_create_tokens ?? 0, color: 'var(--cs-metric-cache-create)' },
  ]
})
</script>

<template>
  <div class="logs-overview">
    <div class="overview-key-head">
      <h2>{{ t('logsPage.overview.metricsTitle', { range: rangeLabel }) }}</h2>
      <select v-model="rangeKey" class="cs-select overview-range" :aria-label="t('logsPage.overview.rangeLabel')">
        <option v-for="option in rangeOptions" :key="option.value" :value="option.value">
          {{ option.value === 'today' ? t('logsPage.overview.todayOption', { date: todayLabel }) : option.label }}
        </option>
      </select>
    </div>

    <div class="overview-metrics" :class="{ 'overview-metrics--loading': loading }">
      <article v-for="metric in metrics" :key="metric.key" class="cs-card overview-metric">
        <p class="overview-metric-label">{{ metric.label }}</p>
        <p class="overview-metric-value" :class="`overview-metric-value--${metric.tone}`">{{ metric.value }}</p>
        <p class="overview-metric-hint">{{ metric.hint }}</p>
      </article>
    </div>

    <section class="cs-card overview-token-card" aria-labelledby="overview-token-title">
      <div class="overview-token-head">
        <div>
          <h2 id="overview-token-title" class="cs-card-title">{{ t('logsPage.overview.tokens.title') }}</h2>
          <p>{{ t('logsPage.overview.tokens.subtitle') }}</p>
        </div>
        <span class="cs-pill cs-pill--brand overview-cache-pill">
          {{ t('logsPage.overview.tokens.cacheHits', { count: (stats?.cache_hit_requests ?? 0).toLocaleString() }) }}
        </span>
      </div>
      <div class="overview-token-grid">
        <div v-for="(item, index) in tokenItems" :key="item.key" class="overview-token-item" :class="{ 'overview-token-item--divided': index > 0 }">
          <p class="overview-token-label">
            <span class="overview-token-dot" :style="{ backgroundColor: item.color }" />
            {{ item.label }}
          </p>
          <p class="overview-token-value">{{ stats ? formatOverviewTokenCount(item.value) : '—' }}</p>
        </div>
      </div>
    </section>

    <UsageTrendChart :series="trendStats?.series ?? []" :loading="loading" />
    <p class="overview-note">{{ t('logsPage.overview.note') }}</p>
  </div>
</template>

<style scoped>
.logs-overview {
  margin-top: 10px;
}

.overview-key-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 36px;
}

.overview-key-head h2 {
  margin: 0;
  color: var(--cs-text);
  font-size: 13px;
}

.overview-range {
  width: 182px;
  min-height: 36px;
  background-color: var(--cs-surface);
}

.overview-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 20px;
  height: 119px;
  margin-top: 16px;
  transition: opacity 0.15s ease;
}

.overview-metrics--loading {
  opacity: 0.65;
}

.overview-metric {
  height: 119px;
  padding: 15px 17px;
  border-radius: 13px;
}

.overview-metric p {
  margin: 0;
}

.overview-metric-label {
  color: var(--cs-text-secondary);
  font-size: 12px;
  font-weight: 500;
}

.overview-metric-value {
  margin-top: 8px !important;
  color: var(--cs-text);
  font-size: 30px;
  font-weight: 700;
  line-height: 38px;
}

.overview-metric-value--brand {
  color: var(--cs-brand);
}

.overview-metric-value--success {
  color: var(--cs-success);
}

.overview-metric-hint {
  margin-top: 4px !important;
  color: var(--cs-text-secondary);
  font-size: 11px;
}

.overview-token-card {
  height: 165px;
  margin-top: 30px;
  padding: 14px 18px;
  border-radius: 13px;
}

.overview-token-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.overview-token-head p {
  margin: 5px 0 0;
  color: var(--cs-text-secondary);
  font-size: 11px;
}

.overview-cache-pill {
  height: 22px;
  border-radius: 6px;
}

.overview-token-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  margin-top: 19px;
}

.overview-token-item {
  min-width: 0;
  padding: 0 12px;
}

.overview-token-item:first-child {
  padding-left: 0;
}

.overview-token-item--divided {
  border-left: 1px solid var(--cs-border);
}

.overview-token-label {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  color: var(--cs-text-secondary);
  font-size: 11px;
  font-weight: 500;
  white-space: nowrap;
}

.overview-token-dot {
  width: 7px;
  height: 7px;
  border-radius: 3px;
  flex-shrink: 0;
}

.overview-token-value {
  margin: 10px 0 0;
  color: var(--cs-text);
  font-size: 23px;
  font-weight: 700;
  line-height: 28px;
}

.logs-overview > .usage-trend {
  margin-top: 26px;
}

.overview-note {
  margin: 18px 0 0;
  color: var(--cs-text-secondary);
  font-size: 10px;
  line-height: 14px;
}

@media (max-width: 1100px) {
  .overview-metrics {
    height: auto;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .overview-token-card {
    height: auto;
  }

  .overview-token-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 18px 0;
  }

  .overview-token-item:nth-child(4) {
    border-left: 0;
    padding-left: 0;
  }
}

@media (max-width: 760px) {
  .overview-token-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .overview-token-item:nth-child(3),
  .overview-token-item:nth-child(5) {
    border-left: 0;
    padding-left: 0;
  }

  .overview-cache-pill {
    display: none;
  }
}
</style>
