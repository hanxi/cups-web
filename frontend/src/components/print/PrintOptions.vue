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
      <!-- 颜色 + 方向 -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
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

        <UFormField label="打印方向">
          <div class="flex rounded-lg border border-muted overflow-hidden">
            <label v-for="item in orientationItems" :key="item.value"
              class="flex-1 flex items-center justify-center gap-1.5 py-2 px-2 cursor-pointer text-sm transition"
              :class="orientation === item.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'">
              <input type="radio" :value="item.value" :checked="orientation === item.value" class="sr-only" @change="$emit('update:orientation', item.value)" />
              <UIcon :name="item.icon" class="w-3.5 h-3.5 shrink-0" />
              <span class="text-xs whitespace-nowrap">{{ item.label }}</span>
            </label>
          </div>
        </UFormField>
      </div>

      <!-- 双面 + 份数 -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
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
      <div class="border-t border-default pt-3">
        <button
          type="button"
          class="flex items-center gap-2 w-full text-sm text-primary hover:text-primary/80 transition cursor-pointer"
          @click="showAdvanced = !showAdvanced"
        >
          <UIcon
            name="i-lucide-chevron-right"
            class="w-4 h-4 transition-transform duration-200"
            :class="showAdvanced ? 'rotate-90' : ''"
          />
          <span class="font-medium">高级选项</span>
          <span v-if="!showAdvanced" class="text-xs text-muted ml-1 truncate">{{ advancedSummary }}</span>
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

            <!-- 镜像打印 -->
            <UFormField label="镜像打印">
              <label class="flex items-center gap-2 p-2 border rounded-lg cursor-pointer transition hover:bg-elevated w-fit"
                :class="mirror ? 'border-primary bg-primary/5' : 'border-muted'">
                <UCheckbox :model-value="mirror" @update:model-value="$emit('update:mirror', $event)" />
                <UIcon name="i-lucide-flip-horizontal" class="w-4 h-4" />
                <span class="text-sm">水平镜像翻转</span>
              </label>
            </UFormField>
          </div>
        </div>
      </div>

      <!-- 预览 -->
      <template v-if="selectedFile">
        <div class="border-t border-default pt-4">
          <div class="flex items-center justify-between mb-3">
            <div class="flex items-center gap-2 font-semibold">
              <UIcon name="i-lucide-eye" class="w-5 h-5" />
              预览
            </div>
            <span class="text-sm text-muted">
              {{ paperSizeLabel }} · {{ orientationLabel }} · {{ paperDimText }}
            </span>
          </div>
          <div class="flex justify-center items-center py-4 bg-elevated rounded-lg" style="min-height: 200px;">
            <div :style="adjustedPreviewStyle"
                 class="bg-white shadow-lg border border-default overflow-hidden transition-all duration-300 ease-in-out relative">
              <img v-if="previewType === 'image'" :src="previewUrl" class="w-full h-full object-contain" />
              <PdfCanvas v-else-if="previewType === 'pdf'" :src="previewUrl" />
              <div v-else-if="previewType === 'text'" class="p-3 text-[8px] leading-tight overflow-hidden h-full text-gray-700 dark:text-gray-300 whitespace-pre-wrap">
                {{ textPreview?.substring(0, 800) }}
              </div>
              <div v-else class="flex items-center justify-center h-full text-muted text-sm">
                {{ paperSizeLabel }}
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- 打印按钮 -->
      <UButton
        color="primary"
        size="lg"
        class="w-full"
        icon="i-lucide-printer"
        :disabled="!canPrint || printing"
        :loading="printing"
        @click="$emit('print')"
      >
        提交打印
      </UButton>
    </div>
  </UCard>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import PdfCanvas from './PdfCanvas.vue'

const props = defineProps({
  isColor: { type: Boolean, default: true },
  duplex: { type: String, default: 'one-sided' },
  orientation: { type: String, default: 'portrait' },
  copies: { type: Number, default: 1 },
  paperSize: { type: String, default: 'A4' },
  paperType: { type: String, default: 'plain' },
  printScaling: { type: String, default: 'fit' },
  pageRange: { type: String, default: '' },
  mirror: { type: Boolean, default: false },
  printing: { type: Boolean, default: false },
  canPrint: { type: Boolean, default: false },
  selectedFile: { type: [File, null], default: null },
  previewUrl: { type: String, default: '' },
  previewType: { type: String, default: '' },
  textPreview: { type: String, default: '' },
  paperSizeLabel: { type: String, default: '' },
  orientationLabel: { type: String, default: '' },
  paperDimText: { type: String, default: '' },
  paperPreviewStyle: { type: Object, default: () => ({}) }
})

const emit = defineEmits([
  'update:isColor', 'update:duplex', 'update:orientation', 'update:copies',
  'update:paperSize', 'update:paperType', 'update:printScaling', 'update:pageRange',
  'update:mirror', 'print'
])

const showAdvanced = ref(false)
const pageRangeError = ref('')
const isMobile = ref(false)

let mediaQuery = null
function updateMobile(e) { isMobile.value = e.matches }
onMounted(() => {
  mediaQuery = window.matchMedia('(max-width: 639px)')
  isMobile.value = mediaQuery.matches
  mediaQuery.addEventListener('change', updateMobile)
})
onUnmounted(() => {
  mediaQuery?.removeEventListener('change', updateMobile)
})

const advancedSummary = computed(() => {
  const sizeLabel = paperSizeItems.find(i => i.value === props.paperSize)?.label?.split(' ')[0] || props.paperSize
  const typeLabel = paperTypeItems.find(i => i.value === props.paperType)?.label || props.paperType
  const scaleLabel = scalingItems.find(i => i.value === props.printScaling)?.label || props.printScaling
  const parts = [sizeLabel, typeLabel, scaleLabel]
  if (props.pageRange) parts.push(`页码: ${props.pageRange}`)
  if (props.mirror) parts.push('镜像')
  return parts.join(' / ')
})

const adjustedPreviewStyle = computed(() => {
  if (!isMobile.value || !props.paperPreviewStyle) return props.paperPreviewStyle
  const style = { ...props.paperPreviewStyle }
  const w = parseInt(style.width) || 380
  const h = parseInt(style.height) || 480
  const maxW = 280
  const maxH = 280
  const scale = Math.min(maxW / w, maxH / h, 1)
  if (scale < 1) {
    style.width = `${Math.round(w * scale)}px`
    style.height = `${Math.round(h * scale)}px`
  }
  return style
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

const orientationItems = [
  { label: '纵向', value: 'portrait', icon: 'i-lucide-rectangle-vertical' },
  { label: '横向', value: 'landscape', icon: 'i-lucide-rectangle-horizontal' }
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
