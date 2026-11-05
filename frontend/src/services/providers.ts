import { TestProvider } from '../../bindings/portico/internal/interfaces/wails/routingfacade'
import type { ProviderProbeResult } from '../../bindings/portico/internal/routing/application/models'
import type { ProviderRecord } from './routing'

export type { ProviderRecord } from './routing'
export type { ProviderProbeResult }

// 供应商连通性探测：以后端代理/TLS 设置对 Base URL 做一次 GET（不携带凭证）。
export const testProviderConnection = async (provider: ProviderRecord): Promise<ProviderProbeResult> => {
  return TestProvider(provider)
}
