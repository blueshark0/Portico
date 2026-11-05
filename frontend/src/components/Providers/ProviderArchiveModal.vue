<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import BaseModal from '../common/BaseModal.vue'

defineProps<{
  open: boolean
  name: string
  busy: boolean
}>()

const emit = defineEmits<{ (e: 'close'): void; (e: 'confirm'): void }>()

const { t } = useI18n()
</script>

<template>
  <BaseModal
    :open="open"
    :title="t('providersPage.archiveConfirm.title')"
    :subtitle="t('providersPage.archiveConfirm.subtitle')"
    width="narrow"
    @close="emit('close')"
  >
    <div class="parchive">
      <div class="parchive-info">
        <p class="parchive-info-title">{{ t('providersPage.archiveConfirm.warningTitle') }}</p>
        <p class="parchive-info-body">{{ t('providersPage.archiveConfirm.warningBody') }}</p>
      </div>
      <p class="parchive-name">{{ t('providersPage.archiveConfirm.target', { name }) }}</p>
    </div>

    <template #footer>
      <button type="button" class="cs-btn" :disabled="busy" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button type="button" class="cs-btn cs-btn--primary" :disabled="busy" @click="emit('confirm')">
        {{ busy ? t('providersPage.archiveConfirm.confirming') : t('providersPage.archiveConfirm.confirm') }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.parchive {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding-bottom: 8px;
}

.parchive-info {
  background: var(--cs-brand-tint);
  border: 1px solid var(--cs-brand-tint-border);
  border-radius: 10px;
  padding: 14px 13px;
}

.parchive-info-title {
  margin: 0;
  font-size: 12px;
  font-weight: 500;
  color: var(--cs-brand);
}

.parchive-info-body {
  margin: 10px 0 0;
  font-size: 10px;
  line-height: 1.6;
  color: var(--cs-text);
}

.parchive-name {
  margin: 0;
  font-size: 12px;
  color: var(--cs-text);
  overflow-wrap: anywhere;
}
</style>
