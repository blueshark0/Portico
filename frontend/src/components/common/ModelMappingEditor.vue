<template>
  <div class="model-mapping-editor">
    <div class="editor-header">
      <FieldHelpLabel
        v-model:open="examplesOpen"
        :label="$t('components.provider.modelMapping.label')"
        :tooltip="$t('components.provider.modelMapping.tooltip')"
        expandable
      />
    </div>

    <div v-if="mappingList.length > 0" class="mapping-list">
      <div
        v-for="mapping in mappingList"
        :key="mapping.key"
        class="mapping-row"
      >
        <template v-if="editingKey === mapping.key">
          <div class="mapping-content">
            <BaseInput
              ref="editKeyInputRefs"
              v-model="editKey"
              type="text"
              @keydown.enter.prevent="saveEdit"
              @keydown.esc.prevent="cancelEdit"
            />
            <svg class="mapping-arrow" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
              <path
                d="M6 4l4 4-4 4"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
            <BaseInput
              v-model="editValue"
              type="text"
              @keydown.enter.prevent="saveEdit"
              @keydown.esc.prevent="cancelEdit"
            />
          </div>
          <div class="mapping-actions">
            <button
              type="button"
              class="mapping-icon-btn mapping-confirm"
              :aria-label="$t('components.provider.modelMapping.confirm')"
              @click="saveEdit"
            >
              <svg viewBox="0 0 24 24" width="12" height="12" aria-hidden="true">
                <path
                  d="M20 6L9 17l-5-5"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
            </button>
            <button
              type="button"
              class="mapping-icon-btn mapping-cancel"
              :aria-label="$t('components.provider.modelMapping.cancel')"
              @click="cancelEdit"
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
        </template>
        <template v-else>
          <div class="mapping-content">
            <code class="mapping-key" :class="{ wildcard: isWildcard(mapping.key) }">
              {{ mapping.key }}
            </code>
            <svg class="mapping-arrow" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
              <path
                d="M6 4l4 4-4 4"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
            <code class="mapping-value" :class="{ wildcard: isWildcard(mapping.value) }">
              {{ mapping.value }}
            </code>
          </div>
          <div class="mapping-actions">
            <button
              type="button"
              class="mapping-icon-btn mapping-edit"
              :aria-label="$t('components.provider.modelMapping.edit')"
              @click="startEdit(mapping)"
            >
              <svg viewBox="0 0 24 24" width="12" height="12" aria-hidden="true">
                <path
                  d="M17 3a2.85 2.85 0 114 4L7.5 20.5 2 22l1.5-5.5L17 3z"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="1.8"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
            </button>
            <button
              type="button"
              class="mapping-icon-btn mapping-remove"
              :aria-label="$t('components.provider.modelMapping.remove')"
              @click="removeMapping(mapping.key)"
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
        </template>
      </div>
    </div>

    <div class="mapping-input-row">
      <BaseInput
        v-model="newKey"
        type="text"
        :placeholder="$t('components.provider.modelMapping.keyPlaceholder')"
        @keydown.enter.prevent="focusValueInput"
      />
      <svg class="input-arrow" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
        <path
          d="M6 4l4 4-4 4"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
      <BaseInput
        ref="valueInputRef"
        v-model="newValue"
        type="text"
        :placeholder="$t('components.provider.modelMapping.valuePlaceholder')"
        @keydown.enter.prevent="addMapping"
      />
      <BaseButton
        type="button"
        variant="outline"
        @click="addMapping"
      >
        {{ $t('components.provider.modelMapping.add') }}
      </BaseButton>
    </div>

    <EditorHelpBox v-show="examplesOpen" :title="$t('components.provider.modelMapping.examples.title')">
      <ul class="help-list">
        <li>
          <code>claude-sonnet-4</code> → <code>anthropic/claude-sonnet-4</code><br />
          <span class="help-desc">{{ $t('components.provider.modelMapping.examples.exact') }}</span>
        </li>
        <li>
          <code>claude-*</code> → <code>anthropic/claude-*</code><br />
          <span class="help-desc">{{ $t('components.provider.modelMapping.examples.wildcard') }}</span>
        </li>
        <li>
          <code>gpt-*</code> → <code>openai/gpt-*</code><br />
          <span class="help-desc">{{ $t('components.provider.modelMapping.examples.prefix') }}</span>
        </li>
      </ul>
    </EditorHelpBox>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick } from 'vue'
import BaseInput from './BaseInput.vue'
import BaseButton from './BaseButton.vue'
import EditorHelpBox from './EditorHelpBox.vue'
import FieldHelpLabel from './FieldHelpLabel.vue'
import { useRecordModelValue } from './useRecordModelValue'

interface Props {
  modelValue?: Record<string, string>
}

interface Emits {
  (e: 'update:modelValue', value: Record<string, string>): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { recordValue, updateRecord } = useRecordModelValue<string>(props, emit)

const mappingList = computed(() => {
  return Object.entries(recordValue.value).map(([key, value]) => ({ key, value }))
})

const examplesOpen = ref(false)
const newKey = ref('')
const newValue = ref('')
const valueInputRef = ref<{ focus: () => void } | null>(null)

// 行内编辑：editingKey 记录正在编辑的条目键，避免依赖会随增删变化的下标。
const editingKey = ref<string | null>(null)
const editKey = ref('')
const editValue = ref('')
const editKeyInputRefs = ref<{ focus: () => void }[]>([])

const isWildcard = (text: string) => text.includes('*')

const focusValueInput = () => {
  valueInputRef.value?.focus()
}

const startEdit = (mapping: { key: string; value: string }) => {
  editingKey.value = mapping.key
  editKey.value = mapping.key
  editValue.value = mapping.value
  nextTick(() => editKeyInputRefs.value[0]?.focus())
}

const saveEdit = () => {
  const oldKey = editingKey.value
  if (!oldKey) return
  const key = editKey.value.trim()
  const value = editValue.value.trim()
  if (!key || !value) return

  // 与新增一致：目标键已存在时覆盖；改键时删除旧键（条目会移到列表末尾）。
  updateRecord((updated) => {
    if (key !== oldKey) {
      delete updated[oldKey]
    }
    updated[key] = value
  })
  editingKey.value = null
}

const cancelEdit = () => {
  editingKey.value = null
}

const addMapping = () => {
  const key = newKey.value.trim()
  const value = newValue.value.trim()

  if (!key || !value) return

  updateRecord((updated) => {
    updated[key] = value
  })
  newKey.value = ''
  newValue.value = ''
}

const removeMapping = (key: string) => {
  updateRecord((updated) => {
    delete updated[key]
  })
}
</script>

<style scoped>
.model-mapping-editor {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.editor-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.mapping-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.mapping-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 10px;
  background: var(--cs-field);
  border: 1px solid var(--cs-border);
  border-radius: 8px;
}

.mapping-content {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
}

.mapping-key,
.mapping-value {
  padding: 3px 7px;
  background: var(--cs-surface);
  border: 1px solid var(--cs-border);
  border-radius: 4px;
  font-family: var(--cs-font-mono);
  font-size: 10px;
  color: var(--cs-text);
  word-break: break-all;
}

.mapping-key.wildcard,
.mapping-value.wildcard {
  color: var(--cs-brand);
  font-weight: 500;
}

.mapping-arrow,
.input-arrow {
  flex-shrink: 0;
  color: var(--cs-text-secondary);
}

.mapping-content :deep(input),
.mapping-input-row :deep(input) {
  flex: 1;
  min-width: 0;
  font-family: var(--cs-font-mono);
}

.mapping-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
}

.mapping-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 4px;
  border: none;
  background: none;
  color: var(--cs-text-secondary);
  cursor: pointer;
  border-radius: 4px;
  transition: color 0.15s, background 0.15s;
}

.mapping-edit:hover,
.mapping-confirm:hover {
  color: var(--cs-brand);
  background: var(--cs-brand-tint);
}

.mapping-remove:hover {
  color: var(--cs-danger);
  background: var(--cs-danger-tint);
}

.mapping-cancel:hover {
  color: var(--cs-text);
  background: var(--cs-row-hover);
}

.mapping-input-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
</style>
