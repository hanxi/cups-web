import { translateError } from './translate-error.js'

// 从 Cookie 提取 CSRF 令牌
export function getCSRF() {
  const m = document.cookie.match('(^|;)\\s*csrf_token\\s*=\\s*([^;]+)')
  return m ? m.pop() : ''
}

// 统一解析错误响应
export async function readError(resp) {
  let raw
  try { raw = await resp.text() } catch { return translateError(resp.statusText) }
  let message = raw || resp.statusText
  try {
    const data = JSON.parse(raw)
    return translateError(data?.error || data?.message || resp.statusText, data?.code, data || {})
  } catch { /* A proxy or external service may return plain text. */ }
  return translateError(message)
}

// 封装 fetch，自动附加 credentials 和 CSRF token，统一处理 401
// onUnauthorized 是一个回调函数，由调用方传入（用于触发登出）
export async function apiFetch(url, options = {}, onUnauthorized = null) {
  const opts = { ...options, credentials: 'include' }

  // 初始化 headers
  const headers = new Headers(opts.headers || {})

  const method = (opts.method || 'GET').toUpperCase()
  const isFormData = opts.body instanceof FormData

  if (method !== 'GET') {
    // 对于非 GET 请求，附加 CSRF token
    headers.set('X-CSRF-Token', getCSRF())

    // 对于非 FormData body，附加 Content-Type
    if (!isFormData) {
      headers.set('Content-Type', 'application/json')
    }
  }

  opts.headers = headers

  const resp = await fetch(url, opts)

  // 检查 401 状态，调用 onUnauthorized 回调
  if (resp.status === 401 && onUnauthorized) {
    onUnauthorized()
  }

  return resp
}
