<template>
  <UCard>
    <template #header>
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2 font-semibold">
          <UIcon name="i-lucide-eye" class="w-5 h-5" />
          预览
        </div>
        <span class="text-sm text-muted">
          {{ paperSizeLabel }} · {{ orientationLabel }} · {{ paperDimText }}
        </span>
      </div>
    </template>
    <!-- 外层加 padding，让白纸比容器略小一圈、四周留出呼吸感；
         内层白纸宽度 = 容器内容区宽度，高度由 aspect-ratio 按纸张真实比例决定 -->
    <div class="bg-elevated rounded-lg p-3 sm:p-4">
      <div :style="adjustedPreviewStyle"
           class="bg-white shadow-lg border border-default overflow-hidden transition-all duration-300 ease-in-out relative mx-auto">
        <!-- 内容预览嵌入 -->
        <img v-if="previewType === 'image'" :src="previewUrl" class="w-full h-full object-contain" />
        <PdfCanvas v-else-if="previewType === 'pdf'" :src="previewUrl" @preview-failed="onPreviewFailed" />
        <div v-else-if="previewType === 'text'" class="p-3 text-[8px] leading-tight overflow-hidden h-full text-gray-700 dark:text-gray-300 whitespace-pre-wrap">
          {{ textPreview?.substring(0, 800) }}
        </div>
        <!-- 空白纸张占位 -->
        <div v-else class="flex items-center justify-center h-full text-muted text-sm">
          {{ paperSizeLabel }}
        </div>
      </div>
    </div>
    <p v-if="pdfPreviewFailed && previewType === 'pdf'" class="mt-2 text-center text-xs text-muted">
      PDF 预览加载失败，不影响打印，可直接点击"开始打印"。
    </p>
  </UCard>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import PdfCanvas from './PdfCanvas.vue'

const props = defineProps({
  selectedFile: { type: [File, null], default: null },
  previewUrl: { type: String, default: '' },
  previewType: { type: String, default: '' },
  textPreview: { type: String, default: '' },
  paperSize: { type: String, default: 'A4' },
  orientation: { type: String, default: 'portrait' },
  paperSizeLabel: { type: String, default: '' },
  orientationLabel: { type: String, default: '' },
  paperDimText: { type: String, default: '' },
  paperPreviewStyle: { type: Object, default: () => ({}) },
  compact: { type: Boolean, default: false }
})

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

// paperPreviewStyle 已由父组件按纸张比例生成（width: 100% + aspect-ratio），
// 直接透传即可：宽度跟容器一致，高度按比例自动。
const adjustedPreviewStyle = computed(() => props.paperPreviewStyle)
// compact / isMobile 参数保留以备将来使用
void isMobile
</script>
