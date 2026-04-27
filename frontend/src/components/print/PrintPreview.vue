<template>
  <UCard>
    <template #header>
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2 font-semibold">
          <UIcon name="i-lucide-eye" class="w-5 h-5" />
          预览
        </div>
        <span class="text-xs sm:text-sm text-muted truncate">
          {{ paperSizeLabel }} · {{ orientationLabel }} · {{ paperDimText }}
        </span>
      </div>
    </template>
    <div
      v-if="selectedFile || isMultiImage"
      class="flex justify-center items-center py-3 sm:py-4 bg-elevated rounded-lg"
      style="min-height: 180px;"
    >
      <div
        :style="adjustedPreviewStyle"
        class="bg-white shadow-lg border border-default overflow-hidden transition-all duration-300 ease-in-out relative"
      >
        <img v-if="previewType === 'image'" :src="previewUrl" class="w-full h-full object-contain" />
        <PdfCanvas v-else-if="previewType === 'pdf'" :src="previewUrl" />
        <div
          v-else-if="previewType === 'text'"
          class="p-3 text-[8px] leading-tight overflow-hidden h-full text-gray-700 dark:text-gray-300 whitespace-pre-wrap"
        >
          {{ textPreview?.substring(0, 800) }}
        </div>
        <div v-else class="flex items-center justify-center h-full text-muted text-sm">
          {{ paperSizeLabel }}
        </div>
      </div>
    </div>
    <div v-else class="py-6 text-center text-xs text-muted">
      上传文件后显示预览
    </div>
  </UCard>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import PdfCanvas from './PdfCanvas.vue'

const props = defineProps({
  selectedFile: { type: [File, null], default: null },
  isMultiImage: { type: Boolean, default: false },
  previewUrl: { type: String, default: '' },
  previewType: { type: String, default: '' },
  textPreview: { type: String, default: '' },
  paperSizeLabel: { type: String, default: '' },
  orientationLabel: { type: String, default: '' },
  paperDimText: { type: String, default: '' },
  paperPreviewStyle: { type: Object, default: () => ({}) }
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
</script>
