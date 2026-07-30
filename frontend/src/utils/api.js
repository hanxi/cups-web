// Извлечение CSRF токена из Cookie
export function getCSRF() {
  const m = document.cookie.match('(^|;)\\s*csrf_token\\s*=\\s*([^;]+)')
  return m ? m.pop() : ''
}

// Унифицированный парсинг ошибок
export async function readError(resp) {
  try {
    const data = await resp.json()
    return data.error || resp.statusText
  } catch (e) {
    try {
      const text = await resp.text()
      return text || resp.statusText
    } catch (err) {
      return resp.statusText
    }
  }
}

// Обертка над fetch: авто-учет credentials, CSRF и обработка 401
// onUnauthorized - коллбэк для выхода из системы
export async function apiFetch(url, options = {}, onUnauthorized = null) {
  const opts = { ...options, credentials: 'include' }

  // Инициализация заголовков
  const headers = new Headers(opts.headers || {})

  const method = (opts.method || 'GET').toUpperCase()
  const isFormData = opts.body instanceof FormData

  if (method !== 'GET') {
    // Для не-GET запросов добавляем CSRF токен
    headers.set('X-CSRF-Token', getCSRF())

    // Для не-FormData добавляем Content-Type
    if (!isFormData) {
      headers.set('Content-Type', 'application/json')
    }
  }

  opts.headers = headers

  const resp = await fetch(url, opts)

  // Если 401, вызываем коллбэк разлогина
  if (resp.status === 401 && onUnauthorized) {
    onUnauthorized()
  }

  return resp
}
