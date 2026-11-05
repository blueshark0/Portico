import { TailAppLog } from '../../bindings/portico/internal/interfaces/wails/applogfacade'
import type {
  AppLogChunk,
  AppLogEntry,
} from '../../bindings/portico/internal/observability/domain/models'

export type { AppLogChunk, AppLogEntry }

export const APP_LOG_LEVELS = ['', 'INFO', 'WARN', 'ERROR', 'DEBUG', 'none'] as const
export type AppLogLevel = (typeof APP_LOG_LEVELS)[number]

export const DEFAULT_APP_LOG_MAX_LINES = 500
export const MAX_APP_LOG_LINES = 2000

type AppLogTailQuery = {
  offset?: number
  maxLines?: number
  level?: AppLogLevel
  keyword?: string
}

export const normalizeAppLogLevel = (value?: string): AppLogLevel => {
  return APP_LOG_LEVELS.includes(value as AppLogLevel) ? (value as AppLogLevel) : ''
}

export const fetchAppLogTail = async (query: AppLogTailQuery = {}): Promise<AppLogChunk> => {
  const offset = Math.max(0, Math.floor(query.offset ?? 0))
  const requestedLines = Math.floor(query.maxLines ?? DEFAULT_APP_LOG_MAX_LINES)
  const maxLines = Math.min(MAX_APP_LOG_LINES, Math.max(1, requestedLines))
  return TailAppLog(offset, maxLines, normalizeAppLogLevel(query.level), query.keyword?.trim() ?? '')
}
