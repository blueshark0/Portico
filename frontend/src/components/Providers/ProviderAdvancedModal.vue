<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import BaseModal from '../common/BaseModal.vue'
import BaseInput from '../common/BaseInput.vue'
import ModelWhitelistEditor from '../common/ModelWhitelistEditor.vue'
import ModelMappingEditor from '../common/ModelMappingEditor.vue'
import RequestBodyReplacementEditor from '../common/RequestBodyReplacementEditor.vue'
import type { ProviderForm } from './useProviderDirectory'

const props = defineProps<{
  state: {
    open: boolean
    entryKey: string
    tab: 'connection' | 'body'
    draft: ProviderForm
    saving: boolean
  }
}>()

const emit = defineEmits<{ (e: 'close'): void; (e: 'submit'): void }>()

const { t } = useI18n()

const setTab = (tab: 'connection' | 'body') => {
  props.state.tab = tab
}
</script>

<template>
  <BaseModal
    :open="state.open"
    :title="t('providersPage.advanced.title')"
    :subtitle="t('providersPage.advanced.subtitle')"
    width="wide"
    @close="emit('close')"
  >
    <div class="padv">
      <div class="cs-seg padv-tabs">
        <button
          type="button"
          :class="{ active: state.tab === 'connection' }"
          @click="setTab('connection')"
        >
          {{ t('providersPage.advanced.tabConnection') }}
        </button>
        <button
          type="button"
          :class="{ active: state.tab === 'body' }"
          @click="setTab('body')"
        >
          {{ t('providersPage.advanced.tabBody') }}
        </button>
      </div>

      <template v-if="state.tab === 'connection'">
        <section class="padv-section">
          <h3 class="padv-heading">{{ t('providersPage.advanced.proxyMode') }}</h3>
          <div class="cs-seg">
            <button
              type="button"
              :class="{ active: state.draft.proxyMode === '' }"
              @click="state.draft.proxyMode = ''"
            >
              {{ t('providersPage.advanced.proxyInherit') }}
            </button>
            <button
              type="button"
              :class="{ active: state.draft.proxyMode === 'custom' }"
              @click="state.draft.proxyMode = 'custom'"
            >
              {{ t('providersPage.advanced.proxyCustom') }}
            </button>
            <button
              type="button"
              :class="{ active: state.draft.proxyMode === 'direct' }"
              @click="state.draft.proxyMode = 'direct'"
            >
              {{ t('providersPage.advanced.proxyDirect') }}
            </button>
          </div>
          <div v-if="state.draft.proxyMode === 'custom'" class="padv-proxy-url">
            <label class="cs-label">{{ t('providersPage.advanced.proxyUrl') }}</label>
            <BaseInput v-model="state.draft.proxyUrl" class="cs-mono" placeholder="http://127.0.0.1:7890" />
          </div>
        </section>

        <section class="padv-section padv-inline">
          <div>
            <h3 class="padv-heading padv-heading--inline">{{ t('providersPage.advanced.skipTls') }}</h3>
            <p class="cs-field-hint">{{ t('providersPage.advanced.skipTlsHint') }}</p>
          </div>
          <label class="cs-switch">
            <input type="checkbox" v-model="state.draft.insecureSkipVerify" />
            <span></span>
          </label>
        </section>

        <section class="padv-section">
          <ModelWhitelistEditor v-model="state.draft.supportedModels" />
        </section>

        <section class="padv-section">
          <ModelMappingEditor v-model="state.draft.modelMapping" />
        </section>
      </template>

      <template v-else>
        <section class="padv-section">
          <RequestBodyReplacementEditor v-model="state.draft.requestBodyReplacements" />
        </section>
      </template>

      <p class="cs-field-hint">{{ t('providersPage.advanced.scopeHint') }}</p>
    </div>

    <template #footer>
      <button type="button" class="cs-btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button type="button" class="cs-btn cs-btn--primary" :disabled="state.saving" @click="emit('submit')">
        {{ state.saving ? t('common.saving') : t('providersPage.advanced.save') }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.padv {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding-bottom: 10px;
}

.padv-tabs {
  align-self: flex-start;
}

.padv-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.padv-heading {
  margin: 0;
  font-size: 13px;
  font-weight: 700;
  color: var(--cs-text);
}

.padv-heading--inline {
  margin-bottom: 4px;
}

.padv-inline {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.padv-proxy-url {
  margin-top: 4px;
}
</style>
