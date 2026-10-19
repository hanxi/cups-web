import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import * as vue from 'vue'
import { i18n, setLanguage } from '../i18n.js'
import { translateError } from '../utils/translate-error.js'
import { readError } from '../utils/api.js'

function compileComponent(file, inlineTemplate, context = {}) {
  const { descriptor } = parse(readFileSync(new URL(file, import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: file, inlineTemplate }).content
  const imports = Object.fromEntries([...script.matchAll(/^import (.+?) from ['"](.+?)['"].*$/gm)].flatMap(([, names, source]) => {
    if (!names.startsWith('{')) return [[names, {}]]
    return names.slice(1, -1).split(',').map(name => {
      const [original, alias = original] = name.trim().split(' as ')
      return [alias, source === 'vue' ? vue[original] : () => {}]
    })
  }))
  return runInNewContext(script.replace(/^import .*$/gm, '').replace('export default', 'const component =') + '\ncomponent', {
    ...imports, useI18n: () => i18n.global, translateError, readError,
    onMounted() {}, onUnmounted() {},
    useToast: () => ({ add() {} }),
    localStorage: { getItem: () => null, setItem() {} },
    window: { innerWidth: 1024 }, File, Blob, FormData, URL,
    ...context
  })
}

// Mount the real PrinterStatus template without a browser or UI-kit plugins.
function createTestRenderer() {
  const node = text => ({ text, children: [], parent: null, style: {}, scrollHeight: 0, offsetHeight: 0 })
  const remove = child => {
    const siblings = child.parent?.children
    if (siblings) siblings.splice(siblings.indexOf(child), 1)
  }
  const renderer = vue.createRenderer({
    createElement: () => node(''), createText: node, createComment: () => node(''),
    setText: (element, text) => { element.text = text },
    setElementText: (element, text) => { element.text = text; element.children = [] },
    patchProp() {}, remove,
    insert(child, parent, anchor = null) {
      remove(child)
      child.parent = parent
      const index = anchor ? parent.children.indexOf(anchor) : parent.children.length
      parent.children.splice(index, 0, child)
    },
    parentNode: element => element.parent,
    nextSibling: element => element.parent?.children[element.parent.children.indexOf(element) + 1] || null
  })
  const root = node('')
  const text = element => element.text + element.children.map(text).join('')
  return { ...renderer, root, text: () => text(root) }
}

for (const initial of ['en', 'zh-CN']) test(`printer connection error switches languages without a new request (initial ${initial})`, async () => {
  setLanguage(initial, {})
  let requests = 0
  const printView = compileComponent('./PrintView.vue', false, {
    apiFetch: async () => { requests++; throw new TypeError('Failed to fetch') }
  })
  const scope = vue.effectScope()
  const state = scope.run(() => printView.setup({}, { expose() {}, emit() {} }))
  const renderer = createTestRenderer()
  const printerStatus = compileComponent('../components/print/PrinterStatus.vue', true)
  const app = renderer.createApp({ render: () => vue.h(printerStatus, {
    printerUri: state.printer.value,
    printerInfo: state.printerInfo.value,
    error: state.printerInfoError.value,
    loading: state.loadingPrinterInfo.value
  }) })
  app.component('UCard', { setup: (_, { slots }) => () => vue.h('div', [slots.header?.(), slots.default?.()]) })
  for (const name of ['UButton', 'UIcon', 'UBadge']) app.component(name, { setup: () => () => vue.h('span') })
  try {
    state.printer.value = 'http://localhost:631/printers/QA_PDF'
    await state.loadPrinterInfo()
    app.mount(renderer.root)
    const requestsAfterFailure = requests
    for (const language of [initial, initial === 'en' ? 'zh-CN' : 'en', initial]) {
      setLanguage(language, {})
      await vue.nextTick()
      const expected = language === 'en' ? 'Cannot connect to the printer' : '无法连接到打印机'
      const stale = language === 'en' ? '无法连接到打印机' : 'Cannot connect to the printer'
      assert.ok(renderer.text().includes(expected), renderer.text())
      assert.ok(!renderer.text().includes(stale), renderer.text())
      assert.equal(requests, requestsAfterFailure, 'locale changes must not reload printer info')
      assert.equal(state.printer.value, 'http://localhost:631/printers/QA_PDF')
    }
  } finally {
    app.unmount()
    scope.stop()
    setLanguage('en', {})
  }
})
