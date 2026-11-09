import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import {
  DEFAULT_LOG_RANGE,
  DEFAULT_LOG_HTTP_STATUS,
  DEFAULT_LOG_PAGE_SIZE,
  fetchRequestLogs,
  fetchRequestLogsForExport,
  fetchLogProviders,
  fetchLogStats,
  LOG_PAGE_SIZE_OPTIONS,
  LOG_RANGE_OPTIONS,
  normalizeLogPageSize,
  normalizeLogRangeKey,
  type LogHttpStatus,
  type LogRangeKey,
  type LogStats,
  type RequestLog,
} from '../../services/logs'
import { showToast } from '../../utils/toast'
import { formatLocalDateTime } from '../../utils/dateTime'
import type { CostTier } from './costTier'
import type { TranslateFn } from '../../types/i18n'

type UseLogsDashboardOptions = {
  t: TranslateFn
}

const REFRESH_INTERVAL = 30

export const useLogsDashboard = ({ t }: UseLogsDashboardOptions) => {
  const logs = ref<RequestLog[]>([])
  const summaryStats = ref<LogStats | null>(null)
  const loading = ref(false)
  const exporting = ref(false)
  const filters = reactive<{
    platform: string
    provider: string
    rangeKey: LogRangeKey
    costTier: CostTier
    httpStatus: LogHttpStatus
  }>({
    platform: '',
    provider: '',
    rangeKey: DEFAULT_LOG_RANGE,
    costTier: 'all',
    httpStatus: DEFAULT_LOG_HTTP_STATUS,
  })
  const page = ref(1)
  const pageSize = ref(DEFAULT_LOG_PAGE_SIZE)
  const totalLogs = ref(0)
  const providerOptions = ref<string[]>([])
  const countdown = ref(REFRESH_INTERVAL)
  let timer: number | undefined
  let dashboardLoadPromise: Promise<void> | null = null
  let queuedDashboardReload = false
  let disposed = false
  const rangeOptions = computed(() =>
    LOG_RANGE_OPTIONS.map((value) => ({
      value,
      label: t('logsPage.filters.rangeOption', { range: t(`logsPage.filters.ranges.${value}`) }),
    })),
  )
  const httpStatusOptions = computed<Array<{ value: LogHttpStatus; label: string }>>(() => [
    {
      value: 'all',
      label: t('logsPage.filters.allStatus'),
    },
    {
      value: 'success',
      label: t('logsPage.filters.successStatus'),
    },
    {
      value: 'failed',
      label: t('logsPage.filters.failedStatus'),
    },
  ])

  const pageSizeOptions = computed(() => LOG_PAGE_SIZE_OPTIONS.map((value) => ({ value })))
  const pagedLogs = computed(() => logs.value)
  const totalPages = computed(() => Math.max(1, Math.ceil(totalLogs.value / pageSize.value)))

  const resetTimer = () => {
    countdown.value = REFRESH_INTERVAL
  }

  const loadDashboard = (options: { skipIfLoading?: boolean } = {}) => {
    if (disposed) return null
    if (dashboardLoadPromise) {
      return options.skipIfLoading ? null : dashboardLoadPromise
    }

    dashboardLoadPromise = (async () => {
      loading.value = true
      try {
        do {
          queuedDashboardReload = false
          const requestedPage = Math.max(1, Math.floor(page.value))
          const requestedPageSize = normalizeLogPageSize(pageSize.value)
          const filterQuery = {
            platform: filters.platform,
            provider: filters.provider,
            rangeKey: normalizeLogRangeKey(filters.rangeKey),
            costTier: filters.costTier,
            httpStatus: filters.httpStatus,
          }
          const [logData, statsData] = await Promise.all([
            fetchRequestLogs({
              ...filterQuery,
              page: requestedPage,
              pageSize: requestedPageSize,
            }),
            // 汇总失败时保留请求列表，避免统计数据不可用拖垮整个页面。
            fetchLogStats(filterQuery).catch((error) => {
              console.error('failed to load filtered log summary', error)
              return null
            }),
          ])
          if (disposed) return
          summaryStats.value = statsData ?? null
          const nextLogs = logData?.items ?? []
          logs.value = nextLogs
          totalLogs.value = logData?.total ?? 0
          pageSize.value = normalizeLogPageSize(logData?.page_size ?? requestedPageSize)
          const nextTotalPages = Math.max(1, Math.ceil(totalLogs.value / pageSize.value))
          if (requestedPage > nextTotalPages) {
            page.value = nextTotalPages
            queuedDashboardReload = true
          } else {
            page.value = logData?.page ?? requestedPage
          }
        } while (queuedDashboardReload && !disposed)
      } catch (error) {
        console.error('failed to load dashboard data', error)
      } finally {
        if (!disposed) loading.value = false
        dashboardLoadPromise = null
      }
    })()

    return dashboardLoadPromise
  }

  const loadProviderOptions = async () => {
    if (disposed) return
    try {
      const list = await fetchLogProviders(filters.platform)
      if (disposed) return
      providerOptions.value = list ?? []
      if (filters.provider && !providerOptions.value.includes(filters.provider)) {
        filters.provider = ''
      }
    } catch (error) {
      console.error('failed to load provider options', error)
    }
  }

  const stopCountdown = () => {
    if (timer) {
      clearInterval(timer)
      timer = undefined
    }
  }

  const startCountdown = () => {
    stopCountdown()
    if (disposed || document.hidden) return
    timer = window.setInterval(() => {
      if (countdown.value <= 1) {
        countdown.value = REFRESH_INTERVAL
        void loadDashboard({ skipIfLoading: true })
      } else {
        countdown.value -= 1
      }
    }, 1000)
  }

  const handleVisibilityChange = () => {
    if (document.hidden) {
      stopCountdown()
    } else {
      startCountdown()
    }
  }

  const applyFilters = async () => {
    page.value = 1
    if (dashboardLoadPromise) {
      queuedDashboardReload = true
    }
    await loadDashboard()
    resetTimer()
  }

  const manualRefresh = () => {
    resetTimer()
    if (dashboardLoadPromise) {
      queuedDashboardReload = true
    }
    void loadDashboard()
  }

  const nextPage = () => {
    if (page.value < totalPages.value) {
      page.value += 1
      void loadDashboard()
    }
  }

  const prevPage = () => {
    if (page.value > 1) {
      page.value -= 1
      void loadDashboard()
    }
  }

  const updatePageSize = () => {
    pageSize.value = normalizeLogPageSize(Number(pageSize.value))
    page.value = 1
    if (dashboardLoadPromise) {
      queuedDashboardReload = true
    }
    void loadDashboard()
  }

  const csvEscape = (value: unknown) => {
    const text = String(value ?? '')
    return /[",\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
  }

  // 导出属于用户发起的一次性任务：离开页面也继续完成下载，不随页面卸载取消。
  const exportCsv = async () => {
    if (exporting.value) return
    exporting.value = true
    try {
      const all = await fetchRequestLogsForExport({
        platform: filters.platform,
        provider: filters.provider,
        rangeKey: normalizeLogRangeKey(filters.rangeKey),
        costTier: filters.costTier,
        httpStatus: filters.httpStatus,
      })

      const header = 'id,created_at,platform,provider,model,session_id,http_code,ttft_sec,duration_sec,input_tokens,output_tokens,cache_create_tokens,cache_read_tokens,reasoning_tokens,total_cost'
      const lines = all.map((log) =>
        [
          log.id,
          formatLocalDateTime(log.created_at),
          log.platform,
          log.provider,
          log.model,
          log.session_id,
          log.http_code,
          log.ttft_sec || '',
          log.duration_sec,
          log.input_tokens,
          log.output_tokens,
          log.cache_create_tokens,
          log.cache_read_tokens,
          log.reasoning_tokens,
          log.total_cost,
        ]
          .map(csvEscape)
          .join(','),
      )
      const blob = new Blob([`${header}\n${lines.join('\n')}`], {
        type: 'text/csv;charset=utf-8',
      })
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `portico-logs-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.csv`
      anchor.click()
      URL.revokeObjectURL(url)
      showToast(t('logsPage.messages.exported', { count: all.length }))
    } catch (error) {
      console.error('export logs failed', error)
      showToast(t('logsPage.messages.exportFailed'), 'error')
    } finally {
      exporting.value = false
    }
  }

  watch(
    () => filters.platform,
    async () => {
      await loadProviderOptions()
    },
  )

  onMounted(async () => {
    await Promise.all([loadDashboard(), loadProviderOptions()])
    if (disposed) return
    startCountdown()
    document.addEventListener('visibilitychange', handleVisibilityChange)
  })

  onUnmounted(() => {
    disposed = true
    queuedDashboardReload = false
    stopCountdown()
    document.removeEventListener('visibilitychange', handleVisibilityChange)
  })

  return {
    applyFilters,
    countdown,
    exporting,
    exportCsv,
    filters,
    httpStatusOptions,
    loading,
    manualRefresh,
    nextPage,
    page,
    pageSize,
    pageSizeOptions,
    pagedLogs,
    prevPage,
    providerOptions,
    rangeOptions,
    summaryStats,
    totalLogs,
    totalPages,
    updatePageSize,
  }
}
