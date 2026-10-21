import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import { ref, computed, watch, effectScope, nextTick } from 'vue'
import { appendPhotoUploads, isPhotoFile } from '../utils/photo-upload.js'
import * as printSettings from '../utils/print-settings.js'
import { i18n } from '../i18n.js'

test('single, combined and batch photos submit originals even when a PDF preview exists', async () => {
  const { descriptor } = parse(readFileSync(new URL('./PrintView.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'photo-print-view-test' }).content
  for (const mode of ['single', 'combined', 'batch', 'single-no-mime', 'batch-no-mime']) {
    const requests = []
    const component = runInNewContext(script.replace(/^import .*$/gm, '').replace('export default', 'const component =') + '\ncomponent', {
      ref, computed, watch, onMounted: () => {}, onUnmounted: () => {},
      useI18n: () => i18n.global,
      readPreference: () => null, writePreference: () => {}, translateError: value => value, jobStateText: () => '', printerChoiceLabel: choice => choice?.label || '',
      File, Blob, FormData, URL,
      appendPhotoUploads, isPhotoFile, ...printSettings,
      useToast: () => ({ add: () => {} }),
      localStorage: { getItem: () => null, setItem: () => {} },
      apiFetch: async (url, options) => {
        requests.push({ url, options })
        return { ok: true, blob: async () => new Blob(['preview PDF'], { type: 'application/pdf' }), json: async () => url === '/api/print-records' ? [] : { ok: true, pages: 1, jobId: '1' } }
      },
      readError: async () => 'error', isOfficeFile: () => false, isOFDFile: () => false,
      paperDimensions: () => ({ width: 152, height: 102 }),
      formatFileSize: () => '', printerLabel: () => '', printerDescription: () => '',
      FileUpload: {}, IdCardUpload: {}, PrintPreview: {}, PrintOptions: {}, PrintPresets: {}, PrintRecordList: {}, PrinterStatus: {}
    })
    const scope = effectScope()
    const state = scope.run(() => component.setup({}, { expose: () => {}, emit: () => {} }))
    try {
      const original = new File([new Uint8Array([255, 216, 255, 225, 4, 5, 6])], 'original.jpg', { type: mode.endsWith('no-mime') ? '' : 'image/jpeg' })
      state.printer.value = 'http://localhost:631/printers/photo'
      state.selectedFile.value = original
      state.photoDateWatermark.value = true
      if (mode === 'combined') {
        state.selectedImages.value = [original, original]
        state.imageRotations.value = [0, 90]
        state.imagesPerPage.value = 2
      }
      if (mode.startsWith('batch')) state.batchFiles.value = [original]
      assert.equal(state.paperDimText.value, '102×152mm')
      assert.equal(state.paperPreviewStyle.value.aspectRatio, '102 / 152')
      state.orientation.value = 'landscape'
      assert.equal(state.paperDimText.value, '152×102mm')
      assert.equal(state.paperPreviewStyle.value.aspectRatio, '152 / 102')
      await nextTick()
      if (!mode.startsWith('batch')) {
        await state.convertToPdf()
        assert.ok(state.pdfBlob.value, `${mode}: PDF preview failed`)
        const conversion = requests.find(r => r.url === '/api/convert')
        assert.ok(conversion, `${mode}: missing preview request`)
        assert.equal(conversion.options.body.get('rotations'), mode === 'combined' ? '0,90' : '0')
      } else {
        state.pdfBlob.value = new Blob(['cached PDF preview'], { type: 'application/pdf' })
      }
      const conversions = requests.filter(r => r.url === '/api/convert').length
      await state.uploadAndPrint()
      assert.equal(requests.filter(r => r.url === '/api/convert').length, conversions)
      const prints = requests.filter(r => r.url === '/api/print')
      assert.equal(prints.length, 1, mode)
      const body = prints[0].options.body
      const photos = body.getAll(mode === 'combined' ? 'files' : 'file')
      assert.equal(photos.length, mode === 'combined' ? 2 : 1)
      for (const photo of photos) {
        assert.equal(photo.name, original.name)
        assert.deepEqual(await photo.arrayBuffer(), await original.arrayBuffer())
      }
      if (mode === 'combined') {
        assert.equal(body.get('rotations'), '0,90')
        assert.equal(body.get('per_page'), '2')
      }
      assert.equal(body.get('photo_date_watermark'), 'true')
    } finally {
      state.clearFile()
      scope.stop()
    }
  }
})

test('print settings are remembered after printing and restored per printer (issue #128)', async () => {
  const { descriptor } = parse(readFileSync(new URL('./PrintView.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'print-settings-view-test' }).content
  const storage = new Map()
  const browser = { localStorage: { getItem: key => storage.get(key) ?? null, setItem: (key, value) => storage.set(key, value) } }
  const mounted = []
  const component = runInNewContext(script.replace(/^import .*$/gm, '').replace('export default', 'const component =') + '\ncomponent', {
    ref, computed, watch, onMounted: fn => mounted.push(fn), onUnmounted: () => {},
    useI18n: () => i18n.global,
    readPreference: () => null, writePreference: () => {}, translateError: value => value, jobStateText: () => '', printerChoiceLabel: choice => choice?.label || '',
    File, Blob, FormData, URL, setInterval: () => 0, clearInterval: () => {},
    appendPhotoUploads, isPhotoFile,
    ...printSettings,
    readLastPrintSettings: () => printSettings.readLastPrintSettings(browser),
    writeLastPrintSettings: value => printSettings.writeLastPrintSettings(value, browser),
    useToast: () => ({ add: () => {} }),
    apiFetch: async url => ({
      ok: true,
      json: async () => url === '/api/printers'
        ? [{ uri: 'ipp://h/printers/a' }, { uri: 'ipp://h/printers/b' }]
        : url === '/api/print-records' ? [] : url.startsWith('/api/printer-info') ? { mediaSourceSupported: ['tray-1'] } : { ok: true, pages: 1, jobId: '1' }
    }),
    readError: async () => 'error', isOfficeFile: () => false, isOFDFile: () => false,
    paperDimensions: () => ({ width: 210, height: 297 }),
    formatFileSize: () => '', printerLabel: () => '', printerDescription: () => '',
    FileUpload: {}, IdCardUpload: {}, PrintPreview: {}, PrintOptions: {}, PrintPresets: {}, PrintRecordList: {}, PrinterStatus: {}
  })
  const create = () => {
    mounted.length = 0
    const scope = effectScope()
    const state = scope.run(() => component.setup({ session: { username: 'alice' } }, { expose: () => {}, emit: () => {} }))
    return { state, scope }
  }

  const pageSize = { attribute: 'PageSize', value: 'A4', label: 'A4', widthMM: 210, heightMM: 297 }
  const first = create()
  try {
    const s = first.state
    s.printer.value = 'ipp://h/printers/b'
    s.isColor.value = false
    s.duplex.value = 'two-sided-long-edge'
    s.copies.value = 2
    s.watermarkText.value = 'Draft'
    s.mediaSource.value = 'tray-1'
    s.driverOptions.value = { pageSize }
    s.selectedFile.value = new File(['%PDF'], 'a.pdf', { type: 'application/pdf' })
    await s.uploadAndPrint()
    const saved = JSON.parse(storage.get(printSettings.LAST_PRINT_SETTINGS_KEY))
    assert.equal(saved.copies, 2)
    assert.equal(saved.printerUri, 'ipp://h/printers/b')
    assert.ok(!('watermarkText' in saved), 'a one-off watermark is not remembered')

    // 套用另一台打印机保存的预置：参数照搬，驱动选项清空并触发重新匹配。
    const rematch = s.driverRematchKey.value
    s.applyPrintSettings({ ...saved, printerUri: 'ipp://h/printers/other', watermarkText: 'Internal' })
    assert.equal(s.watermarkText.value, 'Internal')
    assert.equal(JSON.stringify(s.driverOptions.value), '{}')
    assert.equal(s.driverRematchKey.value, rematch + 1)
    s.applyPrintSettings(saved)
    assert.equal(JSON.stringify(s.driverOptions.value), JSON.stringify({ pageSize }))
    assert.equal(s.watermarkText.value, '', 'missing fields fall back to defaults')

    s.pageRange.value = '2-3'
    s.resetPrintSettings()
    assert.equal(s.isColor.value, true)
    assert.equal(s.copies.value, 1)
    assert.equal(s.pageRange.value, '')

    // 发票模式会改颜色/缩放，切回标准模式恢复上次设置而不是出厂默认。
    s.switchMode('invoice')
    assert.equal(s.isColor.value, false)
    s.switchMode('standard')
    assert.equal(s.isColor.value, false)
    assert.equal(s.copies.value, 2)
    assert.equal(s.duplex.value, 'two-sided-long-edge')
  } finally {
    first.state.clearFile()
    first.scope.stop()
  }

  // 重新进入页面：先恢复参数，确定打印机后再恢复同一台打印机的驱动选项。
  const second = create()
  try {
    assert.equal(second.state.copies.value, 1, 'nothing is applied before mount')
    await mounted[0]()
    assert.equal(second.state.printer.value, 'ipp://h/printers/a')
    assert.equal(second.state.copies.value, 2)
    assert.equal(JSON.stringify(second.state.driverOptions.value), '{}', 'driver options from another printer are not restored')
  } finally {
    second.scope.stop()
  }
})
