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
            <span>点击或拖拽上传文件</span>
          </div>
          <!-- 桌面端：维持原样 -->
          <div class="hidden sm:block">
            <UIcon name="i-lucide-upload-cloud" class="w-8 h-8 sm:w-10 sm:h-10 mx-auto text-muted mb-2" />
            <p class="text-sm text-muted">点击或拖拽文件上传</p>
            <p class="text-xs text-muted mt-1">支持 PDF、Word、Excel、PPT、OFD、图片等格式（可多选图片）</p>
          </div>
        </div>
        <div v-else class="flex items-center gap-2 sm:gap-3 w-full">
          <UIcon name="i-lucide-file-check" class="w-5 h-5 sm:w-8 sm:h-8 text-success shrink-0" />
          <div class="flex-1 min-w-0 text-left">
            <p class="text-sm font-medium break-all line-clamp-2 leading-snug">{{ displayName || (selectedFile && selectedFile.name) }}</p>
            <p v-if="selectedFile" class="text-xs text-muted mt-0.5">{{ formatFileSize(selectedFile.size) }}</p>
            <p v-else-if="isMultiImage && totalSize > 0" class="text-xs text-muted mt-0.5">共 {{ formatFileSize(totalSize) }}</p>
          </div>
          <UButton
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
              ? (isMultiImage ? '正在合并图片…' : '正在转换为 PDF…')
              : (converted ? '已转换为 PDF，可以打印' : '等待转换') }}
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
          >{{ isMultiImage ? '合并' : '转 PDF' }}</UButton>
          <UButton
            v-if="previewUrl"
            variant="ghost"
            size="xs"
            icon="i-lucide-download"
            :href="previewUrl"
            :download="downloadName"
            tag="a"
          >下载</UButton>
        </div>
      </div>
    </div>
  </UCard>
</template>

<script setup>
import { ref } from 'vue'
import { formatFileSize } from '../../utils/format'

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
  totalSize: { type: Number, default: 0 }
})

const emit = defineEmits(['file-selected', 'files-selected', 'clear', 'convert', 'print', 'download'])

const isDragging = ref(false)
const fileInput = ref(null)

function handleFiles(files) {
  if (!files || files.length === 0) return
  if (files.length === 1) {
    emit('file-selected', files[0])
    return
  }
  // 多文件：过滤出图片
  const images = Array.from(files).filter(f => f.type.startsWith('image/'))
  if (images.length === 0) {
    // 无图片，取第一个文件走单文件流程
    emit('file-selected', files[0])
  } else if (images.length === 1) {
    emit('file-selected', images[0])
  } else {
    emit('files-selected', images)
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
