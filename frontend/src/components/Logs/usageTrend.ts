import type { LogStatsSeries } from '../../services/logs'
import { formatUsd } from '../../utils/currency'

export const TREND_METRICS = [
  { key: 'cost', field: 'total_cost', labelKey: 'logsPage.overview.trend.metrics.cost', color: 'var(--cs-metric-cost)' },
  { key: 'input', field: 'input_tokens', labelKey: 'logsPage.overview.trend.metrics.input', color: 'var(--cs-metric-input)' },
  { key: 'output', field: 'output_tokens', labelKey: 'logsPage.overview.trend.metrics.output', color: 'var(--cs-metric-output)' },
  { key: 'cacheCreate', field: 'cache_create_tokens', labelKey: 'logsPage.overview.trend.metrics.cacheCreate', color: 'var(--cs-metric-cache-create)' },
  { key: 'reasoning', field: 'reasoning_tokens', labelKey: 'logsPage.overview.trend.metrics.reasoning', color: 'var(--cs-metric-reasoning)' },
  { key: 'cacheRead', field: 'cache_read_tokens', labelKey: 'logsPage.overview.trend.metrics.cacheRead', color: 'var(--cs-metric-cache-read)' },
] as const

export type TrendMetricKey = (typeof TREND_METRICS)[number]['key']
export type TrendMetric = (typeof TREND_METRICS)[number]

export const trendMetricValue = (item: LogStatsSeries, metric: TrendMetric): number => {
  const value = item[metric.field]
  return Number.isFinite(value) ? Number(value) : 0
}

export const formatTrendValue = (value: number, metric: TrendMetric): string => {
  return metric.key === 'cost' ? formatUsd(value) : Math.round(value).toLocaleString()
}

export const formatCompactMetricValue = (value: number, metric: TrendMetric): string => {
  if (metric.key === 'cost') {
    if (value >= 1000) return `$${(value / 1000).toFixed(1)}K`
    return `$${value.toFixed(value >= 1 ? 1 : 2)}`
  }
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return Math.round(value).toLocaleString()
}

export const formatOverviewTokenCount = (value: number): string => {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return Math.round(value).toLocaleString()
}
