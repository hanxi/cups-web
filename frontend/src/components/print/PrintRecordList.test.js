import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse } from '@vue/compiler-sfc'
import { baseParse, compile } from '@vue/compiler-dom'
import * as vue from 'vue'
import { renderToString } from 'vue/server-renderer'
import { i18n, setLanguage } from '../../i18n.js'

const { descriptor } = parse(readFileSync(new URL('./PrintRecordList.vue', import.meta.url), 'utf8'))

// Render the actual template fragment with real i18n rather than duplicate its
// translation call in a test. No UI-component stubs or browser are needed.
function fragment(tag, expression) {
  let source
  function visit(node) {
    if (node.tag === tag && node.children.some(child => child.type === 5 && child.content.content.includes(expression))) {
      source = node.loc.source
    }
    for (const child of node.children || []) visit(child)
  }
  visit(baseParse(descriptor.template.content))
  assert.ok(source, `Missing ${tag} fragment for ${expression}`)
  const { code } = compile(source, { mode: 'function' })
  const render = runInNewContext(`(function () { ${code} })()`, { Vue: vue })
  return context => renderToString(vue.createSSRApp({ render: () => render(context, []) }))
}

test('history page counts include English spacing and plurals while preserving Chinese', async () => {
  const render = fragment('p', 'rec.pages')
  try {
    for (const [locale, expected] of [
      ['en', ['0 pages', '1 page', '2 pages']],
      ['zh-CN', ['0页', '1页', '2页']]
    ]) {
      setLanguage(locale, {})
      for (const [pages, label] of expected.entries()) {
        const html = await render({ rec: { pages, printerUri: 'printer' }, recordPrinterName: () => 'QA_PDF', t: i18n.global.t })
        assert.equal(html.replace(/<[^>]*>/g, ''), `QA_PDF · ${label}`)
      }
    }
  } finally { setLanguage('en', {}) }
})

test('expanded history keeps locale-owned spacing after each metadata label', async () => {
  const cases = [
    ['rec.isColor', 'Colour: Colour', '颜色：彩色'],
    ['rec.isDuplex', 'Double-sided: No', '双面：否'],
    ['rec.pages', 'Pages: 2', '页数：2'],
    ['rec.jobId', 'Job ID: JOB 中文 @ {id}', '任务ID：JOB 中文 @ {id}']
  ]
  try {
    for (const [expression, en, zh] of cases) {
      const render = fragment('div', expression)
      for (const [locale, expected] of [['en', en], ['zh-CN', zh]]) {
        setLanguage(locale, {})
        const html = await render({ rec: { isColor: true, isDuplex: false, pages: 2, jobId: 'JOB 中文 @ {id}' }, t: i18n.global.t })
        assert.equal(html.replace(/<[^>]*>/g, ''), expected)
      }
    }
  } finally { setLanguage('en', {}) }
})

test('reprint filename spacing belongs to the locale and leaves the filename unchanged', async () => {
  const render = fragment('div', 'reprintRecord?.filename')
  const filename = 'Report 中文 @ {name}.pdf'
  try {
    for (const [locale, prefix] of [['en', 'File: '], ['zh-CN', '文件：']]) {
      setLanguage(locale, {})
      const html = await render({ reprintRecord: { filename }, t: i18n.global.t })
      assert.equal(html.replace(/<[^>]*>/g, ''), `${prefix}${filename}`)
    }
  } finally { setLanguage('en', {}) }
})
