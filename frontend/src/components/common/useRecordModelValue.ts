import { computed, watch } from 'vue'

type RecordUpdateEmitter<T> = (event: 'update:modelValue', value: Record<string, T>) => void

type UseRecordModelValueOptions = {
  initializeEmpty?: boolean
}

export const useRecordModelValue = <T>(
  props: { modelValue?: Record<string, T> },
  emit: RecordUpdateEmitter<T>,
  options: UseRecordModelValueOptions = {},
) => {
  const recordValue = computed(() => props.modelValue ?? {})

  const updateRecord = (mutate: (draft: Record<string, T>) => void) => {
    const nextValue = { ...recordValue.value }
    mutate(nextValue)
    emit('update:modelValue', nextValue)
  }

  if (options.initializeEmpty) {
    watch(
      () => props.modelValue,
      (value) => {
        if (value === undefined) {
          emit('update:modelValue', {})
        }
      },
      { immediate: true },
    )
  }

  return {
    recordValue,
    updateRecord,
  }
}
