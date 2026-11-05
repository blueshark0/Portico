<script setup lang="ts">
import { computed } from 'vue'
import { i18n, setupI18n } from '../../utils/i18n'
import { Locale } from '../../locales'

const locale = computed({
  get: () => i18n.global.locale.value as Locale,
  set: (value: Locale) => {
    void setupI18n(value)
  },
})
</script>

<template>
  <select class="cs-select" style="width:auto;min-width:150px" v-model="locale">
    <option value="zh">简体中文</option>
    <option value="en">English</option>
  </select>
</template>
