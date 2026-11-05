<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import WebDavVersionsModal from './WebDavVersionsModal.vue'
import WebDavRestoreModal from './WebDavRestoreModal.vue'
import {
  applySnapshot,
  createLocalRollback,
  fetchRemoteSnapshotMeta,
  fetchSyncSettings,
  pullSnapshot,
  pushSnapshot,
  saveSyncSettings,
  testConnection,
  type RemoteSnapshotMeta,
  type SyncSettings,
} from '../../services/configSync'
import { getErrorMessage } from '../../utils/errors'
import { showToast } from '../../utils/toast'
import { getCurrentTheme, setTheme, type ThemeMode } from '../../utils/ThemeManager'
import { i18n, setupI18n } from '../../utils/i18n'
import type { Locale } from '../../locales'

const emit = defineEmits<{
  restored: []
}>()

const { t } = useI18n()

const DEFAULT_SETTINGS: SyncSettings = {
  endpoint: '',
  username: '',
  appPassword: '',
  remotePath: '/Portico',
  syncPassphrase: '',
}

const form = reactive<SyncSettings>({ ...DEFAULT_SETTINGS })
const savedSnapshot = ref('')
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const inspecting = ref(false)
const uploading = ref(false)
const restoring = ref(false)
const showPassword = ref(false)
const showPassphrase = ref(false)
const remoteMeta = ref<RemoteSnapshotMeta | null>(null)
const metaLoaded = ref(false)
const versionsOpen = ref(false)
const restoreOpen = ref(false)

const busy = computed(
  () => loading.value || saving.value || testing.value || inspecting.value || uploading.value || restoring.value,
)

const currentSnapshot = () =>
  JSON.stringify({
    endpoint: form.endpoint.trim().replace(/\/+$/, ''),
    username: form.username.trim(),
    appPassword: form.appPassword,
    remotePath: form.remotePath.trim() || '/Portico',
    syncPassphrase: form.syncPassphrase,
  })

const dirty = computed(() => currentSnapshot() !== savedSnapshot.value)
const hasConnectionFields = computed(
  () => form.endpoint.trim() !== '' && form.username.trim() !== '' && form.appPassword.trim() !== '',
)
const hasAllFields = computed(() => hasConnectionFields.value && form.syncPassphrase.trim() !== '')
const canUseRemote = computed(() => !busy.value && !dirty.value && hasConnectionFields.value)
const canSync = computed(() => !busy.value && !dirty.value && hasAllFields.value)

const remoteSummary = computed(() => {
  if (!remoteMeta.value?.exists) {
    return t('components.general.sync.meta.empty')
  }
  return t('components.general.sync.meta.summary', {
    updatedAt: formatTime(remoteMeta.value.updatedAt),
    deviceName: remoteMeta.value.deviceName || t('components.general.sync.meta.unknown'),
    appVersion: remoteMeta.value.appVersion || t('components.general.sync.meta.unknown'),
  })
})

const applyForm = (settings: SyncSettings) => {
  form.endpoint = settings.endpoint ?? ''
  form.username = settings.username ?? ''
  form.appPassword = settings.appPassword ?? ''
  form.remotePath = settings.remotePath ?? '/Portico'
  form.syncPassphrase = settings.syncPassphrase ?? ''
  savedSnapshot.value = currentSnapshot()
}

const loadSyncSettings = async () => {
  loading.value = true
  try {
    applyForm(await fetchSyncSettings())
  } catch (error) {
    showToast(getErrorMessage(error, t('components.general.sync.messages.loadFailed')), 'error')
  } finally {
    loading.value = false
  }
}

const persistSettings = async () => {
  saving.value = true
  try {
    applyForm(
      await saveSyncSettings({
        endpoint: form.endpoint,
        username: form.username,
        appPassword: form.appPassword,
        remotePath: form.remotePath,
        syncPassphrase: form.syncPassphrase,
      }),
    )
    showToast(t('components.general.sync.messages.saveSuccess'))
  } catch (error) {
    showToast(getErrorMessage(error, t('components.general.sync.messages.saveFailed')), 'error')
  } finally {
    saving.value = false
  }
}

const ensureSaved = () => {
  if (!dirty.value) return true
  showToast(t('components.general.sync.messages.saveBeforeAction'), 'error')
  return false
}

const checkConnection = async () => {
  if (!ensureSaved()) return
  testing.value = true
  try {
    await testConnection()
    showToast(t('components.general.sync.messages.connectionSuccess'))
  } catch (error) {
    showToast(getErrorMessage(error, t('components.general.sync.messages.connectionFailed')), 'error')
  } finally {
    testing.value = false
  }
}

// 查看云端版本：拉取远端快照元数据后打开版本弹窗。
const openVersions = async () => {
  if (!ensureSaved()) return
  inspecting.value = true
  try {
    remoteMeta.value = await fetchRemoteSnapshotMeta()
    metaLoaded.value = true
    versionsOpen.value = true
  } catch (error) {
    showToast(getErrorMessage(error, t('components.general.sync.messages.inspectFailed')), 'error')
  } finally {
    inspecting.value = false
  }
}

const uploadSnapshot = async () => {
  if (!ensureSaved()) return
  uploading.value = true
  try {
    remoteMeta.value = await pushSnapshot(currentUIPreferences())
    metaLoaded.value = true
    showToast(t('components.general.sync.messages.uploadSuccess'))
  } catch (error) {
    showToast(getErrorMessage(error, t('components.general.sync.messages.uploadFailed')), 'error')
  } finally {
    uploading.value = false
  }
}

const openRestore = () => {
  if (!ensureSaved()) return
  restoreOpen.value = true
}

// 确认恢复：若口令被修改先保存设置；随后本地回滚备份 → 拉取 → 应用。
const confirmRestore = async (passphrase: string) => {
  if (restoring.value) return
  restoring.value = true
  let rollbackPath = ''
  try {
    if (passphrase !== form.syncPassphrase) {
      form.syncPassphrase = passphrase
      applyForm(await saveSyncSettings({ ...form }))
    }
    rollbackPath = await createLocalRollback(currentUIPreferences())
    const snapshot = await pullSnapshot()
    await applySnapshot(snapshot.backend)
    setTheme(normalizeTheme(snapshot.ui.theme))
    await setupI18n(normalizeLocale(snapshot.ui.locale))
    emit('restored')
    restoreOpen.value = false
    versionsOpen.value = false
    remoteMeta.value = await fetchRemoteSnapshotMeta()
    showToast(
      t('components.general.sync.messages.restoreSuccess', {
        rollback: basename(rollbackPath),
      }),
    )
  } catch (error) {
    const fallback = rollbackPath
      ? t('components.general.sync.messages.restoreFailedWithRollback', {
          rollback: basename(rollbackPath),
        })
      : t('components.general.sync.messages.restoreFailed')
    showToast(getErrorMessage(error, fallback), 'error')
  } finally {
    restoring.value = false
  }
}

const currentUIPreferences = () => ({
  theme: normalizeTheme(getCurrentTheme()),
  locale: normalizeLocale(i18n.global.locale.value as string),
})

const normalizeTheme = (value: string): ThemeMode => {
  if (value === 'light' || value === 'dark' || value === 'systemdefault') {
    return value
  }
  return 'systemdefault'
}

const normalizeLocale = (value: string): Locale => (value === 'en' ? 'en' : 'zh')

const basename = (value: string) => value.split(/[\\/]/).pop() || value

const formatTime = (value?: string) => {
  if (!value) return t('components.general.sync.meta.unknown')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

onMounted(() => {
  void loadSyncSettings()
})
</script>

<template>
  <section class="sync">
    <div class="sync-head">
      <h2 class="cs-section-title">{{ t('components.general.title.sync') }}</h2>
      <span class="cs-pill cs-pill--success">{{ t('settingsPage.sync.encrypted') }}</span>
    </div>
    <p class="cs-section-sub">{{ t('components.general.sync.hint') }}</p>

    <div class="sync-grid">
      <div class="sync-field sync-field--full">
        <label class="cs-label">{{ t('components.general.sync.fields.endpoint') }}</label>
        <input
          v-model="form.endpoint"
          class="cs-input cs-mono"
          :disabled="busy"
          type="text"
          placeholder="https://dav.example.com/dav"
        />
      </div>
      <div class="sync-field">
        <label class="cs-label">{{ t('components.general.sync.fields.username') }}</label>
        <input v-model="form.username" class="cs-input" :disabled="busy" type="text" />
      </div>
      <div class="sync-field">
        <label class="cs-label">{{ t('components.general.sync.fields.remotePath') }}</label>
        <input v-model="form.remotePath" class="cs-input cs-mono" :disabled="busy" type="text" />
      </div>
      <div class="sync-field">
        <label class="cs-label">{{ t('components.general.sync.fields.appPassword') }}</label>
        <input
          v-model="form.appPassword"
          class="cs-input"
          :disabled="busy"
          :type="showPassword ? 'text' : 'password'"
        />
      </div>
      <div class="sync-field sync-field--secret">
        <label class="cs-label">{{ t('components.general.sync.fields.syncPassphrase') }}</label>
        <input
          v-model="form.syncPassphrase"
          class="cs-input"
          :disabled="busy"
          :type="showPassphrase ? 'text' : 'password'"
        />
        <button
          type="button"
          class="cs-btn sync-secret-toggle"
          :disabled="busy"
          @click="((showPassword = !showPassword), (showPassphrase = !showPassphrase))"
        >
          {{ showPassphrase ? t('components.general.sync.actions.hide') : t('components.general.sync.actions.show') }}
        </button>
      </div>
    </div>

    <div class="sync-actions">
      <button type="button" class="cs-btn cs-btn--primary" :disabled="busy" @click="persistSettings">
        {{ saving ? t('components.general.sync.actions.saving') : t('components.general.sync.actions.save') }}
      </button>
      <button type="button" class="cs-btn" :disabled="!canUseRemote" @click="checkConnection">
        {{ testing ? t('components.general.sync.actions.testing') : t('components.general.sync.actions.test') }}
      </button>
      <button type="button" class="cs-btn" :disabled="!canUseRemote" @click="openVersions">
        {{ inspecting ? t('components.general.sync.actions.loadingMeta') : t('settingsPage.sync.viewVersions') }}
      </button>
      <button type="button" class="cs-btn" :disabled="!canSync" @click="uploadSnapshot">
        {{ uploading ? t('components.general.sync.actions.uploading') : t('components.general.sync.actions.upload') }}
      </button>
      <button type="button" class="cs-btn" :disabled="!canSync" @click="openRestore">
        {{ t('components.general.sync.actions.restore') }}
      </button>
    </div>

    <div class="sync-status">
      <span class="cs-pill" :class="metaLoaded && remoteMeta?.exists ? 'cs-pill--success' : 'cs-pill--warning'">
        {{ metaLoaded && remoteMeta?.exists ? t('settingsPage.sync.remoteFound') : t('settingsPage.sync.remoteUnknown') }}
      </span>
      <span class="sync-status-meta">{{ remoteSummary }}</span>
      <p class="sync-status-scope">{{ t('settingsPage.sync.scope') }}</p>
    </div>

    <WebDavVersionsModal
      :open="versionsOpen"
      :remote-path="form.remotePath"
      :meta="remoteMeta"
      @close="versionsOpen = false"
      @restore="((versionsOpen = false), openRestore())"
    />
    <WebDavRestoreModal
      :open="restoreOpen"
      :busy="restoring"
      :passphrase="form.syncPassphrase"
      :snapshot-time="remoteMeta?.exists ? formatTime(remoteMeta.updatedAt) : ''"
      @close="restoreOpen = false"
      @confirm="confirmRestore"
    />
  </section>
</template>

<style scoped>
.sync {
  border: 1px solid var(--cs-border);
  border-radius: 13px;
  padding: 16px 19px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.sync-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.sync-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px 20px;
}

.sync-field--full {
  grid-column: 1 / -1;
}

.sync-field--secret {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: end;
  gap: 10px;
}

.sync-field--secret .cs-label {
  grid-column: 1 / -1;
  margin-bottom: -2px;
}

.sync-secret-toggle {
  height: 38px;
}

.sync-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.sync-status {
  background: var(--cs-field);
  border: 1px solid var(--cs-border);
  border-radius: 9px;
  padding: 13px 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.sync-status-meta {
  font-size: 11px;
  color: var(--cs-text-secondary);
}

.sync-status-scope {
  margin: 0;
  font-size: 11px;
  color: var(--cs-text-secondary);
}

@media (max-width: 960px) {
  .sync-grid {
    grid-template-columns: 1fr;
  }
}
</style>
