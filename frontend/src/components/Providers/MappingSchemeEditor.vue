<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import BaseInput from '../common/BaseInput.vue'
import RuleEditor from '../ModelMapping/RuleEditor.vue'
import type { MappingRuleError, MappingRuleRow } from '../ModelMapping/types'
import { providerTabs } from '../../constants/platforms'
import {
  fetchRouteProfile,
  saveModelMappingScheme,
  type ProviderRecord,
  type RouteProfile,
} from '../../services/routing'
import { mappingManagerPath, schemesOf } from '../../utils/modelMapping'
import { getErrorMessage } from '../../utils/errors'
import { showToast } from '../../utils/toast'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const profile = ref<RouteProfile | null>(null)
const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const name = ref('')
const rules = ref<MappingRuleRow[]>([])
const nameError = ref('')
const ruleErrors = ref<MappingRuleError[]>([])
const originalName = ref('')
const initialSnapshot = ref('')
const saved = ref(false)
let loadSerial = 0

const platform = computed(() => String(route.params.platform ?? ''))
const providerID = computed(() => Number(route.params.providerId))
const creating = computed(() => route.path.endsWith('/new'))
const sourceName = computed(() => {
  const value = creating.value ? route.query.copy : route.query.scheme
  return typeof value === 'string' ? value : ''
})
const copying = computed(() => creating.value && !!sourceName.value)
const provider = computed<ProviderRecord | null>(() =>
  profile.value?.providers.find((item) => item.id === providerID.value) ?? null,
)
const platformLabel = computed(() =>
  providerTabs.find((tab) => tab.id === platform.value)?.label ?? platform.value,
)
const isActive = computed(() =>
  !creating.value && !!originalName.value && provider.value?.activeModelMappingScheme === originalName.value,
)
const pageTitle = computed(() =>
  creating.value ? t('modelMapping.createTitle') : t('modelMapping.editTitle'),
)
const pageSubtitle = computed(() => {
  if (creating.value) return t('modelMapping.createHint')
  if (isActive.value) return t('modelMapping.editActiveHint')
  return t('modelMapping.editInactiveHint', { name: originalName.value })
})
const dirty = computed(() =>
  !saved.value && !!initialSnapshot.value &&
  JSON.stringify({ name: name.value, rules: rules.value }) !== initialSnapshot.value,
)

const copyName = (source: string) => {
  const base = `${source} ${t('modelMapping.copySuffix')}`
  const existing = new Set(schemesOf(provider.value).map((scheme) => scheme.name.toLocaleLowerCase()))
  if (!existing.has(base.toLocaleLowerCase())) return base
  let suffix = 2
  while (existing.has(`${base} ${suffix}`.toLocaleLowerCase())) suffix++
  return `${base} ${suffix}`
}

const load = async () => {
  const serial = ++loadSerial
  loading.value = true
  loadError.value = ''
  saved.value = false
  initialSnapshot.value = ''
  nameError.value = ''
  ruleErrors.value = []
  try {
    const loaded = await fetchRouteProfile(platform.value)
    if (serial !== loadSerial) return
    profile.value = loaded
    const loadedProvider = loaded.providers.find((item) => item.id === providerID.value)
    if (!loadedProvider) {
      loadError.value = t('modelMapping.notFound')
      return
    }
    const source = (loadedProvider.modelMappingSchemes ?? []).find((scheme) => scheme.name === sourceName.value)
    if (!creating.value || copying.value) {
      if (!source) {
        loadError.value = t('modelMapping.notFound')
        return
      }
    }
    originalName.value = creating.value ? '' : sourceName.value
    name.value = copying.value ? copyName(sourceName.value) : (source?.name ?? '')
    rules.value = Object.entries(source?.mapping ?? {})
      .map(([key, value]) => ({ key, value: value ?? '' }))
      .sort((left, right) => left.key.localeCompare(right.key))
    initialSnapshot.value = JSON.stringify({ name: name.value, rules: rules.value })
  } catch (error) {
    if (serial === loadSerial) {
      loadError.value = getErrorMessage(error, t('modelMapping.loadFailed'))
    }
  } finally {
    if (serial === loadSerial) loading.value = false
  }
}

watch([platform, providerID, sourceName], () => void load(), { immediate: true })

const validate = () => {
  nameError.value = ''
  ruleErrors.value = rules.value.map(() => ({}))
  let valid = true
  const trimmedName = name.value.trim()
  if (!trimmedName) {
    nameError.value = t('modelMapping.nameRequired')
    valid = false
  } else {
    const duplicate = schemesOf(provider.value).some((scheme) =>
      scheme.name.toLocaleLowerCase() === trimmedName.toLocaleLowerCase() &&
      scheme.name !== originalName.value,
    )
    if (duplicate) {
      nameError.value = t('modelMapping.nameDuplicate')
      valid = false
    }
  }

  const seen = new Map<string, number>()
  rules.value.forEach((row, index) => {
    const key = row.key.trim()
    const value = row.value.trim()
    if (!key) {
      ruleErrors.value[index].key = t('modelMapping.modelRequired')
      valid = false
    } else if (seen.has(key)) {
      ruleErrors.value[index].key = t('modelMapping.duplicateModel')
      valid = false
    } else {
      seen.set(key, index)
    }
    if (!value) {
      ruleErrors.value[index].value = t('modelMapping.targetRequired')
      valid = false
    }
  })
  return valid
}

const save = async (activate: boolean) => {
  if (saving.value || !provider.value || !validate()) return
  saving.value = true
  const mapping: Record<string, string> = {}
  for (const row of rules.value) mapping[row.key.trim()] = row.value.trim()
  try {
    await saveModelMappingScheme(
      platform.value,
      providerID.value,
      originalName.value,
      { name: name.value.trim(), mapping },
      activate,
    )
    saved.value = true
    showToast(t('modelMapping.saved'))
    await router.push(mappingManagerPath(platform.value, providerID.value))
  } catch (error) {
    showToast(getErrorMessage(error, t('modelMapping.saveFailed')), 'error')
  } finally {
    saving.value = false
  }
}

const cancel = () => {
  void router.push(mappingManagerPath(platform.value, providerID.value))
}

const confirmDiscard = () => !dirty.value || window.confirm(t('modelMapping.unsaved'))
onBeforeRouteLeave(confirmDiscard)
onBeforeRouteUpdate(confirmDiscard)
</script>

<template>
  <div class="cs-page mapping-edit-page">
    <nav class="mapping-breadcrumb" aria-label="Breadcrumb">
      <RouterLink :to="mappingManagerPath(platform, providerID)">{{ t('shell.nav.providers') }}</RouterLink>
      <span>/</span>
      <span>{{ provider?.name || providerID }}</span>
      <span>/</span>
      <span>{{ platformLabel }}</span>
    </nav>

    <div class="cs-page-head">
      <div>
        <h1 class="cs-page-title">{{ pageTitle }}</h1>
        <p class="cs-page-sub">{{ pageSubtitle }}</p>
      </div>
    </div>

    <div v-if="loading" class="cs-empty">{{ t('common.loading') }}</div>
    <div v-else-if="loadError || !provider" class="cs-card cs-card-pad cs-empty" role="alert">
      {{ loadError || t('modelMapping.notFound') }}
    </div>

    <template v-else>
      <section class="cs-card mapping-edit-form">
        <div class="mapping-name-field">
          <label class="mapping-field-label" for="mapping-scheme-name">{{ t('modelMapping.schemeName') }}</label>
          <BaseInput
            id="mapping-scheme-name"
            v-model="name"
            :disabled="saving"
            :placeholder="t('modelMapping.namePlaceholder')"
          />
          <p v-if="nameError" class="cs-field-error">{{ nameError }}</p>
        </div>

        <h2 class="mapping-rules-title">{{ t('modelMapping.rules') }}</h2>
        <p class="mapping-rules-hint">{{ t('modelMapping.rulesHint') }}</p>
        <RuleEditor v-model="rules" :errors="ruleErrors" :disabled="saving" />

        <div class="mapping-unmatched-field">
          <label class="mapping-field-label" for="mapping-unmatched">{{ t('modelMapping.unmatchedLabel') }}</label>
          <select id="mapping-unmatched" class="cs-select" disabled>
            <option>{{ t('modelMapping.passThrough') }}</option>
          </select>
        </div>
      </section>

      <div class="mapping-edit-actions">
        <button type="button" class="cs-btn" :disabled="saving" @click="cancel">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="cs-btn cs-btn--primary" :disabled="saving" @click="save(isActive)">
          {{ saving ? t('common.saving') : (isActive ? t('modelMapping.saveApply') : t('modelMapping.saveScheme')) }}
        </button>
        <button v-if="!isActive" type="button" class="cs-btn" :disabled="saving" @click="save(true)">
          {{ t('modelMapping.saveAndActivate') }}
        </button>
      </div>

      <p class="mapping-edit-note">
        {{ isActive ? t('modelMapping.editActiveHint') : t('modelMapping.previewHint') }}
      </p>
    </template>
  </div>
</template>

<style scoped>
.mapping-edit-page {
  gap: 22px;
}

.mapping-breadcrumb {
  display: flex;
  gap: 8px;
  color: var(--cs-text-secondary);
  font-size: 12px;
  line-height: 19px;
}

.mapping-breadcrumb a {
  color: inherit;
  text-decoration: none;
}

.mapping-breadcrumb a:hover {
  color: var(--cs-brand);
}

.mapping-edit-form {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 24px;
}

.mapping-field-label {
  display: block;
  margin: 0 0 8px;
  color: var(--cs-text);
  font-size: 13px;
  font-weight: 500;
  line-height: 20px;
}

.mapping-name-field :deep(.cs-input) {
  min-height: 46px;
  padding: 11px 12px;
  font-size: 14px;
}

.mapping-rules-title {
  margin: 4px 0 -6px;
  color: var(--cs-text);
  font-size: 16px;
  font-weight: 500;
  line-height: 25px;
}

.mapping-rules-hint {
  margin: -6px 0 0;
  color: var(--cs-text-secondary);
  font-size: 12px;
  line-height: 19px;
}

.mapping-unmatched-field {
  max-width: 320px;
  margin-top: 2px;
}

.mapping-unmatched-field .cs-select {
  font-size: 13px;
}

.mapping-edit-actions {
  display: flex;
  gap: 12px;
}

.mapping-edit-note {
  margin: 0;
  color: var(--cs-text-secondary);
  font-size: 12px;
  line-height: 19px;
}
</style>
