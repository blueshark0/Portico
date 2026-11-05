<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppLog } from './useAppLog'
import type { AppLogEntry } from '../../services/appLog'

const { t } = useI18n()
const { entries, following, keyword, level, loading, manualRefresh, truncated } = useAppLog()

const listEl = ref<HTMLElement | null>(null)

const levelOptions = [
  { value: '', labelKey: 'logsPage.appLog.levels.all' },
  { value: 'INFO', label: 'INFO' },
  { value: 'WARN', label: 'WARN' },
  { value: 'ERROR', label: 'ERROR' },
  { value: 'DEBUG', label: 'DEBUG' },
  { value: 'none', labelKey: 'logsPage.appLog.levels.none' },
]

const isStructuredEntry = (entry: AppLogEntry) => Boolean(entry.time || entry.source || entry.level)

const messageLines = (message: string) => message.split('\n')
const firstMessageLine = (message: string) => messageLines(message)[0] ?? ''
const continuationLines = (message: string) => messageLines(message).slice(1)

const levelClass = (value: string) => {
  switch (value) {
    case 'ERROR':
      return 'app-log-level-pill--error'
    case 'WARN':
      return 'app-log-level-pill--warn'
    case 'DEBUG':
      return 'app-log-level-pill--debug'
    case 'INFO':
      return 'app-log-level-pill--info'
    default:
      return 'app-log-level-pill--empty'
  }
}

const nearBottom = () => {
  const el = listEl.value
  if (!el) return true
  return el.scrollHeight - el.scrollTop - el.clientHeight < 40
}

// 跟随模式或初始加载时吸底；用户上翻浏览时不打断当前位置。
watch(
  () => entries.value,
  async (next, prev) => {
    const stick = following.value || nearBottom() || (prev?.length ?? 0) === 0
    if (!stick || next.length === 0) return
    await nextTick()
    if (listEl.value) {
      listEl.value.scrollTop = listEl.value.scrollHeight
    }
  },
)
</script>

<template>
  <div class="logs-app-view">
    <div class="cs-card app-log-toolbar">
      <span class="app-log-toolbar-label">{{ t('logsPage.appLog.levelLabel') }}</span>
      <select v-model="level" class="cs-select app-log-level-select" :aria-label="t('logsPage.appLog.levelLabel')">
        <option v-for="option in levelOptions" :key="option.value" :value="option.value">
          {{ option.label ?? t(option.labelKey) }}
        </option>
      </select>
      <input
        v-model="keyword"
        type="search"
        class="cs-input app-log-search"
        :placeholder="t('logsPage.appLog.keywordPlaceholder')"
      />
      <div class="app-log-follow">
        <span>{{ t('logsPage.appLog.follow') }}</span>
        <label class="cs-switch">
          <input v-model="following" type="checkbox" :aria-label="t('logsPage.appLog.follow')" />
          <span />
        </label>
      </div>
      <button type="button" class="cs-btn cs-btn--primary app-log-refresh" :disabled="loading" @click="manualRefresh">
        {{ t('logsPage.appLog.refresh') }}
      </button>
    </div>

    <p class="app-log-hint" :class="{ 'app-log-hint--hidden': !truncated }" :aria-hidden="!truncated">
      {{ truncated ? t('logsPage.appLog.truncatedHint') : '' }}
    </p>

    <div ref="listEl" class="cs-card app-log-list">
      <template v-if="entries.length">
        <template v-for="(entry, index) in entries" :key="`${entry.time}-${entry.source}-${index}`">
          <div v-if="isStructuredEntry(entry)" class="app-log-entry">
            <div class="app-log-line">
              <span class="app-log-time cs-mono" :title="entry.time">{{ entry.time }}</span>
              <span class="app-log-source cs-mono" :title="entry.source">{{ entry.source }}</span>
              <span class="app-log-level">
                <span class="app-log-level-pill" :class="levelClass(entry.level)">
                  {{ entry.level || '—' }}
                </span>
              </span>
              <span class="app-log-message">{{ firstMessageLine(entry.message) }}</span>
            </div>
            <div
              v-for="(line, lineIndex) in continuationLines(entry.message)"
              :key="lineIndex"
              class="app-log-continuation"
            >
              {{ line }}
            </div>
          </div>
          <div v-else class="app-log-orphan">{{ entry.message }}</div>
        </template>
      </template>
      <div v-else class="cs-empty app-log-empty">
        {{ loading ? t('logsPage.appLog.loading') : t('logsPage.appLog.empty') }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.logs-app-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
  margin-top: 13px;
}

.app-log-toolbar {
  min-height: 58px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 7px 15px;
}

.app-log-toolbar-label {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 500;
  color: var(--cs-text-secondary);
}

.app-log-level-select {
  width: 160px;
  flex-shrink: 0;
  min-height: 38px;
}

.app-log-search {
  flex: 1 1 280px;
  max-width: 565px;
  min-width: 180px;
  min-height: 38px;
}

.app-log-follow {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
  font-size: 11px;
  color: var(--cs-text-secondary);
  white-space: nowrap;
}

.app-log-refresh {
  width: 76px;
  height: 42px;
  border-radius: 21px;
  flex-shrink: 0;
}

.app-log-hint {
  margin: 0;
  min-height: 12px;
  font-size: 11px;
  line-height: 12px;
  color: var(--cs-text-secondary);
}

.app-log-hint--hidden {
  visibility: hidden;
}

.app-log-list {
  height: clamp(280px, calc(100vh - 426px), 474px);
  border-radius: 12px;
  padding: 8px 12px;
  overflow-y: auto;
  font-size: 10px;
  line-height: 20px;
}

.app-log-entry {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.app-log-entry + .app-log-entry,
.app-log-entry + .app-log-orphan,
.app-log-orphan + .app-log-entry,
.app-log-orphan + .app-log-orphan {
  margin-top: 3px;
}

.app-log-line {
  display: grid;
  grid-template-columns: 150px minmax(100px, 130px) 46px minmax(0, 1fr);
  column-gap: 8px;
  align-items: start;
  min-height: 23px;
}

.app-log-time,
.app-log-source {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.app-log-time {
  color: var(--cs-text-secondary);
}

.app-log-source {
  color: var(--cs-brand);
}

.app-log-level {
  display: flex;
  align-items: flex-start;
}

.app-log-level-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 43px;
  height: 20px;
  border-radius: 5px;
  font-size: 10px;
  font-weight: 500;
  line-height: 1;
  white-space: nowrap;
}

.app-log-level-pill--info {
  background: var(--cs-info-tint);
  color: var(--cs-info-text);
}

.app-log-level-pill--warn {
  background: var(--cs-warning-tint);
  color: var(--cs-warning);
}

.app-log-level-pill--error {
  background: var(--cs-danger-tint);
  color: var(--cs-danger);
}

.app-log-level-pill--debug {
  background: var(--cs-brand-tint);
  color: var(--cs-brand);
}

.app-log-level-pill--empty {
  background: transparent;
  color: var(--cs-text-secondary);
}

.app-log-message,
.app-log-continuation,
.app-log-orphan {
  color: var(--cs-text);
  white-space: pre-wrap;
  word-break: break-word;
  overflow-wrap: anywhere;
}

.app-log-continuation,
.app-log-orphan {
  line-height: 18px;
}

.app-log-empty {
  min-height: 100%;
  justify-content: center;
}

@media (max-width: 980px) {
  .app-log-toolbar {
    flex-wrap: wrap;
    padding-bottom: 9px;
  }

  .app-log-search {
    order: 5;
    flex-basis: 100%;
    max-width: none;
  }

  .app-log-follow {
    margin-left: 0;
  }

  .app-log-refresh {
    margin-left: auto;
  }

  .app-log-list {
    height: clamp(280px, calc(100vh - 460px), 474px);
  }
}

@media (max-width: 720px) {
  .app-log-line {
    grid-template-columns: 138px 96px 44px minmax(0, 1fr);
    column-gap: 6px;
  }
}
</style>
