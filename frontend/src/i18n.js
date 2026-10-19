import { createI18n } from 'vue-i18n'
import { watchEffect } from 'vue'
import en from './locales/en.json' with { type: 'json' }
import zhCN from './locales/zh-CN.json' with { type: 'json' }
import { resolveLocale, saveLocale } from './locale-preference.js'

export const i18n = createI18n({
  legacy: false,
  locale: resolveLocale(typeof window === 'undefined' ? {} : window),
  fallbackLocale: 'en',
  messages: { en, 'zh-CN': zhCN }
})
export const locale = i18n.global.locale
export const t = (...args) => i18n.global.t(...args)

// Only deliberate choices are persisted, never browser detection.
export function setLanguage(value, browser = globalThis) {
  if (value !== 'en' && value !== 'zh-CN') return
  locale.value = value
  saveLocale(value, browser)
}

export function syncDocumentLocale(doc = globalThis.document) {
  if (!doc) return
  doc.documentElement.lang = locale.value
  doc.title = t('ui.cups_printing')
}
watchEffect(() => syncDocumentLocale())

export function formatNumber(value, options = {}) {
  return new Intl.NumberFormat(locale.value, options).format(value)
}
