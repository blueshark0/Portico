<template>
  <button :type="type" :class="['cs-btn', variantClass]" v-bind="$attrs">
    <slot />
  </button>
</template>

<script setup lang="ts">
import { computed, useAttrs } from 'vue'

const props = withDefaults(
  defineProps<{
    variant?: 'primary' | 'outline' | 'danger' | 'ghost' | 'tint'
    type?: 'button' | 'submit' | 'reset'
  }>(),
  {
    variant: 'primary',
    type: 'button',
  },
)

useAttrs()

const variantClass = computed(() => {
  switch (props.variant) {
    case 'danger':
      return 'cs-btn--danger'
    case 'tint':
      return 'cs-btn--tint'
    default:
      return ''
  }
})
</script>
