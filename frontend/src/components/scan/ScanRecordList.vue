<template>
  <UCard>
    <template #header>
      <div class="flex items-center justify-between cursor-pointer select-none" @click="listExpanded = !listExpanded">
        <div class="flex items-center gap-2 font-semibold">
          <UIcon name="i-lucide-history" class="w-5 h-5" />
          扫描记录
          <!-- 折叠时显示最近一条摘要 -->
          <span v-if="!listExpanded && records.length > 0" class="text-xs font-normal text-muted truncate max-w-48">
            — {{ records[0].filename }} · {{ formatTime(records[0].createdAt) }} · {{ statusText(records[0].status) }}
          </span>
        </div>
        <div class="flex items-center gap-1">
          <UButton variant="ghost" size="xs" icon="i-lucide-refresh-cw" @click.stop="$emit('refresh')" />
          <UIcon
            :name="listExpanded ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'"
            class="w-4 h-4 text-muted transition-transform duration-200"
          />
        </div>
      </div>
    </template>
    <div
      class="transition-all duration-300 ease-in-out overflow-hidden"
      :style="{ maxHeight: listExpanded ? '24rem' : '0px', visibility: listExpanded ? 'visible' : 'hidden' }"
    >
      <div class="space-y-2 max-h-96 overflow-y-auto">
        <div v-if="loading" class="text-center py-4">
          <UIcon name="i-lucide-loader-circle" class="w-5 h-5 animate-spin mx-auto text-muted" />
        </div>
        <div v-else-if="records.length === 0" class="text-center py-6 text-muted text-sm">
          暂无扫描记录
        </div>
        <div
          v-for="rec in records"
          :key="rec.id"
          class="border rounded-lg p-3 hover:shadow-sm transition cursor-pointer"
          @click="toggleRecord(rec.id)"
        >
          <div class="flex items-start gap-2">
            <div class="flex-1 min-w-0">
              <p class="text-sm font-medium truncate">{{ rec.filename }}</p>
              <p class="text-xs text-muted mt-0.5">{{ rec.scannerDevice }} · {{ rec.resolution }} DPI</p>
              <p class="text-xs text-muted">{{ formatTime(rec.createdAt) }}</p>
            </div>
            <UBadge :color="statusColor(rec.status)" variant="subtle" size="xs">
              {{ statusText(rec.status) }}
            </UBadge>
          </div>
          <!-- 展开详情 -->
          <div v-if="expandedRecords.has(rec.id)" class="mt-2 pt-2 border-t grid grid-cols-2 gap-1 text-xs text-muted">
            <div>
              <span class="text-muted">颜色模式：</span>
              <span>{{ colorModeText(rec.colorMode) }}</span>
            </div>
            <div>
              <span class="text-muted">纸张大小：</span>
              <span>{{ rec.paperSize }}</span>
            </div>
            <div v-if="rec.completedAt">
              <span class="text-muted">完成时间：</span>
              <span>{{ formatTime(rec.completedAt) }}</span>
            </div>
            <div v-if="rec.status === 'completed' && rec.storedPath" class="col-span-2 mt-1">
              <UButton
                size="xs"
                variant="outline"
                @click.stop="downloadScan(rec)"
              >
                <UIcon name="i-lucide-download" class="w-3 h-3 mr-1" />
                下载
              </UButton>
            </div>
          </div>
        </div>
      </div>
    </div>
  </UCard>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const props = defineProps({
  records: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false }
})

defineEmits(['refresh'])

const listExpanded = ref(false)
const expandedRecords = ref(new Set())

function toggleRecord(id) {
  if (expandedRecords.value.has(id)) {
    expandedRecords.value.delete(id)
  } else {
    expandedRecords.value.add(id)
  }
}

function statusText(status) {
  switch (status) {
    case 'pending': return '等待中'
    case 'scanning': return '扫描中'
    case 'completed': return '已完成'
    case 'failed': return '失败'
    default: return status
  }
}

function statusColor(status) {
  switch (status) {
    case 'pending': return 'neutral'
    case 'scanning': return 'primary'
    case 'completed': return 'success'
    case 'failed': return 'error'
    default: return 'neutral'
  }
}

function colorModeText(mode) {
  switch (mode) {
    case 'color': return '彩色'
    case 'gray': return '灰度'
    case 'lineart': return '黑白'
    default: return mode
  }
}

function formatTime(timeStr) {
  if (!timeStr) return ''
  const date = new Date(timeStr)
  return date.toLocaleString()
}

function downloadScan(rec) {
  if (rec.storedPath) {
    const link = document.createElement('a')
    link.href = `/api/scan/${rec.id}/file`
    link.download = rec.filename || 'scan.png'
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
  }
}
</script>