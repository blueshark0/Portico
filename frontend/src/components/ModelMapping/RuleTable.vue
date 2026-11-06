<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(
  defineProps<{
    rules: Record<string, string>
    compact?: boolean
    highlightChanges?: boolean
    targetLabel?: string
  }>(),
  { compact: false, highlightChanges: false, targetLabel: '' },
)

const { t } = useI18n()
const entries = computed(() =>
  Object.entries(props.rules ?? {}).sort(([left], [right]) => left.localeCompare(right)),
)
</script>

<template>
  <div class="mapping-rule-table" :class="{ 'mapping-rule-table--compact': compact }" role="table">
    <div class="mapping-rule-row mapping-rule-head" role="row">
      <span role="columnheader">{{ t('modelMapping.requestModel') }}</span>
      <span aria-hidden="true"></span>
      <span role="columnheader">{{ targetLabel || t('modelMapping.targetModel') }}</span>
    </div>
    <div v-if="entries.length === 0" class="mapping-rule-empty">
      {{ t('modelMapping.emptyRules') }}
    </div>
    <div v-for="([source, target], index) in entries" v-else :key="`${source}-${index}`" class="mapping-rule-row" role="row">
      <code class="mapping-rule-model" role="cell">{{ source }}</code>
      <span class="mapping-rule-arrow" aria-hidden="true">→</span>
      <code
        class="mapping-rule-model"
        :class="{ 'mapping-rule-model--changed': highlightChanges && source !== target }"
        role="cell"
      >
        {{ target }}
      </code>
    </div>
  </div>
</template>

<style scoped>
.mapping-rule-table {
  display: flex;
  flex-direction: column;
  gap: 1px;
  width: 100%;
  overflow: hidden;
  border-radius: 8px;
  background: var(--cs-border);
  font-size: 13px;
  line-height: 20px;
}

.mapping-rule-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 24px minmax(0, 1fr);
  gap: 12px;
  align-items: start;
  padding: 13px 14px;
  background: var(--cs-surface);
}

.mapping-rule-head {
  background: var(--cs-canvas);
  color: var(--cs-text);
  font-weight: 500;
}

.mapping-rule-model {
  min-width: 0;
  color: var(--cs-text);
  font-family: var(--cs-font-mono);
  overflow-wrap: anywhere;
}

.mapping-rule-model--changed {
  color: var(--cs-brand);
  font-weight: 500;
}

.mapping-rule-arrow {
  color: var(--cs-text-secondary);
  text-align: center;
}

.mapping-rule-empty {
  padding: 18px 14px;
  background: var(--cs-surface);
  color: var(--cs-text-secondary);
  font-size: 12px;
  text-align: center;
}

.mapping-rule-table--compact {
  gap: 0;
  border-radius: 8px;
  background: var(--cs-surface);
  font-size: 12px;
  line-height: 18px;
}

.mapping-rule-table--compact .mapping-rule-row {
  padding: 0;
  background: var(--cs-surface);
}

.mapping-rule-table--compact .mapping-rule-head {
  margin-bottom: 5px;
  color: var(--cs-text-secondary);
  font-size: 11px;
  font-weight: 400;
  line-height: 17px;
}

.mapping-rule-table--compact .mapping-rule-empty {
  padding: 8px 0;
  text-align: left;
}

@media (max-width: 640px) {
  .mapping-rule-row {
    grid-template-columns: minmax(0, 1fr) 18px minmax(0, 1fr);
    gap: 6px;
    padding-inline: 10px;
  }

  .mapping-rule-table--compact .mapping-rule-row {
    padding-inline: 0;
  }
}
</style>
