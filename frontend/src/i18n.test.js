import { test } from 'node:test'
import assert from 'node:assert/strict'
import { resolveLocale, saveLocale, LOCALE_STORAGE_KEY } from './locale-preference.js'
import { computed } from 'vue'
import { i18n, t, setLanguage, syncDocumentLocale } from './i18n.js'

test('changing the locale updates reactive translations and document metadata', () => {
  const label = computed(() => t('ui.print'))
  const doc = { documentElement: {}, title: '' }
  setLanguage('zh-CN', {})
  syncDocumentLocale(doc)
  assert.equal(label.value, '打印')
  assert.equal(doc.documentElement.lang, 'zh-CN')
  assert.equal(doc.title, 'CUPS 打印')
  setLanguage('en', {})
  syncDocumentLocale(doc)
  assert.equal(label.value, 'Print')
  assert.equal(doc.documentElement.lang, 'en')
  assert.equal(doc.title, 'CUPS Printing')
  setLanguage('unsupported', {})
  assert.equal(i18n.global.locale.value, 'en')
})

test('explicit preference wins; automatic selection follows browser order without writing', () => {
  const storage = { getItem: key => key === LOCALE_STORAGE_KEY ? 'zh-CN' : null, setItem: () => assert.fail('detection must not write') }
  assert.equal(resolveLocale({ localStorage: storage, navigator: { languages: ['en-US'] } }), 'zh-CN')
  assert.equal(resolveLocale({ navigator: { languages: ['de', 'en-GB', 'zh-CN'] } }), 'en')
  assert.equal(resolveLocale({ navigator: { languages: ['fr', 'zh-TW', 'en'] } }), 'zh-CN')
  assert.equal(resolveLocale({ navigator: { languages: ['fr'] } }), 'en')
  assert.equal(resolveLocale({}), 'en')
})

test('invalid stored preferences fall through; unavailable storage is safe', () => {
  assert.equal(resolveLocale({ localStorage: { getItem: () => 'fr' }, navigator: { languages: ['zh-Hans'] } }), 'zh-CN')
  const browser = { get localStorage() { throw new Error('SecurityError') }, navigator: { languages: ['en-AU'] } }
  assert.equal(resolveLocale(browser), 'en')
  assert.doesNotThrow(() => saveLocale('zh-CN', browser))
  const writes = []
  saveLocale('en', { localStorage: { setItem: (...args) => writes.push(args) } })
  assert.deepEqual(writes, [[LOCALE_STORAGE_KEY, 'en']])
  saveLocale('fr', { localStorage: { setItem: () => assert.fail('unsupported preference') } })
})
