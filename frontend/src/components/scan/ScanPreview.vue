<template>
  <UCard>
    <template #header>
      <div class="flex items-center justify-between gap-2 flex-wrap">
        <div class="flex items-center gap-2 font-semibold">
          <UIcon name="i-lucide-eye" class="w-5 h-5" />
          扫描预览
        </div>
        <span v-if="scanJob" class="text-xs sm:text-sm text-muted truncate">
          {{ statusText }}
        </span>
      </div>
    </template>

    <div v-if="scanJob" class="space-y-4">
      <!-- 扫描状态 -->
      <div class="flex items-center gap-2">
        <UBadge :color="statusColor" variant="subtle">
          {{ statusText }}
        </UBadge>
        <span v-if="scanJob.status === 'scanning'" class="text-sm text-muted">
          正在扫描中...
        </span>
      </div>

      <!-- 扫描进度 -->
      <div v-if="scanJob.status === 'scanning'" class="space-y-2">
        <UProgress :value="progress" />
        <p class="text-xs text-muted text-center">
          扫描中，请稍候...
        </p>
      </div>

      <!-- 扫描结果预览 -->
      <div v-if="scanJob.status === 'completed' && scanJob.fileUrl" class="bg-elevated rounded-lg p-3 sm:p-4">
        <div class="bg-white shadow-lg border border-default overflow-hidden relative mx-auto">
          <img 
            :src="scanJob.fileUrl" 
            class="w-full h-auto object-contain"
            @load="onImageLoad"
            @error="onImageError"
          />
        </div>
      </div>

      <!-- 扫描信息 -->
      <div v-if="scanJob.status === 'completed'" class="grid grid-cols-2 gap-2 text-sm">
        <div>
          <span class="text-muted">文件名：</span>
          <span>{{ scanJob.filename }}</span>
        </div>
        <div>
          <span class="text-muted">分辨率：</span>
          <span>{{ scanJob.resolution }} DPI</span>
        </div>
        <div>
          <span class="text-muted">颜色模式：</span>
          <span>{{ colorModeText }}</span>
        </div>
        <div>
          <span class="text-muted">纸张大小：</span>
          <span>{{ scanJob.paperSize }}</span>
        </div>
      </div>

      <!-- 错误信息 -->
      <div v-if="scanJob.status === 'failed'" class="text-red-500 text-sm">
        <UIcon name="i-lucide-alert-circle" class="w-4 h-4 inline mr-1" />
        {{ scanJob.errorMessage || '扫描失败' }}
      </div>

      <!-- 操作按钮 -->
      <div v-if="scanJob.status === 'completed'" class="flex gap-2">
        <UButton 
          color="primary" 
          variant="solid" 
          @click="downloadScan"
        >
          <UIcon name="i-lucide-download" class="w-4 h-4 mr-1" />
          下载扫描件
        </UButton>
        <UButton 
          color="neutral" 
          variant="outline" 
          @click="$emit('newScan')"
        >
          <UIcon name="i-lucide-refresh-cw" class="w-4 h-4 mr-1" />
          重新扫描
        </UButton>
      </div>
    </div>

    <div v-else class="py-6 text-center text-xs text-muted">
      点击"开始扫描"后显示预览
    </div>
  </UCard>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'

const props = defineProps({
  scanJob: { type: Object, default: null }
})

defineEmits(['newScan'])

const progress = ref(0)
let progressInterval = null

const statusText = computed(() => {
  if (!props.scanJob) return ''
  switch (props.scanJob.status) {
    case 'pending': return '等待中'
    case 'scanning': return '扫描中'
    case 'completed': return '扫描完成'
    case 'failed': return '扫描失败'
    default: return props.scanJob.status
  }
})

const statusColor = computed(() => {
  if (!props.scanJob) return 'neutral'
  switch (props.scanJob.status) {
    case 'pending': return 'neutral'
    case 'scanning': return 'primary'
    case 'completed': return 'success'
    case 'failed': return 'error'
    default: return 'neutral'
  }
})

const colorModeText = computed(() => {
  if (!props.scanJob) return ''
  switch (props.scanJob.colorMode) {
    case 'color': return '彩色'
    case 'gray': return '灰度'
    case 'lineart': return '黑白'
    default: return props.scanJob.colorMode
  }
})

function downloadScan() {
  if (props.scanJob?.fileUrl) {
    const link = document.createElement('a')
    link.href = props.scanJob.fileUrl
    link.download = props.scanJob.filename || 'scan.png'
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
  }
}

function onImageLoad() {
  console.log('Scan preview loaded')
}

function onImageError() {
  console.error('Failed to load scan preview')
}

onMounted(() => {
  // Simulate progress for scanning state
  progressInterval = setInterval(() => {
    if (props.scanJob?.status === 'scanning') {
      progress.value = Math.min(progress.value + 10, 90)
    } else {
      progress.value = 0
    }
  }, 500)
})

onUnmounted(() => {
  if (progressInterval) {
    clearInterval(progressInterval)
  }
})
</script>