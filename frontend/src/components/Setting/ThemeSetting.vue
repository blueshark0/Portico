<script setup lang="ts">
import { ref } from 'vue'
import { setTheme, getCurrentTheme, ThemeMode } from '../../utils/ThemeManager'

const themevalue = ref<ThemeMode>(getCurrentTheme())

const themeChange = () => {
  setTheme(themevalue.value)
}
</script>

<template>
  <select class="cs-select" style="width:auto;min-width:150px" v-model="themevalue" @change="themeChange">
    <option value="systemdefault">{{ $t('components.themesetting.select.opt_system') }}</option>
    <option value="light">{{ $t('components.themesetting.select.opt_light') }}</option>
    <option value="dark">{{ $t('components.themesetting.select.opt_dark') }}</option>
  </select>
</template>
