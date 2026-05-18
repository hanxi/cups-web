<template>
  <UApp>
    <div v-if="!sessionLoaded" class="flex items-center justify-center min-h-screen bg-default">
      <UIcon name="i-lucide-loader-circle" class="w-8 h-8 animate-spin text-primary" />
    </div>
    <div v-else class="grid grid-rows-[auto_1fr_auto] min-h-screen w-full bg-default">
      <header class="flex items-center justify-between px-4 sm:px-6 py-3 border-b border-default bg-default">
        <div class="flex items-center gap-3 min-w-0">
          <h1 class="text-xl font-bold shrink-0">CUPS 打印</h1>
          <span v-if="session" class="text-sm text-muted truncate">{{ session.username }}</span>
        </div>
        <div class="flex items-center gap-2">
          <!-- 导航分段容器：与主 CTA 视觉区分 -->
          <div
            v-if="isAdmin"
            class="flex items-center gap-0.5 p-0.5 rounded-lg bg-elevated/60 border border-default"
          >
            <UButton
              :variant="route.path === '/print' ? 'soft' : 'ghost'"
              :color="route.path === '/print' ? 'primary' : 'neutral'"
              size="xs"
              icon="i-lucide-file-text"
              @click="router.push('/print')"
            >
              <span class="hidden sm:inline">打印</span>
            </UButton>
            <UButton
              :variant="route.path === '/scan' ? 'soft' : 'ghost'"
              :color="route.path === '/scan' ? 'primary' : 'neutral'"
              size="xs"
              icon="i-lucide-scan"
              @click="router.push('/scan')"
            >
              <span class="hidden sm:inline">扫描</span>
            </UButton>
            <UButton
              :variant="route.path === '/admin' ? 'soft' : 'ghost'"
              :color="route.path === '/admin' ? 'primary' : 'neutral'"
              size="xs"
              icon="i-lucide-settings"
              @click="router.push('/admin')"
            >
              <span class="hidden sm:inline">管理</span>
            </UButton>
          </div>
          <UButton
            v-if="session"
            variant="ghost"
            color="neutral"
            size="xs"
            icon="i-lucide-log-out"
            @click="logout"
          >
            <span class="hidden sm:inline">登出</span>
          </UButton>
        </div>
      </header>
      <div class="overflow-auto relative">
        <router-view :session="session" @login-success="onLogin" @logout="onLogout" />
      </div>
      <footer class="px-6 py-3 border-t border-default bg-default text-sm text-muted flex items-center justify-center gap-3 flex-wrap">
        <span>
          Powered by
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
          <span>赞赏支持</span>
        </button>
      </footer>
    </div>

    <UModal v-model:open="showSponsorModal">
      <template #content>
        <div class="p-6 space-y-4">
          <div class="flex items-center gap-2">
            <UIcon name="i-lucide-heart" class="w-5 h-5 text-primary" />
            <h3 class="text-lg font-semibold">赞赏支持</h3>
          </div>
          <p class="text-sm text-muted">
            如果 cups-web 对你有帮助，欢迎通过以下方式支持作者持续维护 ❤️
          </p>
          <div class="flex flex-col items-center gap-3 py-2">
            <img
              src="/sponsor.png"
              alt="赞赏码"
              class="w-60 h-60 object-contain rounded-lg border border-default bg-white"
              loading="lazy"
            />
            <div class="text-sm text-muted">扫码请作者喝杯奶茶 ☕</div>
          </div>
          <div class="flex flex-col sm:flex-row gap-2 sm:justify-between sm:items-center pt-2 border-t border-default">
            <a
              href="https://afdian.com/a/imhanxi"
              target="_blank"
              rel="noopener"
              class="inline-flex items-center gap-1 text-primary hover:underline text-sm"
            >
              <UIcon name="i-lucide-external-link" class="w-4 h-4" />
              爱发电主页
            </a>
            <UButton variant="ghost" @click="showSponsorModal = false">关闭</UButton>
          </div>
        </div>
      </template>
    </UModal>
  </UApp>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { clearSessionCache, updateSessionCache } from './router'

const router = useRouter()
const route = useRoute()

const session = ref(null)
const sessionLoaded = ref(false)
const showSponsorModal = ref(false)
// 二进制版本号：首次挂载时拉一次 /api/version（公开接口，不要求登录），
// 失败时保持空字符串，footer 上的版本号节点会被 v-if 隐藏，不影响布局。
const appVersion = ref('')

const isAdmin = computed(() => session.value?.role === 'admin')

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

onMounted(() => {
  loadSession()
  loadVersion()
})
</script>
