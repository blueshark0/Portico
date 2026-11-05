<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRoute } from 'vue-router'
import { Archive, Copy, Ellipsis, Pencil, Plus, Settings2, Trash2 } from 'lucide-vue-next'
import { providerTabs, type ProviderTab } from '../../constants/platforms'
import {
  entrySubtitle,
  useProviderDirectory,
  type DirectoryEntry,
} from './useProviderDirectory'
import ProviderListRow from './ProviderListRow.vue'
import ProviderDetailCard from './ProviderDetailCard.vue'
import ProviderEditorModal from './ProviderEditorModal.vue'
import ProviderAdvancedModal from './ProviderAdvancedModal.vue'
import ProviderDeleteModal from './ProviderDeleteModal.vue'
import ProviderCopyModal from './ProviderCopyModal.vue'
import ProviderArchiveModal from './ProviderArchiveModal.vue'

const { t } = useI18n()
const route = useRoute()
const directory = useProviderDirectory(t)
const {
  filter,
  filteredEntries,
  selectedEntry,
  selectedKey,
  entryKey,
  isDefault,
  editorState,
  advancedState,
  deleteState,
  copyState,
  archiveState,
  probeResults,
  probing,
} = directory

const filterTabs = computed(() => [
  { id: 'all' as const, label: t('providersPage.filters.all') },
  ...providerTabs.map((tab) => ({ id: tab.id, label: tab.label })),
])

const totalCount = computed(() => filteredEntries.value.length)
const archiveRoute = computed(() => ({
  path: '/providers/archived',
  query: filter.value === 'all' ? {} : { platform: filter.value },
}))

onMounted(async () => {
  await directory.loadAll()
  const rawPlatform = typeof route.query.platform === 'string' ? route.query.platform : ''
  if ((providerTabs.map((tab) => tab.id) as string[]).includes(rawPlatform)) {
    filter.value = rawPlatform as ProviderTab
  }
})

type ProviderStatus = { tone: 'success' | 'warning' | 'danger'; label: string }

const statusOf = (entry: DirectoryEntry): ProviderStatus => {
  if (isDefault(entry)) return { tone: 'success', label: t('providersPage.status.active') }
  const probe = probeResults[entryKey(entry)]
  if (probe?.success) return { tone: 'success', label: t('providersPage.status.verified') }
  if (probe && !probe.success) return { tone: 'danger', label: t('providersPage.status.failed') }
  return { tone: 'warning', label: t('providersPage.status.unverified') }
}

const probeSummary = computed(() => directory.probeSummary(selectedEntry.value))

// ---- 行内菜单 ----
const menuFor = ref('')
const menuStyle = ref<Record<string, string>>({})
const toggleMenu = (key: string, event: MouseEvent) => {
  if (menuFor.value === key) {
    closeMenu()
    return
  }
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const menuWidth = 180
  const menuHeight = 180
  const openUpward = window.innerHeight - rect.bottom < menuHeight + 12 && rect.top > menuHeight + 12
  const top = openUpward ? rect.top - menuHeight - 6 : rect.bottom + 6
  const left = Math.min(window.innerWidth - menuWidth - 8, Math.max(8, rect.right - menuWidth))
  menuStyle.value = {
    top: `${Math.max(8, top)}px`,
    left: `${left}px`,
  }
  menuFor.value = key
}
const closeMenu = () => {
  menuFor.value = ''
}
onMounted(() => {
  window.addEventListener('click', closeMenu)
  window.addEventListener('resize', closeMenu)
  window.addEventListener('scroll', closeMenu, true)
})
onUnmounted(() => {
  window.removeEventListener('click', closeMenu)
  window.removeEventListener('resize', closeMenu)
  window.removeEventListener('scroll', closeMenu, true)
})

const copySourceEntry = computed(() =>
  copyState.entryKey ? directory.findEntry(copyState.entryKey) : null,
)

const deleteTarget = computed(() =>
  deleteState.entryKey ? directory.findEntry(deleteState.entryKey) : null,
)

const archiveTarget = computed(() =>
  archiveState.entryKey ? directory.findEntry(archiveState.entryKey) : null,
)
</script>

<template>
  <div class="cs-page" @click="closeMenu">
    <div class="cs-page-head">
      <div>
        <h1 class="cs-page-title">{{ t('providersPage.title') }}</h1>
        <p class="cs-page-sub">{{ t('providersPage.subtitle') }}</p>
      </div>
      <div class="cs-page-actions">
        <button
          type="button"
          class="cs-btn"
          :disabled="!selectedEntry"
          @click="selectedEntry && directory.requestCopy(selectedEntry)"
        >
          <Copy :size="14" :stroke-width="2" />
          {{ t('providersPage.actions.copyAcross') }}
        </button>
        <RouterLink class="cs-btn prov-archive-link" :to="archiveRoute">
          <Archive :size="14" :stroke-width="2" />
          {{ t('providersPage.actions.archiveManagement') }}
        </RouterLink>
        <button type="button" class="cs-btn cs-btn--primary" @click="directory.openCreate()">
          <Plus :size="14" :stroke-width="2" />
          {{ t('providersPage.actions.create') }}
        </button>
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
      <span class="cs-filter-count">{{ t('providersPage.filters.count', { count: totalCount }) }}</span>
    </div>

    <div class="prov-grid">
      <div class="cs-card cs-card-pad prov-list">
        <h2 class="cs-card-title">{{ t('providersPage.list.title') }}</h2>
        <p class="cs-section-sub">{{ t('providersPage.list.subtitle') }}</p>

        <div v-if="filteredEntries.length === 0" class="cs-empty">
          {{ t('providersPage.list.empty') }}
        </div>

        <div class="prov-rows">
          <ProviderListRow
            v-for="entry in filteredEntries"
            :key="entryKey(entry)"
            :entry="entry"
            :subtitle="entrySubtitle(entry)"
            :selected="!!selectedEntry && entryKey(selectedEntry) === entryKey(entry)"
            @select="selectedKey = entryKey(entry)"
          >
            <template #status>
              <span class="cs-pill" :class="`cs-pill--${statusOf(entry).tone}`">
                {{ statusOf(entry).label }}
              </span>
            </template>

            <span class="cs-menu-anchor" @click.stop>
              <button
                type="button"
                class="prov-kebab"
                :aria-label="t('providersPage.menu.open')"
                :aria-expanded="menuFor === entryKey(entry)"
                @click="toggleMenu(entryKey(entry), $event)"
              >
                <Ellipsis :size="16" :stroke-width="2" />
              </button>
              <Teleport to="body">
                <div
                  v-if="menuFor === entryKey(entry)"
                  class="cs-menu prov-action-menu"
                  :style="menuStyle"
                  @click.stop
                >
                <button type="button" class="cs-menu-item" @click="directory.openEdit(entry); closeMenu()">
                  <Pencil :size="14" :stroke-width="2" />
                  {{ t('providersPage.menu.edit') }}
                </button>
                <button type="button" class="cs-menu-item" @click="directory.openAdvanced(entry); closeMenu()">
                  <Settings2 :size="14" :stroke-width="2" />
                  {{ t('providersPage.menu.advanced') }}
                </button>
                <button type="button" class="cs-menu-item" @click="directory.requestCopy(entry); closeMenu()">
                  <Copy :size="14" :stroke-width="2" />
                  {{ t('providersPage.menu.copyAcross') }}
                </button>
                <hr class="cs-divider prov-menu-divider" />
                <button type="button" class="cs-menu-item" @click="directory.requestArchive(entry); closeMenu()">
                  <Archive :size="14" :stroke-width="2" />
                  {{ t('providersPage.menu.archive') }}
                </button>
                <button type="button" class="cs-menu-item cs-menu-item--danger" @click="directory.requestDelete(entry); closeMenu()">
                  <Trash2 :size="14" :stroke-width="2" />
                  {{ t('providersPage.menu.delete') }}
                </button>
                </div>
              </Teleport>
            </span>
          </ProviderListRow>
        </div>
      </div>

      <ProviderDetailCard
        v-if="selectedEntry"
        :entry="selectedEntry"
        :title="t('providersPage.detail.title')"
        :status-tone="statusOf(selectedEntry).tone"
        :status-label="statusOf(selectedEntry).label"
        :meta-label="t('providersPage.detail.lastTest')"
        :meta-value="probeSummary"
        :note="t('providersPage.detail.privacyNote')"
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
            :disabled="isDefault(selectedEntry)"
            @click="directory.setDefault(selectedEntry)"
          >
            {{ isDefault(selectedEntry) ? t('providersPage.actions.current') : t('providersPage.actions.setCurrent') }}
          </button>
        </template>
      </ProviderDetailCard>
    </div>

    <ProviderEditorModal
      :state="editorState"
      @close="editorState.open = false"
      @submit="directory.submitEditor"
    />
    <ProviderAdvancedModal
      :state="advancedState"
      @close="advancedState.open = false"
      @submit="directory.submitAdvanced"
    />
    <ProviderDeleteModal
      :open="deleteState.open"
      :name="deleteTarget?.provider.name ?? ''"
      :busy="deleteState.busy"
      @close="deleteState.open = false"
      @confirm="directory.confirmDelete"
    />
    <ProviderCopyModal
      :open="copyState.open"
      :source="copySourceEntry"
      @close="copyState.open = false"
      @confirm="directory.confirmCopy"
    />
    <ProviderArchiveModal
      :open="archiveState.open"
      :name="archiveTarget?.provider.name ?? ''"
      :busy="archiveState.busy"
      @close="directory.closeArchive"
      @confirm="directory.confirmArchive"
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
  /* 列表在卡片内部滚动，不撑高整页；高度为视口减去顶栏/页头/筛选条 */
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

.prov-action-menu {
  position: fixed;
  right: auto;
  width: 180px;
}

.prov-list .cs-section-sub {
  margin-bottom: 8px;
}

.prov-kebab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  color: var(--cs-text-secondary);
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 6px;
}

.prov-kebab:hover {
  background: var(--cs-row-hover);
}

.cs-menu-item {
  gap: 8px;
}

.prov-menu-divider {
  margin: 4px 0;
}

.prov-archive-link {
  text-decoration: none;
}

@media (max-width: 1100px) {
  .prov-grid {
    grid-template-columns: 1fr;
  }
}
</style>
