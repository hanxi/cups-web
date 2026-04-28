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
      <footer class="px-6 py-3 border-t border-default bg-default text-sm text-muted text-center">
        Powered by <a href="https://github.com/hanxi/cups-web" target="_blank" class="text-primary hover:underline">cups-web</a>
      </footer>
    </div>
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

const isAdmin = computed(() => session.value?.role === 'admin')

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

onMounted(() => loadSession())
</script>
