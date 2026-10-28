<template>
  <UCard>
    <template #header>
      <div class="flex items-center justify-between cursor-pointer select-none" @click="listExpanded = !listExpanded">
        <div class="flex items-center gap-2 font-semibold">
          <UIcon name="i-lucide-history" class="w-5 h-5" /> {{ t('ui.print_history') }} <!-- 折叠时显示最近一条摘要 -->
          <span v-if="!listExpanded && records.length > 0" class="text-xs font-normal text-muted truncate max-w-48">
            — {{ records[0].filename }} · {{ formatTime(records[0].createdAt) }} · {{ statusText(records[0].status) }}
          </span>
        </div>
        <div class="flex items-center gap-1">
          <UButton :aria-label="t('accessibility.refresh')" variant="ghost" size="xs" icon="i-lucide-refresh-cw" @click.stop="$emit('refresh')" />
          <UIcon
            :name="listExpanded ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'"
            class="w-4 h-4 text-muted transition-transform duration-200"
          />
        </div>
      </div>
    </template>
    <div
      class="transition-all duration-300 ease-in-out overflow-hidden"
      :style="{ maxHeight: listExpanded ? '24rem' : '0px', visibility: listExpanded ? 'visible' : 'hidden' }"
    >
      <div class="space-y-2 max-h-96 overflow-y-auto">
        <div v-if="loading" class="text-center py-4">
          <UIcon name="i-lucide-loader-circle" class="w-5 h-5 animate-spin mx-auto text-muted" />
        </div>
        <div v-else-if="records.length === 0" class="text-center py-6 text-muted text-sm"> {{ t('ui.no_print_history_yet') }} </div>
        <div
          v-for="rec in records"
          :key="rec.id"
          class="border rounded-lg p-3 hover:shadow-sm transition cursor-pointer"
          @click="toggleRecord(rec.id)"
        >
          <div class="flex items-start gap-2">
            <div class="flex-1 min-w-0">
              <p class="text-sm font-medium truncate">{{ rec.filename }}</p>
              <p class="text-xs text-muted mt-0.5">{{ recordPrinterName(rec.printerUri) }} · {{ t('ui.page_count', rec.pages) }}</p>
              <p class="text-xs text-muted">{{ formatTime(rec.createdAt) }}</p>
            </div>
            <UBadge :color="statusColor(rec.status)" variant="subtle" size="xs">
              {{ statusText(rec.status) }}
            </UBadge>
          </div>
          <!-- 展开详情 -->
          <div v-if="expandedRecords.has(rec.id)" class="mt-2 pt-2 border-t">
            <div class="grid grid-cols-2 gap-1 text-xs text-muted">
              <div><span class="font-medium">{{ t('ui.colour_3') }}</span>{{ rec.isColor ? t('ui.colour_2') : t('ui.black_and_white') }}</div>
              <div><span class="font-medium">{{ t('ui.double_sided') }}</span>{{ rec.isDuplex ? t('ui.yes') : t('ui.no') }}</div>
              <div><span class="font-medium">{{ t('ui.pages_4') }}</span>{{ rec.pages }}</div>
              <div v-if="rec.jobId"><span class="font-medium">{{ t('ui.job_id') }}</span>{{ rec.jobId }}</div>
            </div>
            <div class="mt-2 flex flex-wrap gap-2" @click.stop>
              <a v-if="rec.hasRenderedPDF" :href="`/api/print-records/${rec.id}/file?variant=rendered`" class="text-xs text-primary">{{ t('ui.download_print_pdf') }}</a>
              <a v-for="(source, index) in (rec.photoSources || [])" :key="index" :href="`/api/print-records/${rec.id}/file?source=${index}`" class="text-xs text-primary">{{ t('ui.original_photos') }}{{ source.filename }}</a>
            </div>
            <div class="mt-2 flex justify-end">
              <UButton
                size="xs"
                variant="outline"
                icon="i-lucide-printer"
                :loading="reprintingId === rec.id"
                @click.stop="openReprintDialog(rec)"
              >{{ t('ui.print_again') }}</UButton>
            </div>
          </div>
        </div>
      </div>
    </div>

    <UModal :title="t('ui.print_again')" v-model:open="showReprintModal" :ui="{ content: 'max-w-lg' }">
      <template #content>
        <div class="flex flex-col max-h-[85vh]">
          <div class="p-6 pb-3 border-b border-default shrink-0">
            <h3 class="text-lg font-semibold">{{ t('ui.print_again') }}</h3>
            <div class="text-sm text-muted truncate mt-1">{{ t('ui.file_name', { name: reprintRecord?.filename }) }}</div>
          </div>
          <div class="flex-1 overflow-y-auto p-6 space-y-4">
            <div>
              <label class="block text-sm font-medium mb-1">{{ t('ui.printer') }}</label>
              <USelect
                v-model="reprintForm.printer"
                :items="printerSelectItems"
                value-key="value"
                label-key="label"
                :placeholder="t('ui.select_a_printer')"
                class="w-full"
              />
            </div>
            <PrintOptions
              :printer-uri="reprintForm.printer" v-model:driverOptions="reprintForm.driverOptions"
              v-model:isColor="reprintForm.isColor"
              v-model:duplex="reprintForm.duplex"
              v-model:copies="reprintForm.copies"
              v-model:paperSize="reprintForm.paperSize"
              v-model:paperType="reprintForm.paperType"
              v-model:mediaSource="reprintForm.mediaSource"
              :media-source-supported="mediaSourceSupported"
              v-model:printScaling="reprintForm.printScaling"
              v-model:scalePercent="reprintForm.scalePercent"
              v-model:pageRange="reprintForm.pageRange"
              v-model:pageSet="reprintForm.pageSet"
              v-model:mirror="reprintForm.mirror"
              v-model:compressImages="reprintForm.compressImages"
              :image-compression-visible="!!reprintRecord?.photoSources?.length"
              v-model:invert="reprintForm.invert"
              :invert-visible="!!reprintRecord?.photoSources?.length && !reprintForm.isColor"
              v-model:photoDateWatermark="reprintForm.photoDateWatermark"
              :photo-date-visible="!!reprintRecord?.photoSources?.length || /\.(jpe?g|png|gif)$/i.test(reprintRecord?.filename || '')"
              v-model:watermarkText="reprintForm.watermarkText"
              v-model:numberUp="reprintForm.numberUp"
              v-model:numberUpLayout="reprintForm.numberUpLayout"
              v-model:pageBorder="reprintForm.pageBorder"
            />
          </div>
          <div class="flex justify-end gap-2 p-6 pt-3 border-t border-default shrink-0">
            <UButton variant="ghost" @click="showReprintModal = false">{{ t('ui.cancel') }}</UButton>
            <UButton color="primary" :loading="reprintingId != null" @click="submitReprint">{{ t('ui.confirm_printing') }}</UButton>
          </div>
        </div>
      </template>
    </UModal>
  </UCard>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, computed, watch } from 'vue'
import { formatTime, formatPrinterName, printerLabel, printerDescription, statusColor, statusText } from '../../utils/format'
import PrintOptions from './PrintOptions.vue'

const { t } = useI18n()

const props = defineProps({
  records: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  printers: { type: Array, default: () => [] },
  currentPrinter: { type: String, default: '' },
  mediaSourceSupported: { type: Array, default: () => [] }
})

const emit = defineEmits(['refresh', 'reprint'])

const listExpanded = ref(window.innerWidth >= 1024)
const expandedRecords = ref(new Set())

const showReprintModal = ref(false)
const reprintingId = ref(null)
const reprintRecord = ref(null)

// 重打表单字段与 PrintOptions 组件保持完全一致（duplex 为字符串，isColor 为布尔）；
// sides 保留翻页方式，duplex/color 布尔值兼容原有接口。
function defaultReprintForm() {
  return {
    printer: '',
    orientation: 'portrait',
    isColor: true,
    duplex: 'one-sided',
    copies: 1,
    driverOptions: {},
    paperSize: 'A4',
    paperType: 'plain',
    mediaSource: 'auto',
    printScaling: 'auto',
    scalePercent: 100,
    pageRange: '',
    pageSet: 'all',
    mirror: false,
    photoDateWatermark: false,
    compressImages: false,
    invert: false,
    imagesPerPage: 1,
    watermarkText: '',
    numberUp: 1,
    numberUpLayout: 'lrtb',
    pageBorder: 'none'
  }
}
const reprintForm = ref(defaultReprintForm())
watch(() => reprintForm.value.isColor, value => { if (value) reprintForm.value.invert = false })

const printerSelectItems = computed(() =>
  props.printers.map(p => ({
    label: printerLabel(p),
    description: printerDescription(p),
    value: p.uri
  }))
)

// recordPrinterName 用打印机列表把记录里的 URI 反查成「队列名 — 描述」。
// 记录落库的只有 URI，队列已删或列表还没加载时退回从 URI 取队列名。
const printersByUri = computed(() =>
  new Map(props.printers.map(p => [p.uri, p]))
)
function recordPrinterName(uri) {
  const p = printersByUri.value.get(uri)
  return p ? printerLabel(p) : formatPrinterName(uri)
}

function toggleRecord(id) {
  const s = new Set(expandedRecords.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  expandedRecords.value = s
}

function openReprintDialog(rec) {
  reprintRecord.value = rec
  const def = defaultReprintForm()
  // 用记录里持久化的完整参数精确预填第一次的设置；老记录缺字段则回落默认值（Issue #68）。
  reprintForm.value = {
    printer: props.currentPrinter || rec.printerUri,
    orientation: rec.orientation ?? def.orientation,
    isColor: rec.isColor,
    duplex: rec.sides || (rec.isDuplex ? 'two-sided-long-edge' : 'one-sided'),
    copies: rec.copies ?? def.copies,
    driverOptions: !props.currentPrinter || props.currentPrinter === rec.printerUri ? (rec.driverOptions || {}) : {},
    paperSize: rec.paperSize ?? def.paperSize,
    paperType: rec.paperType ?? def.paperType,
    mediaSource: rec.mediaSource ?? def.mediaSource,
    printScaling: /^\d+$/.test(rec.printScaling) ? 'custom' : (rec.printScaling ?? def.printScaling),
    scalePercent: /^\d+$/.test(rec.printScaling) ? Number(rec.printScaling) : def.scalePercent,
    pageRange: rec.pageRange ?? def.pageRange,
    pageSet: rec.pageSet ?? def.pageSet,
    mirror: rec.mirror ?? def.mirror,
    photoDateWatermark: !!rec.photoDateWatermark,
    compressImages: !!rec.compressImages, invert: !!rec.invert, imagesPerPage: rec.imagesPerPage || 1,
    watermarkText: rec.watermarkText ?? def.watermarkText,
    numberUp: rec.numberUp ?? def.numberUp,
    numberUpLayout: rec.numberUpLayout ?? def.numberUpLayout,
    pageBorder: rec.pageBorder ?? def.pageBorder
  }
  showReprintModal.value = true
}

// 自定义缩放提交给后端的是纯数字字符串；输入框聚焦时可能残留区间外的中间值，这里兜底 clamp。
function clampScalePercent(v) {
  const n = Number(v)
  if (!Number.isFinite(n)) return '100'
  return String(Math.min(400, Math.max(10, Math.round(n))))
}

function submitReprint() {
  const rec = reprintRecord.value
  if (!rec) return
  const f = reprintForm.value
  reprintingId.value = rec.id
  showReprintModal.value = false
  emit('reprint', {
    id: rec.id,
    printer: f.printer,
    sides: f.duplex,
    duplex: f.duplex !== 'one-sided',
    color: f.isColor,
    copies: f.copies,
    orientation: f.orientation,
    paperSize: f.paperSize,
    driverOptions: f.driverOptions,
    paperType: f.paperType,
    mediaSource: f.mediaSource,
    printScaling: f.printScaling === 'custom' ? clampScalePercent(f.scalePercent) : f.printScaling,
    pageRange: f.pageRange.trim(),
    pageSet: f.pageSet,
    mirror: f.mirror,
    photoDateWatermark: f.photoDateWatermark,
    compressImages: f.compressImages, invert: f.invert, imagesPerPage: f.imagesPerPage,
    watermarkText: f.watermarkText.trim(),
    numberUp: f.numberUp,
    numberUpLayout: f.numberUpLayout,
    pageBorder: f.pageBorder
  })
}

defineExpose({ clearReprintLoading: () => { reprintingId.value = null } })
</script>
