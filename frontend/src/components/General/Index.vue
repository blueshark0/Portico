<script setup lang="ts">
import { ref, watch } from 'vue'
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import ListItem from '../Setting/ListRow.vue'
import LanguageSwitcher from '../Setting/LanguageSwitcher.vue'
import ThemeSetting from '../Setting/ThemeSetting.vue'
import WebDavSyncSection from './WebDavSyncSection.vue'
import { useAppSettingsStore } from '../../composables/useAppSettingsStore'
import { runWithToast } from '../../utils/runWithToast'

const { t } = useI18n()
const {
  loadSettings,
  loading: settingsLoading,
  saving: saveBusy,
  proxyEnabled,
  proxyURL: proxyUrlValue,
  updateSettings,
} = useAppSettingsStore()

const proxyInput = ref('')
const proxyError = ref('')

watch(proxyUrlValue, (val) => {
  proxyInput.value = val
}, { immediate: true })

const reloadApplicationSettings = () => {
  void runWithToast(() => loadSettings(true), {
    fallback: t('components.general.messages.loadFailed'),
  })
}

const onProxyToggle = async (event: Event) => {
  const target = event.target as HTMLInputElement | null
  if (settingsLoading.value || saveBusy.value) return
  await runWithToast(() => updateSettings({ proxy_enabled: Boolean(target?.checked) }), {
    fallback: t('components.general.messages.saveFailed'),
  })
}

const PROXY_PREFIXES = ['http://', 'https://', 'socks5://', 'socks5h://']

const onProxySave = async () => {
  const raw = proxyInput.value.trim()
  if (raw && !PROXY_PREFIXES.some(p => raw.startsWith(p))) {
    proxyError.value = t('components.general.messages.invalidProxyUrl')
    return
  }
  proxyError.value = ''
  if (settingsLoading.value || saveBusy.value) return
  await runWithToast(() => updateSettings({ proxy_url: raw }), {
    fallback: t('components.general.messages.saveFailed'),
  })
}

onMounted(() => {
  void runWithToast(() => loadSettings(), {
    fallback: t('components.general.messages.loadFailed'),
  })
})
</script>

<template>
  <div class="cs-page">
    <div class="cs-page-head">
      <div>
        <h1 class="cs-page-title">{{ t('settingsPage.title') }}</h1>
        <p class="cs-page-sub">{{ t('settingsPage.subtitle') }}</p>
      </div>
    </div>

    <div class="cs-card cs-card-pad settings-wrap">
      <section class="settings-group">
        <h2 class="cs-card-title">{{ t('settingsPage.appNetwork') }}</h2>
        <div class="settings-rows">
          <ListItem :label="t('settingsPage.language')">
            <LanguageSwitcher />
          </ListItem>
          <ListItem :label="t('settingsPage.theme')">
            <ThemeSetting />
          </ListItem>
          <ListItem :label="t('settingsPage.proxy')" :sub-label="t('settingsPage.proxyHint')">
            <label class="cs-switch">
              <input
                type="checkbox"
                :disabled="settingsLoading || saveBusy"
                :checked="proxyEnabled"
                @change="onProxyToggle"
              />
              <span></span>
            </label>
          </ListItem>
          <div v-if="proxyEnabled" class="settings-proxy">
            <input
              v-model="proxyInput"
              type="text"
              class="cs-input cs-mono settings-proxy-input"
              :placeholder="t('components.general.placeholder.proxy')"
              :disabled="settingsLoading || saveBusy"
              @keydown.enter="onProxySave"
            />
            <button
              type="button"
              class="cs-btn"
              :disabled="settingsLoading || saveBusy || proxyInput === proxyUrlValue"
              @click="onProxySave"
            >
              {{ t('common.save') }}
            </button>
          </div>
          <p v-if="proxyError" class="cs-field-error">{{ proxyError }}</p>
        </div>
      </section>

      <WebDavSyncSection @restored="reloadApplicationSettings" />
    </div>
  </div>
</template>

<style scoped>
.settings-wrap {
  display: flex;
  flex-direction: column;
  gap: 22px;
}

.settings-group {
  border: 1px solid var(--cs-border);
  border-radius: 11px;
  padding: 14px 15px;
}

.settings-rows {
  margin-top: 6px;
  display: flex;
  flex-direction: column;
}

.settings-proxy {
  display: flex;
  gap: 10px;
  padding: 4px 2px 8px;
}

.settings-proxy-input {
  max-width: 420px;
}
</style>
