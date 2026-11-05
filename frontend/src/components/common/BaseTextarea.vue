<template>
  <textarea
    ref="textareaRef"
    v-bind="$attrs"
    class="cs-textarea"
    :value="modelValue"
    autocorrect="off"
    autocapitalize="none"
    spellcheck="false"
    @input="onInput"
  />
</template>

<script setup lang="ts">
import { ref, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    modelValue?: string
  }>(),
  {
    modelValue: '',
  },
)

const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()
const textareaRef = ref<HTMLTextAreaElement | null>(null)

useAttrs()

const onInput = (event: Event) => {
  const target = event.target as HTMLTextAreaElement
  emit('update:modelValue', target.value)
}

defineExpose({
  blur: () => textareaRef.value?.blur(),
  focus: () => textareaRef.value?.focus(),
})
</script>
