<template>
  <div class="help-text">
    <p v-if="title" class="help-example">
      <strong>{{ title }}</strong>
    </p>
    <div class="help-content">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title?: string
}>()
</script>

<style scoped>
.help-text {
  padding: 12px;
  background-color: var(--cs-field);
  border: 1px solid var(--cs-border);
  border-radius: 8px;
  font-size: 11px;
  color: var(--cs-text-secondary);
}

.help-example {
  margin: 0 0 8px;
  color: var(--cs-text);
}

.help-content :deep(.help-list) {
  margin: 0;
  padding-left: 20px;
  list-style: disc;
}

.help-content :deep(.help-list li) {
  margin-bottom: 6px;
  line-height: 1.5;
}

.help-content :deep(.help-list code) {
  padding: 2px 6px;
  background-color: var(--cs-surface);
  border: 1px solid var(--cs-border);
  border-radius: 4px;
  font-family: var(--cs-font-mono);
  font-size: 10px;
  color: var(--cs-brand);
}

.help-content :deep(.help-desc) {
  font-size: 10px;
  color: var(--cs-text-secondary);
}
</style>
