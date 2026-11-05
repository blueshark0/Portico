<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { DirectoryEntry } from './useProviderDirectory'
import { getErrorMessage } from '../../utils/errors'
import { showToast } from '../../utils/toast'

withDefaults(
  defineProps<{
    entry: DirectoryEntry
    title: string
    statusTone: 'success' | 'warning' | 'danger' | 'brand' | 'muted'
    statusLabel: string
    metaLabel: string
    metaValue: string
    note?: string
    feedback?: string
  }>(),
  { note: '', feedback: '' },
)

const { t } = useI18n()

const copyApiKey = async (value: string) => {
  try {
    await navigator.clipboard.writeText(value)
    showToast(t('providersPage.messages.copied'))
  } catch (error) {
    showToast(getErrorMessage(error, t('providersPage.messages.copyFailed')), 'error')
  }
}
</script>

<template>
  <div class="cs-card cs-card-pad prov-detail">
    <h2 class="cs-card-title">{{ title }}</h2>
    <p class="cs-section-sub">{{ entry.platformLabel }}</p>

    <p class="prov-detail-name">{{ entry.provider.name }}</p>
    <span class="cs-pill" :class="`cs-pill--${statusTone}`">{{ statusLabel }}</span>

    <p class="prov-detail-label">{{ t('providersPage.detail.baseUrl') }}</p>
    <p class="prov-detail-value cs-mono">{{ entry.provider.apiUrl }}</p>

    <hr class="cs-divider" />

    <p class="prov-detail-label">{{ t('providersPage.detail.apiKey') }}</p>
    <div class="prov-key-row">
      <span class="prov-key-value cs-mono">{{ entry.provider.apiKey || '—' }}</span>
      <button
        type="button"
        class="cs-btn"
        :disabled="!entry.provider.apiKey"
        @click="copyApiKey(entry.provider.apiKey)"
      >
        {{ t('providersPage.detail.copy') }}
      </button>
    </div>

    <hr class="cs-divider" />

    <p class="prov-detail-label">{{ metaLabel }}</p>
    <p class="prov-detail-probe">{{ metaValue }}</p>

    <div class="prov-detail-actions">
      <slot name="actions" />
    </div>

    <p v-if="feedback" class="prov-detail-feedback">{{ feedback }}</p>
    <p v-if="note" class="prov-detail-note">{{ note }}</p>
    <slot name="footer" />
  </div>
</template>

<style scoped>
.prov-detail {
  display: flex;
  flex-direction: column;
  gap: 12px;
  position: sticky;
  top: 0;
}

.prov-detail-name {
  margin: 10px 0 0;
  font-size: 14px;
  font-weight: 500;
  color: var(--cs-text);
  overflow-wrap: anywhere;
}

.prov-detail .cs-pill {
  align-self: flex-start;
}

.prov-detail-label {
  margin: 6px 0 0;
  font-size: 10px;
  color: var(--cs-text-secondary);
}

.prov-detail-value {
  margin: 0;
  font-size: 11px;
  color: var(--cs-text);
  word-break: break-all;
  user-select: text;
}

.prov-key-row {
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--cs-field);
  border: 1px solid var(--cs-border);
  border-radius: var(--cs-r-field);
  padding: 2px 2px 2px 10px;
}

.prov-key-value {
  flex: 1;
  min-width: 0;
  font-size: 10px;
  color: var(--cs-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  user-select: text;
}

.prov-detail-probe {
  margin: 0;
  font-size: 12px;
  color: var(--cs-text);
  overflow-wrap: anywhere;
}

.prov-detail-actions {
  display: flex;
  gap: 9px;
  margin-top: 6px;
  flex-wrap: wrap;
}

.prov-detail-feedback,
.prov-detail-note {
  margin: 4px 0 0;
  font-size: 10px;
  color: var(--cs-text-secondary);
  overflow-wrap: anywhere;
}

@media (max-width: 1100px) {
  .prov-detail {
    position: static;
  }
}
</style>
