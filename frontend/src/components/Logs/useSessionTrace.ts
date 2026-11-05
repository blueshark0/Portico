import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { fetchSessionRequestLogs } from '../../services/sessions'
import type { RequestLog } from '../../services/logs'
import { formatBeijingDateTime } from '../../utils/dateTime'

export type SessionTraceRange = 'day' | 'full'
export type SessionTraceStatus = 'all' | 'success' | 'failed'

export type SessionTraceMetrics = {
  requests: number
  successful: number
  failed: number
  cacheHits: number
  inputTokens: number
  outputTokens: number
  cacheCreateTokens: number
  cacheReadTokens: number
  totalCost: number
}

const queryValue = (value: unknown) => Array.isArray(value) ? String(value[0] ?? '') : String(value ?? '')
export const requestDayKey = (log?: RequestLog | null) => log ? formatBeijingDateTime(log.created_at).slice(0, 10) : ''

export const summarizeSessionLogs = (logs: RequestLog[]): SessionTraceMetrics => {
  const summary: SessionTraceMetrics = {
    requests: logs.length,
    successful: 0,
    failed: 0,
    cacheHits: 0,
    inputTokens: 0,
    outputTokens: 0,
    cacheCreateTokens: 0,
    cacheReadTokens: 0,
    totalCost: 0,
  }
  for (const log of logs) {
    if (log.http_code >= 200 && log.http_code < 300) summary.successful += 1
    if (log.http_code >= 400) summary.failed += 1
    if (log.cache_read_tokens > 0) summary.cacheHits += 1
    summary.inputTokens += log.input_tokens
    summary.outputTokens += log.output_tokens
    summary.cacheCreateTokens += log.cache_create_tokens
    summary.cacheReadTokens += log.cache_read_tokens
    summary.totalCost += log.has_pricing ? log.total_cost : 0
  }
  return summary
}

export const useSessionTrace = () => {
  const route = useRoute()
  const platform = computed(() => queryValue(route.query.platform).trim())
  const sessionId = computed(() => queryValue(route.query.session).trim())
  const sourceRequestId = computed(() => Number(queryValue(route.query.request)) || 0)
  const logs = ref<RequestLog[]>([])
  const loading = ref(false)
  const loadFailed = ref(false)
  const range = ref<SessionTraceRange>('day')
  const status = ref<SessionTraceStatus>('all')
  const selectedRequestId = ref(0)
  let sequence = 0

  const load = async () => {
    const currentSequence = ++sequence
    if (!platform.value || !sessionId.value) {
      logs.value = []
      loading.value = false
      loadFailed.value = false
      return
    }
    loading.value = true
    loadFailed.value = false
    try {
      const result = await fetchSessionRequestLogs(platform.value, sessionId.value)
      if (currentSequence !== sequence) return
      logs.value = result ?? []
      range.value = 'day'
      status.value = 'all'
      const requested = logs.value.find((item) => item.id === sourceRequestId.value)
      selectedRequestId.value = requested?.id ?? logs.value[0]?.id ?? 0
    } catch (error) {
      if (currentSequence !== sequence) return
      console.error('failed to load session trace', error)
      logs.value = []
      selectedRequestId.value = 0
      loadFailed.value = true
    } finally {
      if (currentSequence === sequence) loading.value = false
    }
  }

  watch([platform, sessionId], () => void load(), { immediate: true })
  watch(sourceRequestId, (requestId) => {
    if (!logs.value.length) return
    selectedRequestId.value = logs.value.some((item) => item.id === requestId) ? requestId : logs.value[0].id
    range.value = 'day'
    status.value = 'all'
  })

  const sourceRequest = computed(() => logs.value.find((item) => item.id === sourceRequestId.value) ?? logs.value[0] ?? null)
  const selectedRequest = computed(() => logs.value.find((item) => item.id === selectedRequestId.value) ?? sourceRequest.value)
  const referenceDay = computed(() => requestDayKey(sourceRequest.value))
  const scopedLogs = computed(() => range.value === 'day'
    ? logs.value.filter((item) => requestDayKey(item) === referenceDay.value)
    : logs.value)
  const visibleLogs = computed(() => scopedLogs.value.filter((item) => {
    if (status.value === 'success') return item.http_code >= 200 && item.http_code < 300
    if (status.value === 'failed') return item.http_code >= 400
    return true
  }))
  const fullMetrics = computed(() => summarizeSessionLogs(logs.value))
  const scopeMetrics = computed(() => summarizeSessionLogs(scopedLogs.value))
  const latestRequest = computed(() => logs.value[0] ?? null)
  const oldestRequest = computed(() => logs.value[logs.value.length - 1] ?? null)

  return {
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
  }
}
