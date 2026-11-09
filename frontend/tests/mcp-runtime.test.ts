import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import McpPage from '../src/components/Mcp/Index.vue'
import { useMcpServers } from '../src/components/Mcp/useMcpServers'

const api = vi.hoisted(() => ({ snapshot: vi.fn(), servers: vi.fn() }))
vi.mock('../src/services/mcp', () => ({
  McpServer: class { constructor(value: unknown) { Object.assign(this, value) } },
  ServerType: { ServerTypeStdio: 'stdio', ServerTypeHTTP: 'http' },
  fetchMcpRuntimeSnapshot: api.snapshot, fetchMcpServers: api.servers,
  fetchMcpEndpoint: async () => 'http://127.0.0.1:18100/mcp', fetchInjectionStatus: async () => false,
  testMcpConnection: vi.fn(), saveMcpServer: vi.fn(), deleteMcpServer: vi.fn(),
  setMcpServerEnabled: vi.fn(), injectMcp: vi.fn(), removeMcpInjection: vi.fn(),
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('../src/utils/toast', () => ({ showToast: vi.fn() }))

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}
const initial = { statuses: [], catalogRevision: 1, catalogChanged: true, tools: [] }
function page() {
  return mount(McpPage, { global: {
    plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: {} }, missingWarn: false, fallbackWarn: false })],
    stubs: { McpEditorModal: true, BaseModal: true },
  } })
}
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  api.servers.mockReset().mockResolvedValue([])
  api.snapshot.mockReset().mockResolvedValue(initial)
})
afterEach(() => { vi.useRealTimers() })

describe('MCP page lifecycle', () => {
  it('does not leak a poller or click listener when leaving before loading finishes', async () => {
    const add = vi.spyOn(window, 'addEventListener')
    for (let i = 0; i < 10; i++) {
      const pending = deferred<typeof initial>()
      api.snapshot.mockReturnValueOnce(pending.promise)
      const wrapper = page()
      wrapper.unmount()
      pending.resolve(initial)
      await flushPromises()
    }
    expect(vi.getTimerCount()).toBe(0)
    expect(add.mock.calls.filter(([name]) => name === 'click')).toHaveLength(0)
    await vi.advanceTimersByTimeAsync(60000)
    expect(api.snapshot).toHaveBeenCalledTimes(10)
  })
  it('pauses hidden requests and removes resources after normal loading', async () => {
    const remove = vi.spyOn(window, 'removeEventListener')
    const wrapper = page()
    await flushPromises()
    expect(vi.getTimerCount()).toBe(1)
    await vi.advanceTimersByTimeAsync(1000)
    expect(api.snapshot).toHaveBeenCalledTimes(2)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    await vi.advanceTimersByTimeAsync(10000)
    expect(api.snapshot).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    expect(vi.getTimerCount()).toBe(0)
    expect(remove.mock.calls.some(([name]) => name === 'click')).toBe(true)
  })
  it('cleans up after a failed initial load', async () => {
    api.servers.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = page()
    await flushPromises()
    wrapper.unmount()
    expect(vi.getTimerCount()).toBe(0)
  })
})

describe('MCP incremental runtime', () => {
  it('keeps the catalog on unchanged revisions, refreshes equal-count schema changes and clears removals', async () => {
    let state!: ReturnType<typeof useMcpServers>
    const wrapper = mount(defineComponent({ setup() {
      state = useMcpServers((s) => s)
      return () => h('div')
    } }))
    api.snapshot.mockResolvedValueOnce({ ...initial, catalogRevision: 2,
      tools: [{ publicName: 'fs__read', inputSchema: '{"type":"object"}' }] })
    await state.refreshStatuses()
    const catalog = state.tools.value
    api.snapshot.mockResolvedValue({ statuses: [], catalogRevision: 2, catalogChanged: false })
    for (let i = 0; i < 60; i++) await state.refreshStatuses()
    expect(state.tools.value).toBe(catalog)
    expect(api.snapshot).toHaveBeenLastCalledWith(2)
    api.snapshot.mockResolvedValueOnce({ ...initial, catalogRevision: 3,
      tools: [{ publicName: 'fs__read', inputSchema: '{"type":"object","required":["path"]}' }] })
    await state.refreshStatuses()
    expect(state.tools.value[0].inputSchema).toContain('required')
    api.snapshot.mockResolvedValueOnce({ ...initial, catalogRevision: 4 })
    await state.refreshStatuses()
    expect(state.tools.value).toEqual([])
    wrapper.unmount()
  })
  it('coalesces in-flight polls and ignores late results after unmount', async () => {
    let state!: ReturnType<typeof useMcpServers>
    const wrapper = mount(defineComponent({ setup() {
      state = useMcpServers((s) => s)
      return () => h('div')
    } }))
    const pending = deferred<typeof initial>()
    api.snapshot.mockReturnValueOnce(pending.promise)
    const first = state.refreshStatuses()
    expect(state.refreshStatuses()).toBe(first)
    wrapper.unmount()
    pending.resolve({ ...initial, statuses: [{ serverId: 1 }] as never })
    await first
    expect(state.statuses.value).toEqual({})
    await state.refreshStatuses()
    expect(api.snapshot).toHaveBeenCalledTimes(1)
  })
})

describe('MCP status refresh after mutations', () => {
  function harness() {
    let state!: ReturnType<typeof useMcpServers>
    const wrapper = mount(defineComponent({ setup() {
      state = useMcpServers((s) => s)
      return () => h('div')
    } }))
    return { state, wrapper }
  }
  const snapshotOf = (state: string) => ({
    statuses: [{ serverId: 1, state, toolCount: 0 }], catalogRevision: 1, catalogChanged: false,
  })

  it('refetches after a mutation that overlaps an in-flight poll', async () => {
    const { state, wrapper } = harness()
    const pending = deferred<ReturnType<typeof snapshotOf>>()
    api.snapshot.mockReturnValueOnce(pending.promise)
    const poll = state.refreshStatuses({ skipIfLoading: true })
    api.snapshot.mockResolvedValueOnce(snapshotOf('disabled'))
    void state.refreshStatuses()
    pending.resolve(snapshotOf('connected'))
    await poll
    expect(api.snapshot).toHaveBeenCalledTimes(2)
    expect(state.statuses.value[1].state).toBe('disabled')
    wrapper.unmount()
  })
  it('does not queue extra requests for overlapping polls', async () => {
    const { state, wrapper } = harness()
    const pending = deferred<ReturnType<typeof snapshotOf>>()
    api.snapshot.mockReturnValueOnce(pending.promise)
    const poll = state.refreshStatuses({ skipIfLoading: true })
    void state.refreshStatuses({ skipIfLoading: true })
    pending.resolve(snapshotOf('connected'))
    await poll
    expect(api.snapshot).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
  it('keeps the statuses object when polling returns the same content', async () => {
    const { state, wrapper } = harness()
    api.snapshot.mockResolvedValue(snapshotOf('connected'))
    await state.refreshStatuses()
    const known = state.statuses.value
    await state.refreshStatuses()
    expect(state.statuses.value).toBe(known)
    api.snapshot.mockResolvedValue(snapshotOf('failed'))
    await state.refreshStatuses()
    expect(state.statuses.value).not.toBe(known)
    expect(state.statuses.value[1].state).toBe('failed')
    wrapper.unmount()
  })
})
