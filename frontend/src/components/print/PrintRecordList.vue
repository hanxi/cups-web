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
          <div v-if="expandedRecords.has(rec.id)" class="mt-2 pt-2 border-t">
            <div class="grid grid-cols-2 gap-1 text-xs text-muted">
              <div><span class="font-medium">颜色：</span>{{ rec.isColor ? '彩色' : '黑白' }}</div>
              <div><span class="font-medium">双面：</span>{{ rec.isDuplex ? '是' : '否' }}</div>
              <div><span class="font-medium">页数：</span>{{ rec.pages }}</div>
              <div v-if="rec.jobId"><span class="font-medium">任务ID：</span>{{ rec.jobId }}</div>
            </div>
            <div class="mt-2 flex justify-end">
              <UButton
                size="xs"
                variant="outline"
                icon="i-lucide-printer"
                :loading="reprintingId === rec.id"
                @click.stop="openReprintDialog(rec)"
              >重新打印</UButton>
            </div>
          </div>
        </div>
      </div>
    </div>

    <UModal v-model:open="showReprintModal">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">重新打印</h3>
          <div class="text-sm text-muted truncate">文件：{{ reprintRecord?.filename }}</div>
          <div class="space-y-3">
            <div>
              <label class="block text-sm font-medium mb-1">打印机</label>
              <USelect
                v-model="reprintForm.printer"
                :items="printerSelectItems"
                value-key="value"
                label-key="label"
                placeholder="选择打印机"
              />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1">份数</label>
              <UInput type="number" v-model.number="reprintForm.copies" :min="1" />
            </div>
            <div class="flex gap-4">
              <label class="flex items-center gap-2 cursor-pointer">
                <UCheckbox v-model="reprintForm.color" />
                <span class="text-sm">彩色</span>
              </label>
              <label class="flex items-center gap-2 cursor-pointer">
                <UCheckbox v-model="reprintForm.duplex" />
                <span class="text-sm">双面</span>
              </label>
            </div>
            <div>
              <label class="block text-sm font-medium mb-1">每张页数</label>
              <USelect
                v-model="reprintForm.numberUp"
                :items="numberUpItems"
                value-key="value"
                label-key="label"
              />
            </div>
            <div v-if="reprintForm.numberUp > 1" class="space-y-3">
              <div>
                <label class="block text-sm font-medium mb-1">页面顺序</label>
                <USelect
                  v-model="reprintForm.numberUpLayout"
                  :items="numberUpLayoutItems"
                  value-key="value"
                  label-key="label"
                />
              </div>
              <label class="flex items-center gap-2 cursor-pointer">
                <UCheckbox
                  :model-value="reprintForm.pageBorder === 'single'"
                  @update:model-value="reprintForm.pageBorder = $event ? 'single' : 'none'"
                />
                <span class="text-sm">为每个小页添加边框</span>
              </label>
            </div>
          </div>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="showReprintModal = false">取消</UButton>
            <UButton color="primary" :loading="reprintingId != null" @click="submitReprint">确认打印</UButton>
          </div>
        </div>
      </template>
    </UModal>
  </UCard>
</template>

<script setup>
import { ref, computed } from 'vue'
import { formatTime, formatPrinterName, statusColor, statusText } from '../../utils/format'

const props = defineProps({
  records: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  printers: { type: Array, default: () => [] },
  currentPrinter: { type: String, default: '' }
})

const emit = defineEmits(['refresh', 'reprint'])

const listExpanded = ref(window.innerWidth >= 1024)
const expandedRecords = ref(new Set())

const showReprintModal = ref(false)
const reprintingId = ref(null)
const reprintRecord = ref(null)
const reprintForm = ref({
  printer: '',
  duplex: false,
  color: true,
  copies: 1,
  numberUp: 1,
  numberUpLayout: 'lrtb',
  pageBorder: 'none'
})

const numberUpItems = [
  { label: '1 页/张（不缩排）', value: 1 },
  { label: '2 页/张', value: 2 },
  { label: '4 页/张', value: 4 },
  { label: '6 页/张', value: 6 },
  { label: '9 页/张', value: 9 },
  { label: '16 页/张', value: 16 }
]

const numberUpLayoutItems = [
  { label: '横向 Z 形（左→右，上→下）', value: 'lrtb' },
  { label: '横向 Z 形（右→左，上→下）', value: 'rltb' },
  { label: '纵向 N 形（上→下，左→右）', value: 'tblr' },
  { label: '纵向 N 形（上→下，右→左）', value: 'tbrl' }
]

const printerSelectItems = computed(() =>
  props.printers.map(p => ({ label: `${p.name} — ${p.uri}`, value: p.uri }))
)

function toggleRecord(id) {
  const s = new Set(expandedRecords.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  expandedRecords.value = s
}

function openReprintDialog(rec) {
  reprintRecord.value = rec
  reprintForm.value = {
    printer: props.currentPrinter || rec.printerUri,
    duplex: rec.isDuplex,
    color: rec.isColor,
    copies: 1,
    numberUp: 1,
    numberUpLayout: 'lrtb',
    pageBorder: 'none'
  }
  showReprintModal.value = true
}

function submitReprint() {
  const rec = reprintRecord.value
  if (!rec) return
  reprintingId.value = rec.id
  showReprintModal.value = false
  emit('reprint', {
    id: rec.id,
    printer: reprintForm.value.printer,
    duplex: reprintForm.value.duplex,
    color: reprintForm.value.color,
    copies: reprintForm.value.copies,
    numberUp: reprintForm.value.numberUp,
    numberUpLayout: reprintForm.value.numberUpLayout,
    pageBorder: reprintForm.value.pageBorder
  })
}

defineExpose({ clearReprintLoading: () => { reprintingId.value = null } })
</script>
