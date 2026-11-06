<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import RuleTable from './RuleTable.vue'

withDefaults(
  defineProps<{
    activeName: string
    rules: Record<string, string>
    active?: boolean
    busy?: boolean
  }>(),
  { active: false, busy: false },
)

const emit = defineEmits<{
  (e: 'manage'): void
  (e: 'switch'): void
}>()

const { t } = useI18n()
</script>

<template>
  <section class="mapping-scheme-summary" :aria-label="t('modelMapping.title')">
    <div class="mapping-summary-head">
      <h3 class="mapping-summary-title">{{ t('modelMapping.title') }}</h3>
      <button type="button" class="mapping-manage-link" :disabled="busy" @click="emit('manage')">
        {{ t('modelMapping.manageAll') }}
      </button>
    </div>

    <div class="mapping-current-row">
      <strong class="mapping-current-name">{{ activeName }}</strong>
      <span class="cs-pill" :class="active ? 'cs-pill--success' : 'cs-pill--muted'">
        {{ active ? t('modelMapping.active') : t('modelMapping.noMapping') }}
      </span>
      <span class="mapping-current-spacer"></span>
      <button type="button" class="cs-btn" :disabled="busy" @click="emit('switch')">
        {{ t('modelMapping.switchScheme') }}
      </button>
    </div>

    <div v-if="active" class="mapping-summary-rules">
      <RuleTable
        :rules="rules"
        compact
        highlight-changes
        :target-label="t('modelMapping.actualModel')"
      />
    </div>
    <p v-else class="mapping-no-mapping">{{ t('modelMapping.noMappingHint') }}</p>

    <p class="mapping-summary-foot">
      <template v-if="active">
        {{ t('modelMapping.rulesCount', { count: Object.keys(rules ?? {}).length }) }} ·
        {{ t('modelMapping.unmatchedPassThrough') }}
      </template>
      <template v-else>{{ t('modelMapping.selectApplyHint') }}</template>
    </p>
  </section>
</template>

<style scoped>
.mapping-scheme-summary {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px 18px;
  overflow: hidden;
  border: 1px solid var(--cs-brand-tint-border);
  border-radius: 12px;
  background: var(--cs-brand-tint);
}

.mapping-summary-head,
.mapping-current-row {
  display: flex;
  align-items: center;
  width: 100%;
}

.mapping-summary-head {
  justify-content: space-between;
  gap: 16px;
}

.mapping-summary-title {
  margin: 0;
  color: var(--cs-text);
  font-size: 13px;
  font-weight: 500;
  line-height: 20px;
}

.mapping-manage-link {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--cs-brand);
  font-family: inherit;
  font-size: 12px;
  line-height: 18px;
  cursor: pointer;
}

.mapping-manage-link:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.mapping-current-row {
  gap: 12px;
}

.mapping-current-name {
  min-width: 0;
  color: var(--cs-text);
  font-size: 20px;
  line-height: 30px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mapping-current-row .cs-pill {
  flex-shrink: 0;
}

.mapping-current-spacer {
  flex: 1;
}

.mapping-summary-rules {
  padding: 10px;
  border-radius: 8px;
  background: var(--cs-surface);
}

.mapping-summary-foot,
.mapping-no-mapping {
  margin: 0;
  color: var(--cs-text-secondary);
  font-size: 11px;
  line-height: 17px;
}

@media (max-width: 620px) {
  .mapping-current-row {
    flex-wrap: wrap;
  }

  .mapping-current-spacer {
    display: none;
  }

  .mapping-current-row .cs-btn {
    margin-left: auto;
  }
}
</style>
