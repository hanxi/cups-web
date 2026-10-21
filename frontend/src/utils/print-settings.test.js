import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  DEFAULT_PRINT_SETTINGS, LAST_PRINT_SETTINGS_KEY,
  sanitizePrintSettings, snapshotPrintSettings, readLastPrintSettings, writeLastPrintSettings
} from './print-settings.js'

const pageSize = { attribute: 'PageSize', value: 'A4', label: 'A4', widthMM: 210, heightMM: 297 }
const quality = { attribute: 'print-quality', value: '5', label: '' }

test('snapshot keeps the whitelist, pairs driver options with the printer and drops per-file fields', () => {
  const values = {
    ...DEFAULT_PRINT_SETTINGS, isColor: false, duplex: 'two-sided-long-edge', copies: 3, watermarkText: 'Internal',
    pageRange: '1-3', printer: 'ignored', printerUri: 'ipp://host/printers/a', driverOptions: { pageSize, quality, unknown: pageSize }
  }
  const preset = snapshotPrintSettings(values)
  assert.equal(preset.isColor, false)
  assert.equal(preset.copies, 3)
  assert.equal(preset.watermarkText, 'Internal')
  assert.equal(preset.printerUri, 'ipp://host/printers/a')
  assert.deepEqual(preset.driverOptions, { pageSize, quality })
  assert.ok(!('pageRange' in preset) && !('printer' in preset))
  assert.ok(!('watermarkText' in snapshotPrintSettings(values, { includeWatermark: false })), 'last settings never carry a watermark')
})

test('sanitize drops invalid values instead of rejecting the whole snapshot', () => {
  for (const raw of [null, undefined, 'x', [], 42]) assert.deepEqual(sanitizePrintSettings(raw), {})
  const clean = sanitizePrintSettings({
    isColor: 'false', duplex: 'both', copies: 0, paperSize: 'custom:101.6x152.4', paperType: 'photo',
    mediaSource: 'tray 1; rm', printScaling: 'custom', scalePercent: 401, numberUp: 3, numberUpLayout: 'tblr',
    pageBorder: 'single', mirror: true, extra: 'x'
  })
  assert.deepEqual(clean, {
    paperSize: 'custom:101.6x152.4', paperType: 'photo', printScaling: 'custom', numberUpLayout: 'tblr', pageBorder: 'single', mirror: true
  })
  assert.equal(sanitizePrintSettings({ paperSize: 'custom:abc' }).paperSize, undefined)
  // 没有打印机归属的驱动选项无法校验，整体丢弃。
  assert.ok(!('driverOptions' in sanitizePrintSettings({ driverOptions: { pageSize } })))
  assert.deepEqual(sanitizePrintSettings({ printerUri: 'p', driverOptions: { pageSize: { attribute: 'PageSize' }, quality: 'high' } }).driverOptions, {})
})

test('defaults are complete and valid', () => {
  assert.deepEqual(sanitizePrintSettings(DEFAULT_PRINT_SETTINGS), DEFAULT_PRINT_SETTINGS)
})

test('last settings round-trip through storage and tolerate broken or unavailable storage', () => {
  const data = new Map()
  const browser = { localStorage: { getItem: key => data.get(key) ?? null, setItem: (key, value) => data.set(key, value) } }
  assert.equal(readLastPrintSettings(browser), null)
  writeLastPrintSettings({ copies: 2, printerUri: 'p', driverOptions: { pageSize } }, browser)
  assert.deepEqual(readLastPrintSettings(browser), { copies: 2, printerUri: 'p', driverOptions: { pageSize } })
  data.set(LAST_PRINT_SETTINGS_KEY, '{broken')
  assert.equal(readLastPrintSettings(browser), null)
  const blocked = { get localStorage() { throw new Error('SecurityError') } }
  assert.equal(readLastPrintSettings(blocked), null)
  writeLastPrintSettings({ copies: 2 }, blocked)
})
