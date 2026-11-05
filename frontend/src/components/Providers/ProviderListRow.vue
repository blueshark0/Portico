<script setup lang="ts">
import { Server } from 'lucide-vue-next'
import type { DirectoryEntry } from './useProviderDirectory'

defineProps<{
  entry: DirectoryEntry
  subtitle: string
  selected: boolean
}>()

const emit = defineEmits<{ (e: 'select'): void }>()
</script>

<template>
  <div class="prov-row" :class="{ selected }">
    <button type="button" class="prov-row-select" @click="emit('select')">
      <span class="prov-avatar">
        <Server :size="20" :stroke-width="2" />
      </span>
      <span class="prov-main">
        <span class="prov-name">{{ entry.provider.name }}</span>
        <span class="prov-sub cs-mono">{{ subtitle }}</span>
      </span>
      <span class="prov-row-status">
        <slot name="status" />
      </span>
    </button>
    <div class="prov-row-actions">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.prov-row {
  display: flex;
  align-items: center;
  gap: 13px;
  min-height: 78px;
  padding: 12px 11px;
  border-radius: var(--cs-r-row);
  border: 1px solid var(--cs-border);
  background: var(--cs-surface);
  transition: border-color 0.12s ease, background 0.12s ease;
}

.prov-row.selected {
  background: var(--cs-brand-tint);
  border-color: var(--cs-brand-tint-border);
}

.prov-row-select {
  display: flex;
  align-items: center;
  gap: 13px;
  flex: 1;
  min-width: 0;
  padding: 0;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: inherit;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
}

.prov-row-select:focus-visible {
  outline: 2px solid var(--cs-brand);
  outline-offset: 4px;
}

.prov-avatar {
  width: 43px;
  height: 43px;
  border-radius: 10px;
  background: var(--cs-field);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--cs-text-secondary);
  flex-shrink: 0;
}

.prov-row.selected .prov-avatar {
  background: var(--cs-brand-tint);
}

.prov-main {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

.prov-name {
  font-size: 12px;
  font-weight: 500;
  color: var(--cs-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prov-sub {
  font-size: 10px;
  color: var(--cs-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prov-row-status {
  display: flex;
  align-items: center;
  width: 118px;
  flex: 0 0 118px;
}

.prov-row-actions {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

@media (max-width: 640px) {
  .prov-row-status {
    width: auto;
    flex-basis: auto;
  }
}
</style>
