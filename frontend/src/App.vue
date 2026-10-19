<template>
  <UApp :locale="uiLocale">
    <div v-if="!sessionLoaded" class="flex items-center justify-center min-h-screen bg-default">
      <UIcon name="i-lucide-loader-circle" class="w-8 h-8 animate-spin text-primary" />
    </div>
    <div v-else class="grid grid-rows-[auto_1fr_auto] min-h-screen w-full bg-default">
      <header class="flex flex-wrap items-center justify-between gap-2 px-4 sm:px-6 py-3 border-b border-default bg-default">
        <div class="flex items-center gap-3 min-w-0">
          <h1 class="text-xl font-bold shrink-0">{{ t('ui.cups_printing') }}</h1>
          <span v-if="session" class="text-sm text-muted truncate">{{ session.username }}</span>
        </div>
        <div class="flex items-center gap-2">
          <!-- 桌面端（sm+）：分段按钮 + 文字，一目了然 -->
          <LanguageSelect />
          <div class="hidden sm:flex items-center gap-2">
            <!-- 导航分段容器：与主 CTA 视觉区分。
                 「打印 / 定时」对所有登录用户开放；「管理 / 驱动」仅管理员可见。 -->
            <div
              v-if="session"
              class="flex items-center gap-0.5 p-0.5 rounded-lg bg-elevated/60 border border-default"
            >
              <UButton
                :variant="route.path === '/print' ? 'soft' : 'ghost'"
                :color="route.path === '/print' ? 'primary' : 'neutral'"
                size="xs"
                icon="i-lucide-file-text"
                @click="router.push('/print')"
              > {{ t('ui.print') }} </UButton>
              <UButton
                :variant="route.path === '/scheduled' ? 'soft' : 'ghost'"
                :color="route.path === '/scheduled' ? 'primary' : 'neutral'"
                size="xs"
                icon="i-lucide-calendar-clock"
                @click="router.push('/scheduled')"
              > {{ t('ui.schedules') }} </UButton>
              <UButton
                :variant="route.path === '/scan' ? 'soft' : 'ghost'"
                :color="route.path === '/scan' ? 'primary' : 'neutral'"
                size="xs"
                icon="i-lucide-scan-line"
                @click="router.push('/scan')"
              > {{ t('ui.scan') }} </UButton>
              <UButton
                v-if="canManageKeys"
                :variant="route.path === '/api-keys' ? 'soft' : 'ghost'"
                :color="route.path === '/api-keys' ? 'primary' : 'neutral'"
                size="xs"
                icon="i-lucide-key-round"
                @click="router.push('/api-keys')"
              > {{ t('ui.api_keys') }} </UButton>
              <UButton
                v-if="isAdmin"
                :variant="route.path === '/admin' ? 'soft' : 'ghost'"
                :color="route.path === '/admin' ? 'primary' : 'neutral'"
                size="xs"
                icon="i-lucide-settings"
                @click="router.push('/admin')"
              > {{ t('ui.administration') }} </UButton>
              <UButton
                v-if="isAdmin"
                :variant="route.path === '/drivers' ? 'soft' : 'ghost'"
                :color="route.path === '/drivers' ? 'primary' : 'neutral'"
                size="xs"
                icon="i-lucide-puzzle"
                @click="router.push('/drivers')"
              > {{ t('ui.drivers') }} </UButton>
            </div>
            <UButton
              v-if="session"
              variant="ghost"
              color="neutral"
              size="xs"
              icon="i-lucide-log-out"
              @click="logout"
            > {{ t('ui.sign_out') }} </UButton>
          </div>
          <!-- 移动端（<sm）：折叠为汉堡菜单，图标+文字，易点易读 -->
          <UDropdownMenu
            v-if="session"
            :items="menuItems"
            :content="{ align: 'end' }"
            class="sm:hidden"
          >
            <UButton variant="ghost" color="neutral" size="sm" icon="i-lucide-menu" :aria-label="t('language.navigation')" square />
          </UDropdownMenu>
        </div>
      </header>
      <div class="overflow-auto relative">
        <router-view :session="session" @login-success="onLogin" @logout="onLogout" />
      </div>
      <footer class="px-6 py-3 border-t border-default bg-default text-sm text-muted flex items-center justify-center gap-3 flex-wrap">
        <span>
          {{ t('language.powered_by') }}
          <a href="https://github.com/hanxi/cups-web" target="_blank" class="text-primary hover:underline">cups-web</a>
        </span>
        <span v-if="appVersion" class="text-default/40">·</span>
        <!-- 版本号：二进制构建期由 -ldflags 注入到 main.Version，经 /api/version 返回。
             用户二进制覆盖升级后，无需登录即可在 footer 上看到当前运行的版本（Issue #26）。 -->
        <span v-if="appVersion" class="font-mono text-xs" :title="`cups-web ${appVersion}`">
          {{ appVersion }}
        </span>
        <span class="text-default/40">·</span>
        <button
          type="button"
          class="inline-flex items-center gap-1 text-primary hover:underline"
          @click="showSponsorModal = true"
        >
          <UIcon name="i-lucide-heart" class="w-4 h-4" />
          <span>{{ t('ui.support_the_project') }}</span>
        </button>
      </footer>
    </div>

    <UModal :title="t('ui.support_the_project')" v-model:open="showSponsorModal">
      <template #content>
        <div class="p-6 space-y-4">
          <div class="flex items-center gap-2">
            <UIcon name="i-lucide-heart" class="w-5 h-5 text-primary" />
            <h3 class="text-lg font-semibold">{{ t('ui.support_the_project') }}</h3>
          </div>
          <p class="text-sm text-muted"> {{ t('ui.if_cups_web_is_useful_to_you_you_can_support') }} </p>
          <div class="flex flex-col items-center gap-3 py-2">
            <img
              src="/sponsor.png"
              :alt="t('ui.donation_qr_code')"
              class="w-60 h-60 object-contain rounded-lg border border-default bg-white"
              loading="lazy"
            />
            <div class="text-sm text-muted">{{ t('ui.scan_to_buy_the_author_a_cup_of_tea') }}</div>
          </div>
          <div class="flex flex-col sm:flex-row gap-2 sm:justify-between sm:items-center pt-2 border-t border-default">
            <a
              href="https://afdian.com/a/imhanxi"
              target="_blank"
              rel="noopener"
              class="inline-flex items-center gap-1 text-primary hover:underline text-sm"
            >
              <UIcon name="i-lucide-external-link" class="w-4 h-4" /> {{ t('ui.support_on_afdian') }} </a>
            <UButton variant="ghost" @click="showSponsorModal = false">{{ t('ui.close') }}</UButton>
          </div>
        </div>
      </template>
    </UModal>
  </UApp>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { clearSessionCache, updateSessionCache } from './router'
import { en, zh_cn } from '@nuxt/ui/locale'
import LanguageSelect from './components/LanguageSelect.vue'

const { t, locale } = useI18n()

const uiLocale = computed(() => locale.value === 'zh-CN' ? zh_cn : en)

const router = useRouter()
const route = useRoute()

const session = ref(null)
const sessionLoaded = ref(false)
const showSponsorModal = ref(false)
// 二进制版本号：首次挂载时拉一次 /api/version（公开接口，不要求登录），
// 失败时保持空字符串，footer 上的版本号节点会被 v-if 隐藏，不影响布局。
const appVersion = ref('')

const isAdmin = computed(() => session.value?.role === 'admin')
// 访客模式（Issue #55）签发的 guest 账号不允许管理 API 密钥（Issue #113），
// 避免访客通过 UI 生成 key 后带出容器；页面本身有二次校验，这里只是隐入口。
const canManageKeys = computed(() => session.value && session.value.username !== 'guest')

// 移动端汉堡菜单项：导航项（仅 admin）与登出分成两组，组间自动加分隔线
const menuItems = computed(() => {
  const nav = []
  if (session.value) {
    nav.push({ label: t('ui.print'), icon: 'i-lucide-file-text', onSelect: () => router.push('/print') })
    nav.push({ label: t('ui.schedules'), icon: 'i-lucide-calendar-clock', onSelect: () => router.push('/scheduled') })
    nav.push({ label: t('ui.scan'), icon: 'i-lucide-scan-line', onSelect: () => router.push('/scan') })
  }
  if (canManageKeys.value) {
    nav.push({ label: t('ui.api_keys'), icon: 'i-lucide-key-round', onSelect: () => router.push('/api-keys') })
  }
  if (isAdmin.value) {
    nav.push({ label: t('ui.administration'), icon: 'i-lucide-settings', onSelect: () => router.push('/admin') })
    nav.push({ label: t('ui.drivers'), icon: 'i-lucide-puzzle', onSelect: () => router.push('/drivers') })
  }
  const account = [{ label: t('ui.sign_out'), icon: 'i-lucide-log-out', onSelect: () => logout() }]
  return nav.length ? [nav, account] : [account]
})

async function loadVersion() {
  try {
    const resp = await fetch('/api/version', { credentials: 'include' })
    if (resp.ok) {
      const data = await resp.json()
      if (data && typeof data.version === 'string') {
        appVersion.value = data.version
      }
    }
  } catch (e) {
    // 版本号展示属于可降级的信息，静默失败即可
  }
}

// 自定义 CSS 注入（Issue #57）：管理员在设置里贴一段 CSS，登录前也应生效
// （登录页也是应用的一部分），因此走公开接口。用 <style id="app-custom-css">
// 作为幂等的注入点，保存后可以随时刷新样式而不用 reload 整页。
async function loadCustomCSS() {
  try {
    const resp = await fetch('/api/public-settings')
    if (!resp.ok) return
    const data = await resp.json()
    if (data && typeof data.customCss === 'string') {
      applyCustomCSS(data.customCss)
    }
  } catch (e) {
    // 自定义样式属于装饰性功能，接口挂了不该阻塞主界面
  }
}

function applyCustomCSS(css) {
  let el = document.getElementById('app-custom-css')
  if (!el) {
    el = document.createElement('style')
    el.id = 'app-custom-css'
    document.head.appendChild(el)
  }
  el.textContent = css || ''
}

async function loadSession() {
  try {
    const resp = await fetch('/api/session', { credentials: 'include' })
    if (resp.ok) {
      const data = await resp.json()
      session.value = data
      updateSessionCache(data)
      router.push('/print')
    } else {
      session.value = null
      router.push('/login')
    }
  } catch (e) {
    session.value = null
  } finally {
    sessionLoaded.value = true
  }
}

function onLogin() {
  loadSession()
}

function onLogout() {
  session.value = null
  clearSessionCache()
  router.push('/login')
}

async function logout() {
  try {
    await fetch('/api/logout', { method: 'POST', credentials: 'include' })
  } catch (e) {
    // ignore errors
  }
  onLogout()
}

function detectOS() {
  if (navigator.userAgent.indexOf('Windows') !== -1) {
    document.documentElement.classList.add('is-windows')
  }
}

onMounted(() => {
  detectOS()
  loadSession()
  loadVersion()
  loadCustomCSS()
})
</script>
