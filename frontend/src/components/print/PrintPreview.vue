<template>
  <UCard>
    <template #header>
      <div class="flex items-center justify-between gap-2 flex-wrap">
        <div class="flex items-center gap-2 font-semibold">
          <UIcon name="i-lucide-eye" class="w-5 h-5" /> {{ t('ui.preview') }} <!-- 纵向/横向快捷切换（取代移动端冗余尺寸文本） -->
          <div class="flex rounded-md border border-muted overflow-hidden ml-1">
            <button
              v-for="item in orientationItems"
              :key="item.value"
              type="button"
              :title="item.label"
              class="flex items-center gap-1 py-1 px-2 cursor-pointer text-xs transition"
              :class="orientation === item.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'"
              @click="$emit('update:orientation', item.value)"
            >
              <UIcon :name="item.icon" class="w-3.5 h-3.5 shrink-0" />
              <span>{{ item.label }}</span>
            </button>
          </div>
        </div>
        <span class="text-xs sm:text-sm text-muted truncate">
          {{ paperSizeLabel }}<span class="hidden sm:inline"> · {{ paperDimText }}</span><span v-if="contentScale !== 1"> · {{ Math.round(contentScale * 100) }}%</span>
        </span>
      </div>
    </template>
    <!-- 外层 bg-elevated 提供浅灰底 + padding，使白纸比容器略小一圈更耐看；
         内层白纸宽度 = 容器内容区宽度，高度由 aspect-ratio 按纸张真实比例自动决定。 -->
    <div
      v-if="selectedFile || isMultiImage || previewUrl || previewType"
      class="bg-elevated rounded-lg p-3 sm:p-4"
    >
      <div
        :style="adjustedPreviewStyle"
        class="bg-white shadow-lg border border-default overflow-hidden transition-all duration-300 ease-in-out relative mx-auto"
      >
        <div v-if="previewType === 'image'" class="absolute inset-0 transition-transform duration-200" :style="contentScaleStyle">
          <div class="absolute overflow-hidden" :style="imageContentStyle">
            <img :alt="t('accessibility.preview')" :src="previewUrl" class="w-full h-full" :style="{ objectFit: printScaling === 'fill' ? 'cover' : 'contain' }" />
          </div>
        </div>
        <PdfCanvas v-else-if="previewType === 'pdf'" :src="previewUrl" :watermark-text="watermarkText" :content-scale="contentScale" @preview-failed="onPreviewFailed" />
        <div
          v-else-if="previewType === 'text'"
          class="p-3 text-[8px] leading-tight overflow-hidden h-full text-gray-700 dark:text-gray-300 whitespace-pre-wrap transition-transform duration-200"
          :style="contentScaleStyle"
        >
          {{ textPreview?.substring(0, 800) }}
        </div>
        <div v-else class="flex items-center justify-center h-full text-muted text-sm">
          {{ paperSizeLabel }}
        </div>
      </div>
    </div>
    <div v-else class="py-6 text-center text-xs text-muted"> {{ t('ui.upload_a_file_to_see_a_preview') }} </div>
    <p v-if="pdfPreviewFailed && previewType === 'pdf'" class="mt-2 text-center text-xs text-muted"> {{ t('ui.the_pdf_preview_could_not_be_loaded_printing_is_still') }} </p>
    <p v-if="isMultiImage && previewType === 'image'" class="mt-2 text-center text-xs text-muted"> {{ t('ui.showing_the_first_original_photo_merge_to_pdf_to_preview') }} </p>
  </UCard>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import PdfCanvas from './PdfCanvas.vue'

const { t } = useI18n()

const props = defineProps({
  selectedFile: { type: [File, null], default: null },
  isMultiImage: { type: Boolean, default: false },
  previewUrl: { type: String, default: '' },
  previewType: { type: String, default: '' },
  textPreview: { type: String, default: '' },
  paperSizeLabel: { type: String, default: '' },
  orientation: { type: String, default: 'portrait' },
  orientationLabel: { type: String, default: '' },
  paperDimText: { type: String, default: '' },
  paperPreviewStyle: { type: Object, default: () => ({}) },
  watermarkText: { type: String, default: '' },
  printScaling: { type: String, default: 'auto' },
  scalePercent: { type: Number, default: 100 },
  borderless: { type: Boolean, default: false }
})

// 自定义百分比缩放：后端用 gs 把内容按比例缩小并在页面内居中，这里用等比 CSS transform
// 同步预览，用户拖百分比时立刻能看到效果（Issue #98）。
// 图片的 fit/fill 和 10mm/无边距内容区与后端生成 PDF 的排版保持一致；
// PDF 直接显示生成结果，不再对页面做 cover 裁切。
const contentScale = computed(() => {
  if (props.printScaling !== 'custom') return 1
  const p = Number(props.scalePercent)
  if (!Number.isFinite(p) || p <= 0) return 1
  // 输入过程中可能出现区间外的中间值，clamp 一下避免预览抽风
  return Math.min(400, Math.max(10, p)) / 100
})

const contentScaleStyle = computed(() =>
  contentScale.value === 1 ? {} : { transform: `scale(${contentScale.value})` }
)

const imageContentStyle = computed(() => {
  const [width, height] = String(props.paperPreviewStyle.aspectRatio || '').split('/').map(Number)
  if (props.borderless || !(width > 20 && height > 20)) return { inset: '0' }
  return {
    left: `${1000 / width}%`, right: `${1000 / width}%`,
    top: `${1000 / height}%`, bottom: `${1000 / height}%`
  }
})

defineEmits(['update:orientation'])

const orientationItems = computed(() => ([
  { label: t('ui.portrait'), value: 'portrait', icon: 'i-lucide-rectangle-vertical' },
  { label: t('ui.landscape'), value: 'landscape', icon: 'i-lucide-rectangle-horizontal' }
]))

const isMobile = ref(false)
let mediaQuery = null
function updateMobile(e) { isMobile.value = e.matches }

// PDF 预览失败标记：在父组件传入新 previewUrl 时重置
const pdfPreviewFailed = ref(false)
function onPreviewFailed() { pdfPreviewFailed.value = true }
watch(() => props.previewUrl, () => { pdfPreviewFailed.value = false })
onMounted(() => {
  mediaQuery = window.matchMedia('(max-width: 639px)')
  isMobile.value = mediaQuery.matches
  mediaQuery.addEventListener('change', updateMobile)
})
onUnmounted(() => {
  mediaQuery?.removeEventListener('change', updateMobile)
})

// paperPreviewStyle 已由父组件基于纸张真实比例生成（width: 100% + aspect-ratio）；
// 直接透传，不再在组件内做任何尺寸修正，保证预览区宽度 === 容器宽度、高度按比例。
const adjustedPreviewStyle = computed(() => props.paperPreviewStyle)
// isMobile 状态仍保留以备将来使用
void isMobile
</script>
