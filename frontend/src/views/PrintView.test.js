import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import { ref, computed, watch, effectScope, nextTick } from 'vue'
import { appendPhotoUploads, isPhotoFile } from '../utils/photo-upload.js'
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
      appendPhotoUploads, isPhotoFile,
      useToast: () => ({ add: () => {} }),
      localStorage: { getItem: () => null, setItem: () => {} },
      apiFetch: async (url, options) => {
        requests.push({ url, options })
        return { ok: true, blob: async () => new Blob(['preview PDF'], { type: 'application/pdf' }), json: async () => url === '/api/print-records' ? [] : { ok: true, pages: 1, jobId: '1' } }
      },
      readError: async () => 'error', isOfficeFile: () => false, isOFDFile: () => false,
      paperDimensions: () => ({ width: 152, height: 102 }),
      formatFileSize: () => '', printerLabel: () => '', printerDescription: () => '',
      FileUpload: {}, IdCardUpload: {}, PrintPreview: {}, PrintOptions: {}, PrintRecordList: {}, PrinterStatus: {}
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
