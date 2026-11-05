import { Disable, Enable, GetStatus } from '../../bindings/portico/internal/interfaces/wails/platformproxyfacade'
import type { Status as ClaudeProxyStatus } from '../../bindings/portico/internal/platformproxy/domain/models'
import type { ProviderTab } from '../constants/platforms'

type Platform = ProviderTab

export type { ClaudeProxyStatus }

export const fetchProxyStatus = async (platform: Platform): Promise<ClaudeProxyStatus> => {
  return GetStatus(platform)
}

export const enableProxy = async (platform: Platform): Promise<void> => {
  await Enable(platform)
}

export const disableProxy = async (platform: Platform): Promise<void> => {
  await Disable(platform)
}
