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
    <!-- 纸张预览容器 -->
    <div class="flex justify-center items-center py-4 bg-elevated rounded-lg" style="min-height: 300px;">
      <!-- 纸张模拟 -->
      <div :style="adjustedPreviewStyle"
           class="bg-white shadow-lg border border-default overflow-hidden transition-all duration-300 ease-in-out relative">
        <!-- 内容预览嵌入 -->
        <img v-if="previewType === 'image'" :src="previewUrl" class="w-full h-full object-contain" />
        <PdfCanvas v-else-if="previewType === 'pdf'" :src="previewUrl" />
        <div v-else-if="previewType === 'text'" class="p-3 text-[8px] leading-tight overflow-hidden h-full text-gray-700 dark:text-gray-300 whitespace-pre-wrap">
          {{ textPreview?.substring(0, 800) }}
        </div>
        <!-- 空白纸张占位 -->
        <div v-else class="flex items-center justify-center h-full text-muted text-sm">
          {{ paperSizeLabel }}
        </div>
      </div>
    </div>
  </UCard>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
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
onMounted(() => {
  mediaQuery = window.matchMedia('(max-width: 639px)')
  isMobile.value = mediaQuery.matches
  mediaQuery.addEventListener('change', updateMobile)
})
onUnmounted(() => {
  mediaQuery?.removeEventListener('change', updateMobile)
})

const adjustedPreviewStyle = computed(() => {
  if (!props.compact && !isMobile.value) return props.paperPreviewStyle
  if (!props.paperPreviewStyle) return props.paperPreviewStyle
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
</script>
