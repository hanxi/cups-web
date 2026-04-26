<template>
  <UCard>
    <template #header>
      <div class="flex items-center justify-between cursor-pointer select-none" @click="listExpanded = !listExpanded">
        <div class="flex items-center gap-2 font-semibold">
          <UIcon name="i-lucide-history" class="w-5 h-5" />
          打印记录
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
          暂无打印记录
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
              <p class="text-xs text-muted mt-0.5">{{ formatPrinterName(rec.printerUri) }} · {{ rec.pages }}页</p>
              <p class="text-xs text-muted">{{ formatTime(rec.createdAt) }}</p>
            </div>
            <UBadge :color="statusColor(rec.status)" variant="subtle" size="xs">
              {{ statusText(rec.status) }}
            </UBadge>
          </div>
          <!-- 展开详情 -->
          <div v-if="expandedRecords.has(rec.id)" class="mt-2 pt-2 border-t grid grid-cols-2 gap-1 text-xs text-muted">
            <div><span class="font-medium">颜色：</span>{{ rec.isColor ? '彩色' : '黑白' }}</div>
            <div><span class="font-medium">双面：</span>{{ rec.isDuplex ? '是' : '否' }}</div>
            <div><span class="font-medium">页数：</span>{{ rec.pages }}</div>
            <div v-if="rec.jobId"><span class="font-medium">任务ID：</span>{{ rec.jobId }}</div>
          </div>
        </div>
      </div>
    </div>
  </UCard>
</template>

<script setup>
import { ref } from 'vue'
import { formatTime, formatPrinterName, statusColor, statusText } from '../../utils/format'

defineProps({
  records: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false }
})

defineEmits(['refresh'])

const listExpanded = ref(window.innerWidth >= 1024) // 仅用于初始折叠态，用户可手动切换展开/折叠
const expandedRecords = ref(new Set())

function toggleRecord(id) {
  const s = new Set(expandedRecords.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  expandedRecords.value = s
}
</script>
