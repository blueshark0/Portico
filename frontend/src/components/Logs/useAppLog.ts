import { onMounted, onUnmounted, ref, watch } from 'vue'
import {
  fetchAppLogTail,
  type AppLogEntry,
  type AppLogLevel,
} from '../../services/appLog'

const FOLLOW_INTERVAL_MS = 3000
const KEYWORD_DEBOUNCE_MS = 300
// 跟随模式长时间运行时限制内存占用；超出后仍保留最新日志。
const MAX_KEPT_ENTRIES = 2000

export const useAppLog = () => {
  const entries = ref<AppLogEntry[]>([])
  const level = ref<AppLogLevel>('')
  const keyword = ref('')
  const following = ref(true)
  const loading = ref(false)
  const truncated = ref(false)
  let nextOffset = 0
  let loadPromise: Promise<void> | null = null
  let followTimer: number | undefined
  let keywordTimer: number | undefined

  const load = (options: { reset?: boolean } = {}) => {
    if (loadPromise) {
      return loadPromise
    }
    loadPromise = (async () => {
      loading.value = true
      try {
        const chunk = await fetchAppLogTail({
          offset: options.reset ? 0 : nextOffset,
          level: level.value,
          keyword: keyword.value,
        })
        if (options.reset || chunk.reset) {
          entries.value = []
          truncated.value = chunk.truncated
        }
        const fresh = chunk.entries ?? []
        if (fresh.length) {
          const merged = [...entries.value, ...fresh]
          if (merged.length > MAX_KEPT_ENTRIES) {
            truncated.value = true
          }
          entries.value = merged.slice(-MAX_KEPT_ENTRIES)
        }
        nextOffset = chunk.next_offset
      } catch (error) {
        console.error('failed to load app log', error)
      } finally {
        loading.value = false
        loadPromise = null
      }
    })()
    return loadPromise
  }

  // 过滤条件变化/手动刷新：等在途增量请求结束后，从尾部窗口重新读取。
  const reload = async () => {
    if (loadPromise) {
      try {
        await loadPromise
      } catch {
        // load 内部已记录错误
      }
    }
    await load({ reset: true })
  }

  const stopFollow = () => {
    if (followTimer) {
      clearInterval(followTimer)
      followTimer = undefined
    }
  }

  const startFollow = () => {
    stopFollow()
    followTimer = window.setInterval(() => {
      if (!document.hidden) {
        void load()
      }
    }, FOLLOW_INTERVAL_MS)
  }

  watch(following, (enabled) => {
    if (enabled) {
      void load()
      startFollow()
    } else {
      stopFollow()
    }
  })

  watch(level, () => {
    void reload()
  })

  watch(keyword, () => {
    if (keywordTimer) {
      clearTimeout(keywordTimer)
    }
    keywordTimer = window.setTimeout(() => {
      keywordTimer = undefined
      void reload()
    }, KEYWORD_DEBOUNCE_MS)
  })

  onMounted(() => {
    void load({ reset: true })
    if (following.value) {
      startFollow()
    }
  })

  onUnmounted(() => {
    stopFollow()
    if (keywordTimer) {
      clearTimeout(keywordTimer)
      keywordTimer = undefined
    }
  })

  return {
    entries,
    following,
    keyword,
    level,
    loading,
    manualRefresh: reload,
    truncated,
  }
}
