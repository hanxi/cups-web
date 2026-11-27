// 公开的登录方式配置：GET /api/auth/config。请求失败时按「未启用 passkey」处理，
// 保证旧后端或网络异常下登录页仍可用。
export async function fetchAuthConfig() {
  try {
    const resp = await fetch('/api/auth/config', { credentials: 'include' })
    if (!resp.ok) return { passkeyEnabled: false }
    const data = await resp.json()
    return { passkeyEnabled: !!data.passkeyEnabled }
  } catch {
    return { passkeyEnabled: false }
  }
}
