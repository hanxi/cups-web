<template>
  <div class="flex items-center justify-center h-full p-3 sm:p-4 md:p-6">
    <UCard class="w-full max-w-md shadow-lg">
      <template #header>
        <h2 class="text-xl font-bold flex items-center gap-2">
          <UIcon name="i-lucide-user" class="w-5 h-5" /> {{ t('ui.sign_in') }} </h2>
      </template>
      
      <!-- 错误提示 -->
      <UAlert
        v-if="errorText"
        icon="i-lucide-triangle-alert"
        color="error"
        variant="soft"
        :title="errorText"
        class="mb-6"
      />
      
      <UForm @submit="login" :state="state" class="space-y-6">
        <UFormField :label="t('ui.username_2')" name="username" required>
          <UInput v-model="state.username" icon="i-lucide-user" size="lg" class="w-full" />
        </UFormField>
        <UFormField :label="t('ui.password')" name="password" required>
          <UInput v-model="state.password" type="password" icon="i-lucide-lock" size="lg" class="w-full" />
        </UFormField>
        
        <div class="mt-6">
          <UButton 
            type="submit" 
            color="primary" 
            icon="i-lucide-log-in" 
            size="lg"
            class="w-full"
            :loading="loading"
          > {{ t('ui.sign_in') }} </UButton>
        </div>
      </UForm>
    </UCard>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { reactive, ref } from 'vue'
import { translateError } from '../utils/translate-error.js'

const { t } = useI18n()

const state = reactive({
  username: '',
  password: ''
})
const error = ref(null)
const errorText = computed(() => error.value?.() || '')
const loading = ref(false)

const emit = defineEmits(['login-success'])

// 后端 /api/login 的错误串是英文短语，直接展示对终端用户不友好，在此本地化。
const BACKEND_MESSAGES = computed(() => ({
  'invalid credentials': t('ui.incorrect_username_or_password'),
  'missing credentials': t('ui.enter_your_username_and_password'),
  'too many attempts, please try again later': t('ui.too_many_failed_sign_in_attempts_try_again_later_default'),
  'login failed': t('ui.the_server_could_not_process_sign_in_check_the_server'),
  'session error': t('ui.could_not_create_a_session_check_the_server_logs')
}))

// 把失败响应翻译成可诊断的中文提示。
//
// 关键约定：响应体不是后端 JSON 时，绝不能兜底显示「用户名或密码错误」。这类失败说明
// 请求根本没走到 LoginHandler（被跨源防护、反向代理或网关拦掉），谎报成凭据问题会把
// 排查方向彻底带偏 —— 这正是 issue #99 里「内网能登录，反代后显示密码错误」的由来。
async function describeFailure(resp) {
  // body 只能读一次，先取文本再自行尝试解析。
  let raw = ''
  try {
    raw = await resp.text()
  } catch {
    raw = ''
  }

  let payload = null
  try {
    payload = JSON.parse(raw)
  } catch {
    payload = null
  }

  return () => {
  if (payload && (payload.error || payload.message)) {
    const msg = payload.error || payload.message
    return BACKEND_MESSAGES.value[msg] || translateError(msg, payload.code, payload)
  }

  const looksLikeHTML = raw.trimStart().startsWith('<')
  const detail = looksLikeHTML
    ? t('ui.the_server_returned_html_usually_an_error_page_from_a')
    : raw.trim().slice(0, 200)

  if (resp.status === 403) {
    return t('ui.request_rejected_403_if_using_a_reverse_proxy_check_that', { p0: detail })
  }
  if (resp.status === 404 || resp.status === 405) {
    return t('ui.endpoint_unavailable_api_login_did_not_reach_cups_web_check', { p0: resp.status, p1: detail })
  }
  if (resp.status >= 500) {
    return t('ui.server_error_check_the_server_logs', { p0: resp.status, p1: detail })
  }
  return t('ui.sign_in_failed_http', { p0: resp.status, p1: detail ? ': ' + detail : '' })
  }
}

async function login() {
  error.value = null
  loading.value = true
  try {
    const resp = await fetch('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: state.username, password: state.password }),
      credentials: 'include'
    })
    if (!resp.ok) {
      error.value = await describeFailure(resp)
      return
    }
    emit('login-success')
  } catch (e) {
    // fetch 本身抛异常＝请求没能完成（网络不可达、TLS 失败、被浏览器策略阻断）。
    error.value = () => t('ui.cannot_connect_to_the_server', { p0: e.message })
  } finally {
    loading.value = false
  }
}
</script>
