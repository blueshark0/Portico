<template>
  <div class="model-whitelist-editor">
    <div class="editor-header">
      <FieldHelpLabel
        v-model:open="examplesOpen"
        :label="$t('components.provider.modelWhitelist.label')"
        :tooltip="$t('components.provider.modelWhitelist.tooltip')"
        expandable
      />
    </div>

    <div v-if="modelList.length > 0" class="model-tags">
      <div
        v-for="(model, index) in modelList"
        :key="index"
        class="model-tag"
      >
        <span class="model-name" :class="{ wildcard: isWildcard(model) }">{{ model }}</span>
        <button
          type="button"
          class="tag-remove"
          :aria-label="$t('components.provider.modelWhitelist.remove')"
          @click="removeModel(index)"
        >
          <svg viewBox="0 0 12 12" width="10" height="10" aria-hidden="true">
            <path
              d="M3 3l6 6M9 3l-6 6"
              stroke="currentColor"
              stroke-width="1.5"
              stroke-linecap="round"
            />
          </svg>
        </button>
      </div>
    </div>

    <div class="model-input-row">
      <BaseInput
        v-model="newModel"
        type="text"
        :placeholder="$t('components.provider.modelWhitelist.placeholder')"
        @keydown.enter.prevent="addModel"
      />
      <BaseButton
        type="button"
        variant="outline"
        @click="addModel"
      >
        {{ $t('components.provider.modelWhitelist.add') }}
      </BaseButton>
    </div>

    <EditorHelpBox v-show="examplesOpen" :title="$t('components.provider.modelWhitelist.examples.title')">
      <ul class="help-list">
        <li>
          <code>claude-sonnet-4</code> - {{ $t('components.provider.modelWhitelist.examples.exact') }}
        </li>
        <li>
          <code>claude-*</code> - {{ $t('components.provider.modelWhitelist.examples.prefix') }}
        </li>
        <li>
          <code>anthropic/claude-*</code> - {{ $t('components.provider.modelWhitelist.examples.vendor') }}
        </li>
      </ul>
    </EditorHelpBox>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import BaseInput from './BaseInput.vue'
import BaseButton from './BaseButton.vue'
import EditorHelpBox from './EditorHelpBox.vue'
import FieldHelpLabel from './FieldHelpLabel.vue'
import { useRecordModelValue } from './useRecordModelValue'

interface Props {
  modelValue?: Record<string, boolean>
}

interface Emits {
  (e: 'update:modelValue', value: Record<string, boolean>): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { recordValue, updateRecord } = useRecordModelValue<boolean>(props, emit, {
  initializeEmpty: true,
})

const modelList = computed(() => {
  return Object.keys(recordValue.value).filter((key) => recordValue.value[key])
})

const examplesOpen = ref(false)
const newModel = ref('')

const isWildcard = (model: string) => model.includes('*')

const addModel = () => {
  const trimmed = newModel.value.trim()
  if (!trimmed) return

  if (recordValue.value[trimmed]) {
    newModel.value = ''
    return
  }

  updateRecord((updated) => {
    updated[trimmed] = true
  })
  newModel.value = ''
}

const removeModel = (index: number) => {
  const modelName = modelList.value[index]
  if (!modelName) return

  updateRecord((updated) => {
    delete updated[modelName]
  })
}
</script>

<style scoped>
.model-whitelist-editor {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.editor-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.model-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.model-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 24px;
  padding: 0 4px 0 9px;
  background: var(--cs-brand-tint);
  border-radius: var(--cs-r-pill);
  font-size: 10px;
  font-weight: 500;
}

.model-name {
  color: var(--cs-brand);
  font-family: var(--cs-font-mono);
}

.tag-remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border: none;
  background: none;
  color: var(--cs-brand);
  cursor: pointer;
  border-radius: 3px;
  opacity: 0.6;
  transition: opacity 0.15s;
}

.tag-remove:hover {
  opacity: 1;
}

.model-input-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.model-input-row :deep(input) {
  flex: 1;
  font-family: var(--cs-font-mono);
}
</style>
