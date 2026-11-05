<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseModal from '../common/BaseModal.vue'
import type { RemoteSnapshotMeta } from '../../services/configSync'

const props = defineProps<{
  open: boolean
  remotePath: string
  meta: RemoteSnapshotMeta | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'restore'): void
}>()

const { t } = useI18n()

const updatedAtText = computed(() => {
  const value = props.meta?.updatedAt
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
})
</script>

<template>
  <BaseModal
    :open="open"
    :title="t('settingsPage.versions.title')"
    :subtitle="t('settingsPage.versions.subtitle')"
    @close="emit('close')"
  >
    <div class="wv">
      <div class="wv-path">
        <p class="wv-path-label">{{ t('settingsPage.versions.remotePath') }}</p>
        <p class="wv-path-value cs-mono">{{ remotePath || '/Portico' }}</p>
      </div>

      <div v-if="meta?.exists" class="wv-snapshot wv-snapshot--latest">
        <p class="wv-snapshot-time cs-mono">{{ updatedAtText }}</p>
        <p class="wv-snapshot-sub">
          {{ t('settingsPage.versions.latest', { device: meta.deviceName || t('settingsPage.versions.unknown'), version: meta.appVersion || t('settingsPage.versions.unknown') }) }}
        </p>
        <button type="button" class="cs-btn wv-restore" @click="emit('restore')">
          {{ t('settingsPage.versions.restore') }}
        </button>
      </div>
      <div v-else class="cs-empty">{{ t('settingsPage.versions.empty') }}</div>

      <p class="wv-note">{{ t('settingsPage.versions.note') }}</p>
    </div>

    <template #footer>
      <button type="button" class="cs-btn cs-btn--primary" @click="emit('close')">
        {{ t('common.close') }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.wv {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding-bottom: 8px;
}

.wv-path {
  background: var(--cs-field);
  border: 1px solid var(--cs-border);
  border-radius: 9px;
  padding: 8px 12px;
}

.wv-path-label {
  margin: 0;
  font-size: 10px;
  color: var(--cs-text-secondary);
}

.wv-path-value {
  margin: 6px 0 0;
  font-size: 12px;
  font-weight: 500;
  color: var(--cs-text);
}

.wv-snapshot {
  position: relative;
  border: 1px solid var(--cs-border);
  border-radius: 9px;
  padding: 13px 14px;
}

.wv-snapshot--latest {
  background: var(--cs-brand-tint);
  border-color: var(--cs-border);
}

.wv-snapshot-time {
  margin: 0;
  font-size: 14px;
  font-weight: 500;
  color: var(--cs-text);
}

.wv-snapshot-sub {
  margin: 10px 0 0;
  font-size: 11px;
  color: var(--cs-text-secondary);
}

.wv-restore {
  position: absolute;
  top: 24px;
  right: 14px;
}

.wv-note {
  margin: 0;
  font-size: 11px;
  color: var(--cs-warning);
}
</style>
