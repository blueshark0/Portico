<script setup lang="ts">
import { Plus } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import BaseInput from '../common/BaseInput.vue'
import type { MappingRuleError, MappingRuleRow } from './types'

const props = withDefaults(
  defineProps<{
    modelValue: MappingRuleRow[]
    errors?: MappingRuleError[]
    disabled?: boolean
  }>(),
  { errors: () => [], disabled: false },
)

const emit = defineEmits<{ (e: 'update:modelValue', value: MappingRuleRow[]): void }>()
const { t } = useI18n()

const updateRow = (index: number, field: keyof MappingRuleRow, value: string) => {
  const next = props.modelValue.map((row, rowIndex) =>
    rowIndex === index ? { ...row, [field]: value } : row,
  )
  emit('update:modelValue', next)
}

const addRow = () => {
  emit('update:modelValue', [...props.modelValue, { key: '', value: '' }])
}

const removeRow = (index: number) => {
  emit('update:modelValue', props.modelValue.filter((_, rowIndex) => rowIndex !== index))
}
</script>

<template>
  <div class="mapping-rule-editor">
    <div v-for="(row, index) in modelValue" :key="index" class="mapping-edit-row">
      <div class="mapping-edit-field">
        <BaseInput
          :model-value="row.key"
          class="cs-mono"
          :disabled="disabled"
          :placeholder="t('modelMapping.requestPlaceholder')"
          :aria-label="t('modelMapping.requestModel')"
          @update:model-value="updateRow(index, 'key', $event)"
        />
        <p v-if="errors[index]?.key" class="cs-field-error">{{ errors[index]?.key }}</p>
      </div>
      <span class="mapping-edit-arrow" aria-hidden="true">→</span>
      <div class="mapping-edit-field">
        <BaseInput
          :model-value="row.value"
          class="cs-mono"
          :disabled="disabled"
          :placeholder="t('modelMapping.targetPlaceholder')"
          :aria-label="t('modelMapping.targetModel')"
          @update:model-value="updateRow(index, 'value', $event)"
        />
        <p v-if="errors[index]?.value" class="cs-field-error">{{ errors[index]?.value }}</p>
      </div>
      <button
        type="button"
        class="cs-btn mapping-remove-btn"
        :disabled="disabled"
        @click="removeRow(index)"
      >
        {{ t('modelMapping.removeRule') }}
      </button>
    </div>

    <p v-if="modelValue.length === 0" class="mapping-editor-empty">{{ t('modelMapping.emptyEditRules') }}</p>

    <div>
      <button type="button" class="cs-btn" :disabled="disabled" @click="addRow">
        <Plus :size="14" :stroke-width="2" />
        {{ t('modelMapping.addRule') }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.mapping-rule-editor {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.mapping-edit-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 20px minmax(0, 1fr) 90px;
  gap: 12px;
  align-items: start;
}

.mapping-edit-field {
  min-width: 0;
}

.mapping-edit-field :deep(.cs-input) {
  min-height: 44px;
  padding: 11px 12px;
  font-size: 13px;
}

.mapping-edit-arrow {
  align-self: start;
  padding-top: 12px;
  color: var(--cs-text-secondary);
  text-align: center;
}

.mapping-remove-btn {
  width: 90px;
  min-height: 36px;
  margin-top: 4px;
}

.mapping-editor-empty {
  margin: 0;
  padding: 18px;
  border: 1px dashed var(--cs-border);
  border-radius: var(--cs-r-field);
  color: var(--cs-text-secondary);
  font-size: 12px;
  text-align: center;
}

@media (max-width: 760px) {
  .mapping-edit-row {
    grid-template-columns: minmax(0, 1fr) 90px;
  }

  .mapping-edit-arrow {
    display: none;
  }

  .mapping-edit-field:nth-of-type(2) {
    grid-column: 1;
  }

  .mapping-remove-btn {
    grid-column: 2;
    grid-row: 1;
  }
}
</style>
