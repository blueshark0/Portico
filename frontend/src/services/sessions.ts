import { ListByPlatform, Unbind } from '../../bindings/portico/internal/interfaces/wails/sessionfacade'
import { ListSessionRequestLogs } from '../../bindings/portico/internal/interfaces/wails/observabilityfacade'
import type { SessionSummary } from '../../bindings/portico/internal/interfaces/wails/models'
import type { RequestLog } from './logs'

// ListByPlatform 现在返回「绑定 + 用量聚合」的 SessionSummary，
// 它覆盖原 SessionBinding 的全部字段，这里保留旧名以免改动各消费方。
export type SessionBinding = SessionSummary

export const fetchPlatformSessions = async (platform: string): Promise<SessionBinding[]> => {
  return ListByPlatform(platform)
}

export const unbindSession = async (platform: string, sessionId: string): Promise<void> => {
  await Unbind(platform, sessionId)
}

export const fetchSessionRequestLogs = async (platform: string, sessionId: string): Promise<RequestLog[]> => {
  if (!platform.trim() || !sessionId.trim()) return []
  return ListSessionRequestLogs(platform, sessionId)
}
