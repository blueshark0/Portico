import { expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { useDashboard } from '../src/components/Dashboard/useDashboard'

const sessions = vi.hoisted(() => ({ recent: vi.fn(), platform: vi.fn() }))
vi.mock('../src/services/sessions', () => ({
  fetchRecentSessions: sessions.recent,
  fetchPlatformSessions: sessions.platform,
}))
vi.mock('../src/services/routing', () => ({ fetchRouteProfile: async () => null }))
vi.mock('../src/services/claudeSettings', () => ({
  fetchProxyStatus: async () => ({ enabled: false }), enableProxy: vi.fn(), disableProxy: vi.fn(),
}))
const mcp = vi.hoisted(() => ({ statuses: vi.fn(async (): Promise<unknown[]> => []) }))
vi.mock('../src/services/mcp', () => ({
  fetchMcpStatuses: mcp.statuses, fetchMcpServers: async () => [],
  fetchMcpEndpoint: async () => 'http://127.0.0.1:18100/mcp', fetchInjectionStatus: async () => false,
}))

it('loads recent metadata once without requesting platform session usage', async () => {
  const timestamp = '2026-11-06T10:00:00Z'
  sessions.recent.mockResolvedValueOnce([
    { platform: 'codex', session_id: 'recent', provider_name: 'provider', last_success_at: timestamp },
  ])
  let state!: ReturnType<typeof useDashboard>
  const wrapper = mount(defineComponent({ setup() {
    state = useDashboard()
    return () => h('div')
  } }))
  try {
    await flushPromises()
    expect(sessions.recent).toHaveBeenCalledExactlyOnceWith(5)
    expect(sessions.platform).not.toHaveBeenCalled()
    expect(state.recentSessions.value).toEqual([
      { platform: 'codex', platformLabel: 'Codex', providerName: 'provider', sessionId: 'recent', lastSuccessAt: timestamp },
    ])
    expect(state.clients.value.map((client) => client.platform)).toEqual(['claude', 'codex', 'gemini'])
    expect(state.loading.value).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

function mountDashboard() {
  let state!: ReturnType<typeof useDashboard>
  const wrapper = mount(defineComponent({ setup() {
    state = useDashboard()
    return () => h('div')
  } }))
  return { state, wrapper }
}

it('keeps MCP and clients visible when recent sessions fail', async () => {
  const logged = vi.spyOn(console, 'error').mockImplementation(() => {})
  sessions.recent.mockRejectedValueOnce(new Error('session db unavailable'))
  mcp.statuses.mockResolvedValueOnce([{ serverId: 1, state: 'connected', toolCount: 3 }])
  const { state, wrapper } = mountDashboard()
  await flushPromises()
  expect(state.totalToolCount.value).toBe(3)
  expect(state.clients.value).toHaveLength(3)
  expect(state.recentSessions.value).toEqual([])
  expect(state.loading.value).toBe(false)
  expect(logged).toHaveBeenCalledOnce()
  wrapper.unmount()
})

it('does not wait for MCP or sessions before showing clients', async () => {
  let release!: () => void
  const blocked = new Promise<unknown[]>((resolve) => { release = () => resolve([]) })
  mcp.statuses.mockReturnValueOnce(blocked)
  sessions.recent.mockResolvedValueOnce([])
  const { state, wrapper } = mountDashboard()
  await flushPromises()
  expect(state.clients.value).toHaveLength(3)
  expect(state.loading.value).toBe(true)
  release()
  await flushPromises()
  expect(state.loading.value).toBe(false)
  wrapper.unmount()
})
