<template>
  <div class="grid grid-rows-[auto_1fr_auto] min-h-screen w-full bg-base-200">
    <div class="navbar bg-base-100 shadow z-10 px-4 w-full">
      <div class="flex-1 flex items-center gap-3">
        <h1 class="text-xl font-bold text-base-content">CUPS 打印</h1>
        <span v-if="session" class="text-sm text-base-content/60">{{ session.username }}</span>
      </div>
      <div class="flex-none gap-2">
        <button v-if="isAdmin" class="btn btn-sm btn-ghost" :class="{ 'btn-active': view === 'PrintView' }" @click="view = 'PrintView'">打印</button>
        <button v-if="isAdmin" class="btn btn-sm btn-ghost" :class="{ 'btn-active': view === 'AdminView' }" @click="view = 'AdminView'">管理</button>
        <button v-if="session" class="btn btn-sm btn-outline" @click="logout">登出</button>
      </div>
    </div>
    <div class="overflow-auto relative">
      <component :is="view" :session="session" @login-success="onLogin" @logout="onLogout" />
    </div>
    <footer class="footer footer-center p-4 bg-base-100 text-base-content">
      <div class="text-sm">
        <p>Powered by <a href="https://github.com/hanxi/cups-web" target="_blank" class="link link-hover text-primary">cups-web</a></p>
      </div>
    </footer>
  </div>
</template>

<script>
import LoginView from './views/LoginView.vue'
import PrintView from './views/PrintView.vue'
import AdminView from './views/AdminView.vue'

export default {
  data() {
    return { view: 'LoginView', session: null }
  },
  async mounted() {
    await this.loadSession()
  },
  components: { LoginView, PrintView, AdminView },
  computed: {
    isAdmin() {
      return this.session && this.session.role === 'admin'
    }
  },
  methods: {
    async loadSession() {
      try {
        const resp = await fetch('/api/session', { credentials: 'include' })
        if (resp.ok) {
          this.session = await resp.json()
          this.view = 'PrintView'
        } else {
          this.session = null
          this.view = 'LoginView'
        }
      } catch (e) {
        this.session = null
      }
    },
    async onLogin() {
      await this.loadSession()
    },
    onLogout() {
      this.session = null
      this.view = 'LoginView'
    },
    async logout() {
      try {
        await fetch('/api/logout', { method: 'POST', credentials: 'include' })
      } catch (e) {
        // ignore errors
      }
      this.onLogout()
    }
  }
}
</script>
