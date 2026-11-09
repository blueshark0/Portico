import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { useLogsDashboard } from '../src/components/Logs/useLogsDashboard'

const api = vi.hoisted(() => ({ list: vi.fn(), exportLogs: vi.fn(), stats: vi.fn(), providers: vi.fn() }))
vi.mock('../src/services/logs', () => ({
  DEFAULT_LOG_RANGE: 'last7days', DEFAULT_LOG_HTTP_STATUS: 'all', DEFAULT_LOG_PAGE_SIZE: 200,
  LOG_RANGE_OPTIONS: ['today', 'last7days'], LOG_PAGE_SIZE_OPTIONS: [200],
  normalizeLogPageSize: (n: number) => n, normalizeLogRangeKey: (s: string) => s,
  fetchRequestLogs: api.list, fetchRequestLogsForExport: api.exportLogs, fetchLogStats: api.stats, fetchLogProviders: api.providers,
}))

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}
const emptyPage = { items: [], total: 0, page: 1, page_size: 200 }
function harness() {
  let state!: ReturnType<typeof useLogsDashboard>
  const wrapper = mount(defineComponent({ setup() {
    state = useLogsDashboard({ t: ((key: string) => key) as never })
    return () => h('div')
  } }))
  return { wrapper, state }
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  api.list.mockReset().mockResolvedValue(emptyPage)
  api.exportLogs.mockReset().mockResolvedValue([])
  api.stats.mockReset().mockResolvedValue(null)
  api.providers.mockReset().mockResolvedValue([])
})
afterEach(() => { vi.useRealTimers() })

describe('request-log page lifecycle', () => {
  it('ignores late data and does not leak resources when unmounted during loading', async () => {
    const add = vi.spyOn(document, 'addEventListener')
    for (let i = 0; i < 10; i++) {
      const pending = deferred<typeof emptyPage>()
      api.list.mockReturnValueOnce(pending.promise)
      const { wrapper, state } = harness()
      wrapper.unmount()
      pending.resolve({ ...emptyPage, total: 99 })
      await flushPromises()
      expect(state.totalLogs.value).toBe(0)
    }
    expect(vi.getTimerCount()).toBe(0)
    expect(add.mock.calls.filter(([name]) => name === 'visibilitychange')).toHaveLength(0)
    await vi.advanceTimersByTimeAsync(60000)
    expect(api.list).toHaveBeenCalledTimes(10)
  })
  it('cleans up after loading and pauses requests while hidden', async () => {
    const remove = vi.spyOn(document, 'removeEventListener')
    const { wrapper } = harness()
    await flushPromises()
    expect(vi.getTimerCount()).toBe(1)
    await vi.advanceTimersByTimeAsync(30000)
    expect(api.list).toHaveBeenCalledTimes(2)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(60000)
    expect(api.list).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    expect(vi.getTimerCount()).toBe(0)
    expect(remove.mock.calls.some(([name]) => name === 'visibilitychange')).toBe(true)
  })
  it('cleans up when the initial request fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    api.list.mockRejectedValueOnce(new Error('unavailable'))
    const { wrapper } = harness()
    await flushPromises()
    wrapper.unmount()
    expect(vi.getTimerCount()).toBe(0)
  })
})

describe('request-log CSV export', () => {
  it('fetches once without paging and still downloads after leaving the page', async () => {
    const createUrl = vi.fn(() => 'blob:export')
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL: createUrl, revokeObjectURL: vi.fn() }))
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const { wrapper, state } = harness()
    await flushPromises()
    api.list.mockClear()
    const pending = deferred<unknown[]>()
    api.exportLogs.mockReturnValueOnce(pending.promise)
    const exporting = state.exportCsv()
    wrapper.unmount()
    pending.resolve([{ id: 1, created_at: '2026-11-06 10:00:00', platform: 'claude' }])
    await exporting
    expect(api.exportLogs).toHaveBeenCalledTimes(1)
    expect(api.list).not.toHaveBeenCalled()
    expect(createUrl).toHaveBeenCalledTimes(1)
    expect(click).toHaveBeenCalledTimes(1)
    expect(state.exporting.value).toBe(false)
    vi.unstubAllGlobals()
  })
})
