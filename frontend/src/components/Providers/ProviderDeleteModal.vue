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
    :title="t('providersPage.delete.title')"
    :subtitle="t('providersPage.delete.subtitle')"
    width="narrow"
    @close="emit('close')"
  >
    <div class="pdel">
      <div class="pdel-danger">
        <p class="pdel-danger-title">{{ t('providersPage.delete.warningTitle') }}</p>
        <p class="pdel-danger-body">{{ t('providersPage.delete.warningBody') }}</p>
      </div>
      <p class="pdel-name">{{ t('providersPage.delete.target', { name }) }}</p>
    </div>

    <template #footer>
      <button type="button" class="cs-btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button type="button" class="cs-btn cs-btn--danger" :disabled="busy" @click="emit('confirm')">
        {{ busy ? t('common.deleting') : t('providersPage.delete.confirm') }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.pdel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding-bottom: 8px;
}

.pdel-danger {
  background: var(--cs-danger-tint);
  border-radius: 10px;
  padding: 14px 13px;
}

.pdel-danger-title {
  margin: 0;
  font-size: 12px;
  font-weight: 500;
  color: var(--cs-danger);
}

.pdel-danger-body {
  margin: 10px 0 0;
  font-size: 10px;
  color: var(--cs-text);
}

.pdel-name {
  margin: 0;
  font-size: 12px;
  color: var(--cs-text);
}
</style>
