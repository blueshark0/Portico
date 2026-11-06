<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import BaseModal from '../common/BaseModal.vue'
import RuleTable from './RuleTable.vue'
import {
  fetchRouteProfile,
  setActiveModelMappingScheme,
  type RouteProfile,
} from '../../services/routing'
import {
  activeMappingOf,
  firstMappingPair,
  mappingManagerPath,
  schemesOf,
} from '../../utils/modelMapping'
import { getErrorMessage } from '../../utils/errors'
import { showToast } from '../../utils/toast'

const props = withDefaults(
  defineProps<{
    open: boolean
    platform: string
    providerId: number
    platformLabel: string
    providerName: string
    returnLabel?: string
  }>(),
  { returnLabel: '' },
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'updated', profile: RouteProfile): void
}>()

const { t } = useI18n()
const router = useRouter()
const profile = ref<RouteProfile | null>(null)
const loading = ref(false)
const busy = ref(false)
const actionError = ref('')
const success = ref<{ name: string; rules: Record<string, string>; previous: string } | null>(null)
let loadSerial = 0

const provider = computed(() =>
  profile.value?.providers.find((item) => item.id === props.providerId) ?? null,
)
const schemes = computed(() => schemesOf(provider.value))
const activeName = computed(() => provider.value?.activeModelMappingScheme ?? '')
const currentName = computed(() => activeName.value || t('modelMapping.noMapping'))
const subtitle = computed(() => `${props.platformLabel} · ${props.providerName}`)

const load = async () => {
  const serial = ++loadSerial
  loading.value = true
  actionError.value = ''
  success.value = null
  try {
    const loaded = await fetchRouteProfile(props.platform)
    if (serial !== loadSerial) return
    profile.value = loaded
    if (!loaded.providers.some((item) => item.id === props.providerId)) {
      actionError.value = t('modelMapping.notFound')
    }
  } catch (error) {
    if (serial === loadSerial) {
      actionError.value = getErrorMessage(error, t('modelMapping.loadFailed'))
    }
  } finally {
    if (serial === loadSerial) loading.value = false
  }
}

watch(
  () => [props.open, props.platform, props.providerId] as const,
  ([open]) => {
    if (open) {
      void load()
    } else {
      loadSerial++
      loading.value = false
      busy.value = false
      success.value = null
      actionError.value = ''
    }
  },
  { immediate: true },
)

const applyScheme = async (name: string) => {
  if (busy.value || !provider.value) return
  busy.value = true
  actionError.value = ''
  const previous = activeName.value
  try {
    const saved = await setActiveModelMappingScheme(props.platform, props.providerId, name)
    profile.value = saved
    emit('updated', saved)
    const savedProvider = saved.providers.find((item) => item.id === props.providerId)
    success.value = {
      name,
      rules: name ? activeMappingOf(savedProvider) : {},
      previous,
    }
  } catch (error) {
    actionError.value = getErrorMessage(error, t('modelMapping.switchFailedHint'))
  } finally {
    busy.value = false
  }
}

const undo = async () => {
  if (!success.value || busy.value) return
  busy.value = true
  actionError.value = ''
  try {
    const saved = await setActiveModelMappingScheme(
      props.platform,
      props.providerId,
      success.value.previous,
    )
    profile.value = saved
    success.value = null
    emit('updated', saved)
    showToast(t('modelMapping.undoSuccess'))
  } catch (error) {
    actionError.value = getErrorMessage(error, t('modelMapping.switchFailedHint'))
  } finally {
    busy.value = false
  }
}

const close = () => {
  if (!busy.value) emit('close')
}

const manage = () => {
  emit('close')
  void router.push(mappingManagerPath(props.platform, props.providerId))
}

const schemeSummary = (scheme: (typeof schemes.value)[number]) => {
  const count = Object.keys(scheme.mapping ?? {}).length
  if (scheme.name === activeName.value) {
    return `${t('modelMapping.rulesCount', { count })} · ${t('modelMapping.active')}`
  }
  const pair = firstMappingPair(scheme)
  const prefix = pair ? `${pair[0]} → ${pair[1]} · ` : ''
  return `${prefix}${t('modelMapping.rulesCount', { count })}`
}
</script>

<template>
  <BaseModal
    :open="open"
    :title="success ? t('modelMapping.successTitle') : t('modelMapping.switchTitle')"
    :subtitle="subtitle"
    width="wide"
    @close="close"
  >
    <div class="mapping-switch-modal">
      <div v-if="loading" class="cs-empty">{{ t('common.loading') }}</div>

      <section v-else-if="success" class="mapping-switch-success">
        <span class="mapping-applied-label">{{ t('modelMapping.applied') }}</span>
        <h3>{{ success.name || t('modelMapping.noMapping') }}</h3>
        <p>{{ t('modelMapping.successHint') }}</p>
        <RuleTable v-if="success.name" :rules="success.rules" compact />
        <p v-else class="mapping-success-no-rules">{{ t('modelMapping.noMappingHint') }}</p>
      </section>

      <template v-else-if="provider">
        <h3 class="mapping-select-title">{{ t('modelMapping.selectTitle') }}</h3>
        <p class="mapping-current-label">{{ t('modelMapping.current', { name: currentName }) }}</p>
        <div class="mapping-scheme-options" :aria-busy="busy">
          <button
            v-for="scheme in schemes"
            :key="scheme.name"
            type="button"
            class="mapping-scheme-option"
            :class="{ active: scheme.name === activeName }"
            :disabled="busy"
            @click="applyScheme(scheme.name)"
          >
            <span class="mapping-option-name">{{ scheme.name }}</span>
            <span class="mapping-option-summary">{{ schemeSummary(scheme) }}</span>
          </button>
          <button
            type="button"
            class="mapping-scheme-option"
            :class="{ active: !activeName }"
            :disabled="busy"
            @click="applyScheme('')"
          >
            <span class="mapping-option-name">{{ t('modelMapping.noMapping') }}</span>
            <span class="mapping-option-summary">{{ t('modelMapping.noMappingHint') }}</span>
          </button>
        </div>
        <p class="mapping-select-hint">{{ t('modelMapping.selectApplyHint') }}</p>
      </template>

      <section v-if="actionError" class="mapping-switch-error" role="alert">
        <strong v-if="provider && !loading && !success">
          {{ t('modelMapping.switchFailed', { name: currentName }) }}
        </strong>
        <p>{{ actionError }}</p>
        <p v-if="!success">{{ t('modelMapping.switchFailedHint') }}</p>
      </section>
    </div>

    <template #footer>
      <template v-if="success">
        <button type="button" class="cs-btn" :disabled="busy" @click="undo">
          {{ t('modelMapping.undo') }}
        </button>
        <button type="button" class="cs-btn cs-btn--primary" :disabled="busy" @click="close">
          {{ returnLabel || t('modelMapping.backDashboard') }}
        </button>
      </template>
      <template v-else>
        <button
          type="button"
          class="cs-btn"
          :disabled="!provider || busy"
          @click="manage"
        >
          {{ t('modelMapping.manage') }}
        </button>
        <button type="button" class="cs-btn" :disabled="busy" @click="close">
          {{ t('common.cancel') }}
        </button>
      </template>
    </template>
  </BaseModal>
</template>

<style scoped>
.mapping-switch-modal {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding-bottom: 10px;
}

.mapping-select-title {
  margin: 0;
  color: var(--cs-text);
  font-size: 20px;
  line-height: 31px;
}

.mapping-current-label,
.mapping-select-hint {
  margin: -6px 0 0;
  color: var(--cs-text-secondary);
  font-size: 12px;
  line-height: 19px;
}

.mapping-scheme-options {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.mapping-scheme-option {
  display: flex;
  flex-direction: column;
  gap: 5px;
  align-items: flex-start;
  width: 100%;
  padding: 14px 16px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: var(--cs-canvas);
  color: var(--cs-text);
  font-family: inherit;
  text-align: left;
  cursor: pointer;
}

.mapping-scheme-option:hover:not(:disabled) {
  border-color: var(--cs-brand-tint-border);
}

.mapping-scheme-option.active {
  background: var(--cs-brand-tint);
}

.mapping-scheme-option:disabled {
  opacity: 0.6;
  cursor: wait;
}

.mapping-option-name {
  font-size: 15px;
  font-weight: 500;
  line-height: 23px;
}

.mapping-option-summary {
  max-width: 100%;
  overflow: hidden;
  color: var(--cs-text-secondary);
  font-size: 12px;
  line-height: 19px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mapping-switch-success {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 10px 4px;
}

.mapping-applied-label {
  color: var(--cs-success);
  font-size: 12px;
}

.mapping-switch-success h3 {
  margin: 0;
  color: var(--cs-text);
  font-size: 24px;
  line-height: 32px;
}

.mapping-switch-success p {
  margin: 0;
  color: var(--cs-text-secondary);
  font-size: 13px;
  line-height: 20px;
}

.mapping-switch-success :deep(.mapping-rule-table) {
  padding: 10px;
  border: 1px solid var(--cs-border);
}

.mapping-switch-error {
  padding: 14px 16px;
  border-radius: 10px;
  background: var(--cs-danger-tint);
  color: var(--cs-text);
}

.mapping-switch-error strong {
  font-size: 13px;
  font-weight: 500;
}

.mapping-switch-error p {
  margin: 6px 0 0;
  color: var(--cs-text-secondary);
  font-size: 11px;
  line-height: 18px;
}
</style>
