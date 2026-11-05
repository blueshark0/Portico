<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseModal from '../common/BaseModal.vue'
import BaseInput from '../common/BaseInput.vue'
import BaseTextarea from '../common/BaseTextarea.vue'
import { ServerType } from '../../services/mcp'
import type { McpServerForm } from './useMcpServers'

const props = defineProps<{
  state: {
    open: boolean
    form: McpServerForm
    error: string
    saving: boolean
    testing: boolean
  }
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'submit'): void
  (e: 'submitAndTest'): void
}>()

const { t } = useI18n()

type AuthMode = 'none' | 'bearer' | 'custom'
const authMode = ref<AuthMode>('none')
const bearerToken = ref('')

const isEdit = computed(() => props.state.form.id > 0)
const isStdio = computed(() => props.state.form.type === ServerType.ServerTypeStdio)

const title = computed(() =>
  isEdit.value ? t('components.mcp.form.titleEdit') : t('components.mcp.form.titleNew'),
)
const subtitle = computed(() =>
  isStdio.value ? t('components.mcp.form.subtitleStdio') : t('components.mcp.form.subtitleHttp'),
)

// 打开弹窗时从 headersText 反推认证方式：
// 仅含一条 Authorization=Bearer xxx → bearer；其它非空 → custom；空 → none。
watch(
  () => props.state.open,
  (open) => {
    if (!open) return
    const headers = props.state.form.headersText.trim()
    const bearerMatch = headers.match(/^Authorization\s*=\s*Bearer\s+(.+)$/i)
    if (bearerMatch) {
      authMode.value = 'bearer'
      bearerToken.value = bearerMatch[1].trim()
    } else if (headers) {
      authMode.value = 'custom'
      bearerToken.value = ''
    } else {
      authMode.value = 'none'
      bearerToken.value = ''
    }
  },
)

// 提交前把认证方式写回 headersText（请求头行格式 KEY=VALUE）。
const syncAuthToForm = () => {
  if (authMode.value === 'bearer') {
    props.state.form.headersText = bearerToken.value.trim()
      ? `Authorization=Bearer ${bearerToken.value.trim()}`
      : ''
  } else if (authMode.value === 'none') {
    props.state.form.headersText = ''
  }
}

const submit = (andTest: boolean) => {
  syncAuthToForm()
  if (andTest) {
    emit('submitAndTest')
  } else {
    emit('submit')
  }
}
</script>

<template>
  <BaseModal :open="state.open" :title="title" :subtitle="subtitle" @close="emit('close')">
    <div class="medit">
      <div>
        <label class="cs-label">{{ t('components.mcp.form.name') }}</label>
        <BaseInput v-model="state.form.name" :placeholder="t('components.mcp.form.namePlaceholder')" />
      </div>

      <div>
        <label class="cs-label">{{ t('components.mcp.form.type') }}</label>
        <select v-model="state.form.type" class="cs-select">
          <option :value="ServerType.ServerTypeStdio">stdio</option>
          <option :value="ServerType.ServerTypeHTTP">Streamable HTTP</option>
        </select>
      </div>

      <template v-if="isStdio">
        <div>
          <label class="cs-label">{{ t('components.mcp.form.command') }}</label>
          <BaseInput v-model="state.form.command" class="medit-mono" placeholder="uv / npx / /usr/local/bin/xxx" />
        </div>
        <div>
          <label class="cs-label">{{ t('components.mcp.form.args') }}</label>
          <BaseInput v-model="state.form.argsText" class="medit-mono" :placeholder="t('components.mcp.form.argsPlaceholder')" />
          <p class="cs-field-hint">{{ t('components.mcp.form.argsHint') }}</p>
        </div>
        <div>
          <label class="cs-label">{{ t('components.mcp.form.env') }}</label>
          <BaseTextarea v-model="state.form.envText" class="medit-mono" :placeholder="t('components.mcp.form.kvPlaceholder')" rows="3" />
        </div>
      </template>

      <template v-else>
        <div>
          <label class="cs-label">{{ t('components.mcp.form.url') }}</label>
          <BaseInput v-model="state.form.url" class="medit-mono" placeholder="https://example.com/mcp" />
        </div>
        <div>
          <label class="cs-label">{{ t('components.mcp.form.auth') }}</label>
          <select v-model="authMode" class="cs-select">
            <option value="none">{{ t('components.mcp.form.authNone') }}</option>
            <option value="bearer">Bearer Token</option>
            <option value="custom">{{ t('components.mcp.form.authCustom') }}</option>
          </select>
        </div>
        <div v-if="authMode === 'bearer'">
          <label class="cs-label">{{ t('components.mcp.form.token') }}</label>
          <BaseInput v-model="bearerToken" class="medit-mono" placeholder="token" />
        </div>
        <div v-else-if="authMode === 'custom'">
          <label class="cs-label">{{ t('components.mcp.form.headers') }}</label>
          <BaseTextarea v-model="state.form.headersText" class="medit-mono" :placeholder="t('components.mcp.form.kvPlaceholder')" rows="3" />
        </div>
      </template>

      <div>
        <label class="cs-label">{{ t('components.mcp.form.timeout') }}</label>
        <BaseInput v-model="state.form.timeoutText" type="number" min="1" max="300" />
      </div>

      <p v-if="state.error" class="cs-field-error">{{ state.error }}</p>
    </div>

    <template #footer>
      <button type="button" class="cs-btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button type="button" class="cs-btn" :disabled="state.saving" @click="submit(false)">
        {{ t('common.save') }}
      </button>
      <button
        type="button"
        class="cs-btn cs-btn--primary"
        :disabled="state.saving || state.testing"
        @click="submit(true)"
      >
        {{ state.testing ? t('components.mcp.form.testing') : t('components.mcp.form.saveAndTest') }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.medit {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding-bottom: 10px;
}

.medit-mono {
  font-family: var(--cs-font-mono);
}
</style>
