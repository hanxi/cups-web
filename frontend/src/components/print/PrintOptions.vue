<template>
  <UCard>
    <template #header>
      <div class="flex items-center gap-2 font-semibold">
        <UIcon name="i-lucide-settings-2" class="w-5 h-5" />
        打印参数
      </div>
    </template>
    <div class="space-y-4">
      <!-- ═══ 基础选项（始终显示） ═══ -->
      <!-- 颜色 -->
      <UFormField label="颜色模式" :hint="isColor ? undefined : '文档中的彩色内容将以灰阶模式打印输出'">
        <div class="flex rounded-lg border border-muted overflow-hidden">
          <label v-for="item in colorItems" :key="String(item.value)"
            class="flex-1 flex items-center justify-center gap-1.5 py-2 px-2 cursor-pointer text-sm transition"
            :class="isColor === item.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'">
            <input type="radio" :value="item.value" :checked="isColor === item.value" class="sr-only" @change="$emit('update:isColor', item.value)" />
            <UIcon :name="item.icon" class="w-3.5 h-3.5 shrink-0" />
            <span class="text-xs whitespace-nowrap">{{ item.label }}</span>
          </label>
        </div>
      </UFormField>

      <!-- 双面 + 份数 -->
      <div class="grid grid-cols-2 gap-3">
        <UFormField label="双面打印">
          <USelect :model-value="duplex" :items="duplexItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:duplex', $event)" />
        </UFormField>

        <UFormField label="份数">
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
          <span class="font-medium">高级选项</span>
          <span v-if="!showAdvanced" class="text-[11px] sm:text-xs text-muted ml-1 truncate">{{ advancedSummary }}</span>
        </button>

        <div
          class="overflow-hidden transition-all duration-300 ease-in-out"
          :style="{ maxHeight: showAdvanced ? '1000px' : '0px', opacity: showAdvanced ? 1 : 0, visibility: showAdvanced ? 'visible' : 'hidden' }"
        >
          <div class="space-y-4 pt-3">
            <!-- 纸张大小 + 纸张类型 -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <UFormField label="纸张大小">
                <USelect :model-value="paperSize" :items="paperSizeItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:paperSize', $event)" />
              </UFormField>
              <UFormField label="纸张类型">
                <USelect :model-value="paperType" :items="paperTypeItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:paperType', $event)" />
              </UFormField>
            </div>

            <!-- 进纸盒（仅当打印机上报可用纸盒时显示） -->
            <UFormField v-if="mediaSourceItems.length > 1" label="进纸盒" hint="选择从哪个纸盒进纸；「自动」由打印机决定">
              <USelect :model-value="mediaSource" :items="mediaSourceItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:mediaSource', $event)" />
            </UFormField>

            <!-- 缩放 + 页面范围 -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <UFormField label="缩放">
                <USelect :model-value="printScaling" :items="scalingItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:printScaling', $event)" />
              </UFormField>
              <UFormField label="页面范围" :hint="pageRangeError || '如：1-5 8'">
                <UInput
                  :model-value="pageRange"
                  placeholder="留空=全部"
                  class="w-full"
                  :color="pageRangeError ? 'error' : undefined"
                  @update:model-value="onPageRangeInput"
                />
              </UFormField>
            </div>

            <!-- 一张多页（N-up） -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <UFormField label="每张页数" hint="将多页缩排到一张纸上">
                <USelect :model-value="numberUp" :items="numberUpItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:numberUp', Number($event))" />
              </UFormField>
              <UFormField v-if="numberUp > 1" label="页面顺序">
                <USelect :model-value="numberUpLayout" :items="numberUpLayoutItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:numberUpLayout', $event)" />
              </UFormField>
            </div>
            <UFormField v-if="numberUp > 1" label="打印边框">
              <label class="flex items-center gap-2 p-2 border rounded-lg cursor-pointer transition hover:bg-elevated w-fit"
                :class="pageBorder === 'single' ? 'border-primary bg-primary/5' : 'border-muted'">
                <UCheckbox :model-value="pageBorder === 'single'" @update:model-value="$emit('update:pageBorder', $event ? 'single' : 'none')" />
                <UIcon name="i-lucide-square" class="w-4 h-4" />
                <span class="text-sm">为每个小页添加边框</span>
              </label>
            </UFormField>

            <!-- 页面子集（手动双面 / 分册排版） -->
            <UFormField label="页面子集" hint="配合页面范围使用；手动双面可先打奇数页，翻面后再打偶数页">
              <div class="flex rounded-lg border border-muted overflow-hidden">
                <label
                  v-for="item in pageSetItems"
                  :key="item.value"
                  class="flex-1 flex items-center justify-center gap-1.5 py-2 px-2 cursor-pointer text-sm transition"
                  :class="pageSet === item.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'"
                >
                  <input type="radio" :value="item.value" :checked="pageSet === item.value" class="sr-only" @change="$emit('update:pageSet', item.value)" />
                  <UIcon :name="item.icon" class="w-3.5 h-3.5 shrink-0" />
                  <span class="text-xs whitespace-nowrap">{{ item.label }}</span>
                </label>
              </div>
            </UFormField>

            <!-- 镜像打印 -->
            <UFormField label="镜像打印">
              <label class="flex items-center gap-2 p-2 border rounded-lg cursor-pointer transition hover:bg-elevated w-fit"
                :class="mirror ? 'border-primary bg-primary/5' : 'border-muted'">
                <UCheckbox :model-value="mirror" @update:model-value="$emit('update:mirror', $event)" />
                <UIcon name="i-lucide-flip-horizontal" class="w-4 h-4" />
                <span class="text-sm">水平镜像翻转</span>
              </label>
            </UFormField>

            <!-- 水印文字 -->
            <UFormField label="水印文字" hint="留空=不添加水印；如：仅供XX使用">
              <UInput
                :model-value="watermarkText"
                placeholder="留空=不添加水印"
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
import { ref, computed, watch } from 'vue'

const props = defineProps({
  isColor: { type: Boolean, default: true },
  duplex: { type: String, default: 'one-sided' },
  copies: { type: Number, default: 1 },
  paperSize: { type: String, default: 'A4' },
  paperType: { type: String, default: 'plain' },
  mediaSource: { type: String, default: 'auto' },
  mediaSourceSupported: { type: Array, default: () => [] },
  printScaling: { type: String, default: 'fit' },
  pageRange: { type: String, default: '' },
  pageSet: { type: String, default: 'all' },
  mirror: { type: Boolean, default: false },
  watermarkText: { type: String, default: '' },
  numberUp: { type: Number, default: 1 },
  numberUpLayout: { type: String, default: 'lrtb' },
  pageBorder: { type: String, default: 'none' },
  printing: { type: Boolean, default: false }
})

const emit = defineEmits([
  'update:isColor', 'update:duplex', 'update:copies',
  'update:paperSize', 'update:paperType', 'update:mediaSource', 'update:printScaling', 'update:pageRange',
  'update:pageSet', 'update:mirror', 'update:watermarkText',
  'update:numberUp', 'update:numberUpLayout', 'update:pageBorder'
])

const showAdvanced = ref(localStorage.getItem('print_options_expanded') === '1')
watch(showAdvanced, (val) => { localStorage.setItem('print_options_expanded', val ? '1' : '0') })
const pageRangeError = ref('')

// IPP media-source keyword → 中文名映射。不同打印机上报的纸盒关键字差异很大，
// 未命中的关键字（如 tray-3）会走 mediaSourceLabel 的通用规则或原样显示。
const mediaSourceNames = {
  'auto': '自动选择',
  'auto-select': '自动选择',
  'main': '主纸盒',
  'alternate': '备用纸盒',
  'large-capacity': '大容量纸盒',
  'manual': '手动进纸',
  'bypass': '旁路进纸',
  'by-pass-tray': '旁路进纸',
  'multipurpose': '多功能纸盒',
  'envelope': '信封纸盒',
  'top': '上纸盒',
  'middle': '中纸盒',
  'bottom': '下纸盒',
  'left': '左纸盒',
  'right': '右纸盒',
  'center': '中央纸盒',
  'rear': '后纸盒',
  'side': '侧纸盒',
  'photo': '照片纸盒',
  'hagaki': '明信片纸盒',
  'disc': '光盘托盘'
}

function mediaSourceLabel(key) {
  if (mediaSourceNames[key]) return mediaSourceNames[key]
  // tray-1 / tray-2 ... → 纸盒 1 / 纸盒 2
  const m = /^tray-?(\d+)$/i.exec(key)
  if (m) return `纸盒 ${m[1]}`
  return key
}

// 供 USelect 使用的纸盒选项：始终含「自动选择」，其余来自打印机上报的 media-source-supported。
const mediaSourceItems = computed(() => {
  const items = [{ label: '自动选择', value: 'auto' }]
  for (const key of props.mediaSourceSupported) {
    if (key === 'auto' || key === 'auto-select') continue
    items.push({ label: mediaSourceLabel(key), value: key })
  }
  return items
})

const advancedSummary = computed(() => {
  const sizeLabel = paperSizeItems.find(i => i.value === props.paperSize)?.label?.split(' ')[0] || props.paperSize
  const typeLabel = paperTypeItems.find(i => i.value === props.paperType)?.label || props.paperType
  const scaleLabel = scalingItems.find(i => i.value === props.printScaling)?.label || props.printScaling
  const parts = [sizeLabel, typeLabel, scaleLabel]
  if (props.mediaSource && props.mediaSource !== 'auto') parts.push(mediaSourceLabel(props.mediaSource))
  if (props.pageRange) parts.push(`页码: ${props.pageRange}`)
  const pageSetLabel = pageSetItems.find(i => i.value === props.pageSet)?.label
  if (props.pageSet && props.pageSet !== 'all' && pageSetLabel) parts.push(pageSetLabel)
  if (props.numberUp > 1) {
    parts.push(`${props.numberUp} 页/张`)
    if (props.pageBorder === 'single') parts.push('带边框')
  }
  if (props.mirror) parts.push('镜像')
  if (props.watermarkText) parts.push(`水印: ${props.watermarkText}`)
  return parts.join(' / ')
})

const colorItems = [
  { label: '彩色打印', value: true, icon: 'i-lucide-palette' },
  { label: '黑白打印', value: false, icon: 'i-lucide-contrast' }
]

const duplexItems = [
  { label: '单面打印', value: 'one-sided' },
  { label: '双面（长边翻页）', value: 'two-sided-long-edge' },
  { label: '双面（短边翻页）', value: 'two-sided-short-edge' }
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

const paperTypeItems = [
  { label: '普通纸', value: 'plain' },
  { label: '照片纸', value: 'photo' },
  { label: '光面照片纸', value: 'glossy' },
  { label: '哑光照片纸', value: 'matte' },
  { label: '信封', value: 'envelope' },
  { label: '卡片纸', value: 'cardstock' },
  { label: '标签纸', value: 'labels' },
  { label: '自动选择', value: 'auto' }
]

const scalingItems = [
  { label: '自动', value: 'auto' },
  { label: '自动适应', value: 'auto-fit' },
  { label: '适应纸张', value: 'fit' },
  { label: '填充纸张', value: 'fill' },
  { label: '无缩放', value: 'none' }
]

const pageSetItems = [
  { label: '全部页', value: 'all', icon: 'i-lucide-copy' },
  { label: '奇数页', value: 'odd', icon: 'i-lucide-list-ordered' },
  { label: '偶数页', value: 'even', icon: 'i-lucide-list-ordered' },
  { label: '偶数页(倒序)', value: 'even-reverse', icon: 'i-lucide-arrow-down-up' }
]

const numberUpItems = [
  { label: '1 页/张（不缩排）', value: 1 },
  { label: '2 页/张', value: 2 },
  { label: '4 页/张', value: 4 },
  { label: '6 页/张', value: 6 },
  { label: '9 页/张', value: 9 },
  { label: '16 页/张', value: 16 }
]

const numberUpLayoutItems = [
  { label: '横向 Z 形（左→右，上→下）', value: 'lrtb' },
  { label: '横向 Z 形（右→左，上→下）', value: 'rltb' },
  { label: '纵向 N 形（上→下，左→右）', value: 'tblr' },
  { label: '纵向 N 形（上→下，右→左）', value: 'tbrl' }
]

function onPageRangeInput(val) {
  emit('update:pageRange', val)
  validatePageRange(val)
}

function validatePageRange(val) {
  if (typeof val !== 'string') val = ''
  val = val.trim()
  if (!val) { pageRangeError.value = ''; return }

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
  pageRangeError.value = pattern.test(val) ? '' : '格式无效，例如：1-5 8 10-12'
}
</script>
