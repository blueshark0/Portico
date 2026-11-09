<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseButton from '../common/BaseButton.vue'
import BaseInput from '../common/BaseInput.vue'
import { probeSession, setToken } from '../../services/auth'
import { getErrorMessage } from '../../utils/errors'

const { t } = useI18n()

const token = ref('')
const error = ref('')
const busy = ref(false)

const submit = async () => {
  const value = token.value.trim()
  if (!value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    if (!(await probeSession(value))) {
      error.value = t('auth.invalidToken')
      return
    }
    setToken(value)
    // 重新加载以便各页面在已认证状态下重新拉取数据。
    window.location.reload()
  } catch (err) {
    error.value = getErrorMessage(err, t('auth.unreachable'))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login">
    <form class="cs-card cs-card-pad login-card" @submit.prevent="submit">
      <div class="login-logo">
        <span class="login-logo-bar login-logo-bar--blue" />
        <span class="login-logo-bar login-logo-bar--muted" />
      </div>
      <h1 class="cs-page-title login-title">{{ t('auth.title') }}</h1>
      <p class="cs-page-sub">{{ t('auth.subtitle') }}</p>
      <BaseInput
        v-model="token"
        type="password"
        class="cs-mono login-input"
        :placeholder="t('auth.placeholder')"
        :disabled="busy"
        autofocus
      />
      <p v-if="error" class="cs-field-error">{{ error }}</p>
      <BaseButton type="submit" :disabled="busy || !token.trim()">
        {{ busy ? t('auth.verifying') : t('auth.submit') }}
      </BaseButton>
      <p class="login-hint">
        {{ t('auth.hint') }}
        <code class="cs-mono">portico-server token show</code>
      </p>
    </form>
  </div>
</template>

<style scoped>
.login {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: var(--cs-canvas);
}

.login-card {
  width: 100%;
  max-width: 400px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.login-logo {
  display: flex;
  gap: 4px;
  margin-bottom: 4px;
}

.login-logo-bar {
  width: 6px;
  height: 22px;
  border-radius: 3px;
}

.login-logo-bar--blue {
  background: var(--cs-brand);
}

.login-logo-bar--muted {
  background: var(--cs-border);
}

.login-title {
  margin: 0;
}

.login-input {
  width: 100%;
}

.login-hint {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--cs-text-secondary);
  line-height: 1.6;
}

.login-hint code {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--cs-field);
  border: 1px solid var(--cs-border);
}
</style>
