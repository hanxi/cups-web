import { test } from 'node:test'
import assert from 'node:assert/strict'
import { computed } from 'vue'
import { setLanguage } from '../i18n.js'
import { formatTime, formatFileSize, formatDurationSeconds, statusText, jobStateText, printerLabel } from './format.js'
import { printerChoiceLabel } from './printer-options.js'

test('formatters follow the active locale and preserve external names', () => {
  const time = computed(() => formatTime('2026-10-19T09:30:00Z'))
  setLanguage('en', {})
  const english = time.value
  assert.equal(formatDurationSeconds(90000), '1d 1h')
  assert.equal(formatFileSize(2048), '2.0 KB')
  assert.equal(statusText('queued'), 'Queued')
  assert.equal(jobStateText({ state: 5, stateText: 'processing' }), 'Processing')
  setLanguage('zh-CN', {})
  assert.notEqual(time.value, english)
  assert.equal(formatDurationSeconds(90000), '1天1小时')
  assert.equal(statusText('queued'), '排队中')
  assert.equal(formatTime('invalid-time'), 'invalid-time')
  assert.equal(printerLabel({ name: '办公室', info: 'Custom name' }), '办公室 — Custom name')
  assert.equal(statusText('vendor-state'), 'vendor-state')
})

test('printer options translate app-owned labels without modifying protocol values or vendor labels', () => {
  const choice = { attribute: 'print-quality', value: '3', label: '草稿' }
  setLanguage('en', {})
  assert.equal(printerChoiceLabel(choice), 'Draft')
  assert.equal(printerChoiceLabel({ attribute: 'PageSize', value: 'A4', label: 'A4（无边距）' }), 'A4 (borderless)')
  assert.equal(printerChoiceLabel({ attribute: 'VendorQuality', value: 'custom', label: '厂商专用' }), '厂商专用')
  assert.deepEqual(choice, { attribute: 'print-quality', value: '3', label: '草稿' })
  setLanguage('zh-CN', {})
  assert.equal(printerChoiceLabel(choice), '草稿')
})
