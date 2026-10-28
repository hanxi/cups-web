<template>
  <div class="p-3 sm:p-4 md:p-6 max-w-7xl mx-auto">
    <!-- 顶部标题栏：桌面端与主体栅格对齐（2 + 3），移动端单行 flex -->
    <div class="mb-3 grid grid-cols-1 lg:grid-cols-5 gap-x-4 gap-y-2">
      <!-- 左：打印标题 + 打印机下拉（桌面 col-span-2，与左栏对齐） -->
      <div class="lg:col-span-2 flex items-center gap-2 sm:gap-3 min-w-0">
        <h1 class="text-lg font-bold flex items-center gap-2 shrink-0">
          <UIcon name="i-lucide-printer" class="w-5 h-5 text-primary" /> {{ t('ui.print') }} </h1>
        <USelect
          :model-value="printer"
          :items="printerItems"
          value-key="value"
          label-key="label"
          :placeholder="t('ui.select_a_printer')"
          icon="i-lucide-printer"
          class="flex-1 min-w-0"
          @update:model-value="onPrinterSelect"
        />
        <!-- 移动端：任务/刷新按钮紧跟下拉（纯图标），桌面端隐藏 -->
        <UButton :aria-label="t('ui.cups_jobs')"
          variant="ghost"
          size="xs"
          icon="i-lucide-list-todo"
          class="shrink-0 lg:hidden"
          @click="openJobsModal"
          :title="t('ui.cups_jobs')"
        />
        <UButton :aria-label="t('accessibility.refresh')"
          variant="ghost"
          size="xs"
          icon="i-lucide-refresh-cw"
          class="shrink-0 lg:hidden"
          @click="refreshAll"
          :loading="refreshing"
        />
      </div>
      <!-- 右：任务列表 + 刷新按钮（桌面 col-span-3，靠右，与右栏对齐；移动端隐藏） -->
      <div class="hidden lg:flex lg:col-span-3 items-center justify-end gap-2">
        <UButton
          variant="ghost"
          size="xs"
          icon="i-lucide-list-todo"
          @click="openJobsModal"
        >{{ t('ui.cups_jobs') }}</UButton>
        <UButton
          variant="ghost"
          size="xs"
          icon="i-lucide-refresh-cw"
          @click="refreshAll"
          :loading="refreshing"
        >{{ t('ui.refresh') }}</UButton>
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
          <span class="text-xs text-center">{{ m.label }}</span>
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
          :display-name="displayName"
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
                <span>{{ t('ui.upload_invoices_multiple_pdf_image_or_ofd_files_allowed') }}</span>
              </div>
            </div>
            <div v-if="invoiceFiles.length > 0" class="space-y-1">
              <div v-for="(f, idx) in invoiceFiles" :key="idx" class="flex items-center gap-2 text-sm px-2 py-1 rounded hover:bg-elevated">
                <UIcon name="i-lucide-file" class="w-4 h-4 text-muted shrink-0" />
                <span class="flex-1 truncate">{{ f.name }}</span>
                <span class="text-xs text-muted shrink-0">{{ formatFileSize(f.size) }}</span>
                <UButton :aria-label="t('accessibility.remove')" variant="ghost" size="xs" color="error" icon="i-lucide-x" class="shrink-0" @click="removeInvoiceFile(idx)" />
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
            >{{ t('ui.merge_file_count', { count: invoiceFiles.length }) }}</UButton>
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
          <span class="text-sm text-muted shrink-0">{{ t('ui.layout') }}</span>
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
        >{{ t('ui.merge_preview_2') }}</UButton>

        <!-- 文字速印(issue #70):粘贴文字直接打,不必先建 .txt 上传 -->
        <UCard v-if="printMode === 'text'" :ui="{ body: 'p-3 sm:p-4' }">
          <div class="space-y-3">
            <UTextarea
              v-model="quickText"
              :rows="10"
              :placeholder="t('ui.paste_the_text_to_print_here')"
              class="w-full"
              :disabled="converting || printing"
            />
            <div class="flex items-center justify-between gap-2 text-xs text-muted">
              <span>{{ quickText.length }} {{ t('ui.characters') }}</span>
              <UButton
                v-if="quickText"
                variant="ghost"
                size="xs"
                icon="i-lucide-x"
                :disabled="converting || printing"
                @click="quickText = ''"
              >{{ t('ui.clear') }}</UButton>
            </div>
            <UButton
              variant="outline"
              size="sm"
              icon="i-lucide-file-text"
              :loading="converting"
              :disabled="!quickText.trim() || converting || printing"
              @click="convertQuickText"
            >{{ t('ui.convert_and_preview') }}</UButton>
          </div>
        </UCard>

        <!-- 多图片列表（仅标准模式） -->
        <UCard v-if="printMode === 'standard' && selectedImages.length > 1">
          <template #header>
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2 font-semibold">
                <UIcon name="i-lucide-images" class="w-5 h-5" /> {{ t('ui.selected_image_count', { count: selectedImages.length }) }}
              </div>
              <UButton variant="ghost" size="xs" color="error" icon="i-lucide-trash-2" @click="clearFile">{{ t('ui.clear_all') }}</UButton>
            </div>
          </template>
          <!-- 每页张数(issue #37):类 WinXP 打印图片向导,把多图排到同一页 -->
          <div class="flex items-center gap-2 mb-2 text-sm">
            <span class="text-muted shrink-0">{{ t('ui.images_per_page') }}</span>
            <div class="flex rounded-lg border border-muted overflow-hidden">
              <label
                v-for="n in [1, 2, 4, 6, 9]"
                :key="n"
                class="px-3 py-1 cursor-pointer text-sm transition"
                :class="imagesPerPage === n ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'"
              >
                <input type="radio" :value="n" :checked="imagesPerPage === n" class="sr-only" @change="setImagesPerPage(n)" />
                {{ n }}
              </label>
            </div>
          </div>
          <div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
            <div v-for="(img, idx) in selectedImages" :key="idx" class="relative group rounded-lg overflow-hidden border border-default">
              <img :alt="img.name" :src="imageThumbnails[idx]" class="w-full h-20 object-cover" :style="imageRotations[idx] ? `transform: rotate(${imageRotations[idx]}deg)` : ''" />
              <div class="absolute top-1 right-1 z-10 flex gap-1">
                <UButton :aria-label="t('ui.rotate_left_90')" variant="solid" size="xs" color="primary" icon="i-lucide-rotate-ccw" :title="t('ui.rotate_left_90')" @click="rotateImage(idx, -90)" />
                <UButton :aria-label="t('ui.rotate_right_90')" variant="solid" size="xs" color="primary" icon="i-lucide-rotate-cw" :title="t('ui.rotate_right_90')" @click="rotateImage(idx, 90)" />
              </div>
              <div class="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                <UButton :aria-label="t('accessibility.remove')" variant="solid" size="xs" color="error" icon="i-lucide-x" @click="removeImage(idx)" />
              </div>
              <p class="text-xs truncate px-1 py-0.5">{{ img.name }}<span v-if="imageRotations[idx]" class="text-muted"> · {{ imageRotations[idx] }}°</span></p>
            </div>
          </div>
        </UCard>

        <!-- 打印参数 -->
        <PrintOptions
          :printer-uri="printer" v-model:driverOptions="driverOptions"
          v-model:isColor="isColor"
          v-model:invert="invert"
          :invert-visible="isInvertible"
          v-model:compressImages="compressImages"
          :image-compression-visible="printMode === 'standard' && (isImageKind || batchFiles.some(isPhotoFile))"
          :image-compression-disabled="converting || printing || batchPrinting"
          v-model:duplex="duplex"
          v-model:copies="copies"
          v-model:paperSize="paperSize"
          v-model:paperType="paperType"
          v-model:mediaSource="mediaSource"
          :media-source-supported="printerInfo?.mediaSourceSupported || []"
          v-model:printScaling="printScaling"
          v-model:scalePercent="scalePercent"
          v-model:pageRange="pageRange"
          v-model:pageSet="pageSet"
          v-model:mirror="mirror"
          v-model:photoDateWatermark="photoDateWatermark"
          :photo-date-visible="printMode === 'standard' && (isImageKind || batchFiles.some(isPhotoFile))"
          :photo-date-disabled="converting || printing || batchPrinting"
          v-model:watermarkText="watermarkText"
          v-model:numberUp="numberUp"
          v-model:numberUpLayout="numberUpLayout"
          v-model:pageBorder="pageBorder"
          :printing="printing"
          :driver-rematch-key="driverRematchKey"
        >
          <template #header-actions>
            <PrintPresets v-if="printMode === 'standard'" :can-manage="canManagePresets" :snapshot="currentPrintSettings"
              @apply="applyPrintSettings" @reset="resetPrintSettings" @logout="emit('logout')" />
          </template>
        </PrintOptions>

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
            :text-preview="previewText"
            :paper-size-label="paperSizeLabel"
            v-model:orientation="orientation"
            :orientation-label="orientationLabel"
            :paper-dim-text="paperDimText"
            :paper-preview-style="paperPreviewStyle"
            :watermark-text="watermarkText"
            :print-scaling="printScaling"
            :scale-percent="scalePercent"
            :borderless="!!driverOptions.pageSize?.borderless"
          />
        </div>
        <PrintRecordList ref="recordListRef" :records="printRecords" :loading="loadingRecords" :printers="printers" :current-printer="printer" :media-source-supported="printerInfo?.mediaSourceSupported || []" @refresh="loadPrintRecords" @reprint="handleReprint" />
        <PrinterStatus :printer-info="printerInfo" :printer-uri="printer" :loading="loadingPrinterInfo" :error="printerInfoError" @refresh="loadPrinterInfo" />
      </div>
    </div>

    <!-- CUPS 任务列表弹窗（issue #60）：查看/取消 CUPS 未完成任务 -->
    <UModal :title="t('ui.cups_jobs')" v-model:open="showJobsModal" :ui="{ content: 'max-w-3xl' }">
      <template #content>
        <div class="p-4 sm:p-6 space-y-3">
          <div class="flex items-center justify-between gap-2">
            <h3 class="text-lg font-semibold flex items-center gap-2">
              <UIcon name="i-lucide-list-todo" class="w-5 h-5 text-primary" /> {{ t('ui.cups_jobs') }} </h3>
            <div class="flex items-center gap-2">
              <UButton
                variant="ghost"
                size="xs"
                icon="i-lucide-refresh-cw"
                :loading="loadingJobs"
                @click="loadJobs(true)"
              >{{ t('ui.refresh') }}</UButton>
              <UButton :aria-label="t('accessibility.close')" variant="ghost" size="xs" icon="i-lucide-x" @click="showJobsModal = false" />
            </div>
          </div>
          <p class="text-xs text-muted"> {{ t('ui.shows_all_unfinished_cups_jobs_and_refreshes_every_5_seconds') }} </p>

          <div v-if="jobsError" class="text-sm text-error break-all">
            {{ translateError(jobsError) }}
          </div>

          <div v-if="loadingJobs && !cupsJobs.length" class="py-8 text-center text-sm text-muted"> {{ t('ui.loading') }} </div>
          <div v-else-if="!cupsJobs.length" class="py-8 text-center text-sm text-muted"> {{ t('ui.no_unfinished_jobs') }} </div>
          <div v-else class="overflow-x-auto -mx-2 sm:mx-0">
            <table class="w-full text-sm">
              <thead>
                <tr class="text-left text-xs text-muted border-b border-muted">
                  <th class="py-2 px-2">ID</th>
                  <th class="py-2 px-2">{{ t('ui.printer') }}</th>
                  <th class="py-2 px-2">{{ t('ui.file') }}</th>
                  <th class="py-2 px-2">{{ t('ui.user') }}</th>
                  <th class="py-2 px-2">{{ t('ui.status') }}</th>
                  <th class="py-2 px-2 text-right">{{ t('ui.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="j in cupsJobs" :key="j.id" class="border-b border-muted/50">
                  <td class="py-2 px-2 font-mono">{{ j.id }}</td>
                  <td class="py-2 px-2 truncate max-w-[8rem]" :title="j.printerName">{{ j.printerName || '-' }}</td>
                  <td class="py-2 px-2 truncate max-w-[10rem]" :title="j.name">{{ j.name || '-' }}</td>
                  <td class="py-2 px-2 truncate max-w-[6rem]" :title="j.user">{{ j.user || '-' }}</td>
                  <td class="py-2 px-2">
                    <span
                      class="inline-block px-1.5 py-0.5 rounded text-xs"
                      :class="jobStateClass(j)"
                    >{{ jobStateText(j) }}</span>
                    <div v-if="j.stateReasons?.length" class="text-[10px] text-muted mt-0.5 break-all">
                      {{ j.stateReasons.join(', ') }}
                    </div>
                    <div v-if="j.sheetsCompleted > 0" class="text-[10px] text-muted mt-0.5"> {{ t('ui.printed') }} {{ j.sheetsCompleted }} {{ t('ui.pages_2') }} </div>
                  </td>
                  <td class="py-2 px-2 text-right">
                    <UButton
                      size="xs"
                      color="error"
                      variant="soft"
                      :loading="cancelingJobId === j.id"
                      @click="cancelJob(j)"
                    >{{ t('ui.cancel') }}</UButton>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { readPreference, writePreference } from '../locale-preference.js'
import { apiFetch, readError } from '../utils/api'
import { translateError } from '../utils/translate-error.js'
import { isOfficeFile, isOFDFile } from '../utils/file'
import { appendPhotoUploads, isPhotoFile } from '../utils/photo-upload'
import FileUpload from '../components/print/FileUpload.vue'
import IdCardUpload from '../components/print/IdCardUpload.vue'
import PrintPreview from '../components/print/PrintPreview.vue'
import { paperDimensions, printerChoiceLabel } from '../utils/printer-options'
import PrintOptions from '../components/print/PrintOptions.vue'
import PrintPresets from '../components/print/PrintPresets.vue'
import { DEFAULT_PRINT_SETTINGS, sanitizePrintSettings, snapshotPrintSettings, readLastPrintSettings, writeLastPrintSettings } from '../utils/print-settings.js'
import PrintRecordList from '../components/print/PrintRecordList.vue'
import PrinterStatus from '../components/print/PrinterStatus.vue'
import { formatFileSize, printerLabel, printerDescription, jobStateText } from '../utils/format'

const { t } = useI18n()

const props = defineProps({ session: { type: Object, default: null } })
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
const previewText = computed(() => typeof textPreview.value === 'string' ? textPreview.value : t(textPreview.value.key, textPreview.value.params))
const converting = ref(false)
const converted = ref(false)
const pdfBlob = ref(null)
const downloadName = ref('')
const selectedImages = ref([])
// 多图每页张数(issue #37):1=每张一页(默认,历史行为);2/4/6/9=同一页网格排版。
// 前端把这个值透给 /api/convert 的 per_page,后端 convertImagesMultiToPDF 会算网格。
const imagesPerPage = ref(1)
const imageThumbnails = ref([])
const imageRotations = ref([])
const fileDisplayName = ref('')
const displayName = computed(() => typeof fileDisplayName.value === 'string' ? fileDisplayName.value : t(fileDisplayName.value.key, fileDisplayName.value.params))
// 是否对当前 PDF 应用了后端 gs 规范化（仅 PDF 上传后通过 UI 按钮显式触发）
const gsApplying = ref(false)
const gsApplied = ref(false)

// ─── 批量打印 ────────────────────────────────────────────
const batchFiles = ref([])
const batchPrinting = ref(false)
const batchProgress = ref({ current: 0, total: 0 })

// ─── 打印参数 ─────────────────────────────────────────────
const isColor = ref(true)
// 黑白反转（issue #87）：仅图片转 PDF、黑白模式下生效
const invert = ref(false)
const compressImages = ref(false)
let imageConversionVersion = 0
const duplex = ref('one-sided')
const orientation = ref('portrait')
const copies = ref(1)
const driverOptions = ref({})
const paperSize = ref('A4')
const paperType = ref('plain')
const mediaSource = ref('auto')
// 默认走 IPP 的 `auto`：文档不缩放、图片自适应，最贴近 CUPS 原生行为。
// 早先默认 `fit` 会把 PDF 内容拉伸至打印机纸张（尤其上传 PDF 尺寸 ≠ 目标
// paperSize 时），字体变大、多出一页（issue #47）。用户需要撑满时仍可手选 fit。
const printScaling = ref('auto')
const scalePercent = ref(100)
const pageRange = ref('')
const pageSet = ref('all')
const mirror = ref(false)
const photoDateWatermark = ref(false)
const watermarkText = ref('')
const numberUp = ref(1)
const numberUpLayout = ref('lrtb')
const pageBorder = ref('none')

// ─── 打印参数记忆与预置（issue #128） ─────────────────────
// 「上次设置」存 localStorage（跟随设备，guest 也可用），命名预置存服务端。
const settingRefs = {
  isColor, invert, compressImages, duplex, orientation, copies, paperSize, paperType, mediaSource,
  printScaling, scalePercent, pageSet, mirror, photoDateWatermark, watermarkText, numberUp, numberUpLayout, pageBorder
}
// guest 为多人共享账号，预置会互相覆盖，只保留「恢复默认」。
const canManagePresets = computed(() => !!props.session && props.session.username !== 'guest')
// 递增后 PrinterMediaOptions 按当前纸张重新匹配本打印机的驱动选项。
const driverRematchKey = ref(0)

function currentPrintSettings(options) {
  const values = Object.fromEntries(Object.entries(settingRefs).map(([key, r]) => [key, r.value]))
  return snapshotPrintSettings({ ...values, printerUri: printer.value, driverOptions: driverOptions.value }, options)
}

// 缺失或非法字段一律回到出厂默认，保证套用后表单是一组完整、合法的参数。
function applyPrintSettings(raw) {
  const settings = { ...DEFAULT_PRINT_SETTINGS, ...sanitizePrintSettings(raw) }
  for (const [key, r] of Object.entries(settingRefs)) r.value = settings[key]
  const trays = printerInfo.value?.mediaSourceSupported
  if (settings.mediaSource !== 'auto' && trays && !trays.includes(settings.mediaSource)) mediaSource.value = 'auto'
  // 驱动选项的 attribute/value 只对保存时那台打印机有效，换打印机时按纸张重新匹配。
  if (settings.printerUri && settings.printerUri === printer.value) {
    driverOptions.value = settings.driverOptions
  } else {
    driverOptions.value = {}
    driverRematchKey.value++
  }
}

function resetPrintSettings() {
  pageRange.value = ''
  applyPrintSettings({})
}

function rememberPrintSettings() {
  if (printMode.value === 'standard') writeLastPrintSettings(currentPrintSettings({ includeWatermark: false }))
}

// ─── 打印模式 ─────────────────────────────────────────────
const printMode = ref(readPreference('print_mode') || 'standard')
const printModeItems = computed(() => ([
  { label: t('ui.standard_printing'), value: 'standard', icon: 'i-lucide-file-text' },
  { label: t('ui.quick_text_printing'), value: 'text', icon: 'i-lucide-clipboard-type' },
  { label: t('ui.invoice_printing'), value: 'invoice', icon: 'i-lucide-receipt' },
  { label: t('ui.identity_card_printing'), value: 'id_card', icon: 'i-lucide-id-card' }
]))
// 文字速印(issue #70):用户直接粘贴一段文字就能打,省去先建 .txt 再上传的步骤。
// 内容用 File 包装喂给现有 processFile 流程,后端已支持 fileKindText。
const quickText = ref('')
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

// ─── CUPS 任务(issue #60) ────────────────────────────────
// 打开一个 UModal 展示 CUPS 队列里所有未完成任务;每 5s 轮询一次(仅在 modal 开着时)。
// 允许用户直接取消卡在 processing/stopped 的任务 —— 这是 #60 的核心诉求。
const showJobsModal = ref(false)
const cupsJobs = ref([])
const loadingJobs = ref(false)
const jobsError = ref('')
const cancelingJobId = ref(0)
let jobsTimer = null

async function loadJobs(showLoading = false) {
  if (showLoading) loadingJobs.value = true
  jobsError.value = ''
  try {
    const resp = await apiFetch('/api/cups-jobs', {}, () => emit('logout'))
    if (resp.ok) {
      cupsJobs.value = await resp.json() || []
    } else if (resp.status !== 401) {
      jobsError.value = await readError(resp)
    }
  } catch (e) {
    jobsError.value = e?.message || t('ui.could_not_load')
  } finally {
    loadingJobs.value = false
  }
}

function openJobsModal() {
  showJobsModal.value = true
  loadJobs(true)
}

// 打开/关闭 modal 时启停 5s 轮询;避免关闭后仍空转拉后端。
watch(showJobsModal, (open) => {
  if (jobsTimer) { clearInterval(jobsTimer); jobsTimer = null }
  if (open) {
    jobsTimer = setInterval(() => loadJobs(false), 5000)
  }
})

async function cancelJob(job) {
  if (!job?.id || cancelingJobId.value) return
  cancelingJobId.value = job.id
  try {
    const resp = await apiFetch('/api/cups-jobs/cancel', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ printerUri: job.printerUri, jobId: job.id }),
    }, () => emit('logout'))
    if (resp.ok) {
      toast.add({ title: t('ui.cancelled_job', { p0: job.id }), color: 'success' })
      await loadJobs(false)
    } else if (resp.status !== 401) {
      const msg = await readError(resp)
      toast.add({ title: t('ui.could_not_cancel'), description: msg, color: 'error' })
    }
  } catch (e) {
    toast.add({ title: t('ui.could_not_cancel'), description: e.message, color: 'error' })
  } finally {
    cancelingJobId.value = 0
  }
}

function jobStateClass(j) {
  // stopped / 带 reason 的通常是"卡住"的信号,用红/黄突出。
  if (j.state === 6) return 'bg-error/10 text-error'
  if (j.stateReasons?.length) return 'bg-warning/10 text-warning'
  if (j.state === 5) return 'bg-primary/10 text-primary'
  return 'bg-muted text-muted'
}

// ─── 打印记录 ─────────────────────────────────────────────
const printRecords = ref([])
const loadingRecords = ref(false)
const recordListRef = ref(null)

// ─── 打印机状态 ───────────────────────────────────────────
const printerInfo = ref(null)
const loadingPrinterInfo = ref(false)
const printerInfoError = ref('')

// ─── 选项列表（供 PrintOptions 内部的 paperSizeLabel 等计算使用） ──
const orientationItems = computed(() => ([
  { label: t('ui.portrait'), value: 'portrait' },
  { label: t('ui.landscape'), value: 'landscape' }
]))
const paperSizeItems = computed(() => ([
  { label: 'A5 (148×210mm)', value: 'A5' },
  { label: 'A4 (210×297mm)', value: 'A4' },
  { label: 'A3 (297×420mm)', value: 'A3' },
  { label: 'A2 (420×594mm)', value: 'A2' },
  { label: 'A1 (594×841mm)', value: 'A1' },
  { label: t('ui.option_3_5_5_in_89_127mm'), value: '5inch' },
  { label: t('ui.option_4_6_in_102_152mm'), value: '6inch' },
  { label: t('ui.option_5_7_in_127_178mm'), value: '7inch' },
  { label: t('ui.option_6_8_in_152_203mm'), value: '8inch' },
  { label: t('ui.option_8_10_in_203_254mm'), value: '10inch' },
  { label: 'Letter (8.5×11in)', value: 'Letter' },
  { label: 'Legal (8.5×14in)', value: 'Legal' }
]))

// ─── 计算属性 ─────────────────────────────────────────────
const isMultiImage = computed(() => selectedImages.value.length > 1)
const multiImageTotalSize = computed(() => selectedImages.value.reduce((sum, f) => sum + f.size, 0))
// 当前是否为图片场景（单图图片或多图合一），决定「黑白反转」是否可用
const isImageKind = computed(() =>
  isMultiImage.value ||
  isPhotoFile(selectedFile.value)
)
const isTextKind = computed(() => selectedFile.value && (selectedFile.value.type.startsWith('text/') || /\.(txt|md|html)$/i.test(selectedFile.value.name)))
// 黑白反转仅对图片转 PDF 且黑白模式有效
const isInvertible = computed(() => isImageKind.value && !isColor.value)
// 切换到非「图片+黑白」场景时重置反转，避免彩色时残留反色
watch(isInvertible, (v) => { if (!v) invert.value = false })
watch([compressImages, photoDateWatermark], () => {
  imageConversionVersion++
  if (!isImageKind.value) return
  pdfBlob.value = null
  converted.value = false
  gsApplied.value = false
  // HEIC 解码未完成时保留加载提示，等待原有解码流程更新预览。
  const previewFile = isMultiImage.value
    ? selectedImages.value.find(f => !isHeicImage(f))
    : (isHeicImage(selectedFile.value) ? null : selectedFile.value)
  if (!previewFile) return
  clearPreviewUrl()
  previewUrl.value = URL.createObjectURL(previewFile)
  previewType.value = 'image'
}, { flush: 'sync' })
// Only fill changes the photo layout; the other modes share the existing fit PDF.
watch([paperSize, orientation, () => driverOptions.value.pageSize?.borderless, () => printScaling.value === 'fill'], () => {
  if (printMode.value !== 'standard' || (!isImageKind.value && !isTextKind.value)) return
  imageConversionVersion++
  pdfBlob.value = null
  converted.value = false
  gsApplied.value = false
  clearPreviewUrl()
  if (isTextKind.value) { previewType.value = 'text'; return }
  const file = isMultiImage.value ? selectedImages.value.find(f => !isHeicImage(f)) : selectedFile.value
  if (!file || isHeicImage(file)) return
  previewUrl.value = URL.createObjectURL(file)
  previewType.value = 'image'
}, { flush: 'sync' })
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
    return batchFiles.value.length > 0 ? t('ui.batch_print_files', { p0: batchFiles.value.length }) : t('ui.start_printing')
  }
  return t('ui.start_printing')
})

// 打印机下拉选项：
// label 只放队列名 + CUPS 描述，URI/型号挪到 description 那行（issue #101）：
// 选中态的触发按钮只渲染 label，URI 挤在里面会把描述顶出可视区。
const printerItems = computed(() =>
  printers.value.map(p => ({
    label: printerLabel(p),
    description: printerDescription(p),
    value: p.uri
  }))
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
  const item = paperSizeItems.value.find(i => i.value === paperSize.value)
  return printerChoiceLabel(driverOptions.value.pageSize) || item?.label || paperSize.value
})

const orientationLabel = computed(() => {
  const item = orientationItems.value.find(i => i.value === orientation.value)
  return item?.label || (orientation.value === 'portrait' ? t('ui.portrait') : t('ui.landscape'))
})

const previewPaperDimensions = computed(() => {
  const dim = paperDimensions(paperSize.value)
  // Photo direction is applied to portrait dimensions, including driver sizes.
  if (dim && isImageKind.value && dim.width > dim.height) return { width: dim.height, height: dim.width }
  return dim
})
const paperDimText = computed(() => {
  const dim = previewPaperDimensions.value
  if (!dim) return ''
  if (orientation.value === 'landscape') {
    return `${dim.height}×${dim.width}mm`
  }
  return `${dim.width}×${dim.height}mm`
})

// 纸张预览：宽度撑满容器，高度由 aspect-ratio 根据纸张真实比例自动算出。
// 不再限制 maxWidth —— 预览区域宽度始终等于容器宽度，这样"预览区的形状"
// 就能精确同步当前纸张（纵向 / 横向 / 5寸 ... 10寸）的真实比例。
// 提交给后端的 print_scaling：自定义模式下传纯数字百分比。
// 这里再 clamp 一次——输入框在聚焦状态下可能残留区间外的中间值，
// 直接提交会被后端判为非法而退回 100%（见 resolveCustomScaling）。
const printScalingParam = computed(() => {
  if (printScaling.value !== 'custom') return printScaling.value
  const p = Number(scalePercent.value)
  if (!Number.isFinite(p)) return '100'
  return String(Math.round(Math.min(400, Math.max(10, p))))
})

const paperPreviewStyle = computed(() => {
  const dim = previewPaperDimensions.value
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
  // 多图初始预览与缩略图共用地址，由 clearFile/removeImage 负责释放。
  if (previewUrl.value && !imageThumbnails.value.includes(previewUrl.value)) {
    try { URL.revokeObjectURL(previewUrl.value) } catch (_) { /* 忽略 */ }
  }
  previewUrl.value = ''
}

function clearFile() {
  imageConversionVersion++
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
  imageRotations.value = []
  imagesPerPage.value = 1
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
  } else if (isPhotoFile(f)) {
    if (isHeicImage(f)) {
      // HEIC/HEIF 浏览器无法原生解码，先提示"正在转换"，异步用 heic2any 转成 JPEG 再预览
      previewType.value = 'text'
      textPreview.value = { key: 'ui.decoding_heic_heif_image_please_wait', params: {} }
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
          textPreview.value = { key: 'ui.could_not_decode_heic', params: { p0: err.message || t('ui.unknown_error') } }
          toast.add({ title: t('ui.could_not_decode_heic_2'), description: err.message, color: 'error', icon: 'i-lucide-x-circle' })
        })
    } else {
      previewUrl.value = URL.createObjectURL(f)
      previewType.value = 'image'
    }
  } else if (isOfficeFile(f)) {
    previewType.value = 'text'
    textPreview.value = { key: 'ui.office_documents_cannot_be_previewed_directly_select_convert_to_pdf', params: {} }
  } else if (isOFDFile(f)) {
    previewType.value = 'text'
    textPreview.value = { key: 'ui.ofd_documents_cannot_be_previewed_directly_select_convert_to_pdf', params: {} }
  } else if (f.type.startsWith('text/') || /\.(txt|md|html)$/i.test(f.name)) {
    const reader = new FileReader()
    reader.onload = () => {
      textPreview.value = reader.result
      previewType.value = 'text'
    }
    reader.readAsText(f)
  } else {
    previewType.value = 'text'
    textPreview.value = { key: 'ui.this_file_type_cannot_be_previewed_you_can_still_submit', params: {} }
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
    throw new Error(t('ui.could_not_load_the_heic_decoder_check_your_connection_and'))
  }
  try {
    const result = await heic2any({ blob: file, toType: 'image/jpeg', quality: 0.9 })
    // heic2any 多图时可能返回数组，这里只取第一张
    const blob = Array.isArray(result) ? result[0] : result
    // 保留原文件名但改为 .jpg 后缀，便于后续流程展示
    const jpegName = file.name.replace(/\.(heic|heif)$/i, '') + '.jpg'
    return new File([blob], jpegName, { type: 'image/jpeg', lastModified: Date.now() })
  } catch (e) {
    throw new Error(t('ui.could_not_decode_heic_the_file_may_be_damaged_or', { p0: file.name }))
  }
}

// 多图合一逐图旋转（issue #87）：左旋/右旋 90°，归一化到 0/90/180/270。
// 旋转后旧的合并 PDF 失效，清掉 pdfBlob 强制重新转换，避免打印到未旋转的旧 PDF。
function rotateImage(idx, delta) {
  imageConversionVersion++
  const cur = imageRotations.value[idx] || 0
  let next = (cur + delta) % 360
  if (next < 0) next += 360
  imageRotations.value[idx] = next
  pdfBlob.value = null
  converted.value = false
}

// 切换每页张数(issue #37):使上一次的 PDF 失效,让用户再点"合并"重新预览。
function setImagesPerPage(n) {
  imageConversionVersion++
  imagesPerPage.value = n
  pdfBlob.value = null
  converted.value = false
}

function processMultipleImages(files) {
  clearFile()
  const arr = Array.from(files)
  selectedImages.value = arr
  imageRotations.value = arr.map(() => 0)
  fileDisplayName.value = { key: 'ui.images', params: { p0: arr.length } }
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
    textPreview.value = { key: 'ui.decoding_heic_heif_image_please_wait', params: {} }
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
            title: t('ui.could_not_decode_heic', { p0: f.name }),
            description: err.message,
            color: 'error',
            icon: 'i-lucide-x-circle'
          })
        })
    })
  }
}

function removeImage(idx) {
  imageConversionVersion++
  URL.revokeObjectURL(imageThumbnails.value[idx])
  selectedImages.value.splice(idx, 1)
  imageThumbnails.value.splice(idx, 1)
  imageRotations.value.splice(idx, 1)
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
    fileDisplayName.value = { key: 'ui.images', params: { p0: selectedImages.value.length } }
    // 释放独立预览（例如转换后的 PDF），缩略图共用地址由 clearPreviewUrl 保留。
    clearPreviewUrl()
    previewUrl.value = imageThumbnails.value[0]
    converted.value = false
    pdfBlob.value = null
  }
}

function processBatchFiles(files) {
  clearFile()
  batchFiles.value = Array.from(files)
  fileDisplayName.value = { key: 'ui.files_batch_printing', params: { p0: batchFiles.value.length } }
  previewType.value = 'text'
  textPreview.value = { key: 'ui.files_selected_select_start_printing_to_print_them_in_sequence', params: { p0: batchFiles.value.length } }
}

async function uploadAndPrintBatch() {
  if (!printer.value) { toast.add({ title: t('ui.select_a_printer_2'), color: 'warning' }); return }
  if (batchFiles.value.length === 0) return

  batchPrinting.value = true
  batchProgress.value = { current: 0, total: batchFiles.value.length }
  let successCount = 0
  let failCount = 0

  for (let i = 0; i < batchFiles.value.length; i++) {
    batchProgress.value.current = i + 1
    const file = batchFiles.value[i]
    // Keep this file's baked-in photo layout and submitted scaling together,
    // even if the selection changes while its conversion is in flight.
    const scaling = printScalingParam.value
    const dateEnabled = photoDateWatermark.value

    try {
      let fileToSend = file
      // 照片直接提交原图；其余非 PDF 文档先转换。
      if (file.type !== 'application/pdf' && !isPhotoFile(file)) {
        const fd = new FormData()
        fd.append('file', file, file.name)
        fd.append('orientation', orientation.value)
        fd.append('paper_size', paperSize.value)
        fd.append('borderless', String(!!driverOptions.value.pageSize?.borderless))
        fd.append('print_scaling', scaling)
        if (invert.value) fd.append('invert', 'true')
        if (compressImages.value) fd.append('compress_images', 'true')
        const convertResp = await apiFetch('/api/convert', { method: 'POST', body: fd }, () => emit('logout'))
        if (!convertResp.ok) {
          throw new Error(await readError(convertResp))
        }
        const blob = await convertResp.blob()
        fileToSend = new File([blob], file.name.replace(/\.[^/.]+$/, '') + '.pdf', { type: 'application/pdf' })
      }

      const form = new FormData()
      if (isPhotoFile(file)) appendPhotoUploads(form, [file])
      else form.append('file', fileToSend, fileToSend.name)
      form.append('printer', printer.value)
      form.append('sides', duplex.value)
      form.append('duplex', duplex.value === 'one-sided' ? 'false' : 'true')
      form.append('color', isColor.value ? 'true' : 'false')
      form.append('copies', String(copies.value))
      form.append('orientation', orientation.value)
      form.append('paper_size', paperSize.value)
      form.append('paper_type', paperType.value)
      form.append('driver_options', JSON.stringify(driverOptions.value))
      if (mediaSource.value && mediaSource.value !== 'auto') form.append('media_source', mediaSource.value)
      form.append('print_scaling', scaling)
      if (pageRange.value.trim()) form.append('page_range', pageRange.value.trim())
      if (pageSet.value && pageSet.value !== 'all') form.append('page_set', pageSet.value)
      if (mirror.value) form.append('mirror', 'true')
      form.append('compress_images', String(compressImages.value))
      if (invert.value) form.append('invert', 'true')
      form.append('photo_date_watermark', String(dateEnabled))
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
      toast.add({ title: t('ui.printing_failed', { p0: file.name }), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
    }
  }

  if (successCount > 0) {
    toast.add({
      title: t('ui.batch_printing_complete'),
      description: t('ui.succeeded', { p0: successCount, p1: failCount > 0 ? t('ui.batch_failures', { count: failCount }) : '' }),
      color: failCount > 0 ? 'warning' : 'success',
      icon: failCount > 0 ? 'i-lucide-alert-triangle' : 'i-lucide-check-circle'
    })
    writePreference('last_printer', printer.value)
    rememberPrintSettings()
    await loadPrintRecords()
  }
  batchPrinting.value = false
  batchProgress.value = { current: 0, total: 0 }
}

// 通过后端 /api/convert 将单或多张图片合成为单个 PDF。
// - 单图时传 `file` 字段；多图时传多个 `files` 字段，由后端 convertImagesMultiToPDF 合并。
// - HEIC 已由 processFile / processMultipleImages 提前转换为 JPEG，这里无需特殊处理。
// - 原图直接上传，压缩和图片处理统一由后端负责，与打印使用同一条转换链路。
async function convertImagesToPdfViaServer(files, orient, pSize, name, rotations, invert) {
  const list = Array.isArray(files) ? files : [files]
  const compress = compressImages.value
  const borderless = !!driverOptions.value.pageSize?.borderless
  const scaling = printScalingParam.value
  const dateEnabled = photoDateWatermark.value
  const fd = new FormData()
  appendPhotoUploads(fd, list, { rotations, perPage: list.length > 1 ? imagesPerPage.value : 1, name })

  if (dateEnabled) fd.append('photo_date_watermark', 'true')
  if (orient) fd.append('orientation', orient)
  if (pSize) fd.append('paper_size', pSize)
  fd.append('borderless', String(borderless))
  fd.append('print_scaling', scaling)
  if (invert) fd.append('invert', 'true')
  if (compress) fd.append('compress_images', 'true')
  const resp = await apiFetch('/api/convert', { method: 'POST', body: fd }, () => emit('logout'))
  if (!resp.ok) throw new Error(await readError(resp))
  return resp.blob()
}

// 通过后端 /api/convert 将文本文件转成 PDF（使用后端内嵌中文字体，避免 jsPDF 中文乱码）。
async function convertTextViaServer(file, orient, pSize) {
  const fd = new FormData()
  fd.append('file', file, file.name)
  if (orient) fd.append('orientation', orient)
  if (pSize) fd.append('paper_size', pSize)
  const resp = await apiFetch('/api/convert', { method: 'POST', body: fd }, () => emit('logout'))
  if (!resp.ok) throw new Error(await readError(resp))
  return resp.blob()
}

// 通过后端 /api/convert 将 Office / OFD 文件转成 PDF。
async function convertOfficeToPdf(file) {
  const fd = new FormData()
  fd.append('file', file, file.name)
  const resp = await apiFetch('/api/convert', { method: 'POST', body: fd }, () => emit('logout'))
  if (!resp.ok) throw new Error(await readError(resp))
  return resp.blob()
}

async function convertToPdf() {
  if (converting.value) return
  if (!selectedFile.value && !isMultiImage.value) return
  const conversionVersion = imageConversionVersion
  converting.value = true
  try {
    const f = selectedFile.value
    let blob
    if (isMultiImage.value) {
      blob = await convertImagesToPdfViaServer(
        selectedImages.value, orientation.value, paperSize.value, downloadName.value || '合并图片.pdf',
        imageRotations.value, invert.value
      )
    } else if (isOfficeFile(f) || isOFDFile(f)) {
      blob = await convertOfficeToPdf(f)
    } else if (isPhotoFile(f)) {
      blob = await convertImagesToPdfViaServer([f], orientation.value, paperSize.value, null, null, invert.value)
    } else {
      blob = await convertTextViaServer(f, orientation.value, paperSize.value)
    }
    if (conversionVersion !== imageConversionVersion) return
    pdfBlob.value = blob
    clearPreviewUrl()
    previewUrl.value = URL.createObjectURL(blob)
    previewType.value = 'pdf'
    converted.value = true
    toast.add({ title: t('ui.conversion_complete'), color: 'success', icon: 'i-lucide-check-circle' })
  } catch (e) {
    toast.add({ title: t('ui.conversion_failed'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
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
    toast.add({ title: t('ui.ghostscript_normalisation_applied'), color: 'success', icon: 'i-lucide-check-circle' })
  } catch (e) {
    toast.add({ title: t('ui.could_not_apply_ghostscript_normalisation'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    gsApplying.value = false
  }
}

// ─── 打印 ─────────────────────────────────────────────────
async function uploadAndPrint() {
  if (!printer.value) { toast.add({ title: t('ui.select_a_printer_2'), color: 'warning' }); return }

  // 批量打印模式
  if (batchFiles.value.length > 0) {
    await uploadAndPrintBatch()
    return
  }

  // Photos always submit their originals, even after a PDF preview was generated.
  const photos = isImageKind.value
    ? (isMultiImage.value ? [...selectedImages.value] : [selectedFile.value])
    : []
  const actualFile = pdfBlob.value || selectedFile.value
  if (!actualFile && photos.length === 0) { toast.add({ title: t('ui.select_a_file_first'), color: 'warning' }); return }
  const form = new FormData()
  if (photos.length > 0) {
    appendPhotoUploads(form, photos, { rotations: isMultiImage.value ? imageRotations.value : [], perPage: isMultiImage.value ? imagesPerPage.value : 1, name: downloadName.value || '合并照片.pdf' })
    form.append('invert', String(invert.value))
    form.append('compress_images', String(compressImages.value))
  } else {
    form.append('file', actualFile, pdfBlob.value ? (downloadName.value || 'document.pdf') : selectedFile.value.name)
  }
  form.append('printer', printer.value)
  form.append('sides', duplex.value)
  form.append('duplex', duplex.value === 'one-sided' ? 'false' : 'true')
  form.append('color', isColor.value ? 'true' : 'false')
  form.append('copies', String(copies.value))
  form.append('orientation', orientation.value)
  form.append('paper_size', paperSize.value)
  form.append('paper_type', paperType.value)
  form.append('driver_options', JSON.stringify(driverOptions.value))
  if (mediaSource.value && mediaSource.value !== 'auto') form.append('media_source', mediaSource.value)
  form.append('print_scaling', printScalingParam.value)
  if (pageRange.value.trim()) form.append('page_range', pageRange.value.trim())
  if (pageSet.value && pageSet.value !== 'all') form.append('page_set', pageSet.value)
  if (mirror.value) form.append('mirror', 'true')
  form.append('photo_date_watermark', String(photoDateWatermark.value))
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
      title: t('ui.print_job_submitted'),
      description: t('ui.job_id_pages', { p0: j.jobId || '—', p1: j.pages }, j.pages),
      color: 'success',
      icon: 'i-lucide-check-circle'
    })
    writePreference('last_printer', printer.value)
    rememberPrintSettings()
    await loadPrintRecords()
  } catch (e) {
    toast.add({ title: t('ui.printing_failed_2'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
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
      // 重打弹窗需要完整参数快照，包括驱动选项和实际纸张尺寸。
      printRecords.value = data || []
    }
  } catch (e) {
    console.error('加载打印记录失败', e)
  } finally {
    loadingRecords.value = false
  }
}

async function handleReprint(payload) {
  const { id } = payload
  try {
    // 重打对话框已复用 PrintOptions，选项完全由弹窗内的表单决定，直接透传给后端。
    const resp = await apiFetch(`/api/print-records/${id}/reprint`, {
      method: 'POST',
      body: JSON.stringify({
        printer: payload.printer,
        sides: payload.sides,
        duplex: payload.duplex,
        color: payload.color,
        copies: payload.copies,
        orientation: payload.orientation,
        paperSize: payload.paperSize,
        driverOptions: payload.driverOptions,
        paperType: payload.paperType,
        mediaSource: payload.mediaSource,
        printScaling: payload.printScaling,
        pageRange: payload.pageRange,
        pageSet: payload.pageSet,
        mirror: payload.mirror,
        photoDateWatermark: payload.photoDateWatermark,
        compressImages: payload.compressImages, invert: payload.invert, imagesPerPage: payload.imagesPerPage,
        watermarkText: payload.watermarkText,
        numberUp: payload.numberUp,
        numberUpLayout: payload.numberUpLayout,
        pageBorder: payload.pageBorder
      })
    }, () => emit('logout'))
    if (!resp.ok) {
      throw new Error(await readError(resp))
    }
    const j = await resp.json()
    toast.add({
      title: t('ui.reprint_submitted'),
      description: t('ui.pages_job_id', { p0: j.pages, p1: j.jobId || '—' }, j.pages),
      color: 'success',
      icon: 'i-lucide-check-circle'
    })
    await loadPrintRecords()
  } catch (e) {
    toast.add({ title: t('ui.reprint_failed'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
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
      // 若当前所选纸盒不在这台打印机的可用列表里，回退到「自动」
      const trays = printerInfo.value?.mediaSourceSupported || []
      if (mediaSource.value !== 'auto' && !trays.includes(mediaSource.value)) {
        mediaSource.value = 'auto'
      }
    } else if (resp.status !== 401) {
      printerInfoError.value = await readError(resp)
    }
  } catch (_) {
    // Store the app message, not its current translation; PrinterStatus renders it.
    printerInfoError.value = 'cannot connect to the printer'
  } finally {
    loadingPrinterInfo.value = false
  }
}

function onPrinterChange() {
  printerInfo.value = null
  printerInfoError.value = ''
  mediaSource.value = 'auto'
  loadPrinterInfo()
}

// 加载打印机列表(issue #50)。silent=true 时不弹 toast,专供 refreshAll 使用;
// 若当前选中的打印机在最新列表里仍然存在就保留,否则回退到列表第一台。
async function loadPrinters(silent = false) {
  try {
    const resp = await apiFetch('/api/printers', {}, () => emit('logout'))
    if (!resp.ok) {
      if (!silent && resp.status !== 401) {
        toast.add({ title: t('ui.could_not_load_printers'), description: await readError(resp), color: 'error' })
      }
      return
    }
    const list = (await resp.json()) || []
    printers.value = list
    if (list.length === 0) {
      printer.value = ''
      return
    }
    // 首次进入:优先按 localStorage 恢复上次选择;refreshAll 场景下:若当前选中的
    // 打印机仍在列表里就保留(避免刷新时把用户已选的下拉重置)。
    const current = printer.value
    if (current && list.some(p => p.uri === current)) {
      return
    }
    const last = readPreference('last_printer')
    if (last && list.some(p => p.uri === last)) {
      printer.value = last
    } else {
      printer.value = list[0].uri
    }
  } catch (e) {
    if (!silent) {
      toast.add({ title: t('ui.could_not_load_printers'), description: e.message, color: 'error' })
    }
  }
}

async function refreshAll() {
  refreshing.value = true
  // 打印机列表也一并刷新(issue #50):打印机被拔走再插回、或在 CUPS 631 页面
  // 重新添加队列后,用户无需 F5 或退出重进,点一下"刷新"就能重新拉到最新列表。
  // 先拉打印机列表 —— 当前选中的可能已经消失,loadPrinters 会重新选一台,
  // 之后再去查这台的详情/记录才有意义。
  await loadPrinters(true)
  await Promise.all([loadPrintRecords(true), loadPrinterInfo(true)])
  refreshing.value = false
}

// ─── 打印模式 ─────────────────────────────────────────────
function switchMode(mode) {
  if (printMode.value === mode) return
  clearFile()
  clearModeState()
  printMode.value = mode
  writePreference('print_mode', mode)
  if (mode === 'invoice') {
    isColor.value = false
    printScaling.value = 'none'
    scalePercent.value = 100
  } else if (mode === 'id_card') {
    isColor.value = true
    printScaling.value = 'none'
    scalePercent.value = 100
    paperSize.value = 'A4'
    driverOptions.value = { ...driverOptions.value, pageSize: undefined }
  } else {
    applyPrintSettings(readLastPrintSettings())
  }
}

function setIdCardPaper(p) {
  idCardPaper.value = p
  paperSize.value = p
  driverOptions.value = { ...driverOptions.value, pageSize: undefined }
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
  quickText.value = ''
}

// 文字速印(issue #70):把 textarea 里的字符串包成一个 text/plain 的 File,
// 直接喂给现有 processFile → 走标准的 convert→preview→uploadAndPrint 流程,
// 后端的 fileKindText / convertTextToPDF 已经能处理这类输入。
async function convertQuickText() {
  const text = quickText.value
  if (!text.trim()) return
  const filename = `quick-print-${Date.now()}.txt`
  const f = new File([text], filename, { type: 'text/plain' })
  processFile(f)
  // processFile 内部把 selectedFile 设好之后,再触发一次 PDF 转换预览。
  await convertToPdf()
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
    toast.add({ title: t('ui.merge_complete'), color: 'success', icon: 'i-lucide-check-circle' })
  } catch (e) {
    toast.add({ title: t('ui.merge_failed'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    composing.value = false
  }
}

// ─── 定时器 ───────────────────────────────────────────────
let recordsTimer = null
let printerInfoTimer = null

// ─── 生命周期 ─────────────────────────────────────────────
onMounted(async () => {
  // 先套用上次设置避免默认值闪烁；驱动选项要等打印机确定后才能判断是否同一台。
  const last = printMode.value === 'standard' ? readLastPrintSettings() : null
  if (last) applyPrintSettings(last)
  await loadPrinters()
  if (last?.printerUri && last.printerUri === printer.value) driverOptions.value = last.driverOptions
  if (printer.value) loadPrinterInfo()

  await loadPrintRecords()
  recordsTimer = setInterval(() => loadPrintRecords(true), 5000)
  printerInfoTimer = setInterval(() => loadPrinterInfo(true), 15000)
})

onUnmounted(() => {
  clearInterval(recordsTimer)
  clearInterval(printerInfoTimer)
  if (jobsTimer) clearInterval(jobsTimer)
  clearFile()
  clearModeState()
})
</script>
