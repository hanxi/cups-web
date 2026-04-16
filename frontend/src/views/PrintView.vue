<template>
  <div class="p-2 sm:p-4 md:p-6 max-w-7xl mx-auto">
    <!-- 顶部标题栏 -->
    <div class="flex items-center justify-between mb-3">
      <h1 class="text-lg font-bold flex items-center gap-2">
        <UIcon name="i-lucide-printer" class="w-5 h-5 text-primary" />
        打印
      </h1>
      <UButton variant="ghost" size="sm" icon="i-lucide-refresh-cw" @click="refreshAll" :loading="refreshing">刷新</UButton>
    </div>

    <!-- 主体两栏布局 -->
    <div class="grid grid-cols-1 lg:grid-cols-5 gap-4">
      <!-- 左栏：打印设置 + 预览 -->
      <div class="lg:col-span-3 space-y-4">
        <!-- 打印机选择 -->
        <UCard>
          <template #header>
            <div class="flex items-center gap-2 font-semibold">
              <UIcon name="i-lucide-printer" class="w-4 h-4" />
              打印机
            </div>
          </template>
          <UFormField label="选择打印机">
            <USelect
              v-model="printer"
              :items="printerItems"
              value-key="value"
              label-key="label"
              class="w-full"
              @update:model-value="onPrinterChange"
            />
          </UFormField>
        </UCard>

        <!-- 文件上传 -->
        <UCard>
          <template #header>
            <div class="flex items-center gap-2 font-semibold">
              <UIcon name="i-lucide-file-up" class="w-4 h-4" />
              文件
            </div>
          </template>
          <div class="space-y-3">
            <div
              class="border-2 border-dashed rounded-lg p-4 sm:p-6 text-center cursor-pointer transition-colors"
              :class="isDragging ? 'border-primary bg-primary/5' : 'border-muted hover:border-primary/50'"
              @dragover.prevent="isDragging = true"
              @dragleave="isDragging = false"
              @drop.prevent="onDrop"
              @click="fileInput.click()"
            >
              <input ref="fileInput" type="file" class="hidden" @change="onFileChange" />
              <div v-if="!selectedFile">
                <UIcon name="i-lucide-upload-cloud" class="w-8 h-8 sm:w-10 sm:h-10 mx-auto text-muted mb-2" />
                <p class="text-sm text-muted">点击或拖拽文件上传</p>
                <p class="text-xs text-muted mt-1">支持 PDF、Word、Excel、PPT、图片等格式</p>
              </div>
              <div v-else class="flex items-center gap-3 w-full">
                <UIcon name="i-lucide-file-check" class="w-8 h-8 text-success shrink-0" />
                <div class="flex-1 min-w-0 text-left">
                  <p class="text-sm font-medium break-all line-clamp-2 leading-snug">{{ selectedFile.name }}</p>
                  <p class="text-xs text-muted mt-0.5">{{ formatFileSize(selectedFile.size) }}</p>
                </div>
                <UButton
                  variant="ghost"
                  size="xs"
                  icon="i-lucide-x"
                  color="error"
                  class="shrink-0"
                  @click.stop="clearFile"
                />
              </div>
            </div>

            <!-- 转换状态 -->
            <UAlert v-if="converting" color="info" variant="subtle" icon="i-lucide-loader-circle" title="正在转换为 PDF，请稍候…" />
            <UAlert v-if="converted && !converting" color="success" variant="subtle" icon="i-lucide-check-circle" title="已转换为 PDF，可以打印" />

            <!-- 操作按钮 -->
            <div class="flex flex-wrap gap-2">
              <UButton
                v-if="canConvert"
                variant="outline"
                icon="i-lucide-file-text"
                :loading="converting"
                @click="convertToPdf"
              >转换为 PDF</UButton>
              <UButton
                v-if="previewUrl"
                variant="ghost"
                icon="i-lucide-download"
                :href="previewUrl"
                :download="downloadName"
                tag="a"
              >下载预览</UButton>
            </div>
          </div>
        </UCard>

        <!-- 打印参数 -->
        <UCard>
          <template #header>
            <div class="flex items-center gap-2 font-semibold">
              <UIcon name="i-lucide-settings-2" class="w-4 h-4" />
              打印参数
            </div>
          </template>
          <div class="space-y-4">
            <!-- 第一行：颜色 + 方向（紧凑按钮组） -->
            <div class="grid grid-cols-2 gap-3">
              <UFormField label="颜色模式">
                <div class="flex rounded-lg border border-muted overflow-hidden">
                  <label v-for="item in colorItems" :key="String(item.value)"
                    class="flex-1 flex items-center justify-center gap-1.5 py-2 px-2 cursor-pointer text-sm transition"
                    :class="isColor === item.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'">
                    <input type="radio" :value="item.value" v-model="isColor" class="sr-only" />
                    <UIcon :name="item.icon" class="w-3.5 h-3.5 shrink-0" />
                    <span class="truncate text-xs">{{ item.label }}</span>
                  </label>
                </div>
              </UFormField>

              <UFormField label="打印方向">
                <div class="flex rounded-lg border border-muted overflow-hidden">
                  <label v-for="item in orientationItems" :key="item.value"
                    class="flex-1 flex items-center justify-center gap-1.5 py-2 px-2 cursor-pointer text-sm transition"
                    :class="orientation === item.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'">
                    <input type="radio" :value="item.value" v-model="orientation" class="sr-only" />
                    <UIcon :name="item.icon" class="w-3.5 h-3.5 shrink-0" />
                    <span class="truncate text-xs">{{ item.label }}</span>
                  </label>
                </div>
              </UFormField>
            </div>

            <!-- 第二行：双面 + 份数 -->
            <div class="grid grid-cols-2 gap-3">
              <UFormField label="双面打印">
                <USelect v-model="duplex" :items="duplexItems" value-key="value" label-key="label" class="w-full" />
              </UFormField>

              <UFormField label="份数">
                <UInput
                  v-model.number="copies"
                  type="number"
                  :min="1"
                  :max="99"
                  class="w-full"
                />
              </UFormField>
            </div>

            <!-- 第三行：纸张大小 + 纸张类型 -->
            <div class="grid grid-cols-2 gap-3">
              <UFormField label="纸张大小">
                <USelect v-model="paperSize" :items="paperSizeItems" value-key="value" label-key="label" class="w-full" />
              </UFormField>
              <UFormField label="纸张类型">
                <USelect v-model="paperType" :items="paperTypeItems" value-key="value" label-key="label" class="w-full" />
              </UFormField>
            </div>

            <!-- 第四行：打印缩放 + 页面范围 -->
            <div class="grid grid-cols-2 gap-3">
              <UFormField label="缩放">
                <USelect v-model="printScaling" :items="scalingItems" value-key="value" label-key="label" class="w-full" />
              </UFormField>
              <UFormField label="页面范围" :hint="pageRangeError || '如：1-5 8'">
                <UInput
                  v-model="pageRange"
                  placeholder="留空=全部"
                  class="w-full"
                  :color="pageRangeError ? 'error' : undefined"
                  @input="validatePageRange"
                />
              </UFormField>
            </div>

            <!-- 镜像打印 -->
            <UFormField label="镜像打印">
              <label class="flex items-center gap-2 p-2 border rounded-lg cursor-pointer transition hover:bg-elevated w-fit"
                :class="mirror ? 'border-primary bg-primary/5' : 'border-muted'">
                <UCheckbox v-model="mirror" />
                <UIcon name="i-lucide-flip-horizontal" class="w-4 h-4" />
                <span class="text-sm">水平镜像翻转</span>
              </label>
            </UFormField>

            <!-- 打印按钮 -->
            <UButton
              color="primary"
              size="lg"
              class="w-full"
              icon="i-lucide-printer"
              :disabled="!canPrint || printing"
              :loading="printing"
              @click="uploadAndPrint"
            >
              提交打印
            </UButton>
          </div>
        </UCard>

        <!-- 文件预览 -->
        <UCard v-if="selectedFile">
          <template #header>
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2 font-semibold">
                <UIcon name="i-lucide-eye" class="w-4 h-4" />
                预览
              </div>
              <span class="text-sm text-gray-500">
                {{ paperSizeLabel }} · {{ orientationLabel }} · {{ paperDimText }}
              </span>
            </div>
          </template>
          <!-- 纸张预览容器 -->
          <div class="flex justify-center items-center py-4 bg-gray-100 dark:bg-gray-800 rounded-lg" style="min-height: 300px;">
            <!-- 纸张模拟 -->
            <div :style="paperPreviewStyle"
                 class="bg-white shadow-lg border border-gray-200 overflow-hidden transition-all duration-300 ease-in-out relative">
              <!-- 内容预览嵌入 -->
              <img v-if="previewType === 'image'" :src="previewUrl" class="w-full h-full object-contain" />
              <iframe v-else-if="previewType === 'pdf'" :src="previewUrl" class="w-full h-full border-0" />
              <div v-else-if="previewType === 'text'" class="p-3 text-[8px] leading-tight overflow-hidden h-full text-gray-700 whitespace-pre-wrap">
                {{ textPreview?.substring(0, 800) }}
              </div>
              <!-- 空白纸张占位 -->
              <div v-else class="flex items-center justify-center h-full text-gray-300 text-sm">
                {{ paperSizeLabel }}
              </div>
            </div>
          </div>
        </UCard>
      </div>

      <!-- 右栏：打印记录 + 打印机状态 -->
      <div class="lg:col-span-2 space-y-4">
        <!-- 打印记录 -->
        <UCard>
          <template #header>
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2 font-semibold">
                <UIcon name="i-lucide-history" class="w-4 h-4" />
                打印记录
              </div>
              <UButton variant="ghost" size="xs" icon="i-lucide-refresh-cw" @click="loadPrintRecords" />
            </div>
          </template>
          <div class="space-y-2 max-h-96 overflow-y-auto">
            <div v-if="loadingRecords" class="text-center py-4">
              <UIcon name="i-lucide-loader-circle" class="w-5 h-5 animate-spin mx-auto text-muted" />
            </div>
            <div v-else-if="printRecords.length === 0" class="text-center py-6 text-muted text-sm">
              暂无打印记录
            </div>
            <div
              v-for="rec in printRecords"
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
        </UCard>

        <!-- 打印机状态 -->
        <UCard>
          <template #header>
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2 font-semibold">
                <UIcon name="i-lucide-activity" class="w-4 h-4" />
                打印机状态
              </div>
              <UButton variant="ghost" size="xs" icon="i-lucide-refresh-cw" @click="loadPrinterInfo" :loading="loadingPrinterInfo" />
            </div>
          </template>
          <div>
            <div v-if="!printer" class="text-center py-6 text-muted text-sm">
              请先选择打印机
            </div>
            <div v-else-if="loadingPrinterInfo && !printerInfo" class="text-center py-4">
              <UIcon name="i-lucide-loader-circle" class="w-5 h-5 animate-spin mx-auto text-muted" />
            </div>
            <div v-else-if="printerInfoError" class="text-center py-4 text-sm text-error">
              <UIcon name="i-lucide-wifi-off" class="w-5 h-5 mx-auto mb-1" />
              {{ printerInfoError }}
            </div>
            <div v-else-if="printerInfo" class="space-y-3">
              <!-- 基本状态 -->
              <div class="flex items-center justify-between p-2 bg-elevated rounded-lg">
                <div class="flex items-center gap-2">
                  <UIcon name="i-lucide-info" class="w-4 h-4 text-info" />
                  <span class="text-sm font-medium">打印机状态</span>
                </div>
                <UBadge :color="printerStateColor(printerInfo.state)" variant="subtle" size="xs">
                  {{ printerStateText(printerInfo.state) }}
                </UBadge>
              </div>

              <!-- 队列 -->
              <div class="flex items-center justify-between p-2 bg-elevated rounded-lg">
                <div class="flex items-center gap-2">
                  <UIcon name="i-lucide-list-ordered" class="w-4 h-4 text-primary" />
                  <span class="text-sm font-medium">队列任务数</span>
                </div>
                <span class="text-sm font-bold">{{ printerInfo.queuedJobs }}</span>
              </div>

              <!-- 状态持续时间 -->
              <div v-if="printerInfo.attributes && printerInfo.attributes['printer-state-change-date-time']" class="flex items-center justify-between p-2 bg-elevated rounded-lg">
                <div class="flex items-center gap-2">
                  <UIcon name="i-lucide-clock" class="w-4 h-4 text-success" />
                  <span class="text-sm font-medium">状态持续</span>
                </div>
                <span class="text-sm">{{ formatStateDuration(printerInfo.attributes['printer-state-change-date-time']) }}</span>
              </div>

              <!-- 固件版本 -->
              <div v-if="printerInfo.firmwareVersion" class="flex items-center justify-between p-2 bg-elevated rounded-lg">
                <div class="flex items-center gap-2">
                  <UIcon name="i-lucide-cpu" class="w-4 h-4 text-secondary" />
                  <span class="text-sm font-medium">固件版本</span>
                </div>
                <span class="text-xs text-muted truncate max-w-32">{{ printerInfo.firmwareVersion }}</span>
              </div>

              <!-- 状态消息 -->
              <div v-if="printerInfo.stateMessage" class="p-2 bg-warning/10 border border-warning/20 rounded-lg">
                <p class="text-xs text-warning">{{ printerInfo.stateMessage }}</p>
              </div>

              <!-- 墨盒信息 -->
              <div v-if="printerInfo.markerNames && printerInfo.markerNames.length > 0">
                <div class="flex items-center gap-2 mb-2">
                  <UIcon name="i-lucide-droplets" class="w-4 h-4 text-primary" />
                  <span class="text-sm font-semibold">墨盒信息</span>
                </div>
                <div class="space-y-2">
                  <div v-for="(name, i) in printerInfo.markerNames" :key="i" class="space-y-1">
                    <div class="flex justify-between text-xs">
                      <span class="text-muted">{{ name }}</span>
                      <span :class="markerLevelColor(printerInfo.markerLevels?.[i])">
                        {{ printerInfo.markerLevels?.[i] ?? '?' }}%
                      </span>
                    </div>
                    <div class="w-full bg-muted/30 rounded-full h-2">
                      <div
                        class="h-2 rounded-full transition-all"
                        :class="markerBarColor(printerInfo.markerLevels?.[i])"
                        :style="{ width: Math.max(0, Math.min(100, printerInfo.markerLevels?.[i] ?? 0)) + '%' }"
                      ></div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- 纸盒信息 -->
              <div v-if="printerInfo.mediaReady && printerInfo.mediaReady.length > 0">
                <div class="flex items-center gap-2 mb-2">
                  <UIcon name="i-lucide-layers" class="w-4 h-4 text-secondary" />
                  <span class="text-sm font-semibold">纸盒信息</span>
                </div>
                <div class="space-y-1">
                  <div v-for="(media, i) in printerInfo.mediaReady" :key="i"
                    class="flex items-center gap-2 p-1.5 bg-elevated rounded text-xs">
                    <UIcon name="i-lucide-square" class="w-3 h-3 text-muted" />
                    <span>{{ media }}</span>
                  </div>
                </div>
              </div>

              <!-- 状态原因 -->
              <div v-if="printerInfo.stateReasons && printerInfo.stateReasons.filter(r => r !== 'none').length > 0">
                <div class="flex items-center gap-2 mb-1">
                  <UIcon name="i-lucide-alert-triangle" class="w-4 h-4 text-warning" />
                  <span class="text-sm font-semibold">警报</span>
                </div>
                <div class="space-y-1">
                  <div v-for="reason in printerInfo.stateReasons.filter(r => r !== 'none')" :key="reason"
                    class="text-xs text-warning bg-warning/10 px-2 py-1 rounded">
                    {{ reason }}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </UCard>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { jsPDF } from 'jspdf'

const emit = defineEmits(['logout'])
const toast = useToast()

// ─── 打印机 ───────────────────────────────────────────────
const printer = ref('')
const printers = ref([])
const printerItems = computed(() =>
  printers.value.map(p => ({ label: `${p.name} — ${p.uri}`, value: p.uri }))
)

// ─── 文件 ─────────────────────────────────────────────────
const fileInput = ref(null)
const selectedFile = ref(null)
const previewUrl = ref('')
const previewType = ref('')
const textPreview = ref('')
const converting = ref(false)
const converted = ref(false)
const pdfBlob = ref(null)
const downloadName = ref('')
const isDragging = ref(false)

// ─── 打印参数 ─────────────────────────────────────────────
const isColor = ref(true)
const duplex = ref('one-sided')
const orientation = ref('portrait')
const copies = ref(1)
const paperSize = ref('A4')
const paperType = ref('plain')
const printScaling = ref('fit')
const pageRange = ref('')
const pageRangeError = ref('')
const mirror = ref(false)

// ─── 状态 ─────────────────────────────────────────────────
const printing = ref(false)
const refreshing = ref(false)

// ─── 打印记录 ─────────────────────────────────────────────
const printRecords = ref([])
const loadingRecords = ref(false)
const expandedRecords = ref(new Set())

// ─── 打印机状态 ───────────────────────────────────────────
const printerInfo = ref(null)
const loadingPrinterInfo = ref(false)
const printerInfoError = ref('')

// ─── 选项数据 ─────────────────────────────────────────────
const colorItems = [
  { label: '彩色打印', value: true, icon: 'i-lucide-palette' },
  { label: '黑白打印', value: false, icon: 'i-lucide-circle' }
]

const duplexItems = [
  { label: '单面打印', value: 'one-sided' },
  { label: '双面（长边翻页）', value: 'two-sided-long-edge' },
  { label: '双面（短边翻页）', value: 'two-sided-short-edge' }
]

const orientationItems = [
  { label: '纵向', value: 'portrait', icon: 'i-lucide-rectangle-vertical' },
  { label: '横向', value: 'landscape', icon: 'i-lucide-rectangle-horizontal' }
]

const paperSizeItems = [
  { label: 'A5 (148×210mm)', value: 'A5' },
  { label: 'A4 (210×297mm)', value: 'A4' },
  { label: 'A3 (297×420mm)', value: 'A3' },
  { label: 'A2 (420×594mm)', value: 'A2' },
  { label: 'A1 (594×841mm)', value: 'A1' },
  { label: '5寸 (89×127mm)', value: '5inch' },
  { label: '6寸 (102×152mm)', value: '6inch' },
  { label: '7寸 (127×178mm)', value: '7inch' },
  { label: '8寸 (152×203mm)', value: '8inch' },
  { label: '10寸 (203×254mm)', value: '10inch' },
  { label: 'Letter (8.5×11in)', value: 'Letter' },
  { label: 'Legal (8.5×14in)', value: 'Legal' }
]

const paperTypeItems = [
  { label: '普通纸', value: 'plain' },
  { label: '照片纸', value: 'photo' },
  { label: '光面照片纸', value: 'glossy' },
  { label: '哑光照片纸', value: 'matte' },
  { label: '信封', value: 'envelope' },
  { label: '卡片纸', value: 'cardstock' },
  { label: '标签纸', value: 'labels' },
  { label: '自动选择', value: 'auto' }
]

const scalingItems = [
  { label: '自动', value: 'auto' },
  { label: '自动适应', value: 'auto-fit' },
  { label: '适应纸张', value: 'fit' },
  { label: '填充纸张', value: 'fill' },
  { label: '无缩放', value: 'none' }
]

// ─── 纸张尺寸映射 ─────────────────────────────────────────
const paperDimensionsMap = {
  'A5': { width: 148, height: 210 },
  'A4': { width: 210, height: 297 },
  'A3': { width: 297, height: 420 },
  'A2': { width: 420, height: 594 },
  'A1': { width: 594, height: 841 },
  '5inch': { width: 89, height: 127 },
  '6inch': { width: 102, height: 152 },
  '7inch': { width: 127, height: 178 },
  '8inch': { width: 152, height: 203 },
  '10inch': { width: 203, height: 254 },
  'Letter': { width: 216, height: 279 },
  'Legal': { width: 216, height: 356 },
}

// ─── 计算属性 ─────────────────────────────────────────────
const canPrint = computed(() => !!printer.value && (!!pdfBlob.value || !!selectedFile.value) && !pageRangeError.value)
const canConvert = computed(() => !!selectedFile.value && !converting.value && selectedFile.value.type !== 'application/pdf')

// 当前纸张尺寸标签
const paperSizeLabel = computed(() => {
  const item = paperSizeItems.find(i => i.value === paperSize.value)
  return item?.label || paperSize.value
})

// 当前方向标签
const orientationLabel = computed(() => {
  const item = orientationItems.find(i => i.value === orientation.value)
  return item?.label || (orientation.value === 'portrait' ? '纵向' : '横向')
})

// 纸张尺寸文本（如 210×297mm）
const paperDimText = computed(() => {
  const dim = paperDimensionsMap[paperSize.value]
  if (!dim) return ''
  if (orientation.value === 'landscape') {
    return `${dim.height}×${dim.width}mm`
  }
  return `${dim.width}×${dim.height}mm`
})

// 纸张预览样式
const paperPreviewStyle = computed(() => {
  const dim = paperDimensionsMap[paperSize.value]
  if (!dim) return {}
  
  const isLandscape = orientation.value === 'landscape'
  const width = isLandscape ? dim.height : dim.width
  const height = isLandscape ? dim.width : dim.height
  const ratio = width / height
  
  // 容器最大高度 400px，宽度自适应
  const maxHeight = 400
  const maxWidth = 600
  
  let displayWidth, displayHeight
  
  // 根据宽高比计算实际显示尺寸
  if (ratio > 1) {
    // 横向纸张，以最大宽度为限制
    displayWidth = Math.min(maxWidth, maxHeight * ratio)
    displayHeight = displayWidth / ratio
  } else {
    // 纵向纸张，以最大高度为限制
    displayHeight = Math.min(maxHeight, maxWidth / ratio)
    displayWidth = displayHeight * ratio
  }
  
  return {
    width: `${displayWidth}px`,
    height: `${displayHeight}px`,
  }
})

// ─── 工具函数 ─────────────────────────────────────────────
function getCSRF() {
  const m = document.cookie.match('(^|;)\\s*csrf_token\\s*=\\s*([^;]+)')
  return m ? m.pop() : ''
}

function formatFileSize(bytes) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

function formatTime(iso) {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
  } catch { return iso }
}

function formatPrinterName(uri) {
  if (!uri) return ''
  const parts = uri.split('/')
  return parts[parts.length - 1] || uri
}

// formatStateDuration 计算从 ISO 时间字符串到现在经过了多久
function formatStateDuration(isoStr) {
  if (!isoStr) return '未知'
  const past = new Date(isoStr)
  if (isNaN(past.getTime())) return '未知'
  const diffMs = Date.now() - past.getTime()
  if (diffMs < 0) return '未知'
  const totalSeconds = Math.floor(diffMs / 1000)
  const d = Math.floor(totalSeconds / 86400)
  const h = Math.floor((totalSeconds % 86400) / 3600)
  const m = Math.floor((totalSeconds % 3600) / 60)
  if (d > 0) return `${d}天${h}小时`
  if (h > 0) return `${h}小时${m}分钟`
  if (m > 0) return `${m}分钟`
  return `${totalSeconds}秒`
}

function statusColor(status) {
  const map = { queued: 'info', printed: 'success', failed: 'error', cancelled: 'neutral' }
  return map[status] || 'neutral'
}

function statusText(status) {
  const map = { queued: '排队中', printed: '已打印', failed: '失败', cancelled: '已取消' }
  return map[status] || status
}

function printerStateColor(state) {
  const map = { idle: 'success', processing: 'warning', stopped: 'error' }
  return map[state] || 'neutral'
}

function printerStateText(state) {
  const map = { idle: '空闲', processing: '打印中', stopped: '已停止' }
  return map[state] || state || '未知'
}

function markerLevelColor(level) {
  if (level === undefined || level === null) return 'text-muted'
  if (level <= 10) return 'text-error font-bold'
  if (level <= 25) return 'text-warning font-medium'
  return 'text-success'
}

function markerBarColor(level) {
  if (level === undefined || level === null) return 'bg-muted'
  if (level <= 10) return 'bg-error'
  if (level <= 25) return 'bg-warning'
  return 'bg-success'
}

function validatePageRange() {
  let val = pageRange.value.trim()
  if (!val) { pageRangeError.value = ''; return }
  
  // 自动修正：
  // 1. 将全角减号、破折号等转换为半角减号
  // 2. 移除减号前后的空格（如 "1 - 5" → "1-5"）
  // 3. 将逗号转换为空格
  // 4. 合并多个空格
  const normalizedVal = val
    .replace(/[－—–―]/g, '-')  // 全角减号、破折号、短破折号、水平线
    .replace(/\s*-\s*/g, '-')   // 移除减号前后的空格
    .replace(/[，,]/g, ' ')     // 将逗号（全角/半角）转换为空格
    .replace(/\s+/g, ' ')       // 合并多个空格
    .trim()
  
  // 如果修正后的值与原值不同，自动更新输入框
  if (normalizedVal !== val) {
    pageRange.value = normalizedVal
    val = normalizedVal
  }
  
  const pattern = /^(\d+(-\d+)?)(\s+\d+(-\d+)?)*$/
  pageRangeError.value = pattern.test(val) ? '' : '格式无效，例如：1-5 8 10-12'
}

function isOfficeFile(f) {
  return /\.(docx?|pptx?|xlsx?)$/i.test(f.name) || [
    'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    'application/msword',
    'application/vnd.openxmlformats-officedocument.presentationml.presentation',
    'application/vnd.ms-powerpoint',
    'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    'application/vnd.ms-excel'
  ].includes(f.type)
}

// ─── 文件操作 ─────────────────────────────────────────────
function clearFile() {
  if (previewUrl.value) {
    try { URL.revokeObjectURL(previewUrl.value) } catch (_) {}
  }
  previewUrl.value = ''
  previewType.value = ''
  textPreview.value = ''
  pdfBlob.value = null
  converted.value = false
  selectedFile.value = null
  downloadName.value = ''
  if (fileInput.value) fileInput.value.value = ''
}

function onDrop(e) {
  isDragging.value = false
  const f = e.dataTransfer.files[0]
  if (f) processFile(f)
}

function onFileChange(e) {
  const f = e.target.files[0]
  if (f) processFile(f)
}

function processFile(f) {
  clearFile()
  selectedFile.value = f
  downloadName.value = f.name.replace(/\.[^/.]+$/, '') + '.pdf'

  if (f.type === 'application/pdf') {
    previewUrl.value = URL.createObjectURL(f)
    previewType.value = 'pdf'
    pdfBlob.value = f
    converted.value = true
  } else if (f.type.startsWith('image/')) {
    previewUrl.value = URL.createObjectURL(f)
    previewType.value = 'image'
  } else if (isOfficeFile(f)) {
    previewType.value = 'text'
    textPreview.value = 'Office 文档（无法直接预览）。点击"转换为 PDF"生成预览。'
  } else if (f.type.startsWith('text/') || /\.(txt|md|html)$/i.test(f.name)) {
    const reader = new FileReader()
    reader.onload = () => {
      textPreview.value = reader.result
      previewType.value = 'text'
    }
    reader.readAsText(f)
  } else {
    previewType.value = 'text'
    textPreview.value = '无法预览此文件类型，可直接提交打印。'
  }
}

async function imageFileToPdfBlob(file) {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => {
      const canvas = document.createElement('canvas')
      canvas.width = img.width
      canvas.height = img.height
      canvas.getContext('2d').drawImage(img, 0, 0)
      const imgData = canvas.toDataURL('image/jpeg', 1.0)
      const doc = new jsPDF({ unit: 'px', format: [img.width, img.height] })
      doc.addImage(imgData, 'JPEG', 0, 0, img.width, img.height)
      resolve(doc.output('blob'))
    }
    img.onerror = () => reject(new Error('图片加载失败'))
    img.src = URL.createObjectURL(file)
  })
}

function textToPdfBlob(text) {
  const doc = new jsPDF()
  const lines = doc.splitTextToSize(text || '', 180)
  doc.text(lines, 10, 10)
  return doc.output('blob')
}

async function convertOfficeToPdf(file) {
  const fd = new FormData()
  fd.append('file', file, file.name)
  const resp = await fetch('/api/convert', {
    method: 'POST',
    body: fd,
    credentials: 'include',
    headers: { 'X-CSRF-Token': getCSRF() }
  })
  if (!resp.ok) throw new Error('服务端转换失败：' + await resp.text())
  return resp.blob()
}

async function convertToPdf() {
  if (!selectedFile.value) return
  converting.value = true
  try {
    const f = selectedFile.value
    let blob
    if (isOfficeFile(f)) {
      blob = await convertOfficeToPdf(f)
    } else if (f.type.startsWith('image/')) {
      blob = await imageFileToPdfBlob(f)
    } else {
      const text = await f.text()
      blob = textToPdfBlob(text)
    }
    pdfBlob.value = blob
    if (previewUrl.value) try { URL.revokeObjectURL(previewUrl.value) } catch (_) {}
    previewUrl.value = URL.createObjectURL(blob)
    previewType.value = 'pdf'
    converted.value = true
    toast.add({ title: '转换成功', color: 'success', icon: 'i-lucide-check-circle' })
  } catch (e) {
    toast.add({ title: '转换失败', description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    converting.value = false
  }
}

// ─── 打印 ─────────────────────────────────────────────────
async function uploadAndPrint() {
  if (!printer.value) { toast.add({ title: '请选择打印机', color: 'warning' }); return }
  if (pageRangeError.value) { toast.add({ title: '页面范围格式有误', color: 'warning' }); return }

  const fileToSend = pdfBlob.value || selectedFile.value
  const filename = pdfBlob.value
    ? (downloadName.value || 'document.pdf')
    : (selectedFile.value ? selectedFile.value.name : 'document')

  const form = new FormData()
  form.append('file', fileToSend, filename)
  form.append('printer', printer.value)
  form.append('duplex', duplex.value === 'one-sided' ? 'false' : 'true')
  form.append('color', isColor.value ? 'true' : 'false')
  form.append('copies', String(copies.value))
  form.append('orientation', orientation.value)
  form.append('paper_size', paperSize.value)
  form.append('paper_type', paperType.value)
  form.append('print_scaling', printScaling.value)
  if (pageRange.value.trim()) form.append('page_range', pageRange.value.trim())
  if (mirror.value) form.append('mirror', 'true')

  printing.value = true
  try {
    const resp = await fetch('/api/print', {
      method: 'POST',
      body: form,
      credentials: 'include',
      headers: { 'X-CSRF-Token': getCSRF() }
    })
    if (!resp.ok) {
      const data = await resp.json().catch(() => ({}))
      if (resp.status === 401) emit('logout')
      throw new Error(data.error || resp.statusText)
    }
    const j = await resp.json()
    toast.add({
      title: '打印任务已提交',
      description: `任务ID：${j.jobId || '—'}，共 ${j.pages} 页`,
      color: 'success',
      icon: 'i-lucide-check-circle'
    })
    localStorage.setItem('last_printer', printer.value)
    await loadPrintRecords()
  } catch (e) {
    toast.add({ title: '打印失败', description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    printing.value = false
  }
}

// ─── 打印记录 ─────────────────────────────────────────────
async function loadPrintRecords(silent = false) {
  if (!silent) loadingRecords.value = true
  try {
    const resp = await fetch('/api/print-records', { credentials: 'include' })
    if (resp.ok) {
      const data = await resp.json()
      printRecords.value = (data || []).map(r => ({
        id: r.id,
        filename: r.filename,
        printerUri: r.printerUri,
        pages: r.pages,
        status: r.status,
        isColor: r.isColor,
        isDuplex: r.isDuplex,
        jobId: r.jobId,
        createdAt: r.createdAt
      }))
    } else if (resp.status === 401) {
      emit('logout')
    }
  } catch (e) {
    console.error('加载打印记录失败', e)
  } finally {
    loadingRecords.value = false
  }
}

function toggleRecord(id) {
  const s = new Set(expandedRecords.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  expandedRecords.value = s
}

// ─── 打印机状态 ───────────────────────────────────────────
async function loadPrinterInfo(silent = false) {
  if (!printer.value) return
  if (!silent) loadingPrinterInfo.value = true
  printerInfoError.value = ''
  try {
    const resp = await fetch(`/api/printer-info?uri=${encodeURIComponent(printer.value)}`, { credentials: 'include' })
    if (resp.ok) {
      printerInfo.value = await resp.json()
    } else if (resp.status === 401) {
      emit('logout')
    } else {
      const d = await resp.json().catch(() => ({}))
      printerInfoError.value = d.error || '查询失败'
    }
  } catch (_) {
    printerInfoError.value = '无法连接到打印机'
  } finally {
    loadingPrinterInfo.value = false
  }
}

function onPrinterChange() {
  printerInfo.value = null
  printerInfoError.value = ''
  loadPrinterInfo()
}

async function refreshAll() {
  refreshing.value = true
  await Promise.all([loadPrintRecords(true), loadPrinterInfo(true)])
  refreshing.value = false
}

// ─── 定时器 ───────────────────────────────────────────────
let recordsTimer = null
let printerInfoTimer = null

// ─── 生命周期 ─────────────────────────────────────────────
onMounted(async () => {
  try {
    const resp = await fetch('/api/printers', { credentials: 'include' })
    if (resp.ok) {
      printers.value = await resp.json()
      const last = localStorage.getItem('last_printer')
      if (last && printers.value.some(p => p.uri === last)) {
        printer.value = last
      } else if (printers.value.length > 0) {
        printer.value = printers.value[0].uri
      }
      if (printer.value) loadPrinterInfo()
    } else if (resp.status === 401) {
      emit('logout')
    }
  } catch (e) {
    toast.add({ title: '加载打印机失败', description: e.message, color: 'error' })
  }

  await loadPrintRecords()

  // 定时刷新打印记录（每5秒，静默刷新不显示 loading 避免抖动）
  recordsTimer = setInterval(() => loadPrintRecords(true), 5000)
  // 定时刷新打印机状态（每15秒，静默刷新不显示 loading 避免抖动）
  printerInfoTimer = setInterval(() => loadPrinterInfo(true), 15000)
})

onUnmounted(() => {
  clearInterval(recordsTimer)
  clearInterval(printerInfoTimer)
})
</script>
