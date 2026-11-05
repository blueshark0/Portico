<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRoute } from 'vue-router'
import { ArrowLeft, Trash2 } from 'lucide-vue-next'
import { providerTabs, type ProviderTab } from '../../constants/platforms'
import {
  formatArchivedAt,
  hostOf,
  modelOf,
  useProviderDirectory,
  type DirectoryEntry,
} from './useProviderDirectory'
import ProviderListRow from './ProviderListRow.vue'
import ProviderDetailCard from './ProviderDetailCard.vue'
import ProviderEditorModal from './ProviderEditorModal.vue'
import ProviderDeleteModal from './ProviderDeleteModal.vue'

const { t } = useI18n()
const route = useRoute()
const directory = useProviderDirectory(t, 'archived')
const {
  loading,
  filter,
  filteredEntries,
  selectedEntry,
  selectedKey,
  entryKey,
  editorState,
  deleteState,
  probeResults,
  probing,
  restoringKey,
} = directory

const filterTabs = computed(() => [
  { id: 'all' as const, label: t('providersPage.archivedPage.all') },
  ...providerTabs.map((tab) => ({ id: tab.id, label: tab.label })),
])

const totalCount = computed(() => filteredEntries.value.length)
const providersRoute = computed(() => ({
  path: '/providers',
  query: filter.value === 'all' ? {} : { platform: filter.value },
}))

onMounted(async () => {
  await directory.loadAll()
  const rawPlatform = typeof route.query.platform === 'string' ? route.query.platform : ''
  if ((providerTabs.map((tab) => tab.id) as string[]).includes(rawPlatform)) {
    filter.value = rawPlatform as ProviderTab
  }
})

const archivedSubtitle = (entry: DirectoryEntry) => {
  const date = formatArchivedAt(entry.provider.archivedAt)
  const archivedLabel = date
    ? t('providersPage.archivedPage.archivedOn', { date })
    : t('providersPage.archivedPage.archivedOnUnknown')
  return [entry.platformLabel, modelOf(entry) || hostOf(entry.provider.apiUrl), archivedLabel]
    .filter(Boolean)
    .join(' · ')
}

const archivedTime = computed(() => {
  if (!selectedEntry.value) return ''
  return (
    formatArchivedAt(selectedEntry.value.provider.archivedAt, true) ||
    t('providersPage.archivedPage.unknownTime')
  )
})

const probeFeedback = computed(() => {
  const entry = selectedEntry.value
  if (!entry || !probeResults[entryKey(entry)]) return ''
  return directory.probeSummary(entry)
})

const deleteTarget = computed(() =>
  deleteState.entryKey ? directory.findEntry(deleteState.entryKey) : null,
)
</script>

<template>
  <div class="cs-page">
    <div class="cs-page-head">
      <div>
        <h1 class="cs-page-title">{{ t('providersPage.archivedPage.title') }}</h1>
        <p class="cs-page-sub">{{ t('providersPage.archivedPage.subtitle') }}</p>
      </div>
      <div class="cs-page-actions">
        <RouterLink class="cs-btn prov-back-link" :to="providersRoute">
          <ArrowLeft :size="14" :stroke-width="2" />
          {{ t('providersPage.actions.back') }}
        </RouterLink>
      </div>
    </div>

    <div class="cs-filterbar">
      <button
        v-for="tab in filterTabs"
        :key="tab.id"
        type="button"
        class="cs-tab"
        :class="{ active: filter === tab.id }"
        @click="filter = tab.id"
      >
        {{ tab.label }}
      </button>
      <span class="cs-filter-count">
        {{ t('providersPage.archivedPage.count', { count: totalCount }) }}
      </span>
    </div>

    <div class="prov-grid">
      <div class="cs-card cs-card-pad prov-list">
        <h2 class="cs-card-title">{{ t('providersPage.archivedPage.listTitle') }}</h2>
        <p class="cs-section-sub">{{ t('providersPage.archivedPage.listSubtitle') }}</p>

        <div class="prov-rows">
          <div v-if="loading" class="cs-empty">{{ t('common.loading') }}</div>
          <div v-else-if="filteredEntries.length === 0" class="cs-empty">
            {{ t('providersPage.archivedPage.empty') }}
          </div>

          <ProviderListRow
            v-for="entry in filteredEntries"
            :key="entryKey(entry)"
            :entry="entry"
            :subtitle="archivedSubtitle(entry)"
            :selected="!!selectedEntry && entryKey(selectedEntry) === entryKey(entry)"
            @select="selectedKey = entryKey(entry)"
          >
            <template #status>
              <span class="cs-pill cs-pill--muted">{{ t('providersPage.status.archived') }}</span>
            </template>
            <button
              type="button"
              class="cs-btn prov-restore-btn"
              :disabled="!!restoringKey"
              @click="directory.restoreProvider(entry)"
            >
              {{ restoringKey === entryKey(entry) ? t('providersPage.actions.restoring') : t('providersPage.menu.restore') }}
            </button>
          </ProviderListRow>
        </div>

        <div class="prov-restore-rules">
          <h3>{{ t('providersPage.archivedPage.rulesTitle') }}</h3>
          <p>{{ t('providersPage.archivedPage.rulesBody') }}</p>
        </div>
      </div>

      <ProviderDetailCard
        v-if="selectedEntry"
        :entry="selectedEntry"
        :title="t('providersPage.archivedPage.detailTitle')"
        status-tone="muted"
        :status-label="t('providersPage.status.archived')"
        :meta-label="t('providersPage.archivedPage.archivedAt')"
        :meta-value="archivedTime"
        :feedback="probeFeedback"
        :note="t('providersPage.archivedPage.restoreNote')"
      >
        <template #actions>
          <button type="button" class="cs-btn" @click="directory.openEdit(selectedEntry)">
            {{ t('providersPage.actions.edit') }}
          </button>
          <button
            type="button"
            class="cs-btn"
            :disabled="probing === entryKey(selectedEntry)"
            @click="directory.testConnection(selectedEntry)"
          >
            {{ probing === entryKey(selectedEntry) ? t('providersPage.actions.testing') : t('providersPage.actions.test') }}
          </button>
          <button
            type="button"
            class="cs-btn cs-btn--primary"
            :disabled="!!restoringKey"
            @click="directory.restoreProvider(selectedEntry)"
          >
            {{ restoringKey === entryKey(selectedEntry) ? t('providersPage.actions.restoring') : t('providersPage.actions.restore') }}
          </button>
        </template>
        <template #footer>
          <button
            type="button"
            class="prov-delete"
            :disabled="!!restoringKey"
            @click="directory.requestDelete(selectedEntry)"
          >
            <Trash2 :size="12" :stroke-width="2" />
            {{ t('providersPage.archivedPage.delete') }}
          </button>
        </template>
      </ProviderDetailCard>
    </div>

    <ProviderEditorModal
      :state="editorState"
      @close="editorState.open = false"
      @submit="directory.submitEditor"
    />
    <ProviderDeleteModal
      :open="deleteState.open"
      :name="deleteTarget?.provider.name ?? ''"
      :busy="deleteState.busy"
      @close="deleteState.open = false"
      @confirm="directory.confirmDelete"
    />
  </div>
</template>

<style scoped>
.prov-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(320px, 1fr);
  gap: 18px;
  align-items: start;
}

.prov-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: min(560px, calc(100vh - 220px));
  max-height: calc(100vh - 220px);
}

.prov-rows {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.prov-list .cs-section-sub {
  margin-bottom: 8px;
}

.prov-back-link {
  text-decoration: none;
}

.prov-restore-btn {
  min-width: 74px;
  color: var(--cs-brand);
}

.prov-restore-rules {
  flex-shrink: 0;
  padding: 13px;
  border: 1px solid var(--cs-brand-tint-border);
  border-radius: 10px;
  background: var(--cs-brand-tint);
}

.prov-restore-rules h3 {
  margin: 0;
  font-size: 15px;
  color: var(--cs-text);
}

.prov-restore-rules p {
  margin: 6px 0 0;
  font-size: 11px;
  line-height: 1.6;
  color: var(--cs-text-secondary);
}

.prov-delete {
  display: inline-flex;
  align-items: center;
  align-self: flex-start;
  gap: 6px;
  margin-top: 2px;
  padding: 4px 0;
  border: none;
  background: transparent;
  color: var(--cs-danger);
  font-family: inherit;
  font-size: 10px;
  cursor: pointer;
}

.prov-delete:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

@media (max-width: 1100px) {
  .prov-grid {
    grid-template-columns: 1fr;
  }
}
</style>
