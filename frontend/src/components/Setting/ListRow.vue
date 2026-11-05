<script setup lang="ts">
defineProps<{
   label: string
   subLabel?: string
 }>()
</script>

<template>
  <div class="mac-list-row">
    <div class="mac-list-text">
      <div class="mac-list-label">{{ label }}</div>
      <div v-if="subLabel" class="mac-list-sublabel">{{ subLabel }}</div>
    </div>
    <div class="mac-list-control">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.mac-list-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 2px;
  gap: 16px;
}

.mac-list-text {
  text-align: left;
}

.mac-list-label {
  font-size: 12px;
  font-weight: 500;
  color: var(--cs-text);
}

.mac-list-sublabel {
  font-size: 10px;
  color: var(--cs-text-secondary);
  margin-top: 4px;
}

.mac-list-control {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
}
</style>
