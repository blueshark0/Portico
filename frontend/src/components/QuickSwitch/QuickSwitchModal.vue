<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Search, ArrowUp, ArrowDown, CornerDownLeft, ExternalLink } from 'lucide-vue-next'
import BaseModal from '../common/BaseModal.vue'
import { providerTabs, type ProviderTab } from '../../constants/platforms'
import { fetchRouteProfile, saveRouteProfile, type RouteProfile } from '../../services/routing'
import type { ProviderRecord } from '../../services/providers'
import { getErrorMessage } from '../../utils/errors'
import { showToast } from '../../utils/toast'
import { useQuickSwitch } from '../../composables/useQuickSwitch'
import brandClaude from '../../assets/icons/brand-claude.svg'
import brandCodex from '../../assets/icons/brand-codex.svg'
import brandGemini from '../../assets/icons/brand-gemini.svg'

const BRAND_ICONS: Record<ProviderTab, string> = {
  claude: brandClaude,
  codex: brandCodex,
  gemini: brandGemini,
}

const { t } = useI18n()
const router = useRouter()
const { quickSwitchOpen, quickSwitchPlatform, closeQuickSwitch, notifyProfileChanged } =
  useQuickSwitch()

const profiles = ref<Partial<Record<ProviderTab, RouteProfile>>>({})
const platform = ref<ProviderTab>('claude')
const keyword = ref('')
const activeIndex = ref(0)
const applying = ref(false)
const searchInput = ref<HTMLInputElement | null>(null)

const loadProfiles = async () => {
  const entries = await Promise.all(
    providerTabs.map(async (tab) => [tab.id, await fetchRouteProfile(tab.id)] as const),
  )
  const next: Partial<Record<ProviderTab, RouteProfile>> = {}
  for (const [id, profile] of entries) {
    next[id] = profile
  }
  profiles.value = next
}

watch(quickSwitchOpen, async (open) => {
  if (!open) return
  keyword.value = ''
  activeIndex.value = 0
  const preferred = providerTabs.find((tab) => tab.id === quickSwitchPlatform.value)
  platform.value = preferred?.id ?? 'claude'
  await loadProfiles()
  await nextTick()
  searchInput.value?.focus()
})

const activeProfile = computed(() => profiles.value[platform.value])

const currentProviderName = computed(() => {
  const profile = activeProfile.value
  if (!profile || profile.defaultProviderId == null) return t('quickSwitch.noCurrent')
  return (
    profile.providers?.find((p) => p.id === profile.defaultProviderId)?.name ??
    t('quickSwitch.noCurrent')
  )
})

const platformOptions = computed(() =>
  providerTabs.map((tab) => ({ id: tab.id, label: tab.label })),
)

const hostOf = (apiUrl: string) => {
  try {
    return new URL(apiUrl).host
  } catch {
    return apiUrl
  }
}

const modelOf = (provider: ProviderRecord) => {
  const models = Object.keys(provider.supportedModels ?? {}).filter(
    (key) => provider.supportedModels?.[key] !== false,
  )
  return models[0] ?? ''
}

const visibleProviders = computed(() => {
  const all = (activeProfile.value?.providers ?? []).filter((p) => !p.archived)
  const query = keyword.value.trim().toLowerCase()
  if (!query) return all
  return all.filter((p) => {
    const haystack = `${p.name} ${p.apiUrl} ${modelOf(p)}`.toLowerCase()
    return haystack.includes(query)
  })
})

const isCurrent = (provider: ProviderRecord) =>
  activeProfile.value?.defaultProviderId === provider.id

const moveActive = (delta: number) => {
  const count = visibleProviders.value.length
  if (count === 0) return
  activeIndex.value = (activeIndex.value + delta + count) % count
}

const applyProvider = async (provider: ProviderRecord) => {
  const profile = activeProfile.value
  if (!profile || applying.value) return
  applying.value = true
  try {
    await saveRouteProfile({ ...profile, defaultProviderId: provider.id })
    notifyProfileChanged()
    showToast(
      t('quickSwitch.applied', {
        platform: platformOptions.value.find((o) => o.id === platform.value)?.label,
        name: provider.name,
      }),
    )
    closeQuickSwitch()
  } catch (error) {
    showToast(getErrorMessage(error, t('quickSwitch.applyFailed')), 'error')
  } finally {
    applying.value = false
  }
}

const applyActive = () => {
  const provider = visibleProviders.value[activeIndex.value]
  if (provider) void applyProvider(provider)
}

const openAdvanced = () => {
  const target = platform.value
  closeQuickSwitch()
  void router.push({ path: '/providers', query: { platform: target } })
}

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    moveActive(1)
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    moveActive(-1)
  } else if (event.key === 'Enter') {
    event.preventDefault()
    applyActive()
  }
}
</script>

<template>
  <BaseModal
    :open="quickSwitchOpen"
    :title="t('quickSwitch.title')"
    :subtitle="t('quickSwitch.subtitle')"
    width="default"
    @close="closeQuickSwitch"
  >
    <div class="qs" @keydown="onKeydown">
      <label class="cs-label">{{ t('quickSwitch.currentClient') }}</label>
      <div class="qs-client">
        <img :src="BRAND_ICONS[platform]" :alt="platform" class="qs-client-brand" />
        <select v-model="platform" class="cs-select qs-client-select">
          <option v-for="opt in platformOptions" :key="opt.id" :value="opt.id">
            {{ opt.label }}
          </option>
        </select>
        <span class="qs-client-current">{{ t('quickSwitch.currentPrefix') }}{{ currentProviderName }}</span>
      </div>

      <div class="qs-search-wrap">
        <Search :size="14" :stroke-width="2" class="qs-search-icon" />
        <input
          ref="searchInput"
          v-model="keyword"
          class="cs-input qs-search"
          :placeholder="t('quickSwitch.searchPlaceholder')"
          @input="activeIndex = 0"
        />
      </div>

      <label class="cs-label qs-list-label">{{ t('quickSwitch.available') }}</label>
      <div class="qs-list">
        <button
          v-for="(provider, index) in visibleProviders"
          :key="provider.id"
          type="button"
          class="qs-option"
          :class="{ active: index === activeIndex }"
          @mouseenter="activeIndex = index"
          @click="applyProvider(provider)"
        >
          <span class="qs-option-main">
            <span class="qs-option-name">{{ provider.name }}</span>
            <span class="qs-option-meta cs-mono">
              {{ [modelOf(provider), hostOf(provider.apiUrl)].filter(Boolean).join(' · ') }}
            </span>
          </span>
          <span v-if="isCurrent(provider)" class="cs-pill cs-pill--success">
            {{ t('quickSwitch.currentActive') }}
          </span>
        </button>
        <div v-if="visibleProviders.length === 0" class="cs-empty">
          {{ t('quickSwitch.empty') }}
        </div>
      </div>

      <div class="qs-hints">
        <span class="qs-hint-keys">
          <ArrowUp :size="10" :stroke-width="2" />
          <ArrowDown :size="10" :stroke-width="2" />
        </span>
        <span>{{ t('quickSwitch.hintSelect') }}</span>
        <span class="qs-hint-keys">
          <CornerDownLeft :size="10" :stroke-width="2" />
        </span>
        <span>{{ t('quickSwitch.hintApply') }}</span>
        <span class="cs-mono">Esc</span>
        <span>{{ t('quickSwitch.hintClose') }}</span>
        <button type="button" class="qs-advanced" @click="openAdvanced">
          {{ t('quickSwitch.advanced') }}
          <ExternalLink :size="12" :stroke-width="2" />
        </button>
      </div>
    </div>

    <template #footer>
      <button type="button" class="cs-btn" @click="closeQuickSwitch">
        {{ t('common.cancel') }}
      </button>
      <button
        type="button"
        class="cs-btn cs-btn--primary"
        :disabled="applying || visibleProviders.length === 0"
        @click="applyActive"
      >
        {{ t('quickSwitch.apply') }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.qs {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding-bottom: 8px;
  outline: none;
}

.qs-client {
  position: relative;
}

.qs-client-brand {
  position: absolute;
  left: 10px;
  top: 50%;
  transform: translateY(-50%);
  width: 16px;
  height: 16px;
  pointer-events: none;
  z-index: 1;
}

.qs-client-select {
  height: 44px;
  font-size: 12px;
  padding-left: 32px;
}

.qs-client-current {
  position: absolute;
  right: 34px;
  top: 50%;
  transform: translateY(-50%);
  font-size: 11px;
  color: var(--cs-text-secondary);
  pointer-events: none;
  max-width: 45%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.qs-search-wrap {
  position: relative;
}

.qs-search-icon {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  color: var(--cs-text-secondary);
  pointer-events: none;
}

.qs-search {
  height: 40px;
  font-size: 12px;
  padding-left: 32px;
}

.qs-list-label {
  margin-bottom: -6px;
}

.qs-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 260px;
  overflow-y: auto;
}

.qs-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 62px;
  padding: 9px 12px;
  border-radius: var(--cs-r-row);
  border: 1px solid var(--cs-border);
  background: var(--cs-surface);
  cursor: pointer;
  font-family: inherit;
  text-align: left;
  transition: border-color 0.12s ease, background 0.12s ease;
}

.qs-option.active {
  background: var(--cs-brand-tint);
  border-color: var(--cs-brand);
}

.qs-option-main {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.qs-option-name {
  font-size: 12px;
  font-weight: 500;
  color: var(--cs-text);
}

.qs-option-meta {
  font-size: 10px;
  color: var(--cs-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.qs-hints {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 10px;
  color: var(--cs-text-secondary);
  border-top: 1px solid var(--cs-border);
  padding-top: 14px;
}

.qs-hint-keys {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  color: var(--cs-text-secondary);
}

.qs-advanced {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  color: var(--cs-brand);
  font-size: 10px;
  font-weight: 500;
  font-family: inherit;
  cursor: pointer;
}
</style>
