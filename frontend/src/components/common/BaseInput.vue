<template>
  <input
    ref="inputRef"
    v-bind="$attrs"
    :type="type"
    class="cs-input"
    :value="modelValue"
    autocorrect="off"
    autocapitalize="none"
    spellcheck="false"
    autocomplete="off"
    @input="onInput"
  />
</template>

<script setup lang="ts">
import { ref, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    modelValue?: string
    type?: string
  }>(),
  {
    modelValue: '',
    type: 'text',
  },
)

const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()
const inputRef = ref<HTMLInputElement | null>(null)

useAttrs()

const onInput = (event: Event) => {
  const target = event.target as HTMLInputElement
  emit('update:modelValue', target.value)
}

defineExpose({
  blur: () => inputRef.value?.blur(),
  focus: () => inputRef.value?.focus(),
})
</script>
