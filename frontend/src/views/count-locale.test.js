import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse } from '@vue/compiler-sfc'
import { baseParse } from '@vue/compiler-dom'
import { i18n, setLanguage } from '../i18n.js'

const source = file => readFileSync(new URL(file, import.meta.url), 'utf8')
const expressions = []
function visit(node) {
  if (node.type === 5) expressions.push(node.content.content)
  for (const child of node.children || []) visit(child)
}
visit(baseParse(parse(source('./ScheduledView.vue')).descriptor.template.content))
const evaluate = (key, context) => runInNewContext(expressions.find(exp => exp.includes(`'ui.${key}'`)), { t: i18n.global.t, ...context })

test('scheduled photo and copy counts pluralize independently in actual template expressions', () => {
  try {
    for (const locale of ['en', 'zh-CN']) {
      setLanguage(locale, {})
      for (const photos of [0, 1, 2]) for (const copies of [0, 1, 2]) {
        const rec = { photosPerRun: photos, copies, files: Array(photos).fill({}), filename: 'Original 中文.jpg' }
        const form = { photosPerRun: photos, copies, photos: rec.files, loop: false }
        const summary = evaluate('run_photo_summary', { rec })
        const formSummary = evaluate('queue_form_summary', { form, scheduleLabel: () => 'Daily' })
        if (locale === 'en') {
          assert.ok(summary.includes(`${photos || 1} ${(photos || 1) === 1 ? 'photo' : 'photos'} per run, ${copies} ${copies === 1 ? 'copy' : 'copies'} each`), summary)
          assert.ok(formSummary.includes(`${photos} ${photos === 1 ? 'photo' : 'photos'} per run, ${copies} ${copies === 1 ? 'copy' : 'copies'} each`), formSummary)
          if (photos) assert.ok(evaluate('photo_count', { rec }).endsWith(`${photos} ${photos === 1 ? 'photo' : 'photos'}`))
        } else {
          assert.ok(summary.includes(`每次打印 ${photos || 1} 张照片，每张 ${copies} 份`), summary)
          assert.ok(formSummary.includes(`打印 ${photos} 张照片，每张 ${copies} 份`), formSummary)
        }
      }
    }
  } finally { setLanguage('en', {}) }
})

test('print and reprint toast page counts use Vue I18n plurals without changing job IDs', () => {
  const text = source('./PrintView.vue')
  try {
    for (const locale of ['en', 'zh-CN']) {
      setLanguage(locale, {})
      for (const pages of [0, 1, 2]) for (const key of ['job_id_pages', 'pages_job_id']) {
        const expression = text.split('\n').find(line => line.includes(`description: t('ui.${key}'`)).trim().slice('description: '.length).replace(/,$/, '')
        const result = runInNewContext(expression, { t: i18n.global.t, j: { pages, jobId: 'JOB 中文 @ {id}' } })
        assert.ok(result.includes('JOB 中文 @ {id}'))
        assert.ok(result.includes(locale === 'en' ? `${pages} ${pages === 1 ? 'page' : 'pages'}` : `${pages} 页`), result)
        if (locale === 'en' && pages === 1) assert.ok(!result.includes('1 pages'))
      }
    }
  } finally { setLanguage('en', {}) }
})

test('other scheduled count messages agree for singular and plural queues', () => {
  setLanguage('en', {})
  for (const key of ['selected_photo_count', 'queue_stop_summary', 'queue_progress', 'queue_cycle_progress', 'queue_total']) {
    for (const count of [0, 1, 2]) {
      const result = i18n.global.t(`ui.${key}`, { count, submitted: count }, count)
      assert.ok(result.includes(`${count} ${count === 1 ? 'photo' : 'photos'}`), result)
      if (count === 1) assert.ok(!result.includes('1 photos'), result)
    }
  }
})
