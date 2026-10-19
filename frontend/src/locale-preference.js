export const LOCALE_STORAGE_KEY = 'cups-web.locale'

export function readPreference(key, browser = globalThis) {
  try { return browser.localStorage?.getItem(key) ?? null } catch { return null }
}

export function writePreference(key, value, browser = globalThis) {
  try { browser.localStorage?.setItem(key, value) } catch { /* Storage is optional. */ }
}

export function normalizeLocale(value) {
  if (typeof value !== 'string') return null
  if (/^en(?:-|$)/i.test(value)) return 'en'
  if (/^zh(?:-|$)/i.test(value)) return 'zh-CN'
  return null
}

export function resolveLocale(browser = globalThis) {
  const saved = readPreference(LOCALE_STORAGE_KEY, browser)
  if (saved === 'en' || saved === 'zh-CN') return saved
  const languages = browser.navigator?.languages || [browser.navigator?.language]
  for (const language of languages) {
    const locale = normalizeLocale(language)
    if (locale) return locale
  }
  return 'en'
}

export function saveLocale(locale, browser = globalThis) {
  if (locale !== 'en' && locale !== 'zh-CN') return
  writePreference(LOCALE_STORAGE_KEY, locale, browser)
}
