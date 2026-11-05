<template>
  <Teleport to="body">
    <div v-if="open" class="cs-modal-mask" @mousedown.self="onMaskClose">
      <div class="cs-modal" :class="widthClass" role="dialog" aria-modal="true">
        <header class="cs-modal-head">
          <h2 class="cs-modal-title">{{ title }}</h2>
          <p v-if="subtitle" class="cs-modal-sub">{{ subtitle }}</p>
        </header>
        <hr class="cs-divider" style="margin: 0 24px" />
        <div class="cs-modal-body">
          <slot />
        </div>
        <footer v-if="$slots.footer" class="cs-modal-foot">
          <slot name="footer" />
        </footer>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    subtitle?: string
    width?: 'narrow' | 'default' | 'wide'
  }>(),
  { subtitle: '', width: 'default' },
)

const emit = defineEmits<{ (e: 'close'): void }>()

const widthClass = computed(() => {
  if (props.width === 'narrow') return 'cs-modal--narrow'
  if (props.width === 'wide') return 'cs-modal--wide'
  return ''
})

const onMaskClose = () => {
  emit('close')
}

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') {
    event.stopPropagation()
    emit('close')
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) {
      window.addEventListener('keydown', onKeydown, true)
    } else {
      window.removeEventListener('keydown', onKeydown, true)
    }
  },
)
</script>
