<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseModal from '../common/BaseModal.vue'
import BaseInput from '../common/BaseInput.vue'

const props = defineProps<{
  open: boolean
  busy: boolean
  passphrase: string
  snapshotTime: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm', passphrase: string): void
}>()

const { t } = useI18n()

const input = ref('')

watch(
  () => props.open,
  (open) => {
    if (open) input.value = props.passphrase
  },
)
</script>

<template>
  <BaseModal
    :open="open"
    :title="t('settingsPage.restore.title')"
    :subtitle="t('settingsPage.restore.subtitle')"
    @close="emit('close')"
  >
    <div class="wr">
      <div class="wr-warning">
        <p class="wr-warning-title">{{ t('settingsPage.restore.warningTitle') }}</p>
        <p class="wr-warning-body">{{ t('settingsPage.restore.warningBody') }}</p>
        <p class="wr-warning-meta">{{ t('settingsPage.restore.snapshotAt', { time: snapshotTime || '—' }) }}</p>
        <p class="wr-warning-hint">{{ t('settingsPage.restore.overwriteHint') }}</p>
      </div>

      <div>
        <label class="cs-label">{{ t('settingsPage.restore.passphrase') }}</label>
        <BaseInput
          v-model="input"
          type="password"
          :placeholder="t('settingsPage.restore.passphrasePlaceholder')"
          @keydown.enter="emit('confirm', input)"
        />
        <p class="cs-field-hint">{{ t('settingsPage.restore.passphraseHint') }}</p>
      </div>
    </div>

    <template #footer>
      <button type="button" class="cs-btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button
        type="button"
        class="cs-btn cs-btn--primary"
        :disabled="busy || !input.trim()"
        @click="emit('confirm', input)"
      >
        {{ busy ? t('settingsPage.restore.restoring') : t('settingsPage.restore.confirm') }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.wr {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding-bottom: 8px;
}

.wr-warning {
  background: var(--cs-danger-tint);
  border-radius: 10px;
  padding: 14px 15px;
}

.wr-warning-title {
  margin: 0;
  font-size: 12px;
  font-weight: 500;
  color: var(--cs-danger);
}

.wr-warning-body {
  margin: 10px 0 0;
  font-size: 12px;
  color: var(--cs-text);
}

.wr-warning-meta {
  margin: 8px 0 0;
  font-size: 11px;
  color: var(--cs-text-secondary);
}

.wr-warning-hint {
  margin: 6px 0 0;
  font-size: 11px;
  color: var(--cs-text-secondary);
}
</style>
