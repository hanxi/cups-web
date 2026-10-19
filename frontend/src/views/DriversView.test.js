import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import * as vue from 'vue'
import { i18n, setLanguage } from '../i18n.js'
import { translateError } from '../utils/translate-error.js'

const { descriptor } = parse(readFileSync(new URL('./DriversView.vue', import.meta.url), 'utf8'))
const script = compileScript(descriptor, { id: 'driver-locale-test' }).content

for (const operation of ['install', 'remove']) test(`${operation} driver messages translate app names reactively, not identifiers or logs`, async () => {
  const requests = [], toasts = []
  let finishSubmit
  const component = runInNewContext(script.replace(/^import .*$/gm, '').replace('export default', 'const component =') + '\ncomponent', {
    ...vue, useI18n: () => i18n.global, translateError,
    onMounted() {}, onUnmounted() {},
    useToast: () => ({ add: toast => toasts.push(toast) }),
    AbortSignal, setTimeout: resolve => { resolve(); return 1 }, clearTimeout() {},
    apiFetch: async (url, options = {}) => {
      requests.push({ url, options })
      if (options.method === 'POST') await new Promise(resolve => { finishSubmit = resolve })
      return { ok: true, json: async () => url.includes('/jobs/')
        ? { status: 'succeeded', log: 'RAW Epson 国行驱动' }
        : options.method === 'POST' ? { jobId: 'fixture-job' } : { drivers: [] } }
    }
  })
  const scope = vue.effectScope()
  const state = scope.run(() => component.setup({}, { expose() {}, emit() {} }))
  try {
    setLanguage('en', {})
    state.pendingDriver.value = { name: 'epson-cn', displayName: 'Epson 国行驱动' }
    const done = state[`${operation}Driver`]()
    const enTitle = operation === 'install' ? 'Installing driver Epson China-market driver' : 'Uninstalling driver Epson China-market driver'
    assert.equal(state.jobTitleText.value, enTitle)
    setLanguage('zh-CN', {})
    assert.ok(state.jobTitleText.value.includes('Epson 国行驱动'))
    setLanguage('en', {})
    assert.equal(state.jobTitleText.value, enTitle)
    finishSubmit()
    await done
    assert.equal(toasts[0].description, operation === 'install' ? 'Driver Epson China-market driver installed' : 'Driver Epson China-market driver uninstalled')
    assert.deepEqual(JSON.parse(requests[0].options.body), { name: 'epson-cn' })
    assert.equal(state.jobLog.value, 'RAW Epson 国行驱动')
  } finally { scope.stop(); setLanguage('en', {}) }
})
