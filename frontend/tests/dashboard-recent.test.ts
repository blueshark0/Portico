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
vi.mock('../src/services/mcp', () => ({
  fetchMcpStatuses: async () => [], fetchMcpServers: async () => [],
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
