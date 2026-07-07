<template>
  <div class="p-3 sm:p-4 md:p-6 max-w-7xl mx-auto">
    <!-- 顶部标题栏：桌面端与主体栅格对齐（2 + 3），移动端单行 flex -->
    <div class="mb-3 grid grid-cols-1 lg:grid-cols-5 gap-x-4 gap-y-2">
      <!-- 左：打印标题 + 打印机下拉（桌面 col-span-2，与左栏对齐） -->
      <div class="lg:col-span-2 flex items-center gap-2 sm:gap-3 min-w-0">
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
      <!-- 右：刷新按钮（桌面 col-span-3，靠右，与右栏对齐；移动端隐藏） -->
      <div class="hidden lg:flex lg:col-span-3 items-center justify-end">
        <UButton
          variant="ghost"
          size="xs"
          icon="i-lucide-refresh-cw"
          @click="refreshAll"
          :loading="refreshing"
        >刷新</UButton>
      </div>
    </div>

    <!-- 打印模式选择器 -->
    <div class="mb-3">
      <div class="flex rounded-lg border border-muted overflow-hidden">
        <label
          v-for="m in printModeItems"
          :key="m.value"
          class="flex-1 flex items-center justify-center gap-1.5 py-2 px-2 cursor-pointer text-sm transition"
          :class="printMode === m.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'"
        >
          <input type="radio" :value="m.value" :checked="printMode === m.value" class="sr-only" @change="switchMode(m.value)" />
          <UIcon :name="m.icon" class="w-3.5 h-3.5 shrink-0" />
          <span class="text-xs whitespace-nowrap">{{ m.label }}</span>
        </label>
      </div>
    </div>

    <!-- 主体两栏布局：左栏操作区（上传+参数），右栏预览区 -->
    <div class="grid grid-cols-1 lg:grid-cols-5 gap-4">
      <!-- 左栏：上传 + 打印参数 -->
      <div class="lg:col-span-2 space-y-4">
        <!-- 1. 文件上传 — 仅标准模式 -->
        <FileUpload
          v-if="printMode === 'standard'"
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
          :gs-applying="gsApplying"
          :gs-applied="gsApplied"
          @file-selected="processFile"
          @files-selected="processMultipleImages"
          @files-batch-selected="processBatchFiles"
          @clear="clearFile"
          @convert="convertToPdf"
          @apply-gs="applyGsNormalization"
          @print="uploadAndPrint"
        />

        <!-- 发票模式上传 -->
        <UCard v-if="printMode === 'invoice'" :ui="{ body: 'p-3 sm:p-4' }">
          <div class="space-y-3">
            <div
              class="border-2 border-dashed rounded-lg p-2 sm:p-4 text-center cursor-pointer transition-colors"
              :class="invoiceDragging ? 'border-primary bg-primary/5' : 'border-muted hover:border-primary/50'"
              @dragover.prevent="invoiceDragging = true"
              @dragleave="invoiceDragging = false"
              @drop.prevent="onInvoiceDrop"
              @click="invoiceInput.click()"
            >
              <input ref="invoiceInput" type="file" class="hidden" multiple @change="onInvoiceFileChange" />
              <div class="flex items-center justify-center gap-2 text-sm text-muted py-1">
                <UIcon name="i-lucide-receipt" class="w-5 h-5" />
                <span>上传发票文件（支持多选，PDF / 图片 / OFD）</span>
              </div>
            </div>
            <div v-if="invoiceFiles.length > 0" class="space-y-1">
              <div v-for="(f, idx) in invoiceFiles" :key="idx" class="flex items-center gap-2 text-sm px-2 py-1 rounded hover:bg-elevated">
                <UIcon name="i-lucide-file" class="w-4 h-4 text-muted shrink-0" />
                <span class="flex-1 truncate">{{ f.name }}</span>
                <span class="text-xs text-muted shrink-0">{{ formatFileSize(f.size) }}</span>
                <UButton variant="ghost" size="xs" color="error" icon="i-lucide-x" class="shrink-0" @click="removeInvoiceFile(idx)" />
              </div>
            </div>
            <UButton
              v-if="invoiceFiles.length > 0"
              variant="outline"
              size="sm"
              icon="i-lucide-layers"
              :loading="composing"
              :disabled="composing"
              @click="composeAndPreview"
            >合并预览 ({{ invoiceFiles.length }} 个文件)</UButton>
          </div>
        </UCard>

        <!-- 身份证模式上传 -->
        <IdCardUpload
          v-if="printMode === 'id_card'"
          :front="idCardFront"
          :back="idCardBack"
          :front-preview="idCardFrontPreview"
          :back-preview="idCardBackPreview"
          @update:front="onIdCardFront"
          @update:back="onIdCardBack"
        />
        <div v-if="printMode === 'id_card'" class="flex items-center gap-3">
          <span class="text-sm text-muted shrink-0">版面</span>
          <div class="flex rounded-lg border border-muted overflow-hidden">
            <label
              v-for="p in ['A4', 'A5']"
              :key="p"
              class="px-3 py-1 cursor-pointer text-sm transition"
              :class="idCardPaper === p ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'"
            >
              <input type="radio" :value="p" :checked="idCardPaper === p" class="sr-only" @change="setIdCardPaper(p)" />
              {{ p }}
            </label>
          </div>
        </div>
        <UButton
          v-if="printMode === 'id_card' && idCardFront && idCardBack"
          variant="outline"
          size="sm"
          icon="i-lucide-layers"
          :loading="composing"
          :disabled="composing"
          @click="composeAndPreview"
        >合并预览</UButton>

        <!-- 多图片列表（仅标准模式） -->
        <UCard v-if="printMode === 'standard' && selectedImages.length > 1">
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

        <!-- 打印参数 -->
        <PrintOptions
          v-model:isColor="isColor"
          v-model:duplex="duplex"
          v-model:copies="copies"
          v-model:paperSize="paperSize"
          v-model:paperType="paperType"
          v-model:printScaling="printScaling"
          v-model:pageRange="pageRange"
          v-model:pageSet="pageSet"
          v-model:mirror="mirror"
          v-model:watermarkText="watermarkText"
          v-model:numberUp="numberUp"
          v-model:numberUpLayout="numberUpLayout"
          v-model:pageBorder="pageBorder"
          :printing="printing"
        />

        <!-- 开始打印按钮 -->
        <UButton
          color="primary"
          size="xl"
          :ui="{ base: 'justify-center', label: 'flex-1 text-center' }"
          class="w-full font-semibold tracking-wide shadow-lg shadow-primary/25 hover:shadow-xl hover:shadow-primary/35 transition-all ring-1 ring-primary/30"
          icon="i-lucide-send"
          :disabled="!canPrint || printing || batchPrinting"
          :loading="printing || batchPrinting"
          @click="uploadAndPrint"
        >
          {{ printButtonLabel }}
        </UButton>
      </div>

      <!-- 右栏：预览 + 打印记录 + 打印机状态 -->
      <div class="lg:col-span-3 space-y-4">
        <div class="lg:sticky lg:top-4 space-y-4">
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
            :watermark-text="watermarkText"
          />
        </div>
        <PrintRecordList ref="recordListRef" :records="printRecords" :loading="loadingRecords" :printers="printers" :current-printer="printer" @refresh="loadPrintRecords" @reprint="handleReprint" />
        <PrinterStatus :printer-info="printerInfo" :printer-uri="printer" :loading="loadingPrinterInfo" :error="printerInfoError" @refresh="loadPrinterInfo" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { apiFetch, readError } from '../utils/api'
import { isOfficeFile, isOFDFile } from '../utils/file'
import { downscaleImageIfNeeded } from '../utils/image'
import FileUpload from '../components/print/FileUpload.vue'
import IdCardUpload from '../components/print/IdCardUpload.vue'
import PrintPreview from '../components/print/PrintPreview.vue'
import PrintOptions from '../components/print/PrintOptions.vue'
import PrintRecordList from '../components/print/PrintRecordList.vue'
import PrinterStatus from '../components/print/PrinterStatus.vue'
import { formatFileSize } from '../utils/format'

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
// 是否对当前 PDF 应用了后端 gs 规范化（仅 PDF 上传后通过 UI 按钮显式触发）
const gsApplying = ref(false)
const gsApplied = ref(false)

// ─── 批量打印 ────────────────────────────────────────────
const batchFiles = ref([])
const batchPrinting = ref(false)
const batchProgress = ref({ current: 0, total: 0 })

// ─── 打印参数 ─────────────────────────────────────────────
const isColor = ref(true)
const duplex = ref('one-sided')
const orientation = ref('portrait')
const copies = ref(1)
const paperSize = ref('A4')
const paperType = ref('plain')
const printScaling = ref('fit')
const pageRange = ref('')
const pageSet = ref('all')
const mirror = ref(false)
const watermarkText = ref('')
const numberUp = ref(1)
const numberUpLayout = ref('lrtb')
const pageBorder = ref('none')

// ─── 打印模式 ─────────────────────────────────────────────
const printMode = ref(localStorage.getItem('print_mode') || 'standard')
const printModeItems = [
  { label: '标准打印', value: 'standard', icon: 'i-lucide-file-text' },
  { label: '发票打印', value: 'invoice', icon: 'i-lucide-receipt' },
  { label: '身份证打印', value: 'id_card', icon: 'i-lucide-id-card' }
]
const invoiceFiles = ref([])
const invoiceDragging = ref(false)
const invoiceInput = ref(null)
const idCardFront = ref(null)
const idCardBack = ref(null)
const idCardFrontPreview = ref('')
const idCardBackPreview = ref('')
const idCardPaper = ref('A4')
const composing = ref(false)

// ─── 状态 ─────────────────────────────────────────────────
const printing = ref(false)
const refreshing = ref(false)

// ─── 打印记录 ─────────────────────────────────────────────
const printRecords = ref([])
const loadingRecords = ref(false)
const recordListRef = ref(null)

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
const canPrint = computed(() => {
  if (!printer.value) return false
  if (printMode.value === 'standard') {
    return !!pdfBlob.value || !!selectedFile.value || isMultiImage.value || batchFiles.value.length > 0
  }
  // 发票/身份证模式：需要已合并出 pdfBlob
  return !!pdfBlob.value
})

const printButtonLabel = computed(() => {
  if (printMode.value === 'standard') {
    return batchFiles.value.length > 0 ? `批量打印 (${batchFiles.value.length} 个文件)` : '开始打印'
  }
  return '开始打印'
})

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

// 纸张预览：宽度撑满容器，高度由 aspect-ratio 根据纸张真实比例自动算出。
// 不再限制 maxWidth —— 预览区域宽度始终等于容器宽度，这样"预览区的形状"
// 就能精确同步当前纸张（纵向 / 横向 / 5寸 ... 10寸）的真实比例。
const paperPreviewStyle = computed(() => {
  const dim = paperDimensionsMap[paperSize.value]
  if (!dim) return {}
  const isLandscape = orientation.value === 'landscape'
  const width = isLandscape ? dim.height : dim.width
  const height = isLandscape ? dim.width : dim.height
  return {
    aspectRatio: `${width} / ${height}`,
    width: '100%'
  }
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
  gsApplying.value = false
  gsApplied.value = false
  batchFiles.value = []
}

function processFile(f) {
  clearFile()
  selectedFile.value = f
  fileDisplayName.value = ''
  downloadName.value = f.name.replace(/\.[^/.]+$/, '') + '.pdf'

  if (f.type === 'application/pdf') {
    // 默认不再对上传 PDF 走 /api/convert (gs 规范化)：直接用原始字节做预览和打印，
    // 上传即可秒开预览。如需修复 CJK 字体乱码等问题，用户可点击"应用 GS 规范化"
    // 显式触发 applyGsNormalization()，把当前 PDF 替换为 gs 产物。
    clearPreviewUrl()
    previewUrl.value = URL.createObjectURL(f)
    previewType.value = 'pdf'
    textPreview.value = ''
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
    const heicBatch = arr
    arr.forEach((f, idx) => {
      if (!isHeicImage(f)) return
      heicBlobToJpegBlob(f)
        .then(jpegFile => {
          // 若用户在转码期间已切换/清空文件，则丢弃结果
          if (selectedImages.value !== heicBatch) return
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
          if (selectedImages.value !== heicBatch) return
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

function processBatchFiles(files) {
  clearFile()
  batchFiles.value = Array.from(files)
  fileDisplayName.value = `${batchFiles.value.length} 个文件（批量打印）`
  previewType.value = 'text'
  textPreview.value = `已选择 ${batchFiles.value.length} 个文件，点击"开始打印"将逐个打印。`
}

async function uploadAndPrintBatch() {
  if (!printer.value) { toast.add({ title: '请选择打印机', color: 'warning' }); return }
  if (batchFiles.value.length === 0) return

  batchPrinting.value = true
  batchProgress.value = { current: 0, total: batchFiles.value.length }
  let successCount = 0
  let failCount = 0

  for (let i = 0; i < batchFiles.value.length; i++) {
    batchProgress.value.current = i + 1
    const file = batchFiles.value[i]

    try {
      let fileToSend = file
      // 非 PDF 文件需要先转换
      if (file.type !== 'application/pdf') {
        const fd = new FormData()
        fd.append('file', file, file.name)
        fd.append('orientation', orientation.value)
        fd.append('paper_size', paperSize.value)
        const convertResp = await apiFetch('/api/convert', { method: 'POST', body: fd }, () => emit('logout'))
        if (!convertResp.ok) {
          throw new Error(await readError(convertResp))
        }
        const blob = await convertResp.blob()
        fileToSend = new File([blob], file.name.replace(/\.[^/.]+$/, '') + '.pdf', { type: 'application/pdf' })
      }

      const form = new FormData()
      form.append('file', fileToSend, fileToSend.name)
      form.append('printer', printer.value)
      form.append('duplex', duplex.value === 'one-sided' ? 'false' : 'true')
      form.append('color', isColor.value ? 'true' : 'false')
      form.append('copies', String(copies.value))
      form.append('orientation', orientation.value)
      form.append('paper_size', paperSize.value)
      form.append('paper_type', paperType.value)
      form.append('print_scaling', printScaling.value)
      if (pageRange.value.trim()) form.append('page_range', pageRange.value.trim())
      if (pageSet.value && pageSet.value !== 'all') form.append('page_set', pageSet.value)
      if (mirror.value) form.append('mirror', 'true')
      if (watermarkText.value.trim()) form.append('watermark_text', watermarkText.value.trim())
      if (numberUp.value > 1) {
        form.append('number_up', String(numberUp.value))
        form.append('number_up_layout', numberUpLayout.value)
        form.append('page_border', pageBorder.value)
      }

      const resp = await apiFetch('/api/print', { method: 'POST', body: form }, () => emit('logout'))
      if (!resp.ok) throw new Error(await readError(resp))
      successCount++
    } catch (e) {
      failCount++
      toast.add({ title: `打印失败：${file.name}`, description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
    }
  }

  if (successCount > 0) {
    toast.add({
      title: '批量打印完成',
      description: `成功 ${successCount} 个${failCount > 0 ? `，失败 ${failCount} 个` : ''}`,
      color: failCount > 0 ? 'warning' : 'success',
      icon: failCount > 0 ? 'i-lucide-alert-triangle' : 'i-lucide-check-circle'
    })
    localStorage.setItem('last_printer', printer.value)
    await loadPrintRecords()
  }
  batchPrinting.value = false
  batchProgress.value = { current: 0, total: 0 }
}

// 通过后端 /api/convert 将一或多张图片合成为单个 PDF。
// - 单图时传 `file` 字段；多图时传多个 `files` 字段，由后端 convertImagesMultiToPDF 合并。
// - HEIC 已由 processFile / processMultipleImages 提前转换为 JPEG，这里无需特殊处理。
// - 上传前用 downscaleImageIfNeeded 在浏览器端预压缩：长边 >3000px 的大图缩成 JPEG，
//   避免多张原图合并时撞到反向代理的 client_max_body_size 触发 413（Issue #42）。
//   阈值与后端 imageDownscaleMaxEdge 对齐，服务端拿到时已是合理尺寸，无需再 downscale。
//   sequential 而非 Promise.all 是为了避免移动端同时持有多张大 canvas 导致 OOM。
async function convertImagesToPdfViaServer(files, orient, pSize, name) {
  const list = Array.isArray(files) ? files : [files]
  const downscaled = []
  for (const f of list) {
    downscaled.push(await downscaleImageIfNeeded(f))
  }
  const fd = new FormData()
  if (downscaled.length === 1) {
    fd.append('file', downscaled[0], downscaled[0].name)
  } else {
    for (const f of downscaled) fd.append('files', f, f.name)
    if (name) fd.append('name', name)
  }
  if (orient) fd.append('orientation', orient)
  if (pSize) fd.append('paper_size', pSize)
  const resp = await apiFetch('/api/convert', { method: 'POST', body: fd }, () => emit('logout'))
  if (!resp.ok) throw new Error('服务端转换失败：' + await readError(resp))
  return resp.blob()
}

// 通过后端 /api/convert 将文本文件转成 PDF（使用后端内嵌中文字体，避免 jsPDF 中文乱码）。
async function convertTextViaServer(file, orient, pSize) {
  const fd = new FormData()
  fd.append('file', file, file.name)
  if (orient) fd.append('orientation', orient)
  if (pSize) fd.append('paper_size', pSize)
  const resp = await apiFetch('/api/convert', { method: 'POST', body: fd }, () => emit('logout'))
  if (!resp.ok) throw new Error('服务端转换失败：' + await readError(resp))
  return resp.blob()
}

// 通过后端 /api/convert 将 Office / OFD 文件转成 PDF。
async function convertOfficeToPdf(file) {
  const fd = new FormData()
  fd.append('file', file, file.name)
  const resp = await apiFetch('/api/convert', { method: 'POST', body: fd }, () => emit('logout'))
  if (!resp.ok) throw new Error('服务端转换失败：' + await readError(resp))
  return resp.blob()
}

async function convertToPdf() {
  if (!selectedFile.value && !isMultiImage.value) return
  converting.value = true
  try {
    const f = selectedFile.value
    let blob
    if (isMultiImage.value) {
      blob = await convertImagesToPdfViaServer(
        selectedImages.value, orientation.value, paperSize.value, downloadName.value || '合并图片.pdf'
      )
    } else if (isOfficeFile(f) || isOFDFile(f)) {
      blob = await convertOfficeToPdf(f)
    } else if (f.type.startsWith('image/')) {
      blob = await convertImagesToPdfViaServer([f], orientation.value, paperSize.value)
    } else {
      blob = await convertTextViaServer(f, orientation.value, paperSize.value)
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

// 对当前 PDF 显式应用后端 gs 规范化。
// 后端 /api/convert?normalize=true 会把 PDF 重写为 1.4 版本并嵌入所有字体，
// 用于修复 CJK 字体外挂 CMap 导致的乱码等问题。规范化结果替换 pdfBlob 与
// previewUrl，后续打印发的就是这份字节流。
async function applyGsNormalization() {
  const f = selectedFile.value
  if (!f || f.type !== 'application/pdf') return
  if (gsApplying.value || gsApplied.value) return
  gsApplying.value = true
  try {
    const fd = new FormData()
    fd.append('file', f, f.name)
    fd.append('normalize', 'true')
    const resp = await apiFetch('/api/convert', { method: 'POST', body: fd }, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    const blob = await resp.blob()
    // 用户在期间换/清了文件，丢弃结果
    if (selectedFile.value !== f) return
    clearPreviewUrl()
    previewUrl.value = URL.createObjectURL(blob)
    previewType.value = 'pdf'
    textPreview.value = ''
    pdfBlob.value = blob
    gsApplied.value = true
    toast.add({ title: '已应用 GS 规范化', color: 'success', icon: 'i-lucide-check-circle' })
  } catch (e) {
    toast.add({ title: '应用 GS 失败', description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    gsApplying.value = false
  }
}

// ─── 打印 ─────────────────────────────────────────────────
async function uploadAndPrint() {
  if (!printer.value) { toast.add({ title: '请选择打印机', color: 'warning' }); return }

  // 批量打印模式
  if (batchFiles.value.length > 0) {
    await uploadAndPrintBatch()
    return
  }

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
  if (pageSet.value && pageSet.value !== 'all') form.append('page_set', pageSet.value)
  if (mirror.value) form.append('mirror', 'true')
  if (watermarkText.value.trim()) form.append('watermark_text', watermarkText.value.trim())
  if (numberUp.value > 1) {
    form.append('number_up', String(numberUp.value))
    form.append('number_up_layout', numberUpLayout.value)
    form.append('page_border', pageBorder.value)
  }

  printing.value = true
  try {
    const resp = await apiFetch('/api/print', {
      method: 'POST',
      body: form
    }, () => emit('logout'))
    if (!resp.ok) {
      throw new Error(await readError(resp))
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
    const resp = await apiFetch('/api/print-records', {}, () => emit('logout'))
    if (resp.ok) {
      const data = await resp.json()
      printRecords.value = (data || []).map(r => ({
        id: r.id, filename: r.filename, printerUri: r.printerUri,
        pages: r.pages, status: r.status, isColor: r.isColor,
        isDuplex: r.isDuplex, jobId: r.jobId, createdAt: r.createdAt
      }))
    }
  } catch (e) {
    console.error('加载打印记录失败', e)
  } finally {
    loadingRecords.value = false
  }
}

async function handleReprint({ id, printer: reprintPrinter, duplex: reprintDuplex, color: reprintColor, copies: reprintCopies, numberUp: reprintNumberUp, numberUpLayout: reprintNumberUpLayout, pageBorder: reprintPageBorder }) {
  try {
    const resp = await apiFetch(`/api/print-records/${id}/reprint`, {
      method: 'POST',
      body: JSON.stringify({
        printer: reprintPrinter,
        duplex: reprintDuplex,
        color: reprintColor,
        copies: reprintCopies,
        orientation: orientation.value,
        paperSize: paperSize.value,
        paperType: paperType.value,
        printScaling: printScaling.value,
        numberUp: reprintNumberUp,
        numberUpLayout: reprintNumberUpLayout,
        pageBorder: reprintPageBorder
      })
    }, () => emit('logout'))
    if (!resp.ok) {
      throw new Error(await readError(resp))
    }
    const j = await resp.json()
    toast.add({
      title: '重新打印已提交',
      description: `${j.pages} 页，任务ID：${j.jobId || '—'}`,
      color: 'success',
      icon: 'i-lucide-check-circle'
    })
    await loadPrintRecords()
  } catch (e) {
    toast.add({ title: '重新打印失败', description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    recordListRef.value?.clearReprintLoading()
  }
}

// ─── 打印机状态 ───────────────────────────────────────────
async function loadPrinterInfo(silent = false) {
  if (!printer.value) return
  if (!silent) loadingPrinterInfo.value = true
  printerInfoError.value = ''
  try {
    const resp = await apiFetch(
      `/api/printer-info?uri=${encodeURIComponent(printer.value)}`,
      {},
      () => emit('logout')
    )
    if (resp.ok) {
      printerInfo.value = await resp.json()
    } else if (resp.status !== 401) {
      printerInfoError.value = await readError(resp)
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

// ─── 打印模式 ─────────────────────────────────────────────
function switchMode(mode) {
  if (printMode.value === mode) return
  clearFile()
  clearModeState()
  printMode.value = mode
  localStorage.setItem('print_mode', mode)
  if (mode === 'invoice') {
    isColor.value = false
    printScaling.value = 'none'
  } else if (mode === 'id_card') {
    isColor.value = true
    printScaling.value = 'none'
    paperSize.value = 'A4'
  } else {
    isColor.value = true
    printScaling.value = 'fit'
  }
}

function setIdCardPaper(p) {
  idCardPaper.value = p
  paperSize.value = p
}

function clearModeState() {
  invoiceFiles.value = []
  if (idCardFrontPreview.value) { try { URL.revokeObjectURL(idCardFrontPreview.value) } catch (_) {} }
  if (idCardBackPreview.value) { try { URL.revokeObjectURL(idCardBackPreview.value) } catch (_) {} }
  idCardFront.value = null
  idCardBack.value = null
  idCardFrontPreview.value = ''
  idCardBackPreview.value = ''
  idCardPaper.value = 'A4'
  composing.value = false
}

function onInvoiceDrop(e) {
  invoiceDragging.value = false
  addInvoiceFiles(e.dataTransfer.files)
}

function onInvoiceFileChange(e) {
  addInvoiceFiles(e.target.files)
  if (invoiceInput.value) invoiceInput.value.value = ''
}

function addInvoiceFiles(files) {
  if (!files || files.length === 0) return
  invoiceFiles.value = [...invoiceFiles.value, ...Array.from(files)]
  // 清除之前的合并结果
  clearPreviewUrl()
  pdfBlob.value = null
  converted.value = false
  previewType.value = ''
}

function removeInvoiceFile(idx) {
  invoiceFiles.value.splice(idx, 1)
  clearPreviewUrl()
  pdfBlob.value = null
  converted.value = false
  previewType.value = ''
}

function onIdCardFront(file) {
  if (idCardFrontPreview.value) { try { URL.revokeObjectURL(idCardFrontPreview.value) } catch (_) {} }
  idCardFront.value = file
  idCardFrontPreview.value = file ? URL.createObjectURL(file) : ''
  clearPreviewUrl()
  pdfBlob.value = null
  converted.value = false
  previewType.value = ''
}

function onIdCardBack(file) {
  if (idCardBackPreview.value) { try { URL.revokeObjectURL(idCardBackPreview.value) } catch (_) {} }
  idCardBack.value = file
  idCardBackPreview.value = file ? URL.createObjectURL(file) : ''
  clearPreviewUrl()
  pdfBlob.value = null
  converted.value = false
  previewType.value = ''
}

async function composeAndPreview() {
  composing.value = true
  try {
    const fd = new FormData()
    if (printMode.value === 'invoice') {
      fd.append('mode', 'invoice')
      for (const f of invoiceFiles.value) {
        fd.append('files', f, f.name)
      }
    } else if (printMode.value === 'id_card') {
      fd.append('mode', 'id_card')
      fd.append('paper', idCardPaper.value)
      fd.append('files', idCardFront.value, idCardFront.value.name)
      fd.append('files', idCardBack.value, idCardBack.value.name)
    }

    const resp = await apiFetch('/api/compose', { method: 'POST', body: fd }, () => emit('logout'))
    if (!resp.ok) {
      throw new Error(await readError(resp))
    }
    const blob = await resp.blob()
    clearPreviewUrl()
    pdfBlob.value = blob
    previewUrl.value = URL.createObjectURL(blob)
    previewType.value = 'pdf'
    converted.value = true
    downloadName.value = printMode.value === 'invoice' ? '发票合并.pdf' : '身份证.pdf'
    toast.add({ title: '合并成功', color: 'success', icon: 'i-lucide-check-circle' })
  } catch (e) {
    toast.add({ title: '合并失败', description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    composing.value = false
  }
}

// ─── 定时器 ───────────────────────────────────────────────
let recordsTimer = null
let printerInfoTimer = null

// ─── 生命周期 ─────────────────────────────────────────────
onMounted(async () => {
  try {
    const resp = await apiFetch('/api/printers', {}, () => emit('logout'))
    if (resp.ok) {
      printers.value = await resp.json()
      const last = localStorage.getItem('last_printer')
      if (last && printers.value.some(p => p.uri === last)) {
        printer.value = last
      } else if (printers.value.length > 0) {
        printer.value = printers.value[0].uri
      }
      if (printer.value) loadPrinterInfo()
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
  clearModeState()
})
</script>
