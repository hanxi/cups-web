import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import * as vue from 'vue'
import { i18n, setLanguage } from './i18n.js'

const cases = [
  ['views/LoginView.vue', 'BACKEND_MESSAGES'],
  ['views/PrintView.vue', 'printModeItems', 'orientationItems', 'paperSizeItems'],
  ['views/ScheduledView.vue', 'modeItems', 'endModeItems', 'scheduleTypeItems', 'weekdayItems', 'paperSizeItems', 'orientationItems', 'duplexItems', 'colorItems', 'paperTypeItems', 'scalingItems'],
  ['views/ScanView.vue', 'modeItems', 'formatItems', 'resolutionSelectItems', 'sourceItems'],
  ['views/ApiKeysView.vue', 'expiryOptions'],
  ['views/AdminView.vue', 'roleItems', 'userColumns', 'printColumns'],
  ['views/DriversView.vue', 'detectColumns', 'driverColumns'],
  ['components/print/PrintOptions.vue', 'colorItems', 'duplexItems', 'paperSizeItems', 'paperTypeItems', 'scalingItems', 'pageSetItems', 'numberUpItems', 'numberUpLayoutItems', 'mediaSourceItems'],
  ['components/print/PrintPreview.vue', 'orientationItems']
]

for (const [file, ...bindings] of cases) test(`${file}: options and headers react without resetting form state`, async () => {
  setLanguage('en', {})
  const { descriptor } = parse(readFileSync(new URL(file, import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: file }).content
  const imports = Object.fromEntries([...script.matchAll(/^import (.+?) from .+$/gm)].flatMap(([, names]) => {
    if (names.startsWith('{')) return names.slice(1,-1).split(',').map(s=>[s.trim().split(' as ').at(-1), () => {}])
    return [[names, {}]]
  }))
  const context = {
    ...imports, ...vue,
    useI18n: () => i18n.global,
    useToast: () => ({ add() {}, clear() {} }),
    onMounted() {}, onUnmounted() {}, onBeforeUnmount() {},
    localStorage: { getItem: () => null, setItem() {} },
    window: { innerWidth: 1024 },
    URL, File, Blob, FormData, setTimeout, clearTimeout,
    apiFetch: async () => ({ ok: true, json: async () => ({}) })
  }
  const component = runInNewContext(script.replace(/^import .*$/gm, '').replace('export default', 'const component =') + '\ncomponent', context)
  const props = vue.reactive(Object.fromEntries(Object.entries(component.props || {}).map(([k,v]) => [k, typeof v.default === 'function' ? v.default() : v.default])))
  const scope = vue.effectScope()
  const state = scope.run(() => component.setup(props, { expose() {}, emit() {} }))
  try {
    const json = key => JSON.stringify(vue.unref(state[key]))
    const before = Object.fromEntries(bindings.map(key => [key, json(key)]))
    const form = state.form || state.state || state.createForm
    if (form) vue.unref(form).username = 'User 中文 @ {name}'
    setLanguage('zh-CN', {})
    await vue.nextTick()
    for (const key of bindings) assert.notEqual(json(key), before[key], `${file}: ${key} did not update`)
    if (form) assert.equal(vue.unref(form).username, 'User 中文 @ {name}')
    setLanguage('en', {})
    for (const key of bindings) assert.equal(json(key), before[key], `${file}: ${key} did not switch back`)
  } finally { scope.stop() }
})
