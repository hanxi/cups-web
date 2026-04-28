<template>
  <div class="p-3 sm:p-4 md:p-6 max-w-7xl mx-auto">
    <!-- 顶部标题栏：桌面端与主体栅格对齐（3 + 2），移动端单行 flex -->
    <div class="mb-3 grid grid-cols-1 lg:grid-cols-5 gap-x-4 gap-y-2">
      <!-- 左：打印标题 + 打印机下拉（桌面 col-span-3，与左栏 FileUpload 对齐） -->
      <div class="lg:col-span-3 flex items-center gap-2 sm:gap-3 min-w-0">
        <h1 class="text-lg font-bold flex items-center gap-2 shrink-0">
          <UIcon name="i-lucide-printer" class="w-5 h-5 text-primary" />
          打印
        </h1>
        <USelect
          :model-value="printer"
          :items="printerItems"
          value-key="value"
          label-key="label"
          placeholder="选择打印机"
          icon="i-lucide-printer"
          class="flex-1 min-w-0"
          @update:model-value="onPrinterSelect"
        />
        <!-- 移动端：刷新按钮紧跟下拉（纯图标），桌面端隐藏 -->
        <UButton
          variant="ghost"
          size="xs"
          icon="i-lucide-refresh-cw"
          class="shrink-0 lg:hidden"
          @click="refreshAll"
          :loading="refreshing"
        />
      </div>
      <!-- 右：刷新按钮（桌面 col-span-2，靠右，与右栏对齐；移动端隐藏） -->
      <div class="hidden lg:flex lg:col-span-2 items-center justify-end">
        <UButton
          variant="ghost"
          size="xs"
          icon="i-lucide-refresh-cw"
          @click="refreshAll"
          :loading="refreshing"
        >刷新</UButton>
      </div>
    </div>

    <!-- 主体两栏布局 -->
    <div class="grid grid-cols-1 lg:grid-cols-5 gap-4">
      <!-- 左栏：打印设置 + 预览 -->
      <div class="lg:col-span-3 space-y-4">
        <!-- 1. 文件上传（打印机已合并到顶部标题栏） -->
        <FileUpload
          :selected-file="selectedFile"
          :display-name="fileDisplayName"
          :converting="converting"
          :converted="converted"
          :preview-url="previewUrl"
          :download-name="downloadName"
          :pdf-blob="pdfBlob"
          :can-print="canPrint"
          :can-convert="canConvert"
          :printing="printing"
          :is-multi-image="isMultiImage"
          :total-size="multiImageTotalSize"
          @file-selected="processFile"
          @files-selected="processMultipleImages"
          @clear="clearFile"
          @convert="convertToPdf"
          @print="uploadAndPrint"
        />

        <!-- 3. 预览（紧贴上传） -->
        <PrintPreview
          :selected-file="selectedFile"
          :is-multi-image="isMultiImage"
          :preview-url="previewUrl"
          :preview-type="previewType"
          :text-preview="textPreview"
          :paper-size-label="paperSizeLabel"
          v-model:orientation="orientation"
          :orientation-label="orientationLabel"
          :paper-dim-text="paperDimText"
          :paper-preview-style="paperPreviewStyle"
        />

        <!-- 4. 开始打印按钮（紧随预览之后，首屏即可见；主 CTA，视觉上与顶部导航区分） -->
        <UButton
          color="primary"
          size="xl"
          :ui="{ base: 'justify-center', label: 'flex-1 text-center' }"
          class="w-full font-semibold tracking-wide shadow-lg shadow-primary/25 hover:shadow-xl hover:shadow-primary/35 transition-all ring-1 ring-primary/30"
          icon="i-lucide-send"
          :disabled="!canPrint || printing"
          :loading="printing"
          @click="uploadAndPrint"
        >
          开始打印
        </UButton>

        <!-- 5. 多图片列表 -->
        <UCard v-if="selectedImages.length > 1">
          <template #header>
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2 font-semibold">
                <UIcon name="i-lucide-images" class="w-5 h-5" />
                已选图片 ({{ selectedImages.length }})
              </div>
              <UButton variant="ghost" size="xs" color="error" icon="i-lucide-trash-2" @click="clearFile">清空全部</UButton>
            </div>
          </template>
          <div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
            <div v-for="(img, idx) in selectedImages" :key="idx" class="relative group rounded-lg overflow-hidden border border-default">
              <img :src="imageThumbnails[idx]" class="w-full h-20 object-cover" />
              <div class="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                <UButton variant="solid" size="xs" color="error" icon="i-lucide-x" @click="removeImage(idx)" />
              </div>
              <p class="text-xs truncate px-1 py-0.5">{{ img.name }}</p>
            </div>
          </div>
        </UCard>

        <!-- 6. 打印参数（不再内嵌预览与提交按钮） -->
        <PrintOptions
          v-model:isColor="isColor"
          v-model:duplex="duplex"
          v-model:copies="copies"
          v-model:paperSize="paperSize"
          v-model:paperType="paperType"
          v-model:printScaling="printScaling"
          v-model:pageRange="pageRange"
          v-model:mirror="mirror"
          :printing="printing"
        />
      </div>

      <!-- 右栏：打印记录 + 打印机状态 -->
      <div class="lg:col-span-2 space-y-4">
        <PrintRecordList :records="printRecords" :loading="loadingRecords" @refresh="loadPrintRecords" />
        <PrinterStatus :printer-info="printerInfo" :printer-uri="printer" :loading="loadingPrinterInfo" :error="printerInfoError" @refresh="loadPrinterInfo" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { jsPDF } from 'jspdf'
import { getCSRF } from '../utils/api'
import { isOfficeFile, isOFDFile } from '../utils/file'
import FileUpload from '../components/print/FileUpload.vue'
import PrintPreview from '../components/print/PrintPreview.vue'
import PrintOptions from '../components/print/PrintOptions.vue'
import PrintRecordList from '../components/print/PrintRecordList.vue'
import PrinterStatus from '../components/print/PrinterStatus.vue'

const emit = defineEmits(['logout'])
const toast = useToast()

// ─── 打印机 ───────────────────────────────────────────────
const printer = ref('')
const printers = ref([])

// ─── 文件 ─────────────────────────────────────────────────
const selectedFile = ref(null)
const previewUrl = ref('')
const previewType = ref('')
const textPreview = ref('')
const converting = ref(false)
const converted = ref(false)
const pdfBlob = ref(null)
const downloadName = ref('')
const selectedImages = ref([])
const imageThumbnails = ref([])
const fileDisplayName = ref('')

// ─── 打印参数 ─────────────────────────────────────────────
const isColor = ref(true)
const duplex = ref('one-sided')
const orientation = ref('portrait')
const copies = ref(1)
const paperSize = ref('A4')
const paperType = ref('plain')
const printScaling = ref('fit')
const pageRange = ref('')
const mirror = ref(false)

// ─── 状态 ─────────────────────────────────────────────────
const printing = ref(false)
const refreshing = ref(false)

// ─── 打印记录 ─────────────────────────────────────────────
const printRecords = ref([])
const loadingRecords = ref(false)

// ─── 打印机状态 ───────────────────────────────────────────
const printerInfo = ref(null)
const loadingPrinterInfo = ref(false)
const printerInfoError = ref('')

// ─── 纸张尺寸映射 ─────────────────────────────────────────
const paperDimensionsMap = {
  'A5': { width: 148, height: 210 },
  'A4': { width: 210, height: 297 },
  'A3': { width: 297, height: 420 },
  'A2': { width: 420, height: 594 },
  'A1': { width: 594, height: 841 },
  '5inch': { width: 89, height: 127 },
  '6inch': { width: 102, height: 152 },
  '7inch': { width: 127, height: 178 },
  '8inch': { width: 152, height: 203 },
  '10inch': { width: 203, height: 254 },
  'Letter': { width: 216, height: 279 },
  'Legal': { width: 216, height: 356 },
}

// ─── 选项列表（供 PrintOptions 内部的 paperSizeLabel 等计算使用） ──
const orientationItems = [
  { label: '纵向', value: 'portrait' },
  { label: '横向', value: 'landscape' }
]
const paperSizeItems = [
  { label: 'A5 (148×210mm)', value: 'A5' },
  { label: 'A4 (210×297mm)', value: 'A4' },
  { label: 'A3 (297×420mm)', value: 'A3' },
  { label: 'A2 (420×594mm)', value: 'A2' },
  { label: 'A1 (594×841mm)', value: 'A1' },
  { label: '5寸 (89×127mm)', value: '5inch' },
  { label: '6寸 (102×152mm)', value: '6inch' },
  { label: '7寸 (127×178mm)', value: '7inch' },
  { label: '8寸 (152×203mm)', value: '8inch' },
  { label: '10寸 (203×254mm)', value: '10inch' },
  { label: 'Letter (8.5×11in)', value: 'Letter' },
  { label: 'Legal (8.5×14in)', value: 'Legal' }
]

// ─── 计算属性 ─────────────────────────────────────────────
const isMultiImage = computed(() => selectedImages.value.length > 1)
const multiImageTotalSize = computed(() => selectedImages.value.reduce((sum, f) => sum + f.size, 0))
const canPrint = computed(() => !!printer.value && (!!pdfBlob.value || !!selectedFile.value || isMultiImage.value))

// 打印机下拉选项（原 PrinterSelector.vue 迁移过来）
const printerItems = computed(() =>
  printers.value.map(p => ({ label: `${p.name} — ${p.uri}`, value: p.uri }))
)
function onPrinterSelect(val) {
  printer.value = val
  onPrinterChange()
}
const canConvert = computed(() => {
  if (isMultiImage.value) return !converting.value && !converted.value
  return !!selectedFile.value && !converting.value && selectedFile.value.type !== 'application/pdf'
})

const paperSizeLabel = computed(() => {
  const item = paperSizeItems.find(i => i.value === paperSize.value)
  return item?.label || paperSize.value
})

const orientationLabel = computed(() => {
  const item = orientationItems.find(i => i.value === orientation.value)
  return item?.label || (orientation.value === 'portrait' ? '纵向' : '横向')
})

const paperDimText = computed(() => {
  const dim = paperDimensionsMap[paperSize.value]
  if (!dim) return ''
  if (orientation.value === 'landscape') {
    return `${dim.height}×${dim.width}mm`
  }
  return `${dim.width}×${dim.height}mm`
})

const paperPreviewStyle = computed(() => {
  const dim = paperDimensionsMap[paperSize.value]
  if (!dim) return {}
  const isLandscape = orientation.value === 'landscape'
  const width = isLandscape ? dim.height : dim.width
  const height = isLandscape ? dim.width : dim.height
  const ratio = width / height
  const maxW = 380
  const maxH = 480
  let displayWidth, displayHeight
  if (ratio > 1) {
    // Landscape: width-constrained
    displayWidth = maxW
    displayHeight = displayWidth / ratio
    if (displayHeight > maxH) {
      displayHeight = maxH
      displayWidth = displayHeight * ratio
    }
  } else {
    // Portrait: height-constrained
    displayHeight = maxH
    displayWidth = displayHeight * ratio
    if (displayWidth > maxW) {
      displayWidth = maxW
      displayHeight = displayWidth / ratio
    }
  }
  return { width: `${Math.round(displayWidth)}px`, height: `${Math.round(displayHeight)}px`, maxHeight: '70vh' }
})

// ─── 文件操作 ─────────────────────────────────────────────
function clearPreviewUrl() {
  if (previewUrl.value) {
    try { URL.revokeObjectURL(previewUrl.value) } catch (_) { /* 忽略 */ }
  }
  previewUrl.value = ''
}

function clearFile() {
  clearPreviewUrl()
  previewType.value = ''
  textPreview.value = ''
  pdfBlob.value = null
  converted.value = false
  selectedFile.value = null
  downloadName.value = ''
  // 清空多图片状态
  imageThumbnails.value.forEach(url => { try { URL.revokeObjectURL(url) } catch (_) {} })
  selectedImages.value = []
  imageThumbnails.value = []
  fileDisplayName.value = ''
}

function processFile(f) {
  clearFile()
  selectedFile.value = f
  fileDisplayName.value = ''
  downloadName.value = f.name.replace(/\.[^/.]+$/, '') + '.pdf'

  if (f.type === 'application/pdf') {
    previewUrl.value = URL.createObjectURL(f)
    previewType.value = 'pdf'
    pdfBlob.value = f
    converted.value = true
  } else if (f.type.startsWith('image/') || /\.(heic|heif)$/i.test(f.name)) {
    if (isHeicImage(f)) {
      // HEIC/HEIF 浏览器无法原生解码，先提示"正在转换"，异步用 heic2any 转成 JPEG 再预览
      previewType.value = 'text'
      textPreview.value = '正在解码 HEIC/HEIF 图片，请稍候…'
      const originalFile = f
      heicBlobToJpegBlob(originalFile)
        .then(jpegFile => {
          // 若用户在转码期间已切换/清空文件，则丢弃结果
          if (selectedFile.value !== originalFile) return
          selectedFile.value = jpegFile
          downloadName.value = jpegFile.name.replace(/\.[^/.]+$/, '') + '.pdf'
          clearPreviewUrl()
          previewUrl.value = URL.createObjectURL(jpegFile)
          previewType.value = 'image'
          textPreview.value = ''
        })
        .catch(err => {
          if (selectedFile.value !== originalFile) return
          previewType.value = 'text'
          textPreview.value = `HEIC 解码失败：${err.message || '未知错误'}`
          toast.add({ title: 'HEIC 解码失败', description: err.message, color: 'error', icon: 'i-lucide-x-circle' })
        })
    } else {
      previewUrl.value = URL.createObjectURL(f)
      previewType.value = 'image'
    }
  } else if (isOfficeFile(f)) {
    previewType.value = 'text'
    textPreview.value = 'Office 文档（无法直接预览）。点击"转换为 PDF"生成预览。'
  } else if (isOFDFile(f)) {
    previewType.value = 'text'
    textPreview.value = 'OFD文件（开放版式文档）无法直接预览。点击"转换为PDF"生成预览。'
  } else if (f.type.startsWith('text/') || /\.(txt|md|html)$/i.test(f.name)) {
    const reader = new FileReader()
    reader.onload = () => {
      textPreview.value = reader.result
      previewType.value = 'text'
    }
    reader.readAsText(f)
  } else {
    previewType.value = 'text'
    textPreview.value = '无法预览此文件类型，可直接提交打印。'
  }
}

// 判断是否为 HEIC/HEIF 格式（iPhone 默认的高效图片格式，浏览器无法原生解码）
function isHeicImage(file) {
  const type = (file.type || '').toLowerCase()
  if (type === 'image/heic' || type === 'image/heif') return true
  if (/\.(heic|heif)$/i.test(file.name)) return true
  return false
}

// 动态加载 heic2any 并将 HEIC/HEIF Blob 转成 JPEG Blob。
// 使用动态 import 是为了不把 heic2any（~800KB）打进主包，仅当用户真的上传 HEIC 时才按需加载。
async function heicBlobToJpegBlob(file) {
  let heic2any
  try {
    const mod = await import('heic2any')
    heic2any = mod.default || mod
  } catch (e) {
    throw new Error('加载 HEIC 解码器失败，请检查网络后重试')
  }
  try {
    const result = await heic2any({ blob: file, toType: 'image/jpeg', quality: 0.9 })
    // heic2any 多图时可能返回数组，这里只取第一张
    const blob = Array.isArray(result) ? result[0] : result
    // 保留原文件名但改为 .jpg 后缀，便于后续流程展示
    const jpegName = file.name.replace(/\.(heic|heif)$/i, '') + '.jpg'
    return new File([blob], jpegName, { type: 'image/jpeg', lastModified: Date.now() })
  } catch (e) {
    throw new Error(`HEIC 解码失败：${file.name}（文件可能已损坏或非标准 HEIC 格式）`)
  }
}

// 将 File 加载为 HTMLImageElement，使用 decode() 保证手机端宽高可用。
// 遇到 HEIC/HEIF 会先用 heic2any 转成 JPEG，再走正常解码。
async function loadImageElement(file) {
  const realFile = isHeicImage(file) ? await heicBlobToJpegBlob(file) : file
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(realFile)
    const img = new Image()
    img.decoding = 'async'
    const done = (err) => {
      URL.revokeObjectURL(url)
      if (err) reject(err)
      else resolve(img)
    }
    img.onload = () => {
      // 优先使用 decode() 确保像素已解码，避免手机端 naturalWidth 读到 0
      if (typeof img.decode === 'function') {
        img.decode().then(() => done()).catch(() => done())
      } else {
        done()
      }
    }
    img.onerror = () => done(new Error(`图片加载失败：${realFile.name}（格式可能不受支持）`))
    img.src = url
  })
}

// 将图片重绘到 canvas 并编码为 JPEG dataURL。
// 用于规避 iOS Safari canvas 最大尺寸限制（约 4096px）导致 PDF 预览空白的问题，
// 同时将 webp/gif 首帧等各种格式统一为 jsPDF 可靠支持的 JPEG。
function imageToJpegDataUrl(img, maxEdge = 3000, quality = 0.9) {
  const srcW = img.naturalWidth || img.width
  const srcH = img.naturalHeight || img.height
  if (!srcW || !srcH) {
    throw new Error('图片尺寸读取失败，可能格式不受支持')
  }
  let targetW = srcW
  let targetH = srcH
  if (Math.max(srcW, srcH) > maxEdge) {
    const scale = maxEdge / Math.max(srcW, srcH)
    targetW = Math.max(1, Math.round(srcW * scale))
    targetH = Math.max(1, Math.round(srcH * scale))
  }
  const canvas = document.createElement('canvas')
  canvas.width = targetW
  canvas.height = targetH
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('当前浏览器不支持 Canvas 绘制')
  // 填充白底，避免透明 PNG 在 JPEG 中变黑
  ctx.fillStyle = '#FFFFFF'
  ctx.fillRect(0, 0, targetW, targetH)
  ctx.drawImage(img, 0, 0, targetW, targetH)
  const dataUrl = canvas.toDataURL('image/jpeg', quality)
  if (!dataUrl || dataUrl === 'data:,') {
    throw new Error('图片编码失败（分辨率可能超过浏览器限制）')
  }
  return { dataUrl, width: targetW, height: targetH }
}

async function imagesToPdfBlob(files, orient, pSize) {
  const fileList = Array.isArray(files) ? files : [files]
  const dims = paperDimensionsMap[pSize] || { width: 210, height: 297 }
  const isLandscape = orient === 'landscape'
  const doc = new jsPDF({
    orientation: isLandscape ? 'l' : 'p',
    unit: 'mm',
    format: [dims.width, dims.height]
  })
  const pageWidth = doc.internal.pageSize.getWidth()
  const pageHeight = doc.internal.pageSize.getHeight()
  const margin = 10
  const maxW = pageWidth - margin * 2
  const maxH = pageHeight - margin * 2

  for (let i = 0; i < fileList.length; i++) {
    if (i > 0) doc.addPage([dims.width, dims.height], isLandscape ? 'l' : 'p')
    const file = fileList[i]
    const img = await loadImageElement(file)
    const { dataUrl } = imageToJpegDataUrl(img)
    const naturalW = img.naturalWidth || img.width
    const naturalH = img.naturalHeight || img.height
    const imgRatio = naturalW / naturalH
    let drawW, drawH
    if (imgRatio > maxW / maxH) { drawW = maxW; drawH = maxW / imgRatio }
    else { drawH = maxH; drawW = maxH * imgRatio }
    const x = margin + (maxW - drawW) / 2
    const y = margin + (maxH - drawH) / 2
    doc.addImage(dataUrl, 'JPEG', x, y, drawW, drawH)
  }
  return doc.output('blob')
}

function processMultipleImages(files) {
  clearFile()
  const arr = Array.from(files)
  selectedImages.value = arr
  fileDisplayName.value = `${arr.length}张图片`
  downloadName.value = '合并图片.pdf'
  converted.value = false

  // HEIC 无法直接生成缩略图，先用占位图，再异步转码后替换
  const hasHeic = arr.some(isHeicImage)
  imageThumbnails.value = arr.map(f => isHeicImage(f) ? '' : URL.createObjectURL(f))
  // 用第一张可用的缩略图作为预览；若没有则等待转码
  const firstReadyIdx = imageThumbnails.value.findIndex(u => !!u)
  if (firstReadyIdx >= 0) {
    previewUrl.value = imageThumbnails.value[firstReadyIdx]
    previewType.value = 'image'
  } else {
    previewType.value = 'text'
    textPreview.value = '正在解码 HEIC/HEIF 图片，请稍候…'
  }

  if (hasHeic) {
    const batchFiles = arr
    arr.forEach((f, idx) => {
      if (!isHeicImage(f)) return
      heicBlobToJpegBlob(f)
        .then(jpegFile => {
          // 若用户在转码期间已切换/清空文件，则丢弃结果
          if (selectedImages.value !== batchFiles) return
          selectedImages.value[idx] = jpegFile
          imageThumbnails.value[idx] = URL.createObjectURL(jpegFile)
          // 若之前没有可用预览，此时用第一张已就绪的
          if (!previewUrl.value || previewType.value !== 'image') {
            const firstIdx = imageThumbnails.value.findIndex(u => !!u)
            if (firstIdx >= 0) {
              previewUrl.value = imageThumbnails.value[firstIdx]
              previewType.value = 'image'
              textPreview.value = ''
            }
          }
        })
        .catch(err => {
          if (selectedImages.value !== batchFiles) return
          toast.add({
            title: `HEIC 解码失败：${f.name}`,
            description: err.message,
            color: 'error',
            icon: 'i-lucide-x-circle'
          })
        })
    })
  }
}

function removeImage(idx) {
  URL.revokeObjectURL(imageThumbnails.value[idx])
  selectedImages.value.splice(idx, 1)
  imageThumbnails.value.splice(idx, 1)
  if (selectedImages.value.length === 1) {
    // 切换为单图片模式
    const f = selectedImages.value[0]
    selectedImages.value = []
    imageThumbnails.value.forEach(url => { try { URL.revokeObjectURL(url) } catch (_) {} })
    imageThumbnails.value = []
    fileDisplayName.value = ''
    processFile(f)
  } else if (selectedImages.value.length === 0) {
    clearFile()
  } else {
    fileDisplayName.value = `${selectedImages.value.length}张图片`
    // 更新预览为第一张（不调用 clearPreviewUrl，因为旧 previewUrl 与 imageThumbnails 共享同一 URL）
    previewUrl.value = imageThumbnails.value[0]
    converted.value = false
    pdfBlob.value = null
  }
}

function textToPdfBlob(text, orient, pSize) {
  const dims = paperDimensionsMap[pSize] || { width: 210, height: 297 }
  const isLandscape = orient === 'landscape'
  const doc = new jsPDF({
    orientation: isLandscape ? 'l' : 'p',
    unit: 'mm',
    format: [dims.width, dims.height]
  })
  const pageWidth = doc.internal.pageSize.getWidth()
  const margin = 15
  const maxWidth = pageWidth - margin * 2
  const lines = doc.splitTextToSize(text || '', maxWidth)
  doc.text(lines, margin, margin)
  return doc.output('blob')
}

async function convertOfficeToPdf(file) {
  const fd = new FormData()
  fd.append('file', file, file.name)
  const resp = await fetch('/api/convert', {
    method: 'POST',
    body: fd,
    credentials: 'include',
    headers: { 'X-CSRF-Token': getCSRF() }
  })
  if (!resp.ok) throw new Error('服务端转换失败：' + await resp.text())
  return resp.blob()
}

async function convertToPdf() {
  if (!selectedFile.value && !isMultiImage.value) return
  converting.value = true
  try {
    const f = selectedFile.value
    let blob
    if (isMultiImage.value) {
      blob = await imagesToPdfBlob(selectedImages.value, orientation.value, paperSize.value)
    } else if (isOfficeFile(f) || isOFDFile(f)) {
      blob = await convertOfficeToPdf(f)
    } else if (f.type.startsWith('image/')) {
      blob = await imagesToPdfBlob(f, orientation.value, paperSize.value)
    } else {
      const text = await f.text()
      blob = textToPdfBlob(text, orientation.value, paperSize.value)
    }
    pdfBlob.value = blob
    clearPreviewUrl()
    previewUrl.value = URL.createObjectURL(blob)
    previewType.value = 'pdf'
    converted.value = true
    toast.add({ title: '转换成功', color: 'success', icon: 'i-lucide-check-circle' })
  } catch (e) {
    toast.add({ title: '转换失败', description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    converting.value = false
  }
}

// ─── 打印 ─────────────────────────────────────────────────
async function uploadAndPrint() {
  if (!printer.value) { toast.add({ title: '请选择打印机', color: 'warning' }); return }

  const fileToSend = pdfBlob.value || selectedFile.value
  if (!fileToSend && !isMultiImage.value) { toast.add({ title: '请先选择文件', color: 'warning' }); return }
  // 多图片未转换时自动转换
  if (isMultiImage.value && !pdfBlob.value) {
    await convertToPdf()
    if (!pdfBlob.value) return
  }
  const actualFile = pdfBlob.value || selectedFile.value
  const filename = pdfBlob.value
    ? (downloadName.value || 'document.pdf')
    : (selectedFile.value ? selectedFile.value.name : 'document')

  const form = new FormData()
  form.append('file', actualFile, filename)
  form.append('printer', printer.value)
  form.append('duplex', duplex.value === 'one-sided' ? 'false' : 'true')
  form.append('color', isColor.value ? 'true' : 'false')
  form.append('copies', String(copies.value))
  form.append('orientation', orientation.value)
  form.append('paper_size', paperSize.value)
  form.append('paper_type', paperType.value)
  form.append('print_scaling', printScaling.value)
  if (pageRange.value.trim()) form.append('page_range', pageRange.value.trim())
  if (mirror.value) form.append('mirror', 'true')

  printing.value = true
  try {
    const resp = await fetch('/api/print', {
      method: 'POST',
      body: form,
      credentials: 'include',
      headers: { 'X-CSRF-Token': getCSRF() }
    })
    if (!resp.ok) {
      const data = await resp.json().catch(() => ({}))
      if (resp.status === 401) emit('logout')
      throw new Error(data.error || resp.statusText)
    }
    const j = await resp.json()
    toast.add({
      title: '打印任务已提交',
      description: `任务ID：${j.jobId || '—'}，共 ${j.pages} 页`,
      color: 'success',
      icon: 'i-lucide-check-circle'
    })
    localStorage.setItem('last_printer', printer.value)
    await loadPrintRecords()
  } catch (e) {
    toast.add({ title: '打印失败', description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    printing.value = false
  }
}

// ─── 打印记录 ─────────────────────────────────────────────
async function loadPrintRecords(silent = false) {
  if (!silent) loadingRecords.value = true
  try {
    const resp = await fetch('/api/print-records', { credentials: 'include' })
    if (resp.ok) {
      const data = await resp.json()
      printRecords.value = (data || []).map(r => ({
        id: r.id, filename: r.filename, printerUri: r.printerUri,
        pages: r.pages, status: r.status, isColor: r.isColor,
        isDuplex: r.isDuplex, jobId: r.jobId, createdAt: r.createdAt
      }))
    } else if (resp.status === 401) {
      emit('logout')
    }
  } catch (e) {
    console.error('加载打印记录失败', e)
  } finally {
    loadingRecords.value = false
  }
}

// ─── 打印机状态 ───────────────────────────────────────────
async function loadPrinterInfo(silent = false) {
  if (!printer.value) return
  if (!silent) loadingPrinterInfo.value = true
  printerInfoError.value = ''
  try {
    const resp = await fetch(`/api/printer-info?uri=${encodeURIComponent(printer.value)}`, { credentials: 'include' })
    if (resp.ok) {
      printerInfo.value = await resp.json()
    } else if (resp.status === 401) {
      emit('logout')
    } else {
      const d = await resp.json().catch(() => ({}))
      printerInfoError.value = d.error || '查询失败'
    }
  } catch (_) {
    printerInfoError.value = '无法连接到打印机'
  } finally {
    loadingPrinterInfo.value = false
  }
}

function onPrinterChange() {
  printerInfo.value = null
  printerInfoError.value = ''
  loadPrinterInfo()
}

async function refreshAll() {
  refreshing.value = true
  await Promise.all([loadPrintRecords(true), loadPrinterInfo(true)])
  refreshing.value = false
}

// ─── 定时器 ───────────────────────────────────────────────
let recordsTimer = null
let printerInfoTimer = null

// ─── 生命周期 ─────────────────────────────────────────────
onMounted(async () => {
  try {
    const resp = await fetch('/api/printers', { credentials: 'include' })
    if (resp.ok) {
      printers.value = await resp.json()
      const last = localStorage.getItem('last_printer')
      if (last && printers.value.some(p => p.uri === last)) {
        printer.value = last
      } else if (printers.value.length > 0) {
        printer.value = printers.value[0].uri
      }
      if (printer.value) loadPrinterInfo()
    } else if (resp.status === 401) {
      emit('logout')
    }
  } catch (e) {
    toast.add({ title: '加载打印机失败', description: e.message, color: 'error' })
  }

  await loadPrintRecords()
  recordsTimer = setInterval(() => loadPrintRecords(true), 5000)
  printerInfoTimer = setInterval(() => loadPrinterInfo(true), 15000)
})

onUnmounted(() => {
  clearInterval(recordsTimer)
  clearInterval(printerInfoTimer)
  clearFile()
})
</script>
