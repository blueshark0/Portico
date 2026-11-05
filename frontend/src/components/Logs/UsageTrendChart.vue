<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { LogStatsSeries } from '../../services/logs'
import { parseDateTime } from '../../utils/dateTime'
import {
  TREND_METRICS,
  formatCompactMetricValue,
  formatTrendValue,
  trendMetricValue,
  type TrendMetricKey,
} from './usageTrend'

const props = withDefaults(defineProps<{
  series?: LogStatsSeries[]
  loading?: boolean
}>(), {
  series: () => [],
  loading: false,
})

const { locale, t } = useI18n()
const selectedMetric = ref<TrendMetricKey>('cost')
const activeIndex = ref<number | null>(null)
const plotEl = ref<HTMLDivElement | null>(null)
const width = ref(1190)
const height = 164
const plotTop = 10
const plotBottom = 130
let resizeObserver: ResizeObserver | undefined

const metric = computed(() => TREND_METRICS.find((item) => item.key === selectedMetric.value) ?? TREND_METRICS[0])
const metricUnit = computed(() => metric.value.key === 'cost' ? 'USD' : 'Token')
const values = computed(() => props.series.map((item) => trendMetricValue(item, metric.value)))
const maximum = computed(() => {
  const maxValue = Math.max(0, ...values.value)
  if (maxValue <= 0) return 1
  const roughStep = maxValue / 4
  const magnitude = 10 ** Math.floor(Math.log10(roughStep))
  const normalized = roughStep / magnitude
  const step = (normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10) * magnitude
  return step * 4
})

const xFor = (index: number) => {
  const left = 56
  const right = Math.max(left + 1, width.value - 56)
  if (props.series.length <= 1) return (left + right) / 2
  return left + (right - left) * (index / (props.series.length - 1))
}
const yFor = (value: number) => plotBottom - (Math.min(value, maximum.value) / maximum.value) * (plotBottom - plotTop)
const points = computed(() => values.value.map((value, index) => ({ x: xFor(index), y: yFor(value), value })))
const linePath = computed(() => points.value.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x} ${point.y}`).join(' '))
const areaPath = computed(() => {
  if (!points.value.length) return ''
  return `${linePath.value} L ${points.value[points.value.length - 1].x} ${plotBottom} L ${points.value[0].x} ${plotBottom} Z`
})
const gridLines = computed(() => [0, 1, 2, 3, 4].map((index) => {
  const value = maximum.value * (4 - index) / 4
  return { y: plotTop + (plotBottom - plotTop) * index / 4, value }
}))

const dayLabel = (item?: LogStatsSeries) => {
  const date = parseDateTime(item?.day)
  if (!date) return item?.day ?? ''
  return new Intl.DateTimeFormat(locale.value, { weekday: 'short' }).format(date)
}
const fullDayLabel = (item?: LogStatsSeries) => {
  const date = parseDateTime(item?.day)
  if (!date) return item?.day ?? ''
  return new Intl.DateTimeFormat(locale.value, { month: 'short', day: 'numeric', weekday: 'short' }).format(date)
}
const pointAriaLabel = (index: number) => {
  const item = props.series[index]
  return `${fullDayLabel(item)}: ${formatTrendValue(values.value[index] ?? 0, metric.value)}`
}

const showPoint = (index: number) => {
  activeIndex.value = index
}
const handlePointerMove = (event: PointerEvent) => {
  if (!points.value.length || !plotEl.value) return
  const bounds = plotEl.value.getBoundingClientRect()
  const x = (event.clientX - bounds.left) * (width.value / bounds.width)
  let nearest = 0
  let distance = Number.POSITIVE_INFINITY
  points.value.forEach((point, index) => {
    const nextDistance = Math.abs(point.x - x)
    if (nextDistance < distance) {
      nearest = index
      distance = nextDistance
    }
  })
  activeIndex.value = nearest
}
const activePoint = computed(() => activeIndex.value === null ? null : points.value[activeIndex.value] ?? null)
const handlePointerLeave = () => {
  if (document.activeElement?.classList.contains('usage-trend-hit')) return
  activeIndex.value = null
}
const tooltipStyle = computed(() => {
  if (!activePoint.value) return {}
  const x = activePoint.value.x
  return {
    left: `${Math.min(Math.max(12, x), Math.max(12, width.value - 150))}px`,
    top: `${Math.max(4, activePoint.value.y - 54)}px`,
  }
})

onMounted(() => {
  if (!plotEl.value) return
  width.value = Math.max(280, plotEl.value.clientWidth)
  resizeObserver = new ResizeObserver(([entry]) => {
    width.value = Math.max(280, entry.contentRect.width)
  })
  resizeObserver.observe(plotEl.value)
})
onUnmounted(() => resizeObserver?.disconnect())
</script>

<template>
  <section class="cs-card usage-trend" aria-labelledby="usage-trend-title">
    <div class="usage-trend-head">
      <div class="usage-trend-heading">
        <h2 id="usage-trend-title" class="cs-card-title">{{ t('logsPage.overview.trend.title') }}</h2>
        <p>{{ t('logsPage.overview.trend.subtitle', { metric: t(metric.labelKey), unit: metricUnit }) }}</p>
      </div>
      <div class="usage-trend-metrics" role="group" :aria-label="t('logsPage.overview.trend.metricLabel')">
        <button
          v-for="option in TREND_METRICS"
          :key="option.key"
          type="button"
          class="usage-trend-metric"
          :class="{ active: selectedMetric === option.key }"
          :aria-pressed="selectedMetric === option.key"
          @click="selectedMetric = option.key; activeIndex = null"
        >
          <span class="usage-trend-swatch" :style="{ backgroundColor: option.color }" />
          {{ t(option.labelKey) }}
        </button>
      </div>
    </div>

    <div
      ref="plotEl"
      class="usage-trend-plot"
      :class="{ 'usage-trend-plot--loading': loading }"
      @pointermove="handlePointerMove"
      @pointerleave="handlePointerLeave"
    >
      <svg
        class="usage-trend-svg"
        :viewBox="`0 0 ${width} ${height}`"
        role="group"
        :aria-label="t('logsPage.overview.trend.chartLabel', { metric: t(metric.labelKey) })"
      >
        <g v-for="(line, index) in gridLines" :key="index">
          <line class="usage-trend-grid" :x1="36" :x2="width - 18" :y1="line.y" :y2="line.y" />
          <text class="usage-trend-axis-label" x="0" :y="line.y + 3">{{ formatCompactMetricValue(line.value, metric) }}</text>
        </g>
        <template v-if="points.length">
          <path :d="areaPath" :fill="metric.color" fill-opacity="0.1" />
          <path class="usage-trend-line" :d="linePath" :stroke="metric.color" />
          <line
            v-if="activePoint"
            class="usage-trend-crosshair"
            :x1="activePoint.x"
            :x2="activePoint.x"
            :y1="plotTop"
            :y2="plotBottom"
          />
          <g v-for="(point, index) in points" :key="`${selectedMetric}-${index}`">
            <circle
              class="usage-trend-point"
              :class="{ active: activeIndex === index }"
              :cx="point.x"
              :cy="point.y"
              :r="activeIndex === index ? 5 : 4"
              :fill="metric.color"
            />
            <circle
              class="usage-trend-hit"
              :cx="point.x"
              :cy="point.y"
              r="12"
              tabindex="0"
              :aria-label="pointAriaLabel(index)"
              @focus="showPoint(index)"
              @blur="activeIndex = null"
            />
          </g>
        </template>
        <text v-else class="usage-trend-empty" :x="width / 2" y="76" text-anchor="middle">
          {{ t('logsPage.overview.trend.empty') }}
        </text>
        <text
          v-for="(item, index) in series"
          :key="`label-${item.day}-${index}`"
          class="usage-trend-day"
          :x="xFor(index)"
          y="154"
          text-anchor="middle"
        >
          {{ dayLabel(item) }}
        </text>
      </svg>

      <div v-if="activePoint && activeIndex !== null" class="usage-trend-tooltip" :style="tooltipStyle" role="status">
        <strong>{{ formatTrendValue(activePoint.value, metric) }}</strong>
        <span>{{ fullDayLabel(series[activeIndex]) }} · {{ t(metric.labelKey) }}</span>
      </div>
    </div>

    <details class="usage-trend-data">
      <summary>{{ t('logsPage.overview.trend.table') }}</summary>
      <div class="usage-trend-table-wrap">
        <table>
          <caption>{{ t('logsPage.overview.trend.tableCaption', { metric: t(metric.labelKey) }) }}</caption>
          <thead><tr><th scope="col">{{ t('logsPage.overview.trend.day') }}</th><th scope="col">{{ t(metric.labelKey) }}</th></tr></thead>
          <tbody>
            <tr v-for="(item, index) in series" :key="`row-${item.day}-${index}`">
              <th scope="row">{{ fullDayLabel(item) }}</th>
              <td>{{ formatTrendValue(values[index] ?? 0, metric) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </details>
  </section>
</template>

<style scoped>
.usage-trend {
  position: relative;
  height: 221px;
  padding: 12px 18px 8px;
  overflow: visible;
}

.usage-trend-head {
  display: flex;
  align-items: center;
  gap: 16px;
  min-height: 29px;
}

.usage-trend-heading {
  display: flex;
  align-items: baseline;
  gap: 13px;
  flex-shrink: 0;
}

.usage-trend-heading p {
  margin: 0;
  color: var(--cs-text-secondary);
  font-size: 11px;
  white-space: nowrap;
}

.usage-trend-metrics {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  margin-left: auto;
}

.usage-trend-metric {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  height: 29px;
  min-width: 75px;
  padding: 0 10px;
  border: 1px solid var(--cs-border);
  border-radius: 7px;
  background: var(--cs-surface);
  color: var(--cs-text-secondary);
  font: 500 10px var(--cs-font);
  cursor: pointer;
  white-space: nowrap;
}

.usage-trend-metric:nth-child(4),
.usage-trend-metric:nth-child(6) {
  min-width: 98px;
}

.usage-trend-metric.active {
  border-color: var(--cs-brand-tint-border);
  background: var(--cs-brand-tint);
  color: var(--cs-brand);
}

.usage-trend-swatch {
  width: 7px;
  height: 7px;
  border-radius: 3px;
  flex-shrink: 0;
}

.usage-trend-plot {
  position: relative;
  height: 164px;
  margin-top: 4px;
  transition: opacity 0.15s ease;
}

.usage-trend-plot--loading {
  opacity: 0.55;
}

.usage-trend-svg {
  display: block;
  width: 100%;
  height: 164px;
  overflow: visible;
}

.usage-trend-grid {
  stroke: var(--cs-border);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}

.usage-trend-axis-label,
.usage-trend-day,
.usage-trend-empty {
  fill: var(--cs-text-secondary);
  font: 10px var(--cs-font);
  font-variant-numeric: tabular-nums;
}

.usage-trend-line {
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
  vector-effect: non-scaling-stroke;
}

.usage-trend-point {
  stroke: var(--cs-surface);
  stroke-width: 2;
  vector-effect: non-scaling-stroke;
}

.usage-trend-crosshair {
  stroke: var(--cs-text-secondary);
  stroke-opacity: 0.45;
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}

.usage-trend-hit {
  fill: transparent;
  cursor: crosshair;
  outline: none;
}

.usage-trend-tooltip {
  position: absolute;
  z-index: 5;
  display: flex;
  flex-direction: column;
  gap: 3px;
  width: 142px;
  padding: 8px 10px;
  border: 1px solid var(--cs-border);
  border-radius: 8px;
  background: var(--cs-surface);
  box-shadow: var(--cs-shadow-menu);
  pointer-events: none;
}

.usage-trend-tooltip strong {
  color: var(--cs-text);
  font-size: 12px;
}

.usage-trend-tooltip span {
  color: var(--cs-text-secondary);
  font-size: 9px;
}

.usage-trend-data {
  position: absolute;
  right: 18px;
  bottom: 3px;
  z-index: 6;
  color: var(--cs-text-secondary);
  font-size: 9px;
}

.usage-trend-data summary {
  cursor: pointer;
  list-style: none;
}

.usage-trend-table-wrap {
  position: absolute;
  right: 0;
  bottom: 20px;
  width: 260px;
  max-height: 190px;
  overflow: auto;
  padding: 10px;
  border: 1px solid var(--cs-border);
  border-radius: 9px;
  background: var(--cs-surface);
  box-shadow: var(--cs-shadow-menu);
}

.usage-trend-table-wrap table {
  width: 100%;
  border-collapse: collapse;
  color: var(--cs-text);
  font-size: 10px;
  text-align: left;
}

.usage-trend-table-wrap caption {
  margin-bottom: 7px;
  color: var(--cs-text-secondary);
  text-align: left;
}

.usage-trend-table-wrap th,
.usage-trend-table-wrap td {
  padding: 5px;
  border-bottom: 1px solid var(--cs-border);
  font-variant-numeric: tabular-nums;
}

@media (max-width: 1100px) {
  .usage-trend {
    height: auto;
    min-height: 221px;
  }

  .usage-trend-head {
    align-items: flex-start;
    flex-direction: column;
    gap: 8px;
  }

  .usage-trend-metrics {
    width: 100%;
    justify-content: flex-start;
    flex-wrap: wrap;
  }
}

@media (forced-colors: active) {
  .usage-trend-line,
  .usage-trend-point {
    stroke: CanvasText;
  }
}
</style>
