<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLogViewer from './AppLogViewer.vue'
import RequestLogPanel from './RequestLogPanel.vue'
import UsageOverview from './UsageOverview.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

type LogsTab = 'overview' | 'requests' | 'app'
const tabs: LogsTab[] = ['overview', 'requests', 'app']
const activeTab = computed<LogsTab>(() => {
  if (route.query.view === 'app') return 'app'
  if (route.query.view === 'requests') return 'requests'
  return 'overview'
})

const switchTab = (tab: LogsTab) => {
  if (activeTab.value === tab) return
  const query = { ...route.query }
  if (tab === 'overview') {
    delete query.view
  } else {
    query.view = tab
  }
  void router.replace({ query })
}
</script>

<template>
  <div class="cs-page logs-page">
    <div class="cs-page-head">
      <div>
        <h1 class="cs-page-title">{{ t('logsPage.title') }}</h1>
        <p class="cs-page-sub">{{ t(`logsPage.subtitle.${activeTab}`) }}</p>
      </div>
    </div>

    <div class="cs-card logs-subnav">
      <div class="logs-subnav-tabs" role="tablist" :aria-label="t('logsPage.title')">
        <button
          v-for="tab in tabs"
          :key="tab"
          type="button"
          class="logs-subnav-tab"
          :class="[`logs-subnav-tab--${tab}`, { active: activeTab === tab }]"
          role="tab"
          :aria-selected="activeTab === tab"
          @click="switchTab(tab)"
        >
          {{ t(`logsPage.tabs.${tab}`) }}
        </button>
      </div>
      <p class="logs-subnav-note">{{ t('logsPage.tabs.hint') }}</p>
    </div>

    <UsageOverview v-if="activeTab === 'overview'" />
    <RequestLogPanel v-else-if="activeTab === 'requests'" />
    <AppLogViewer v-else />
  </div>
</template>

<style scoped>
.logs-page {
  gap: 0;
  padding-right: 19px;
  padding-bottom: 27px;
}

.logs-page .cs-page-head {
  margin-bottom: 20px;
}

.logs-page .cs-page-sub {
  margin-top: 4px;
  font-size: 12px;
  line-height: 17px;
}

.logs-subnav {
  display: flex;
  align-items: center;
  flex-shrink: 0;
  gap: 16px;
  height: 45px;
  margin-bottom: 0;
  padding: 4px 8px;
  border-radius: 11px;
}

.logs-subnav-tabs {
  display: flex;
  align-items: center;
  flex-shrink: 0;
  gap: 5px;
}

.logs-subnav-tab {
  width: 130px;
  height: 33px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: var(--cs-surface);
  color: var(--cs-text-secondary);
  font: 500 12px var(--cs-font);
  cursor: pointer;
  white-space: nowrap;
  transition: color 0.15s ease, background 0.15s ease, border-color 0.15s ease;
}

.logs-subnav-tab--overview {
  width: 104px;
}

.logs-subnav-tab:hover {
  background: var(--cs-row-hover);
  color: var(--cs-text);
}

.logs-subnav-tab.active {
  border-color: var(--cs-brand-tint-border);
  background: var(--cs-brand-tint);
  color: var(--cs-brand);
}

.logs-subnav-note {
  margin: 0 52px 0 auto;
  color: var(--cs-text-secondary);
  font-size: 10px;
  text-align: right;
  white-space: nowrap;
}

@media (max-width: 900px) {
  .logs-subnav-note {
    display: none;
  }
}
</style>
