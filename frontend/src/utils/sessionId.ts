// 绑定和解绑仍使用原始值，这里只在展示时提取 Claude Code 的会话 UUID。
// 新版 metadata.user_id 是含 session_id 的 JSON 对象，旧版则是
// user_<hash>_account__session_<uuid>；Codex/Gemini 直接是 UUID。
const claudeSessionMarker = '_session_'

export const displaySessionId = (sessionId: string): string => {
  if (!sessionId) return ''

  const trimmed = sessionId.trim()
  if (trimmed.startsWith('{')) {
    try {
      const metadata = JSON.parse(trimmed) as { session_id?: unknown } | null
      if (typeof metadata?.session_id === 'string' && metadata.session_id) {
        return metadata.session_id
      }
    } catch {
      // 非 JSON 的历史数据继续按旧字符串格式处理。
    }
  }

  const markerIndex = sessionId.lastIndexOf(claudeSessionMarker)
  return markerIndex >= 0 ? sessionId.slice(markerIndex + claudeSessionMarker.length) : sessionId
}

export const shortSessionId = (sessionId: string): string => {
  const readable = displaySessionId(sessionId)
  return readable.length <= 8 ? readable : readable.slice(0, 8)
}
