<template>
  <UCard>
    <template #header>
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2 font-semibold">
          <UIcon name="i-lucide-settings-2" class="w-5 h-5" /> {{ t('ui.print_settings') }} </div>
        <!-- 打印页放预置入口（issue #128）；重打对话框复用本组件时不提供 -->
        <slot name="header-actions" />
      </div>
    </template>
    <div class="space-y-4">
      <!-- ═══ 基础选项（始终显示） ═══ -->
      <!-- 颜色 -->
      <UFormField :label="t('ui.colour_mode')" :hint="isColor ? undefined : t('ui.colour_content_in_the_document_will_be_printed_in_greyscale')">
        <div class="flex rounded-lg border border-muted overflow-hidden">
          <label v-for="item in colorItems" :key="String(item.value)"
            class="flex-1 flex items-center justify-center gap-1.5 py-2 px-2 cursor-pointer text-sm transition"
            :class="isColor === item.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'">
            <input type="radio" :value="item.value" :checked="isColor === item.value" class="sr-only" @change="$emit('update:isColor', item.value)" />
            <UIcon :name="item.icon" class="w-3.5 h-3.5 shrink-0" />
            <span class="text-xs text-center">{{ item.label }}</span>
          </label>
        </div>
      </UFormField>

      <!-- 黑白反转（仅图片 + 黑白模式时显示，issue #87） -->
      <UFormField v-if="invertVisible" :label="t('ui.invert_black_and_white')" :hint="t('ui.convert_white_text_on_a_black_background_to_normal_ordinary')">
        <label class="flex items-center gap-2 p-2 border rounded-lg cursor-pointer transition hover:bg-elevated w-fit"
          :class="invert ? 'border-primary bg-primary/5' : 'border-muted'">
          <UCheckbox :model-value="invert" @update:model-value="$emit('update:invert', $event)" />
          <UIcon name="i-lucide-contrast" class="w-4 h-4" />
          <span class="text-sm">{{ t('ui.invert_colours_negative') }}</span>
        </label>
      </UFormField>

      <UFormField v-if="imageCompressionVisible" :label="t('ui.image_compression')">
        <UCheckbox :model-value="compressImages" :disabled="imageCompressionDisabled" :label="t('ui.enable_image_compression')" :description="t('ui.when_off_original_pixels_are_preserved_when_on_each_image')" @update:model-value="$emit('update:compressImages', $event)" />
      </UFormField>

      <!-- 双面 + 份数 -->
      <div class="grid grid-cols-2 gap-3">
        <UFormField :label="t('ui.double_sided_printing')">
          <USelect :model-value="duplex" :items="duplexItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:duplex', $event)" />
        </UFormField>

        <UFormField :label="t('ui.copies')">
          <UInput
            :model-value="copies"
            type="number"
            :min="1"
            :max="99"
            class="w-full"
            @update:model-value="$emit('update:copies', Number($event))"
          />
        </UFormField>
      </div>

      <!-- ═══ 高级选项折叠区 ═══ -->
      <div class="border-t border-default pt-2">
        <button
          type="button"
          class="flex items-center gap-1.5 w-full text-xs sm:text-sm text-primary hover:text-primary/80 transition cursor-pointer py-1"
          @click="showAdvanced = !showAdvanced"
        >
          <UIcon
            name="i-lucide-chevron-right"
            class="w-3.5 h-3.5 transition-transform duration-200 shrink-0"
            :class="showAdvanced ? 'rotate-90' : ''"
          />
          <span class="font-medium">{{ t('ui.advanced_options') }}</span>
          <span v-if="!showAdvanced" class="text-[11px] sm:text-xs text-muted ml-1 truncate">{{ advancedSummary }}</span>
        </button>

        <div
          v-show="showAdvanced"
        >
          <div class="space-y-4 pt-3">
            <PrinterMediaOptions :printer-uri="printerUri" :driver-options="driverOptions" :rematch-key="driverRematchKey"
              :paper-size="paperSize" :paper-type="paperType" :paper-size-items="paperSizeItems" :paper-type-items="paperTypeItems"
              @update:driver-options="$emit('update:driverOptions', $event)"
              @update:paper-size="$emit('update:paperSize', $event)" @update:paper-type="$emit('update:paperType', $event)" />

            <!-- 进纸盒（仅当打印机上报可用纸盒时显示） -->
            <UFormField v-if="mediaSourceItems.length > 1" :label="t('ui.paper_tray')" :hint="t('ui.choose_a_paper_tray_automatic_lets_the_printer_decide')">
              <USelect :model-value="mediaSource" :items="mediaSourceItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:mediaSource', $event)" />
            </UFormField>

            <!-- 缩放 + 页面范围 -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <UFormField :label="t('ui.scaling')">
                <div class="flex gap-2">
                  <USelect :model-value="printScaling" :items="scalingItems" value-key="value" label-key="label" :class="printScaling === 'custom' ? 'w-28' : 'w-full'" @update:model-value="$emit('update:printScaling', $event)" />
                  <div v-if="printScaling === 'custom'" class="flex items-center gap-1 flex-1">
                    <UInput
                      type="number"
                      :model-value="scalePercent"
                      :min="10"
                      :max="400"
                      :step="5"
                      class="w-full"
                      @update:model-value="onScalePercentInput"
                      @blur="onScalePercentBlur"
                    />
                    <span class="text-sm text-gray-500 shrink-0">%</span>
                  </div>
                </div>
              </UFormField>
              <UFormField :label="t('ui.page_range')" :hint="pageRangeError || t('ui.example_1_5_8')">
                <UInput
                  :model-value="pageRange"
                  :placeholder="t('ui.leave_blank_for_all_pages')"
                  class="w-full"
                  :color="pageRangeError ? 'error' : undefined"
                  @update:model-value="onPageRangeInput"
                />
              </UFormField>
            </div>

            <!-- 一张多页（N-up） -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <UFormField :label="t('ui.pages_per_sheet')" :hint="t('ui.reduce_multiple_pages_to_fit_on_one_sheet')">
                <USelect :model-value="numberUp" :items="numberUpItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:numberUp', Number($event))" />
              </UFormField>
              <UFormField v-if="numberUp > 1" :label="t('ui.page_order')">
                <USelect :model-value="numberUpLayout" :items="numberUpLayoutItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:numberUpLayout', $event)" />
              </UFormField>
            </div>
            <UFormField v-if="numberUp > 1" :label="t('ui.print_borders')">
              <label class="flex items-center gap-2 p-2 border rounded-lg cursor-pointer transition hover:bg-elevated w-fit"
                :class="pageBorder === 'single' ? 'border-primary bg-primary/5' : 'border-muted'">
                <UCheckbox :model-value="pageBorder === 'single'" @update:model-value="$emit('update:pageBorder', $event ? 'single' : 'none')" />
                <UIcon name="i-lucide-square" class="w-4 h-4" />
                <span class="text-sm">{{ t('ui.add_a_border_around_each_reduced_page') }}</span>
              </label>
            </UFormField>

            <!-- 页面子集（手动双面 / 分册排版） -->
            <UFormField :label="t('ui.page_subset')" :hint="pageSetHint">
              <div class="grid grid-cols-2 sm:grid-cols-4 rounded-lg border border-muted overflow-hidden">
                <label
                  v-for="item in pageSetItems"
                  :key="item.value"
                  class="min-w-0 flex items-center justify-center gap-1.5 py-2 px-2 cursor-pointer text-sm transition"
                  :class="pageSet === item.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'"
                >
                  <input type="radio" :value="item.value" :checked="pageSet === item.value" class="sr-only" @change="onPageSetChange(item.value)" />
                  <UIcon :name="item.icon" class="w-3.5 h-3.5 shrink-0" />
                  <span class="text-xs text-center">{{ item.label }}</span>
                </label>
              </div>
            </UFormField>

            <!-- 镜像打印 -->
            <UFormField :label="t('ui.mirror_printing')">
              <label class="flex items-center gap-2 p-2 border rounded-lg cursor-pointer transition hover:bg-elevated w-fit"
                :class="mirror ? 'border-primary bg-primary/5' : 'border-muted'">
                <UCheckbox :model-value="mirror" @update:model-value="$emit('update:mirror', $event)" />
                <UIcon name="i-lucide-flip-horizontal" class="w-4 h-4" />
                <span class="text-sm">{{ t('ui.flip_horizontally') }}</span>
              </label>
            </UFormField>

            <UFormField v-if="photoDateVisible" :label="t('ui.photo_date')" :help="t('ui.uses_the_exif_capture_date_skipping_photos_without_a_valid')">
              <USwitch :model-value="photoDateWatermark" :disabled="photoDateDisabled || printing" :label="t('ui.print_photo_date')" @update:model-value="$emit('update:photoDateWatermark', $event)" />
            </UFormField>
            <!-- 水印文字 -->
            <UFormField :label="t('ui.watermark_text')" :hint="t('ui.leave_blank_for_no_watermark_for_example_for_internal_use')">
              <UInput
                :model-value="watermarkText"
                :placeholder="t('ui.leave_blank_for_no_watermark_2')"
                class="w-full"
                @update:model-value="$emit('update:watermarkText', $event)"
              />
            </UFormField>
          </div>
        </div>
      </div>

    </div>
  </UCard>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import PrinterMediaOptions from './PrinterMediaOptions.vue'
import { ref, computed, watch } from 'vue'
import { readPreference, writePreference } from '../../locale-preference.js'
import { printerChoiceLabel } from '../../utils/printer-options'

const { t } = useI18n()

const props = defineProps({
  printerUri: { type: String, default: '' },
  driverOptions: { type: Object, default: () => ({}) },
  isColor: { type: Boolean, default: true },
  invert: { type: Boolean, default: false },
  invertVisible: { type: Boolean, default: false },
  compressImages: { type: Boolean, default: false },
  imageCompressionVisible: { type: Boolean, default: false },
  imageCompressionDisabled: { type: Boolean, default: false },
  duplex: { type: String, default: 'one-sided' },
  copies: { type: Number, default: 1 },
  paperSize: { type: String, default: 'A4' },
  paperType: { type: String, default: 'plain' },
  mediaSource: { type: String, default: 'auto' },
  mediaSourceSupported: { type: Array, default: () => [] },
  printScaling: { type: String, default: 'auto' },
  scalePercent: { type: Number, default: 100 },
  pageRange: { type: String, default: '' },
  pageSet: { type: String, default: 'all' },
  mirror: { type: Boolean, default: false },
  photoDateWatermark: { type: Boolean, default: false },
  photoDateVisible: { type: Boolean, default: false },
  photoDateDisabled: { type: Boolean, default: false },
  watermarkText: { type: String, default: '' },
  numberUp: { type: Number, default: 1 },
  numberUpLayout: { type: String, default: 'lrtb' },
  pageBorder: { type: String, default: 'none' },
  printing: { type: Boolean, default: false },
  driverRematchKey: { type: Number, default: 0 }
})

const emit = defineEmits([
  'update:driverOptions',
  'update:isColor', 'update:invert', 'update:compressImages',
  'update:duplex', 'update:copies',
  'update:paperSize', 'update:paperType', 'update:mediaSource', 'update:printScaling', 'update:scalePercent', 'update:pageRange',
  'update:photoDateWatermark', 'update:pageSet', 'update:mirror', 'update:watermarkText',
  'update:numberUp', 'update:numberUpLayout', 'update:pageBorder'
])

const showAdvanced = ref(readPreference('print_options_expanded') === '1')
watch(showAdvanced, (val) => { writePreference('print_options_expanded', val ? '1' : '0') })
const pageRangeInvalid = ref(false)
const pageRangeError = computed(() => pageRangeInvalid.value ? t('ui.invalid_format_example_1_5_8_10_12') : '')

// IPP media-source keyword → 中文名映射。不同打印机上报的纸盒关键字差异很大，
// 未命中的关键字（如 tray-3）会走 mediaSourceLabel 的通用规则或原样显示。
const mediaSourceNames = computed(() => ({
  'auto': t('ui.automatic_selection'),
  'auto-select': t('ui.automatic_selection'),
  'main': t('ui.main_tray'),
  'alternate': t('ui.alternate_tray'),
  'large-capacity': t('ui.high_capacity_tray'),
  'manual': t('ui.manual_feed'),
  'bypass': t('ui.bypass_tray'),
  'by-pass-tray': t('ui.bypass_tray'),
  'multipurpose': t('ui.multipurpose_tray'),
  'envelope': t('ui.envelope_tray'),
  'top': t('ui.top_tray'),
  'middle': t('ui.middle_tray'),
  'bottom': t('ui.bottom_tray'),
  'left': t('ui.left_tray'),
  'right': t('ui.right_tray'),
  'center': t('ui.centre_tray'),
  'rear': t('ui.rear_tray'),
  'side': t('ui.side_tray'),
  'photo': t('ui.photo_tray'),
  'hagaki': t('ui.postcard_tray'),
  'disc': t('ui.disc_tray')
}))

function mediaSourceLabel(key) {
  if (mediaSourceNames.value[key]) return mediaSourceNames.value[key]
  // tray-1 / tray-2 ... → 纸盒 1 / 纸盒 2
  const m = /^tray-?(\d+)$/i.exec(key)
  if (m) return t('ui.tray', { p0: m[1] })
  return key
}

// 供 USelect 使用的纸盒选项：始终含「自动选择」，其余来自打印机上报的 media-source-supported。
const mediaSourceItems = computed(() => {
  const items = [{ label: t('ui.automatic_selection'), value: 'auto' }]
  for (const key of props.mediaSourceSupported) {
    if (key === 'auto' || key === 'auto-select') continue
    items.push({ label: mediaSourceLabel(key), value: key })
  }
  return items
})

const advancedSummary = computed(() => {
  const sizeLabel = printerChoiceLabel(props.driverOptions.pageSize) || paperSizeItems.value.find(i => i.value === props.paperSize)?.label?.split(' ')[0] || props.paperSize
  const typeLabel = printerChoiceLabel(props.driverOptions.mediaType) || paperTypeItems.value.find(i => i.value === props.paperType)?.label || props.paperType
  const scaleLabel = scalingItems.value.find(i => i.value === props.printScaling)?.label || props.printScaling
  const parts = [sizeLabel, typeLabel, scaleLabel]
  if (props.driverOptions.quality) parts.push(printerChoiceLabel(props.driverOptions.quality))
  if (props.mediaSource && props.mediaSource !== 'auto') parts.push(mediaSourceLabel(props.mediaSource))
  if (props.pageRange) parts.push(t('ui.pages_3', { p0: props.pageRange }))
  const pageSetLabel = pageSetItems.value.find(i => i.value === props.pageSet)?.label
  if (props.pageSet && props.pageSet !== 'all' && pageSetLabel) parts.push(pageSetLabel)
  if (props.numberUp > 1) {
    parts.push(t('ui.pages_per_sheet_2', { p0: props.numberUp }))
    if (props.pageBorder === 'single') parts.push(t('ui.with_borders'))
  }
  if (props.mirror) parts.push(t('ui.mirror'))
  if (props.photoDateWatermark) parts.push(t('ui.photo_date'))
  if (props.watermarkText) parts.push(t('ui.watermark', { p0: props.watermarkText }))
  return parts.join(' / ')
})

const colorItems = computed(() => ([
  { label: t('ui.colour_printing'), value: true, icon: 'i-lucide-palette' },
  { label: t('ui.black_and_white_printing'), value: false, icon: 'i-lucide-contrast' }
]))

const duplexItems = computed(() => ([
  { label: t('ui.single_sided_printing'), value: 'one-sided' },
  { label: t('ui.double_sided_long_edge_binding'), value: 'two-sided-long-edge' },
  { label: t('ui.double_sided_short_edge_binding'), value: 'two-sided-short-edge' }
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

const paperTypeItems = computed(() => ([
  { label: t('ui.plain_paper'), value: 'plain' },
  { label: t('ui.photo_paper'), value: 'photo' },
  { label: t('ui.glossy_photo_paper'), value: 'glossy' },
  { label: t('ui.matt_photo_paper'), value: 'matte' },
  { label: t('ui.envelope'), value: 'envelope' },
  { label: t('ui.card_stock'), value: 'cardstock' },
  { label: t('ui.labels'), value: 'labels' },
  { label: t('ui.automatic_selection'), value: 'auto' }
]))

const scalingItems = computed(() => ([
  { label: t('ui.automatic'), value: 'auto' },
  { label: t('ui.auto_fit'), value: 'auto-fit' },
  { label: t('ui.fit_to_paper'), value: 'fit' },
  { label: t('ui.fill_paper'), value: 'fill' },
  { label: t('ui.no_scaling'), value: 'none' },
  { label: t('ui.custom_2'), value: 'custom' }
]))

const pageSetItems = computed(() => ([
  { label: t('ui.all_pages'), value: 'all', icon: 'i-lucide-copy' },
  { label: t('ui.odd_pages'), value: 'odd', icon: 'i-lucide-list-ordered' },
  { label: t('ui.even_pages'), value: 'even', icon: 'i-lucide-list-ordered' },
  { label: t('ui.even_pages_reverse_order'), value: 'even-reverse', icon: 'i-lucide-arrow-down-up' }
]))

const numberUpItems = computed(() => ([
  { label: t('ui.option_1_page_per_sheet_no_reduction'), value: 1 },
  { label: t('ui.option_2_pages_per_sheet'), value: 2 },
  { label: t('ui.option_4_pages_per_sheet'), value: 4 },
  { label: t('ui.option_6_pages_per_sheet'), value: 6 },
  { label: t('ui.option_9_pages_per_sheet'), value: 9 },
  { label: t('ui.option_16_pages_per_sheet'), value: 16 }
]))

const numberUpLayoutItems = computed(() => ([
  { label: t('ui.across_then_down_left_to_right'), value: 'lrtb' },
  { label: t('ui.across_then_down_right_to_left'), value: 'rltb' },
  { label: t('ui.down_then_across_left_to_right'), value: 'tblr' },
  { label: t('ui.down_then_across_right_to_left'), value: 'tbrl' }
]))

// 输入过程中只夹上限：若这里连下限一起夹，用户想输 40 时刚敲下 "4" 就会被弹成 10，
// 后面再敲 "0" 就变成 100。下限留到 blur 时归一。
function onScalePercentInput(val) {
  const n = Number(val)
  if (!Number.isFinite(n)) return
  emit('update:scalePercent', Math.min(400, Math.max(0, Math.round(n))))
}

function onScalePercentBlur() {
  const n = Number(props.scalePercent)
  const fixed = Number.isFinite(n) ? Math.min(400, Math.max(10, Math.round(n))) : 100
  if (fixed !== props.scalePercent) emit('update:scalePercent', fixed)
}

// 页面子集切换：odd / even 是"手动双面"用法，语义上必须单面输出；如果此时
// 双面仍是 two-sided-*，CUPS pdftopdf 会用空白页填补被过滤掉的另一面，产生
// 间隔空白页（issue #109）。选中奇/偶时联动把 duplex 拉回 one-sided，同时
// 用 hint 告知用户。even-reverse 也一样：它由后端把偶数页 PDF 层重排后再送
// CUPS，链路语义仍是单面输出，不能带双面。
function onPageSetChange(val) {
  emit('update:pageSet', val)
  const manualDuplex = val === 'odd' || val === 'even' || val === 'even-reverse'
  if (manualDuplex && props.duplex !== 'one-sided') {
    emit('update:duplex', 'one-sided')
  }
}

const pageSetHint = computed(() => {
  if (props.pageSet === 'odd' || props.pageSet === 'even' || props.pageSet === 'even-reverse') {
    return t('ui.manual_duplex_switched_to_single_sided_output_print_odd_pages')
  }
  return t('ui.combine_with_a_page_range_for_manual_duplex_print_odd')
})

function onPageRangeInput(val) {
  emit('update:pageRange', val)
  validatePageRange(val)
}

function validatePageRange(val) {
  if (typeof val !== 'string') val = ''
  val = val.trim()
  if (!val) { pageRangeInvalid.value = false; return }

  const normalizedVal = val
    .replace(/[－—–―]/g, '-')
    .replace(/\s*-\s*/g, '-')
    .replace(/[，,]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()

  if (normalizedVal !== val) {
    emit('update:pageRange', normalizedVal)
    val = normalizedVal
  }

  const pattern = /^(\d+(-\d+)?)(\s+\d+(-\d+)?)*$/
  pageRangeInvalid.value = !pattern.test(val)
}
</script>
