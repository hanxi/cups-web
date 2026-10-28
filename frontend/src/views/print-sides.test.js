import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import * as vue from 'vue'
import { i18n } from '../i18n.js'
import * as format from '../utils/format.js'

function componentAt(path, globals = {}) {
  const { descriptor } = parse(readFileSync(new URL(path, import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'sides-regression-test' }).content
  return runInNewContext(script.replace(/^import .*$/gm, '').replace('export default', 'const component =') + '\ncomponent', {
    ...vue, ...format, useI18n: () => i18n.global,
    onMounted() {}, onUnmounted() {}, window: { innerWidth: 1024 },
    useToast: () => ({ add() {}, clear() {} }),
    File, Blob, FormData, URL, setInterval: () => 0, clearInterval() {},
    translateError: value => value, readError: async () => 'error',
    PrintOptions: {}, PrinterMediaOptions: {},
    ...globals
  })
}

test('reprint dialog restores and submits each binding and falls back for old records', () => {
  const events = []
  const component = componentAt('../components/print/PrintRecordList.vue')
  const scope = vue.effectScope()
  const state = scope.run(() => component.setup({ printers: [], currentPrinter: '' }, {
    expose() {}, emit: (...args) => events.push(args)
  }))
  try {
    for (const sides of ['two-sided-short-edge', 'two-sided-long-edge', 'one-sided']) {
      state.openReprintDialog({ id: 129, printerUri: 'ipp://test/printers/test', sides, isDuplex: sides !== 'one-sided' })
      assert.equal(state.reprintForm.value.duplex, sides)
      state.submitReprint()
      assert.equal(events.at(-1)[0], 'reprint')
      assert.equal(events.at(-1)[1].sides, sides)
      assert.equal(events.at(-1)[1].duplex, sides !== 'one-sided')
    }
    for (const isDuplex of [false, true]) {
      state.openReprintDialog({ isDuplex })
      assert.equal(state.reprintForm.value.duplex, isDuplex ? 'two-sided-long-edge' : 'one-sided')
    }
  } finally { scope.stop() }
})

test('scheduled create, edit, pause and resume retain the selected binding', async () => {
  const requests = []
  const component = componentAt('./ScheduledView.vue', {
    apiFetch: async (url, options = {}) => {
      requests.push({ url, options })
      return { ok: true, json: async () => [] }
    }
  })
  const scope = vue.effectScope()
  const state = scope.run(() => component.setup({}, { expose() {}, emit() {} }))
  try {
    assert.ok(state.duplexItems.value.some(item => item.value === 'two-sided-short-edge'))
    for (const sides of ['two-sided-short-edge', 'two-sided-long-edge', 'one-sided']) {
      state.openCreate()
      Object.assign(state.form.value, {
        mode: 'single', file: new File(['document'], 'test.pdf'),
        printerUri: 'ipp://test/printers/test', duplex: sides
      })
      await state.createNew(state.form.value)
      const create = requests.at(-1)
      assert.equal(create.options.body.get('sides'), sides)
      assert.equal(create.options.body.get('duplex'), String(sides !== 'one-sided'))

      const record = { id: 129, sides, isDuplex: sides !== 'one-sided', enabled: true, printerUri: 'ipp://test/printers/test', scheduleType: 'daily' }
      state.openEdit(record)
      assert.equal(state.form.value.duplex, sides)
      await state.saveEdit(state.form.value)
      assert.equal(JSON.parse(requests.at(-1).options.body).sides, sides)
      for (const enabled of [true, false]) {
        const start = requests.length
        await state.toggleEnabled({ ...record, enabled })
        const request = requests.slice(start).find(r => r.options.method === 'PUT')
        const payload = JSON.parse(request.options.body)
        assert.equal(payload.sides, sides)
        assert.equal(payload.enabled, !enabled)
      }
    }
    state.openEdit({ isDuplex: true })
    assert.equal(state.form.value.duplex, 'two-sided-long-edge')
  } finally { scope.stop() }
})
