<template>
  <div class="request-body-replacement-editor">
    <div class="editor-section">
      <div class="editor-header">
        <FieldHelpLabel
          v-model:open="overrideExamplesOpen"
          :label="$t('components.provider.requestBodyReplacements.overrideLabel')"
          :tooltip="$t('components.provider.requestBodyReplacements.overrideTooltip')"
          expandable
        />
      </div>

      <div v-if="overrideEntries.length > 0" class="replacement-list">
        <div
          v-for="{ entry, index } in overrideEntries"
          :key="index"
          class="replacement-row"
        >
          <div class="replacement-content">
            <code class="replacement-path">{{ entry.path }}</code>
            <svg class="replacement-arrow" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
              <path
                d="M6 4l4 4-4 4"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
            <code class="replacement-value">{{ entry.value }}</code>
          </div>
          <button
            type="button"
            class="replacement-remove"
            :aria-label="$t('components.provider.requestBodyReplacements.remove')"
            @click="removeEntry(index)"
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

      <div class="replacement-inputs">
        <BaseInput
          v-model="newOverridePath"
          type="text"
          :placeholder="$t('components.provider.requestBodyReplacements.pathPlaceholder')"
          @keydown.enter.prevent="focusOverrideValueInput"
        />
        <BaseTextarea
          ref="overrideValueInputRef"
          v-model="newOverrideValue"
          rows="3"
          :placeholder="$t('components.provider.requestBodyReplacements.valuePlaceholder')"
        />
        <BaseButton type="button" variant="outline" @click="addOverride">
          {{ $t('components.provider.requestBodyReplacements.add') }}
        </BaseButton>
      </div>

      <EditorHelpBox
        v-show="overrideExamplesOpen"
        :title="$t('components.provider.requestBodyReplacements.overrideExamples.title')"
      >
        <ul class="help-list">
          <li>
            <code>metadata.user_id</code> → <code>"fixed-user"</code><br />
            <span class="help-desc">{{ $t('components.provider.requestBodyReplacements.overrideExamples.string') }}</span>
          </li>
          <li>
            <code>temperature</code> → <code>0.2</code><br />
            <span class="help-desc">{{ $t('components.provider.requestBodyReplacements.overrideExamples.number') }}</span>
          </li>
          <li>
            <code>thinking</code> → <code>{"type":"disabled"}</code><br />
            <span class="help-desc">{{ $t('components.provider.requestBodyReplacements.overrideExamples.object') }}</span>
          </li>
        </ul>
      </EditorHelpBox>
    </div>

    <div class="editor-section">
      <div class="editor-header">
        <FieldHelpLabel
          v-model:open="replaceExamplesOpen"
          :label="$t('components.provider.requestBodyReplacements.replaceLabel')"
          :tooltip="$t('components.provider.requestBodyReplacements.replaceTooltip')"
          expandable
        />
      </div>

      <div v-if="replaceEntries.length > 0" class="replacement-list">
        <div
          v-for="{ entry, index } in replaceEntries"
          :key="index"
          class="replacement-row"
        >
          <div class="replacement-content">
            <code class="replacement-path">{{ entry.path }}</code>
            <span class="replacement-match-op">==</span>
            <code class="replacement-match">{{ entry.matchValue }}</code>
            <svg class="replacement-arrow" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
              <path
                d="M6 4l4 4-4 4"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
            <code class="replacement-value">{{ entry.value }}</code>
          </div>
          <button
            type="button"
            class="replacement-remove"
            :aria-label="$t('components.provider.requestBodyReplacements.remove')"
            @click="removeEntry(index)"
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

      <div class="replacement-inputs">
        <BaseInput
          v-model="newReplacePath"
          type="text"
          :placeholder="$t('components.provider.requestBodyReplacements.pathPlaceholder')"
          @keydown.enter.prevent="focusReplaceMatchInput"
        />
        <BaseInput
          ref="replaceMatchInputRef"
          v-model="newReplaceMatch"
          type="text"
          :placeholder="$t('components.provider.requestBodyReplacements.matchValuePlaceholder')"
          @keydown.enter.prevent="focusReplaceValueInput"
        />
        <BaseTextarea
          ref="replaceValueInputRef"
          v-model="newReplaceValue"
          rows="3"
          :placeholder="$t('components.provider.requestBodyReplacements.valuePlaceholder')"
        />
        <BaseButton type="button" variant="outline" @click="addReplace">
          {{ $t('components.provider.requestBodyReplacements.add') }}
        </BaseButton>
      </div>

      <EditorHelpBox
        v-show="replaceExamplesOpen"
        :title="$t('components.provider.requestBodyReplacements.replaceExamples.title')"
      >
        <ul class="help-list">
          <li>
            <code>temperature</code> == <code>1</code> → <code>0.2</code><br />
            <span class="help-desc">{{ $t('components.provider.requestBodyReplacements.replaceExamples.hint') }}</span>
          </li>
          <li>
            <code>model</code> == <code>"claude-sonnet-4"</code> → <code>"vendor/claude-sonnet-4"</code><br />
            <span class="help-desc">{{ $t('components.provider.requestBodyReplacements.replaceExamples.string') }}</span>
          </li>
        </ul>
      </EditorHelpBox>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import BaseButton from './BaseButton.vue'
import BaseInput from './BaseInput.vue'
import BaseTextarea from './BaseTextarea.vue'
import EditorHelpBox from './EditorHelpBox.vue'
import FieldHelpLabel from './FieldHelpLabel.vue'
import type { RequestBodyReplacement } from '../../../bindings/portico/internal/routing/domain/models'

interface Props {
  modelValue?: RequestBodyReplacement[]
}

interface Emits {
  (e: 'update:modelValue', value: RequestBodyReplacement[]): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const entries = computed(() => props.modelValue ?? [])
// matchValue 非空为条件替换，为空为覆写；带原始下标供删除用。
const withIndex = computed(() => entries.value.map((entry, index) => ({ entry, index })))
const overrideEntries = computed(() => withIndex.value.filter(({ entry }) => !entry.matchValue))
const replaceEntries = computed(() => withIndex.value.filter(({ entry }) => !!entry.matchValue))

const overrideExamplesOpen = ref(false)
const replaceExamplesOpen = ref(false)

const newOverridePath = ref('')
const newOverrideValue = ref('')
const overrideValueInputRef = ref<{ focus: () => void } | null>(null)

const newReplacePath = ref('')
const newReplaceMatch = ref('')
const newReplaceValue = ref('')
const replaceMatchInputRef = ref<{ focus: () => void } | null>(null)
const replaceValueInputRef = ref<{ focus: () => void } | null>(null)

const focusOverrideValueInput = () => {
  overrideValueInputRef.value?.focus()
}
const focusReplaceMatchInput = () => {
  replaceMatchInputRef.value?.focus()
}
const focusReplaceValueInput = () => {
  replaceValueInputRef.value?.focus()
}

// 覆写在前、替换在后稳定分区，保证存储顺序与两个分区的展示顺序一致。
const emitEntries = (next: RequestBodyReplacement[]) => {
  emit('update:modelValue', [
    ...next.filter((entry) => !entry.matchValue),
    ...next.filter((entry) => !!entry.matchValue),
  ])
}

const addOverride = () => {
  const path = newOverridePath.value.trim()
  const value = newOverrideValue.value.trim()
  if (!path || !value) return

  emitEntries([...entries.value, { path, value }])
  newOverridePath.value = ''
  newOverrideValue.value = ''
}

const addReplace = () => {
  const path = newReplacePath.value.trim()
  const matchValue = newReplaceMatch.value.trim()
  const value = newReplaceValue.value.trim()
  if (!path || !matchValue || !value) return

  emitEntries([...entries.value, { path, matchValue, value }])
  newReplacePath.value = ''
  newReplaceMatch.value = ''
  newReplaceValue.value = ''
}

const removeEntry = (index: number) => {
  emitEntries(entries.value.filter((_, itemIndex) => itemIndex !== index))
}
</script>

<style scoped>
.editor-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.editor-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.replacement-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.replacement-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 10px;
  background: var(--cs-field);
  border: 1px solid var(--cs-border);
  border-radius: 8px;
}

.replacement-content {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
}

.replacement-path,
.replacement-value,
.replacement-match {
  padding: 3px 7px;
  background: var(--cs-surface);
  border: 1px solid var(--cs-border);
  border-radius: 4px;
  font-family: var(--cs-font-mono);
  font-size: 10px;
  color: var(--cs-text);
  word-break: break-all;
}

.replacement-path {
  color: var(--cs-brand);
  font-weight: 500;
}

.replacement-match {
  max-width: 35%;
  color: var(--cs-warning);
}

.replacement-value {
  max-width: 45%;
}

.replacement-match-op {
  flex-shrink: 0;
  font-family: var(--cs-font-mono);
  font-size: 10px;
  color: var(--cs-text-secondary);
}

.replacement-arrow {
  flex-shrink: 0;
  color: var(--cs-text-secondary);
}

.replacement-remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 4px;
  border: none;
  background: none;
  color: var(--cs-text-secondary);
  cursor: pointer;
  border-radius: 4px;
  flex-shrink: 0;
  transition: color 0.15s, background 0.15s;
}

.replacement-remove:hover {
  color: var(--cs-danger);
  background: var(--cs-danger-tint);
}

.replacement-inputs {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 8px;
}

.replacement-inputs :deep(input),
.replacement-inputs :deep(textarea) {
  font-family: var(--cs-font-mono);
}

.replacement-inputs :deep(textarea) {
  min-height: 72px;
  resize: vertical;
}
</style>
