import { createRouter, createWebHashHistory } from 'vue-router'
import LoginView from '../views/LoginView.vue'
import PrintView from '../views/PrintView.vue'
import ScanView from '../views/ScanView.vue'
import AdminView from '../views/AdminView.vue'

const routes = [
  { path: '/', redirect: '/login' },
  { path: '/login', name: 'login', component: LoginView, meta: { requiresAuth: false } },
  { path: '/print', name: 'print', component: PrintView, meta: { requiresAuth: true } },
  { path: '/scan', name: 'scan', component: ScanView, meta: { requiresAuth: true } },
  { path: '/admin', name: 'admin', component: AdminView, meta: { requiresAuth: true, requiresAdmin: true } }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

// 缓存 session 信息，避免每次路由切换都请求 API
let cachedSession = null
let sessionChecked = false

// 提供方法清除缓存（登出时调用）
export function clearSessionCache() {
  cachedSession = null
  sessionChecked = false
}

// 提供方法更新缓存（登录成功时调用）
export function updateSessionCache(session) {
  cachedSession = session
  sessionChecked = true
}

// 提供方法获取缓存的 session
export function getCachedSession() {
  return cachedSession
}

router.beforeEach(async (to, from, next) => {
  // 不需要认证的路由直接放行
  if (to.meta.requiresAuth === false) {
    next()
    return
  }

  // 检查 session
  if (!sessionChecked) {
    try {
      const resp = await fetch('/api/session', { credentials: 'include' })
      if (resp.ok) {
        cachedSession = await resp.json()
        sessionChecked = true
      } else {
        cachedSession = null
        sessionChecked = true
      }
    } catch (e) {
      cachedSession = null
      sessionChecked = true
    }
  }

  // 未登录，跳转到登录页
  if (!cachedSession) {
    if (to.path !== '/login') {
      next('/login')
    } else {
      next()
    }
    return
  }

  // 需要管理员权限但不是管理员
  if (to.meta.requiresAdmin && cachedSession.role !== 'admin') {
    next('/print')
    return
  }

  next()
})

export default router
