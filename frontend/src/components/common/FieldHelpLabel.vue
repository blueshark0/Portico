<template>
  <div class="field-help-label">
    <span>{{ label }}</span>
    <button
      type="button"
      class="help-icon"
      :class="{ 'is-open': expandable && open }"
      :data-tooltip="tooltip"
      :aria-label="tooltip"
      :aria-expanded="expandable ? !!open : undefined"
      @click="toggle"
    >
      <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
        <path
          d="M8 1a7 7 0 100 14A7 7 0 008 1zm0 13A6 6 0 118 2a6 6 0 010 12zm0-9.5a.75.75 0 01.75.75v4a.75.75 0 01-1.5 0v-4A.75.75 0 018 4.5zm0 7.5a1 1 0 100-2 1 1 0 000 2z"
          fill="currentColor"
        />
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{
  label: string
  tooltip: string
  expandable?: boolean
  open?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:open', value: boolean): void
}>()

// expandable 模式下图标变为展开/收起开关；否则保持原有的纯展示行为。
const toggle = () => {
  if (props.expandable) {
    emit('update:open', !props.open)
  }
}
</script>

<style scoped>
.field-help-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 700;
  font-size: 13px;
  color: var(--cs-text);
}

.help-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border: none;
  background: none;
  color: var(--cs-text-secondary);
  cursor: help;
  border-radius: 4px;
  transition: color 0.15s, background 0.15s;
}

.help-icon:hover {
  color: var(--cs-text);
  background-color: var(--cs-row-hover);
}

.help-icon.is-open {
  color: var(--cs-brand);
}
</style>
