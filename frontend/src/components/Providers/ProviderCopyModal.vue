<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowLeftRight, TriangleAlert } from 'lucide-vue-next'
import BaseModal from '../common/BaseModal.vue'
import { providerTabs, type ProviderTab } from '../../constants/platforms'
import type { DirectoryEntry } from './useProviderDirectory'

const props = defineProps<{
  open: boolean
  source: DirectoryEntry | null
}>()

const emit = defineEmits<{ (e: 'close'): void; (e: 'confirm', target: ProviderTab): void }>()

const { t } = useI18n()

const target = ref<ProviderTab>('codex')

const targetOptions = computed(() =>
  providerTabs.filter((tab) => tab.id !== props.source?.platform),
)

watch(
  () => [props.open, props.source?.platform],
  () => {
    if (props.open) {
      target.value = targetOptions.value[0]?.id ?? 'codex'
    }
  },
  { immediate: true },
)

const swapDirection = () => {
  const options = targetOptions.value
  const index = options.findIndex((tab) => tab.id === target.value)
  target.value = options[(index + 1) % options.length]?.id ?? target.value
}

const targetLabel = computed(
  () => providerTabs.find((tab) => tab.id === target.value)?.label ?? '',
)

const RULES = ['baseUrl', 'apiKey', 'model', 'headers', 'proxy', 'whitelist'] as const
</script>

<template>
  <BaseModal
    :open="open"
    :title="t('providersPage.copy.title')"
    :subtitle="t('providersPage.copy.subtitle')"
    width="wide"
    @close="emit('close')"
  >
    <div v-if="source" class="pcopy">
      <div class="pcopy-lanes">
        <div class="pcopy-lane">
          <p class="pcopy-lane-title">{{ source.platformLabel }}</p>
          <p class="pcopy-lane-name">{{ source.provider.name }}</p>
          <span class="cs-pill cs-pill--success pcopy-lane-pill">{{ t('providersPage.copy.sourceReadonly') }}</span>
        </div>
        <button type="button" class="pcopy-swap" :title="t('providersPage.copy.swap')" @click="swapDirection">
          <ArrowLeftRight :size="18" :stroke-width="2" />
        </button>
        <div class="pcopy-lane">
          <p class="pcopy-lane-title">{{ targetLabel }}</p>
          <p class="pcopy-lane-name">{{ t('providersPage.copy.newDraft') }}</p>
          <span class="cs-pill cs-pill--warning pcopy-lane-pill">{{ t('providersPage.copy.pending') }}</span>
        </div>
      </div>

      <h3 class="pcopy-heading">{{ t('providersPage.copy.rulesTitle') }}</h3>
      <p class="cs-field-hint">{{ t('providersPage.copy.rulesHint') }}</p>

      <div class="pcopy-table">
        <div class="pcopy-table-head">
          <span>{{ t('providersPage.copy.colField') }}</span>
          <span>{{ t('providersPage.copy.colAction') }}</span>
          <span>{{ t('providersPage.copy.colStatus') }}</span>
        </div>
        <div v-for="rule in RULES" :key="rule" class="pcopy-table-row">
          <span class="pcopy-field">{{ t(`providersPage.copy.rules.${rule}.field`) }}</span>
          <span>{{ t(`providersPage.copy.rules.${rule}.action`) }}</span>
          <span :class="{ 'pcopy-warn': rule === 'baseUrl' || rule === 'apiKey' }">
            {{ t(`providersPage.copy.rules.${rule}.status`) }}
          </span>
        </div>
      </div>

      <div class="pcopy-warning">
        <p class="pcopy-warning-title">
          <TriangleAlert :size="13" :stroke-width="2" />
          {{ t('providersPage.copy.warnTitle') }}
        </p>
        <p class="pcopy-warning-body">{{ t('providersPage.copy.warnBody') }}</p>
      </div>

      <p class="cs-field-hint">{{ t('providersPage.copy.note') }}</p>
    </div>

    <template #footer>
      <button type="button" class="cs-btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button type="button" class="cs-btn cs-btn--primary" @click="emit('confirm', target)">
        {{ t('providersPage.copy.confirm') }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.pcopy {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding-bottom: 8px;
}

.pcopy-lanes {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 56px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
}

.pcopy-lane {
  position: relative;
  background: var(--cs-field);
  border: 1px solid var(--cs-border);
  border-radius: 10px;
  padding: 12px 13px;
  min-height: 76px;
}

.pcopy-lane-title {
  margin: 0;
  font-size: 12px;
  font-weight: 500;
  color: var(--cs-text);
}

.pcopy-lane-name {
  margin: 8px 0 0;
  font-size: 11px;
  color: var(--cs-text-secondary);
}

.pcopy-lane-pill {
  position: absolute;
  top: 12px;
  right: 12px;
}

.pcopy-swap {
  width: 56px;
  height: 44px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 12px;
  background: var(--cs-brand-tint);
  color: var(--cs-brand);
  cursor: pointer;
}

.pcopy-heading {
  margin: 6px 0 0;
  font-size: 14px;
  font-weight: 700;
  color: var(--cs-text);
}

.pcopy-table {
  border: 1px solid var(--cs-border);
  border-radius: 9px;
  overflow: hidden;
}

.pcopy-table-head,
.pcopy-table-row {
  display: grid;
  grid-template-columns: 1.2fr 1.2fr 1fr;
  gap: 12px;
  padding: 8px 12px;
  font-size: 10px;
  color: var(--cs-text);
}

.pcopy-table-head {
  background: var(--cs-field);
  color: var(--cs-text-secondary);
  font-weight: 500;
}

.pcopy-table-row {
  border-top: 1px solid var(--cs-border);
}

.pcopy-field {
  font-weight: 500;
}

.pcopy-warn {
  color: var(--cs-warning);
}

.pcopy-warning {
  background: var(--cs-warning-tint);
  border: 1px solid var(--cs-warning-tint-border);
  border-radius: 9px;
  padding: 10px 11px;
}

.pcopy-warning-title {
  margin: 0;
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  font-weight: 500;
  color: var(--cs-warning);
}

.pcopy-warning-body {
  margin: 8px 0 0;
  font-size: 10px;
  color: var(--cs-text);
}
</style>
