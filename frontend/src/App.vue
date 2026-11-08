<script setup lang="ts">
import AppShell from './components/shell/AppShell.vue'
import LoginView from './components/Auth/Login.vue'
import { authRequired } from './services/auth'
</script>

<template>
  <LoginView v-if="authRequired" />
  <AppShell v-else />
</template>
