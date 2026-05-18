<template>
  <div class="p-3 sm:p-4 md:p-6 max-w-7xl mx-auto">
    <!-- 顶部标题栏 -->
    <div class="mb-3 grid grid-cols-1 lg:grid-cols-5 gap-x-4 gap-y-2">
      <div class="lg:col-span-3 flex items-center gap-2 sm:gap-3 min-w-0">
        <h1 class="text-lg font-bold flex items-center gap-2 shrink-0">
          <UIcon name="i-lucide-scan" class="w-5 h-5 text-primary" />
          扫描
        </h1>
        <UButton
          variant="ghost"
          size="xs"
          icon="i-lucide-refresh-cw"
          class="shrink-0"
          @click="refreshScanners"
          :loading="refreshing"
        />
      </div>
    </div>

    <!-- 主体两栏布局 -->
    <div class="grid grid-cols-1 lg:grid-cols-5 gap-4">
      <!-- 左栏：扫描设置 -->
      <div class="lg:col-span-3 space-y-4">
        <!-- 扫描仪选择 -->
        <ScannerSelector v-model="scanner" />

        <!-- 扫描参数 -->
        <ScanOptions
          v-model:resolution="resolution"
          v-model:colorMode="colorMode"
          v-model:paperSize="paperSize"
          v-model:scanArea="scanArea"
        />

        <!-- 开始扫描按钮 -->
        <UButton
          color="primary"
          size="xl"
          :ui="{ base: 'justify-center', label: 'flex-1 text-center' }"
          class="w-full font-semibold tracking-wide shadow-lg shadow-primary/25 hover:shadow-xl hover:shadow-primary/35 transition-all ring-1 ring-primary/30"
          icon="i-lucide-scan"
          :disabled="!canScan || scanning"
          :loading="scanning"
          @click="startScan"
        >
          开始扫描
        </UButton>

        <!-- 扫描记录 -->
        <ScanRecordList
          :records="scanRecords"
          :loading="loadingRecords"
          @refresh="fetchScanRecords"
        />
      </div>

      <!-- 右栏：扫描预览 -->
      <div class="lg:col-span-2 space-y-4">
        <ScanPreview
          :scan-job="currentScanJob"
          @new-scan="resetScan"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import ScannerSelector from '../components/scan/ScannerSelector.vue'
import ScanOptions from '../components/scan/ScanOptions.vue'
import ScanPreview from '../components/scan/ScanPreview.vue'
import ScanRecordList from '../components/scan/ScanRecordList.vue'

const scanner = ref('')
const resolution = ref(300)
const colorMode = ref('color')
const paperSize = ref('A4')
const scanArea = ref('')

const scanning = ref(false)
const refreshing = ref(false)
const loadingRecords = ref(false)
const currentScanJob = ref(null)
const scanRecords = ref([])

const canScan = computed(() => {
  return scanner.value !== ''
})

async function refreshScanners() {
  refreshing.value = true
  try {
    // Scanner list will be refreshed by ScannerSelector component
    await new Promise(resolve => setTimeout(resolve, 500))
  } finally {
    refreshing.value = false
  }
}

async function startScan() {
  if (!canScan.value) return
  
  scanning.value = true
  try {
    const response = await fetch('/api/scan', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrfToken()
      },
      body: JSON.stringify({
        scannerDevice: scanner.value,
        resolution: resolution.value,
        colorMode: colorMode.value,
        paperSize: paperSize.value,
        scanArea: scanArea.value
      })
    })

    if (response.ok) {
      const result = await response.json()
      currentScanJob.value = {
        id: result.jobId,
        status: 'scanning',
        filename: `scan_${new Date().toISOString().slice(0, 19).replace(/[:-]/g, '')}`,
        resolution: resolution.value,
        colorMode: colorMode.value,
        paperSize: paperSize.value
      }
      
      // Poll for status
      pollScanStatus(result.jobId)
    } else {
      console.error('Failed to start scan')
    }
  } catch (error) {
    console.error('Error starting scan:', error)
  } finally {
    scanning.value = false
  }
}

async function pollScanStatus(jobId) {
  let retryCount = 0
  const maxRetries = 10
  const pollInterval = setInterval(async () => {
    try {
      const response = await fetch(`/api/scan/${jobId}/status`)
      if (response.ok) {
        retryCount = 0
        const status = await response.json()
        currentScanJob.value = { ...currentScanJob.value, ...status }
        
        if (status.status === 'completed' || status.status === 'failed') {
          clearInterval(pollInterval)
          fetchScanRecords()
        }
      } else {
        retryCount++
        if (retryCount >= maxRetries) {
          clearInterval(pollInterval)
          if (currentScanJob.value) {
            currentScanJob.value = { ...currentScanJob.value, status: 'failed', errorMessage: '轮询失败，请刷新重试' }
          }
        }
      }
    } catch (error) {
      retryCount++
      console.error('Error polling scan status:', error)
      if (retryCount >= maxRetries) {
        clearInterval(pollInterval)
        if (currentScanJob.value) {
          currentScanJob.value = { ...currentScanJob.value, status: 'failed', errorMessage: '网络错误，请刷新重试' }
        }
      }
    }
  }, 1000)
}

function resetScan() {
  currentScanJob.value = null
}

async function fetchScanRecords() {
  loadingRecords.value = true
  try {
    const response = await fetch('/api/scan-records')
    if (response.ok) {
      scanRecords.value = await response.json()
    }
  } catch (error) {
    console.error('Error fetching scan records:', error)
  } finally {
    loadingRecords.value = false
  }
}

function getCsrfToken() {
  const cookies = document.cookie.split(';')
  for (let cookie of cookies) {
    const [name, value] = cookie.trim().split('=')
    if (name === 'csrf_token') {
      return value
    }
  }
  return ''
}

onMounted(() => {
  fetchScanRecords()
})
</script>