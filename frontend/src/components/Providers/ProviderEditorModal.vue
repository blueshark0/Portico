<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseModal from '../common/BaseModal.vue'
import BaseInput from '../common/BaseInput.vue'
import { providerTabs } from '../../constants/platforms'
import { showToast } from '../../utils/toast'
import type { ProviderForm } from './useProviderDirectory'

const props = defineProps<{
  state: {
    open: boolean
    editing: boolean
    platform: string
    providerId: number
    form: ProviderForm
    errors: { name: string; apiUrl: string; proxyUrl: string }
    saving: boolean
  }
}>()

const emit = defineEmits<{ (e: 'close'): void; (e: 'submit'): void }>()

const { t } = useI18n()

const title = computed(() =>
  props.state.editing ? t('providersPage.editor.titleEdit') : t('providersPage.editor.titleNew'),
)

const copyKey = async () => {
  if (!props.state.form.apiKey) return
  try {
    await navigator.clipboard.writeText(props.state.form.apiKey)
    showToast(t('providersPage.messages.copied'))
  } catch (error) {
    console.error('copy api key failed', error)
  }
}
</script>

<template>
  <BaseModal
    :open="state.open"
    :title="title"
    :subtitle="t('providersPage.editor.subtitle')"
    @close="emit('close')"
  >
    <div class="pedit">
      <div class="pedit-field">
        <label class="cs-label">{{ t('providersPage.editor.name') }}</label>
        <BaseInput
          v-model="state.form.name"
          :disabled="state.editing"
          :placeholder="t('providersPage.editor.namePlaceholder')"
        />
        <p v-if="state.errors.name" class="cs-field-error">{{ state.errors.name }}</p>
        <p v-else-if="state.editing" class="cs-field-hint">{{ t('providersPage.editor.nameLocked') }}</p>
      </div>

      <div v-if="!state.editing" class="pedit-field">
        <label class="cs-label">{{ t('providersPage.editor.platform') }}</label>
        <select v-model="state.platform" class="cs-select">
          <option v-for="tab in providerTabs" :key="tab.id" :value="tab.id">{{ tab.label }}</option>
        </select>
      </div>

      <div class="pedit-field">
        <label class="cs-label">{{ t('providersPage.editor.apiUrl') }}</label>
        <BaseInput v-model="state.form.apiUrl" class="cs-mono" placeholder="https://api.example.com" />
        <p v-if="state.errors.apiUrl" class="cs-field-error">{{ state.errors.apiUrl }}</p>
      </div>

      <div class="pedit-field">
        <label class="cs-label">{{ t('providersPage.editor.officialSite') }}</label>
        <BaseInput v-model="state.form.officialSite" class="cs-mono" placeholder="https://" />
      </div>

      <div class="pedit-field">
        <label class="cs-label">{{ t('providersPage.editor.apiKey') }}</label>
        <BaseInput v-model="state.form.apiKey" class="cs-mono" placeholder="sk-..." />
        <p class="cs-field-hint">{{ t('providersPage.editor.keyHint') }}</p>
        <div>
          <button type="button" class="cs-btn" :disabled="!state.form.apiKey" @click="copyKey">
            {{ t('providersPage.editor.copyKey') }}
          </button>
        </div>
      </div>
    </div>

    <template #footer>
      <button type="button" class="cs-btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button type="button" class="cs-btn cs-btn--primary" :disabled="state.saving" @click="emit('submit')">
        {{ state.saving ? t('common.saving') : t('providersPage.editor.save') }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.pedit {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding-bottom: 10px;
}

.pedit-field :deep(.cs-input) {
  font-family: var(--cs-font-mono);
}
</style>
