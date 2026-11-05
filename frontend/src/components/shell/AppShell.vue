<script setup lang="ts">
import { computed, onMounted, onUnmounted, type Component } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Home, Layers, History, MessagesSquare, Plug, Settings } from 'lucide-vue-next'
import QuickSwitchModal from '../QuickSwitch/QuickSwitchModal.vue'
import { useQuickSwitch } from '../../composables/useQuickSwitch'

const route = useRoute()
const { t } = useI18n()
const { quickSwitchOpen, openQuickSwitch, closeQuickSwitch } = useQuickSwitch()

const NAV_ITEMS: ReadonlyArray<{ key: string; icon: Component; to: string }> = [
  { key: 'dashboard', icon: Home, to: '/' },
  { key: 'providers', icon: Layers, to: '/providers' },
  { key: 'logs', icon: History, to: '/logs' },
  { key: 'sessions', icon: MessagesSquare, to: '/sessions' },
  { key: 'mcp', icon: Plug, to: '/mcp' },
]

const activeNav = computed(() => (route.meta.nav as string) || 'dashboard')
const crumb = computed(() => {
  const section = t(`shell.nav.${activeNav.value}`)
  const sub = route.meta.crumb ? t(`shell.nav.${route.meta.crumb as string}`) : ''
  if (route.meta.crumbOnly && sub) return `PORTICO  /  ${sub}`
  return sub ? `PORTICO  /  ${section}  /  ${sub}` : `PORTICO  /  ${section}`
})

const onGlobalKeydown = (event: KeyboardEvent) => {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    if (quickSwitchOpen.value) {
      closeQuickSwitch()
    } else {
      openQuickSwitch()
    }
  }
}

onMounted(() => window.addEventListener('keydown', onGlobalKeydown))
onUnmounted(() => window.removeEventListener('keydown', onGlobalKeydown))
</script>

<template>
  <div class="shell">
    <aside class="shell-sidebar">
      <RouterLink to="/" class="shell-logo" :title="t('shell.nav.dashboard')">
        <span class="shell-logo-bar shell-logo-bar--blue" />
        <span class="shell-logo-bar shell-logo-bar--white" />
      </RouterLink>
      <nav class="shell-nav">
        <RouterLink
          v-for="item in NAV_ITEMS"
          :key="item.key"
          :to="item.to"
          class="shell-nav-item"
          :class="{ active: activeNav === item.key }"
          :title="t(`shell.nav.${item.key}`)"
        >
          <component :is="item.icon" :size="20" :stroke-width="2" class="shell-nav-icon" />
        </RouterLink>
      </nav>
      <div class="shell-sidebar-bottom">
        <RouterLink
          to="/settings"
          class="shell-nav-item"
          :class="{ active: activeNav === 'settings' }"
          :title="t('shell.nav.settings')"
        >
          <Settings :size="20" :stroke-width="2" class="shell-nav-icon" />
        </RouterLink>
      </div>
    </aside>
    <div class="shell-main">
      <header class="shell-topbar">
        <span class="shell-crumb">{{ crumb }}</span>
      </header>
      <main class="shell-content">
        <RouterView />
      </main>
    </div>
    <QuickSwitchModal />
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  height: 100vh;
  overflow: hidden;
  background: var(--cs-canvas);
}

.shell-sidebar {
  width: var(--cs-sidebar-w);
  flex-shrink: 0;
  background: var(--cs-surface);
  border-right: 1px solid var(--cs-border);
  display: flex;
  flex-direction: column;
  align-items: center;
  padding-top: 18px;
}

.shell-logo {
  width: 44px;
  height: 44px;
  border-radius: 13px;
  background: #20242d;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 2px;
  flex-shrink: 0;
}

.shell-logo-bar {
  width: 11px;
  height: 24px;
  border-radius: 4px;
}

.shell-logo-bar--blue {
  background: #356df3;
}

.shell-logo-bar--white {
  background: #fff;
}

.shell-nav {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 30px;
}

.shell-nav-item {
  width: 52px;
  height: 48px;
  border-radius: var(--cs-r-nav);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--cs-text-secondary);
  text-decoration: none;
  transition: background 0.15s ease, color 0.15s ease;
}

.shell-nav-item:hover {
  background: var(--cs-row-hover);
  color: var(--cs-text);
}

.shell-nav-item.active {
  background: var(--cs-brand-tint);
  color: var(--cs-brand);
}

.shell-nav-icon {
  flex-shrink: 0;
}

.shell-sidebar-bottom {
  margin-top: auto;
  padding-bottom: 24px;
}

.shell-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.shell-topbar {
  height: var(--cs-topbar-h);
  flex-shrink: 0;
  background: var(--cs-surface);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  padding: 0 28px;
}

.shell-crumb {
  font-family: var(--cs-font-mono);
  font-size: 11px;
  font-weight: 500;
  color: var(--cs-text-secondary);
  letter-spacing: 0.02em;
  white-space: pre;
}

.shell-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
</style>
