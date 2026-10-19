import { t } from '../i18n.js'
import rules from '../locales/api-errors.json' with { type: 'json' }
import en from '../locales/en.json' with { type: 'json' }
import zhCN from '../locales/zh-CN.json' with { type: 'json' }

// Match only application-owned messages at the presentation boundary. Never
// translate user text, filenames or raw logs. Captured diagnostics stay intact.
const patterns = rules.map(rule => ({ ...rule, expression: new RegExp(rule.pattern) }))
const escape = value => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
// Recognise an already displayed API error after switching languages as well.
for (const catalog of [en, zhCN]) {
  for (const rule of rules) {
    const message = catalog.api[rule.key.slice(4)]
    const params = [...message.matchAll(/\{(p\d+)\}/g)].map(match => match[1])
    patterns.push({ ...rule, params, expression: new RegExp('^' + message.split(/\{p\d+\}/).map(escape).join('([\\s\\S]*?)') + '$') })
  }
}
export function translateError(message, code, details = {}) {
  if (code === 'cross_origin_blocked') return t('api.cross_origin_blocked', { origin: details.origin || '—', host: details.host || '—' })
  if (typeof message !== 'string') return ''
  for (const rule of patterns) {
    const match = rule.expression.exec(message)
    if (match) return t(rule.key, Object.fromEntries(match.slice(1).map((value, i) => [rule.params?.[i] || 'p' + i, value])))
  }
  return message
}
