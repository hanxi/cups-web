<template>
  <UApp>
    <div v-if="!sessionLoaded" class="flex items-center justify-center min-h-screen bg-default">
      <UIcon name="i-lucide-loader-circle" class="w-8 h-8 animate-spin text-primary" />
    </div>
    <div v-else class="grid grid-rows-[auto_1fr_auto] min-h-screen w-full bg-default">
      <header class="flex items-center justify-between px-4 sm:px-6 py-3 border-b border-default bg-default">
        <div class="flex items-center gap-3 min-w-0">
          <h1 class="text-xl font-bold shrink-0">CUPS Печать</h1>
          <span v-if="session" class="text-sm text-muted truncate">{{ session.username }}</span>
        </div>
        <div class="flex items-center gap-2">
          <!-- Десктоп (sm+): Кнопки с текстом -->
          <div class="hidden sm:flex items-center gap-2">
            <!-- Навигация -->
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
                Печать
              </UButton>
              <UButton
                :variant="route.path === '/admin' ? 'soft' : 'ghost'"
                :color="route.path === '/admin' ? 'primary' : 'neutral'"
                size="xs"
                icon="i-lucide-settings"
                @click="router.push('/admin')"
              >
                Управление
              </UButton>
              <UButton
                :variant="route.path === '/drivers' ? 'soft' : 'ghost'"
                :color="route.path === '/drivers' ? 'primary' : 'neutral'"
                size="xs"
                icon="i-lucide-puzzle"
                @click="router.push('/drivers')"
              >
                Драйверы
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
              Выйти
            </UButton>
          </div>
          <!-- Мобильная версия (<sm): Бургер-меню -->
          <UDropdownMenu
            v-if="session"
            :items="menuItems"
            :content="{ align: 'end' }"
            class="sm:hidden"
          >
            <UButton variant="ghost" color="neutral" size="sm" icon="i-lucide-menu" square />
          </UDropdownMenu>
        </div>
      </header>
      <div class="overflow-auto relative">
        <router-view :session="session" @login-success="onLogin" @logout="onLogout" />
      </div>
      <footer class="px-6 py-3 border-t border-default bg-default text-sm text-muted flex items-center justify-center gap-3 flex-wrap">
        <span>
          Работает на
          <a href="https://github.com/hanxi/cups-web" target="_blank" class="text-primary hover:underline">cups-web</a>
        </span>
        <span v-if="appVersion" class="text-default/40">·</span>
        <!-- Версия: Внедряется через -ldflags в main.Version при сборке. -->
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
          <span>Поддержать проект</span>
        </button>
      </footer>
    </div>

    <UModal v-model:open="showSponsorModal">
      <template #content>
        <div class="p-6 space-y-4">
          <div class="flex items-center gap-2">
            <UIcon name="i-lucide-heart" class="w-5 h-5 text-primary" />
            <h3 class="text-lg font-semibold">Поддержать проект</h3>
          </div>
          <p class="text-sm text-muted">
            Если cups-web был вам полезен, вы можете поддержать автора ❤️
          </p>
          <div class="flex flex-col items-center gap-3 py-2">
            <img
              src="/sponsor.png"
              alt="QR-код"
              class="w-60 h-60 object-contain rounded-lg border border-default bg-white"
              loading="lazy"
            />
            <div class="text-sm text-muted">Отсканируйте код, чтобы угостить автора чаем ☕</div>
          </div>
          <div class="flex flex-col sm:flex-row gap-2 sm:justify-between sm:items-center pt-2 border-t border-default">
            <a
              href="https://afdian.com/a/imhanxi"
              target="_blank"
              rel="noopener"
              class="inline-flex items-center gap-1 text-primary hover:underline text-sm"
            >
              <UIcon name="i-lucide-external-link" class="w-4 h-4" />
              Страница на Afdian
            </a>
            <UButton variant="ghost" @click="showSponsorModal = false">Закрыть</UButton>
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
// Версия: загружается через /api/version при монтировании
const appVersion = ref('')

const isAdmin = computed(() => session.value?.role === 'admin')

// Элементы меню для мобильной версии
const menuItems = computed(() => {
  const nav = []
  if (isAdmin.value) {
    nav.push({ label: 'Печать', icon: 'i-lucide-file-text', onSelect: () => router.push('/print') })
    nav.push({ label: 'Управление', icon: 'i-lucide-settings', onSelect: () => router.push('/admin') })
    nav.push({ label: 'Драйверы', icon: 'i-lucide-puzzle', onSelect: () => router.push('/drivers') })
  }
  const account = [{ label: 'Выйти', icon: 'i-lucide-log-out', onSelect: () => logout() }]
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
    // Ошибка получения версии не критична
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

function detectOS() {
  if (navigator.userAgent.indexOf('Windows') !== -1) {
    document.documentElement.classList.add('is-windows')
  }
}

onMounted(() => {
  detectOS()
  loadSession()
  loadVersion()
})
</script>
