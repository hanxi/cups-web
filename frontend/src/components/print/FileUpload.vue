<template>
  <UCard :ui="{ body: 'p-3 sm:p-4' }">
    <div class="space-y-2">
      <!-- 拖拽区 -->
      <div
        class="border-2 border-dashed rounded-lg p-2 sm:p-4 text-center cursor-pointer transition-colors"
        :class="isDragging ? 'border-primary bg-primary/5' : 'border-muted hover:border-primary/50'"
        @dragover.prevent="isDragging = true"
        @dragleave="isDragging = false"
        @drop.prevent="onDrop"
        @click="fileInput.click()"
      >
        <input ref="fileInput" type="file" class="hidden" multiple @change="onFileChange" />
        <div v-if="!selectedFile && !displayName">
          <!-- 移动端：单行紧凑 -->
          <div class="flex sm:hidden items-center justify-center gap-2 text-sm text-muted py-1">
            <UIcon name="i-lucide-upload-cloud" class="w-4 h-4" />
            <span>{{ t('ui.click_or_drag_files_here_to_upload') }}</span>
          </div>
          <!-- 桌面端：维持原样 -->
          <div class="hidden sm:block">
            <UIcon name="i-lucide-upload-cloud" class="w-8 h-8 sm:w-10 sm:h-10 mx-auto text-muted mb-2" />
            <p class="text-sm text-muted">{{ t('ui.click_or_drag_files_to_upload') }}</p>
            <p class="text-xs text-muted mt-1">{{ t('ui.supports_pdf_word_excel_powerpoint_ofd_and_images_multiple_images') }}</p>
          </div>
        </div>
        <div v-else class="flex items-center gap-2 sm:gap-3 w-full">
          <UIcon name="i-lucide-file-check" class="w-5 h-5 sm:w-8 sm:h-8 text-success shrink-0" />
          <div class="flex-1 min-w-0 text-left">
            <p class="text-sm font-medium break-all line-clamp-2 leading-snug">{{ displayName || (selectedFile && selectedFile.name) }}</p>
            <p v-if="selectedFile" class="text-xs text-muted mt-0.5">{{ formatFileSize(selectedFile.size) }}</p>
            <p v-else-if="isMultiImage && totalSize > 0" class="text-xs text-muted mt-0.5">{{ t('ui.total_2') }} {{ formatFileSize(totalSize) }}</p>
          </div>
          <UButton :aria-label="t('accessibility.remove')"
            variant="ghost"
            size="xs"
            icon="i-lucide-x"
            color="error"
            class="shrink-0 !w-6 !h-6 !min-h-0 !p-0"
            @click.stop="$emit('clear')"
          />
        </div>
      </div>

      <!-- 状态 + 操作合并成一行；仅在需要时显示 -->
      <div
        v-if="selectedFile || isMultiImage || converting"
        class="flex items-center justify-between gap-2 text-xs"
      >
        <!-- 左侧：状态文字 -->
        <div class="flex items-center gap-1.5 min-w-0 text-muted">
          <UIcon v-if="converting" name="i-lucide-loader-circle" class="w-3.5 h-3.5 animate-spin text-info shrink-0" />
          <UIcon v-else-if="converted" name="i-lucide-check-circle" class="w-3.5 h-3.5 text-success shrink-0" />
          <UIcon v-else name="i-lucide-clock" class="w-3.5 h-3.5 shrink-0" />
          <span class="truncate">
            {{ converting
              ? (isMultiImage ? t('ui.merging_images') : t('ui.converting_to_pdf'))
              : (converted ? t('ui.converted_to_pdf_and_ready_to_print') : t('ui.awaiting_conversion')) }}
          </span>
        </div>
        <!-- 右侧：按钮 -->
        <div class="flex items-center gap-1 shrink-0">
          <UButton
            v-if="canConvert"
            variant="ghost"
            size="xs"
            icon="i-lucide-file-text"
            :loading="converting"
            @click="$emit('convert')"
          >{{ isMultiImage ? t('ui.merge') : t('ui.convert_to_pdf') }}</UButton>
          <!--
            "应用 GS 规范化"按钮：仅在已上传 PDF 时显示。点击后调用
            /api/convert?normalize=true 走 Ghostscript 重写 PDF（嵌入所有字体、统一为 1.4 版本），
            用于修复 CJK 字体外挂 CMap 导致的乱码等问题。
          -->
          <UButton
            v-if="isPdf"
            variant="ghost"
            size="xs"
            :icon="gsApplied ? 'i-lucide-check-circle' : 'i-lucide-wand-2'"
            :loading="gsApplying"
            :disabled="gsApplied || gsApplying"
            @click="$emit('apply-gs')"
          >{{ gsApplied ? t('ui.normalised') : t('ui.apply_ghostscript_normalisation') }}</UButton>
          <UButton
            v-if="converted && pdfBlob && previewUrl"
            variant="ghost"
            size="xs"
            icon="i-lucide-download"
            :href="previewUrl"
            :download="downloadName"
            tag="a"
          >{{ t('ui.download') }}</UButton>
        </div>
      </div>
    </div>
  </UCard>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, computed } from 'vue'
import { formatFileSize } from '../../utils/format'

const { t } = useI18n()

const props = defineProps({
  selectedFile: { type: [File, null], default: null },
  displayName: { type: String, default: '' },
  converting: { type: Boolean, default: false },
  converted: { type: Boolean, default: false },
  previewUrl: { type: String, default: '' },
  downloadName: { type: String, default: '' },
  pdfBlob: { type: [Blob, null], default: null },
  canConvert: { type: Boolean, default: false },
  canPrint: { type: Boolean, default: false },
  printing: { type: Boolean, default: false },
  isMultiImage: { type: Boolean, default: false },
  totalSize: { type: Number, default: 0 },
  gsApplying: { type: Boolean, default: false },
  gsApplied: { type: Boolean, default: false }
})

const emit = defineEmits(['file-selected', 'files-selected', 'files-batch-selected', 'clear', 'convert', 'apply-gs', 'print', 'download'])

const isPdf = computed(() => props.selectedFile?.type === 'application/pdf')

const isDragging = ref(false)
const fileInput = ref(null)

function handleFiles(files) {
  if (!files || files.length === 0) return
  if (files.length === 1) {
    emit('file-selected', files[0])
    return
  }
  const arr = Array.from(files)
  const images = arr.filter(f => f.type.startsWith('image/') || /\.(heic|heif)$/i.test(f.name))
  if (images.length === arr.length) {
    // 全部是图片：走多图合并流程
    if (images.length === 1) {
      emit('file-selected', images[0])
    } else {
      emit('files-selected', images)
    }
  } else {
    // 混合文件类型：走批量打印流程
    emit('files-batch-selected', arr)
  }
}

function onDrop(e) {
  isDragging.value = false
  handleFiles(e.dataTransfer.files)
}

function onFileChange(e) {
  handleFiles(e.target.files)
  // 重置 input 以便可以重新选择相同文件
  if (fileInput.value) fileInput.value.value = ''
}
</script>
