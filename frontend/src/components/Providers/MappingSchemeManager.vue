<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Plus } from 'lucide-vue-next'
import BaseModal from '../common/BaseModal.vue'
import RuleTable from '../ModelMapping/RuleTable.vue'
import { providerTabs } from '../../constants/platforms'
import {
  deleteModelMappingScheme,
  fetchRouteProfile,
  setActiveModelMappingScheme,
  type RouteProfile,
} from '../../services/routing'
import {
  mappingCreatePath,
  mappingEditPath,
  schemeMapping,
  schemesOf,
} from '../../utils/modelMapping'
import { getErrorMessage } from '../../utils/errors'
import { showToast } from '../../utils/toast'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const profile = ref<RouteProfile | null>(null)
const loading = ref(true)
const busy = ref(false)
const loadError = ref('')
const selectedName = ref('')
const deleteTarget = ref('')
let loadSerial = 0

const platform = computed(() => String(route.params.platform ?? ''))
const providerID = computed(() => Number(route.params.providerId))
const provider = computed(() =>
  profile.value?.providers.find((item) => item.id === providerID.value) ?? null,
)
const schemes = computed(() => schemesOf(provider.value))
const activeScheme = computed(() =>
  schemes.value.find((scheme) => scheme.name === provider.value?.activeModelMappingScheme) ?? null,
)
const selectedScheme = computed(() =>
  schemes.value.find((scheme) => scheme.name === selectedName.value) ?? null,
)
const platformLabel = computed(() =>
  providerTabs.find((tab) => tab.id === platform.value)?.label ?? platform.value,
)
const activeName = computed(() => activeScheme.value?.name ?? '')

const ensureSelected = () => {
  if (schemes.value.some((scheme) => scheme.name === selectedName.value)) return
  selectedName.value = activeName.value || schemes.value[0]?.name || ''
}

const load = async () => {
  const serial = ++loadSerial
  loading.value = true
  loadError.value = ''
  try {
    const loaded = await fetchRouteProfile(platform.value)
    if (serial !== loadSerial) return
    profile.value = loaded
    if (!loaded.providers.some((item) => item.id === providerID.value)) {
      loadError.value = t('modelMapping.notFound')
    }
    ensureSelected()
  } catch (error) {
    if (serial === loadSerial) {
      loadError.value = getErrorMessage(error, t('modelMapping.loadFailed'))
    }
  } finally {
    if (serial === loadSerial) loading.value = false
  }
}

watch([platform, providerID], () => void load(), { immediate: true })
watch(schemes, ensureSelected)

const applyProfile = (saved: RouteProfile) => {
  profile.value = saved
  ensureSelected()
}

const activate = async (name: string) => {
  if (busy.value || !provider.value) return
  busy.value = true
  try {
    const saved = await setActiveModelMappingScheme(platform.value, providerID.value, name)
    applyProfile(saved)
    showToast(t('modelMapping.activated', { name }))
  } catch (error) {
    showToast(getErrorMessage(error, t('modelMapping.switchFailedHint')), 'error')
  } finally {
    busy.value = false
  }
}

const deactivate = async () => {
  if (busy.value || !activeName.value) return
  busy.value = true
  try {
    const saved = await setActiveModelMappingScheme(platform.value, providerID.value, '')
    applyProfile(saved)
    showToast(t('modelMapping.deactivated'))
  } catch (error) {
    showToast(getErrorMessage(error, t('modelMapping.switchFailedHint')), 'error')
  } finally {
    busy.value = false
  }
}

const confirmDelete = async () => {
  const name = deleteTarget.value
  if (!name || busy.value || name === activeName.value) return
  busy.value = true
  try {
    const saved = await deleteModelMappingScheme(platform.value, providerID.value, name)
    deleteTarget.value = ''
    applyProfile(saved)
    showToast(t('modelMapping.deleted'))
  } catch (error) {
    showToast(getErrorMessage(error, t('modelMapping.deleteFailed')), 'error')
  } finally {
    busy.value = false
  }
}

const openCreate = () => {
  void router.push(mappingCreatePath(platform.value, providerID.value))
}

const openCopy = () => {
  if (!selectedScheme.value) return
  void router.push(mappingCreatePath(platform.value, providerID.value, selectedScheme.value.name))
}

const openEdit = () => {
  if (!selectedScheme.value) return
  void router.push(mappingEditPath(platform.value, providerID.value, selectedScheme.value.name))
}

const schemeMeta = (name: string, count: number) => {
  const parts = [t('modelMapping.rulesCount', { count })]
  if (name === activeName.value) parts.push(t('modelMapping.active'))
  else if (name === selectedName.value) parts.push(t('modelMapping.selectedPreview'))
  return parts.join(' · ')
}
</script>

<template>
  <div class="cs-page mapping-manager-page">
    <nav class="mapping-breadcrumb" aria-label="Breadcrumb">
      <RouterLink :to="`/providers?platform=${encodeURIComponent(platform)}`">
        {{ t('shell.nav.providers') }}
      </RouterLink>
      <span>/</span>
      <span>{{ provider?.name || providerID }}</span>
      <span>/</span>
      <span>{{ platformLabel }}</span>
    </nav>

    <div class="cs-page-head">
      <div>
        <h1 class="cs-page-title">{{ t('modelMapping.title') }}</h1>
        <p class="cs-page-sub">{{ t('modelMapping.subtitle') }}</p>
      </div>
      <div class="cs-page-actions">
        <RouterLink class="cs-btn mapping-back-link" :to="`/providers?platform=${encodeURIComponent(platform)}`">
          {{ t('providersPage.actions.back') }}
        </RouterLink>
      </div>
    </div>

    <div v-if="loading" class="cs-empty">{{ t('common.loading') }}</div>
    <div v-else-if="loadError || !provider" class="cs-card cs-card-pad cs-empty" role="alert">
      {{ loadError || t('modelMapping.notFound') }}
    </div>

    <template v-else>
      <section class="mapping-active-banner" :class="{ 'mapping-active-banner--off': !activeScheme }">
        <div>
          <h2>
            {{ activeScheme ? t('modelMapping.activePrefix', { name: activeScheme.name }) : t('modelMapping.noMapping') }}
          </h2>
          <p>
            {{ activeScheme
              ? `${t('modelMapping.rulesCount', { count: Object.keys(activeScheme.mapping ?? {}).length })} · ${t('modelMapping.unmatchedPassThrough')}`
              : t('modelMapping.noMappingHint') }}
          </p>
        </div>
        <button v-if="activeScheme" type="button" class="cs-btn" :disabled="busy" @click="deactivate">
          {{ t('modelMapping.deactivate') }}
        </button>
      </section>

      <div class="mapping-manager-toolbar">
        <h2>{{ t('modelMapping.savedCount', { count: schemes.length }) }}</h2>
        <button type="button" class="cs-btn cs-btn--primary" :disabled="busy" @click="openCreate">
          <Plus :size="14" :stroke-width="2" />
          {{ t('modelMapping.newScheme') }}
        </button>
      </div>

      <div class="mapping-manager-grid">
        <div class="mapping-scheme-list">
          <button
            v-for="scheme in schemes"
            :key="scheme.name"
            type="button"
            class="mapping-scheme-list-item"
            :class="{ selected: scheme.name === selectedName }"
            @click="selectedName = scheme.name"
          >
            <span class="mapping-scheme-list-name">{{ scheme.name }}</span>
            <span class="mapping-scheme-list-meta">
              {{ schemeMeta(scheme.name, Object.keys(scheme.mapping ?? {}).length) }}
            </span>
          </button>
          <div v-if="schemes.length === 0" class="cs-empty">{{ t('modelMapping.emptySchemes') }}</div>
        </div>

        <section v-if="selectedScheme" class="cs-card mapping-scheme-preview">
          <div class="mapping-preview-head">
            <div class="mapping-preview-title">
              <h2>{{ selectedScheme.name }}</h2>
              <span>{{ selectedScheme.name === activeName ? t('modelMapping.active') : t('modelMapping.inactive') }}</span>
            </div>
            <div class="mapping-preview-actions">
              <button type="button" class="cs-btn" :disabled="busy" @click="openEdit">
                {{ t('modelMapping.edit') }}
              </button>
              <button type="button" class="cs-btn" :disabled="busy" @click="openCopy">
                {{ t('modelMapping.copy') }}
              </button>
              <button
                type="button"
                class="cs-btn mapping-delete-btn"
                :disabled="busy || selectedScheme.name === activeName"
                :title="selectedScheme.name === activeName ? t('modelMapping.activeDeleteHint') : t('modelMapping.delete')"
                @click="deleteTarget = selectedScheme.name"
              >
                {{ t('modelMapping.delete') }}
              </button>
            </div>
          </div>

          <RuleTable :rules="schemeMapping(selectedScheme)" />
          <p class="mapping-unmatched-hint">{{ t('modelMapping.unmatchedPreview') }}</p>
          <div v-if="selectedScheme.name !== activeName">
            <button type="button" class="cs-btn cs-btn--primary" :disabled="busy" @click="activate(selectedScheme.name)">
              {{ t('modelMapping.activate') }}
            </button>
          </div>
        </section>

        <section v-else class="cs-card cs-card-pad mapping-scheme-preview mapping-preview-empty">
          <p>{{ t('modelMapping.emptyPreview') }}</p>
          <button type="button" class="cs-btn cs-btn--primary" @click="openCreate">
            {{ t('modelMapping.newScheme') }}
          </button>
        </section>
      </div>

      <p class="mapping-manager-foot">{{ t('modelMapping.previewHint') }}</p>
    </template>

    <BaseModal
      :open="!!deleteTarget"
      :title="t('modelMapping.deleteTitle')"
      width="narrow"
      @close="deleteTarget = ''"
    >
      <p class="mapping-delete-confirm">
        {{ t('modelMapping.deleteBody', { name: deleteTarget }) }}
      </p>
      <template #footer>
        <button type="button" class="cs-btn" :disabled="busy" @click="deleteTarget = ''">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="cs-btn cs-btn--danger" :disabled="busy" @click="confirmDelete">
          {{ busy ? t('common.deleting') : t('modelMapping.delete') }}
        </button>
      </template>
    </BaseModal>
  </div>
</template>

<style scoped>
.mapping-manager-page {
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

.mapping-back-link {
  text-decoration: none;
}

.mapping-active-banner {
  display: flex;
  align-items: center;
  gap: 28px;
  padding: 20px;
  border-radius: 12px;
  background: var(--cs-brand-tint);
}

.mapping-active-banner--off {
  border: 1px solid var(--cs-border);
  background: var(--cs-surface);
}

.mapping-active-banner h2 {
  margin: 0;
  color: var(--cs-text);
  font-size: 16px;
  font-weight: 500;
  line-height: 25px;
}

.mapping-active-banner p {
  margin: 10px 0 0;
  color: var(--cs-text-secondary);
  font-size: 12px;
  line-height: 19px;
}

.mapping-manager-toolbar {
  display: flex;
  align-items: center;
  gap: 24px;
}

.mapping-manager-toolbar h2 {
  margin: 0;
  color: var(--cs-text);
  font-size: 16px;
  font-weight: 500;
  line-height: 25px;
}

.mapping-manager-grid {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr);
  gap: 24px;
  align-items: start;
}

.mapping-scheme-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.mapping-scheme-list-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
  width: 100%;
  padding: 17px;
  border: 1px solid var(--cs-border);
  border-radius: 10px;
  background: var(--cs-surface);
  color: var(--cs-text);
  font-family: inherit;
  text-align: left;
  cursor: pointer;
}

.mapping-scheme-list-item.selected {
  border-color: var(--cs-brand);
  background: var(--cs-brand-tint);
}

.mapping-scheme-list-name {
  font-size: 15px;
  font-weight: 500;
  line-height: 23px;
}

.mapping-scheme-list-meta {
  color: var(--cs-text-secondary);
  font-size: 12px;
  line-height: 19px;
}

.mapping-scheme-preview {
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-width: 0;
  padding: 24px;
}

.mapping-preview-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
}

.mapping-preview-title {
  display: flex;
  align-items: baseline;
  gap: 12px;
  min-width: 0;
}

.mapping-preview-title h2 {
  margin: 0;
  color: var(--cs-text);
  font-size: 20px;
  font-weight: 500;
  line-height: 31px;
  overflow-wrap: anywhere;
}

.mapping-preview-title span {
  flex-shrink: 0;
  color: var(--cs-text-secondary);
  font-size: 12px;
}

.mapping-preview-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.mapping-delete-btn {
  color: var(--cs-danger);
}

.mapping-unmatched-hint,
.mapping-manager-foot,
.mapping-preview-empty p {
  margin: 0;
  color: var(--cs-text-secondary);
  font-size: 12px;
  line-height: 19px;
}

.mapping-preview-empty {
  align-items: flex-start;
}

.mapping-delete-confirm {
  margin: 0 0 14px;
  color: var(--cs-text);
  font-size: 13px;
  line-height: 22px;
  overflow-wrap: anywhere;
}

@media (max-width: 980px) {
  .mapping-manager-grid {
    grid-template-columns: 240px minmax(0, 1fr);
  }

  .mapping-preview-head {
    flex-direction: column;
  }
}

@media (max-width: 720px) {
  .mapping-manager-grid {
    grid-template-columns: 1fr;
  }
}
</style>
