<template>
  <UCard>
    <template #header>
      <div class="flex items-center justify-between gap-2 flex-wrap">
        <div class="flex items-center gap-2 font-semibold">
          <UIcon name="i-lucide-eye" class="w-5 h-5" />
          预览
          <!-- 纵向/横向快捷切换（取代移动端冗余尺寸文本） -->
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
          {{ paperSizeLabel }}<span class="hidden sm:inline"> · {{ paperDimText }}</span>
        </span>
      </div>
    </template>
    <!-- 外层 bg-elevated 提供浅灰底 + padding，使白纸比容器略小一圈更耐看；
         内层白纸宽度 = 容器内容区宽度，高度由 aspect-ratio 按纸张真实比例自动决定。 -->
    <div
      v-if="selectedFile || isMultiImage"
      class="bg-elevated rounded-lg p-3 sm:p-4"
    >
      <div
        :style="adjustedPreviewStyle"
        class="bg-white shadow-lg border border-default overflow-hidden transition-all duration-300 ease-in-out relative mx-auto"
      >
        <img v-if="previewType === 'image'" :src="previewUrl" class="w-full h-full object-contain" />
        <PdfCanvas v-else-if="previewType === 'pdf'" :src="previewUrl" @preview-failed="onPreviewFailed" />
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
  isMultiImage: { type: Boolean, default: false },
  previewUrl: { type: String, default: '' },
  previewType: { type: String, default: '' },
  textPreview: { type: String, default: '' },
  paperSizeLabel: { type: String, default: '' },
  orientation: { type: String, default: 'portrait' },
  orientationLabel: { type: String, default: '' },
  paperDimText: { type: String, default: '' },
  paperPreviewStyle: { type: Object, default: () => ({}) }
})

defineEmits(['update:orientation'])

const orientationItems = [
  { label: '纵向', value: 'portrait', icon: 'i-lucide-rectangle-vertical' },
  { label: '横向', value: 'landscape', icon: 'i-lucide-rectangle-horizontal' }
]

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
