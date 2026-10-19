import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import { ref, computed, watch, reactive, effectScope } from 'vue'
import { i18n } from '../../i18n.js'

// Compile the component's actual setup so the event contract is tested without a DOM.
const { descriptor } = parse(readFileSync(new URL('./PrintOptions.vue', import.meta.url), 'utf8'))
const compiled = compileScript(descriptor, { id: 'print-options-test' }).content
const component = runInNewContext(compiled
  .replace(/^import .*$/gm, '')
  .replace('export default', 'const component =') + '\ncomponent', {
  ref, computed, watch, PrinterMediaOptions: {}, useI18n: () => i18n.global,
  readPreference: () => null, writePreference: () => {}, printerChoiceLabel: choice => choice?.label || '',
  localStorage: { getItem: () => null, setItem: () => {} }
})

test('manual duplex page selection preserves the photo date setting', () => {
  for (const photoDateWatermark of [false, true]) {
    for (const duplex of ['one-sided', 'two-sided-long-edge']) {
      for (const pageSet of ['all', 'odd', 'even', 'even-reverse']) {
        const props = reactive(Object.fromEntries(Object.entries(component.props).map(([key, value]) =>
          [key, typeof value.default === 'function' ? value.default() : value.default])))
        Object.assign(props, { photoDateWatermark, duplex })
        const events = []
        const scope = effectScope()
        try {
          const setup = scope.run(() => component.setup(props, {
            expose: () => {}, emit: (...args) => events.push(args)
          }))
          setup.onPageSetChange(pageSet)
          const expected = [['update:pageSet', pageSet]]
          if (pageSet !== 'all' && duplex !== 'one-sided') expected.push(['update:duplex', 'one-sided'])
          assert.deepEqual(events, expected)
          assert.equal(props.photoDateWatermark, photoDateWatermark)
        } finally {
          scope.stop()
        }
      }
    }
  }
})
