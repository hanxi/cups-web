<template>
  <div class="p-3 sm:p-4 md:p-6 max-w-5xl mx-auto space-y-4">
    <div class="flex items-center justify-between gap-2 flex-wrap">
      <h1 class="text-lg font-bold flex items-center gap-2">
        <UIcon name="i-lucide-scan-line" class="w-5 h-5 text-primary" /> {{ t('ui.scan') }} </h1>
      <div class="flex items-center gap-2">
        <UButton
          variant="ghost"
          size="xs"
          icon="i-lucide-refresh-cw"
          :loading="loadingDevices"
          @click="loadDevices(true)"
        >{{ t('ui.refresh_devices') }}</UButton>
      </div>
    </div>

    <UAlert
      color="neutral"
      variant="soft"
      icon="i-lucide-info"
      :title="t('ui.usage')"
      :description="t('ui.the_aio_image_includes_scanimage_and_several_sane_backends_zero')"
    />

    <UCard>
      <template #header>
        <div class="flex items-center gap-2 text-sm font-medium">
          <UIcon name="i-lucide-settings-2" class="w-4 h-4" /> {{ t('ui.scan_settings') }} </div>
      </template>
      <div class="space-y-3">
        <UFormField :label="t('ui.device')" required>
          <div class="flex gap-2">
            <USelect
              v-model="form.device"
              :items="deviceItems"
              value-key="value"
              label-key="label"
              :placeholder="t('ui.refresh_the_device_list_first')"
              class="flex-1"
              icon="i-lucide-scanner"
            />
          </div>
          <template #hint>
            <span v-if="!devices.length && !loadingDevices" class="text-warning"> {{ t('ui.no_devices_detected_check_that_the_scanner_is_on_and') }} </span>
            <span v-else-if="loadingDevices">{{ t('ui.checking') }}</span>
            <span v-else-if="currentProbe?.state === 'probing'" class="text-muted"> {{ t('ui.checking_whether_this_backend_can_open_the_device') }} </span>
            <span v-else-if="currentProbe?.state === 'healthy'" class="text-success"> {{ t('ui.this_backend_can_open_the_device') }} </span>
            <span v-else-if="currentProbe?.state === 'unhealthy'" class="text-error"> {{ t('ui.this_backend_cannot_open_the_device') }}{{ currentProbe.detail ? ': ' + translateError(currentProbe.detail) : '' }}{{ t('ui.try_another_backend_such_as_hpljm1005_or_escl') }} </span>
          </template>
        </UFormField>

        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <UFormField :label="t('ui.mode')">
            <USelect v-model="form.mode" :items="modeItems" value-key="value" label-key="label" />
          </UFormField>
          <UFormField :label="t('ui.resolution_dpi')">
            <USelect v-model="form.resolution" :items="resolutionSelectItems" value-key="value" label-key="label" />
            <UInput
              v-if="form.resolution === 'custom'"
              v-model="customResolution"
              type="number"
              :min="50"
              :max="4800"
              placeholder="50 - 4800"
              class="mt-2"
            />
          </UFormField>
          <UFormField :label="t('ui.output_format')">
            <USelect v-model="form.format" :items="formatItems" value-key="value" label-key="label" />
          </UFormField>
        </div>

        <UFormField v-if="sourceItems.length > 1" :label="t('ui.source')">
          <USelect v-model="form.source" :items="sourceItems" value-key="value" label-key="label" />
          <template #hint>{{ t('ui.single_page_scanning_adf_and_duplex_are_not_supported_yet') }}</template>
        </UFormField>

        <UFormField :label="t('ui.filename_optional')">
          <UInput v-model="form.filename" :placeholder="t('ui.scan_generated_automatically_if_blank')" />
          <template #hint>{{ t('ui.scan_filename_help') }}</template>
        </UFormField>

        <div class="flex justify-end gap-2 pt-2">
          <UButton
            size="sm"
            color="primary"
            icon="i-lucide-play"
            :loading="starting || currentJob?.status === 'running'"
            :disabled="!form.device"
            @click="startScan"
          >
            {{ currentJob?.status === 'running' ? t('ui.scanning') : t('ui.start_scanning') }}
          </UButton>
          <UButton
            v-if="currentJob?.status === 'running'"
            size="sm"
            color="error"
            variant="ghost"
            icon="i-lucide-x"
            @click="cancelCurrentJob"
          >{{ t('ui.cancel') }}</UButton>
        </div>
      </div>
    </UCard>

    <!-- 当前任务日志/状态 -->
    <UCard v-if="currentJob">
      <template #header>
        <div class="flex items-center gap-2">
          <UIcon
            :name="jobStatusIcon(currentJob.status)"
            :class="['w-4 h-4', currentJob.status === 'running' ? 'animate-spin text-primary' : jobStatusColor(currentJob.status)]"
          />
          <span class="text-sm font-medium">{{ t('ui.current_scan_job') }}</span>
          <UBadge :color="jobBadgeColor(currentJob.status)" variant="subtle" size="sm">
            {{ jobStatusLabel(currentJob.status) }}
          </UBadge>
        </div>
      </template>
      <div class="space-y-2">
        <div class="text-xs text-muted flex flex-wrap gap-x-4 gap-y-0.5">
          <span>ID:{{ currentJob.id }}</span>
          <span>{{ t('ui.device_2') }}{{ currentJob.device }}</span>
          <span>{{ currentJob.mode }} / {{ currentJob.resolution }}dpi / {{ currentJob.format }}</span>
          <span v-if="currentJob.filename">{{ t('ui.file_2') }}{{ currentJob.filename }}</span>
          <span v-if="currentJob.sizeBytes">{{ t('ui.size') }}{{ formatSize(currentJob.sizeBytes) }}</span>
        </div>
        <div v-if="currentJob.error" class="text-sm text-error">{{ translateError(currentJob.error) }}</div>
        <pre v-if="currentJob.log" class="text-xs bg-elevated/60 border border-default rounded p-2 max-h-48 overflow-auto whitespace-pre-wrap">{{ currentJob.log }}</pre>
      </div>
    </UCard>

    <!-- 历史记录 -->
    <div class="flex items-center justify-between gap-2 pt-2">
      <div class="text-sm font-medium flex items-center gap-2">
        <UIcon name="i-lucide-history" class="w-4 h-4" /> {{ t('ui.scan_history') }} </div>
      <UButton variant="ghost" size="xs" icon="i-lucide-refresh-cw" :loading="loadingRecords" @click="loadRecords">{{ t('ui.refresh') }}</UButton>
    </div>

    <div v-if="loadingRecords" class="flex items-center justify-center py-8 text-muted gap-2">
      <UIcon name="i-lucide-loader-circle" class="w-5 h-5 animate-spin" /> {{ t('ui.loading') }} </div>
    <div v-else-if="!records.length" class="text-center py-12 text-muted">
      <UIcon name="i-lucide-scan-line" class="w-10 h-10 mx-auto mb-2 opacity-40" />
      <div>{{ t('ui.no_scan_history_yet') }}</div>
    </div>
    <div v-else class="space-y-2">
      <UCard v-for="rec in records" :key="rec.id" class="overflow-hidden">
        <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
          <div class="min-w-0 space-y-1">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="font-medium truncate">{{ rec.filename }}</span>
              <UBadge :color="jobBadgeColor(rec.status)" variant="subtle" size="sm">{{ jobStatusLabel(rec.status) }}</UBadge>
              <UBadge color="neutral" variant="outline" size="sm">{{ rec.format.toUpperCase() }}</UBadge>
            </div>
            <div class="text-xs text-muted flex flex-wrap gap-x-4 gap-y-0.5">
              <span>{{ rec.username }}</span>
              <span>{{ rec.device }}</span>
              <span>{{ rec.mode }} / {{ rec.resolution }}dpi</span>
              <span v-if="rec.sizeBytes">{{ formatSize(rec.sizeBytes) }}</span>
              <span>{{ formatTime(rec.createdAt) }}</span>
            </div>
            <div v-if="rec.errMsg" class="text-xs text-error truncate">{{ t('ui.error') }}{{ translateError(rec.errMsg) }}</div>
          </div>
          <div class="flex items-center gap-1 flex-shrink-0">
            <UButton
              v-if="rec.status === 'succeeded'"
              size="xs"
              variant="ghost"
              icon="i-lucide-download"
              @click="download(rec)"
            >{{ t('ui.download') }}</UButton>
            <UButton
              size="xs"
              variant="ghost"
              color="error"
              icon="i-lucide-trash-2"
              @click="confirmDelete(rec)"
            >{{ t('ui.delete') }}</UButton>
          </div>
        </div>
      </UCard>
    </div>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { apiFetch, readError } from '../utils/api'
import { translateError } from '../utils/translate-error.js'

const { t, locale, n } = useI18n()

const emit = defineEmits(['logout'])
const toast = useToast()

const devices = ref([])
const loadingDevices = ref(false)
// deviceProbes:每台设备的探测状态缓存,key 是设备 URI。
// state: 'probing' | 'healthy' | 'unhealthy'。同一台设备重复选中时命中缓存不再重复探。
const deviceProbes = ref({})
const records = ref([])
const loadingRecords = ref(false)
const currentJob = ref(null)
const starting = ref(false)
let pollTimer = null

const form = ref({
  device: '',
  mode: 'Color',
  resolution: '300',
  source: '',
  format: 'png',
  filename: ''
})

const modeItems = computed(() => ([
  { value: 'Color', label: t('ui.colour_color') },
  { value: 'Gray', label: t('ui.greyscale_gray') },
  { value: 'Lineart', label: t('ui.black_and_white_line_art_lineart') }
]))
// 分辨率默认档位:/api/scan/options 拉取失败或设备未上报 resolution 时兜底。
const DEFAULT_RESOLUTION_ITEMS = computed(() => ([
  { value: '75', label: '75 dpi' },
  { value: '100', label: '100 dpi' },
  { value: '150', label: '150 dpi' },
  { value: '200', label: '200 dpi' },
  { value: '300', label: t('ui.option_300_dpi_recommended') },
  { value: '600', label: t('ui.option_600_dpi_high_quality') }
]))
// 设备上报档位优先(issue #114):list 型直接用枚举值(如 M1005 含 1200),
// range 型由 min/default/max 生成;末尾始终追加「自定义…」。
const deviceResolutionItems = ref(null)
const resolutionSelectItems = computed(() => [
  ...(deviceResolutionItems.value
    ? deviceResolutionItems.value.map(item => ({ ...item, label: item.isDefault ? t('ui.dpi_device_default', { p0: item.value }) : `${item.value} dpi` }))
    : DEFAULT_RESOLUTION_ITEMS.value),
  { value: 'custom', label: t('ui.custom') }
])
// 选「自定义…」时的数值输入;前端按后端 parseResolution 同样的 50..4800 校验。
const customResolution = ref('')
const formatItems = computed(() => ([
  { value: 'png', label: t('ui.png_lossless_bitmap') },
  { value: 'jpeg', label: t('ui.jpeg_smaller_bitmap') },
  { value: 'pdf', label: t('ui.pdf_single_page_lossless') }
]))
// 动态从 /api/scan/options 拉;后端返回不足时用默认单项 Flatbed 兜底。
const deviceSources = ref([])
const sourceItems = computed(() => deviceSources.value.length
  ? deviceSources.value.map(value => ({ value, label: value }))
  : [{ value: '', label: t('ui.default') }])

const deviceItems = computed(() =>
  devices.value.map((d) => ({
    value: d.name,
    label: d.model ? `${d.vendor || ''} ${d.model}`.trim() + ` — ${d.name}` : d.name
  }))
)

const currentProbe = computed(() => {
  const dev = form.value.device
  if (!dev) return null
  return deviceProbes.value[dev] || null
})

function formatSize(bytes) {
  if (!bytes) return ''
  const units = ['B', 'KB', 'MB', 'GB']
  let size = bytes
  let i = 0
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++ }
  return `${n(size, { minimumFractionDigits: 1, maximumFractionDigits: 1 })} ${units[i]}`
}
function formatTime(iso) {
  if (!iso) return '—'
  try {
    const date = new Date(iso)
    return Number.isNaN(date.getTime()) ? iso : date.toLocaleString(locale.value)
  } catch { return iso }
}
function jobStatusLabel(s) {
  return {
    running: t('ui.scanning_2'),
    succeeded: t('ui.completed'),
    failed: t('ui.failed'),
    cancelled: t('ui.cancelled')
  }[s] || s
}
function jobBadgeColor(s) {
  return {
    running: 'primary',
    succeeded: 'success',
    failed: 'error',
    cancelled: 'warning'
  }[s] || 'neutral'
}
function jobStatusIcon(s) {
  return {
    running: 'i-lucide-loader-circle',
    succeeded: 'i-lucide-check-circle',
    failed: 'i-lucide-x-circle',
    cancelled: 'i-lucide-slash'
  }[s] || 'i-lucide-circle'
}
function jobStatusColor(s) {
  return {
    succeeded: 'text-success',
    failed: 'text-error',
    cancelled: 'text-warning'
  }[s] || 'text-muted'
}

async function loadDevices(force = false) {
  loadingDevices.value = true
  // force=true 时同步清掉旧的探测缓存——用户手动刷新通常是"设备接线/开关刚变过",
  // 老状态可能已经不准了。挂载时的默认加载走服务端缓存,不清 probe。
  if (force) deviceProbes.value = {}
  try {
    // 挂载时默认走服务端 TTL 缓存(issue #111 复测反馈:每次真调 scanimage -L 要 ~17s);
    // 点【刷新设备】按钮时才 force=1 打穿缓存重新发现。
    const url = force ? '/api/scan/devices?force=1' : '/api/scan/devices'
    const resp = await apiFetch(url, {}, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    const data = await resp.json()
    devices.value = data.devices || []
    if (devices.value.length && !form.value.device) {
      form.value.device = devices.value[0].name
      await loadOptions(form.value.device)
      probeDevice(form.value.device)
    }
  } catch (e) {
    toast.add({ title: t('ui.could_not_load_devices'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    loadingDevices.value = false
  }
}

async function probeDevice(device) {
  if (!device) return
  // 命中缓存:已 healthy/unhealthy 的设备不重复探。用户可以刷新设备列表来清空。
  const cached = deviceProbes.value[device]
  if (cached && cached.state !== 'probing') return
  deviceProbes.value = { ...deviceProbes.value, [device]: { state: 'probing', detail: '' } }
  try {
    const resp = await apiFetch(
      `/api/scan/devices/probe?device=${encodeURIComponent(device)}`,
      {},
      () => emit('logout')
    )
    if (!resp.ok) {
      // 接口级失败(设备已消失、scanimage 异常等):当作 unhealthy 提示,不弹 toast。
      const detail = await readError(resp)
      deviceProbes.value = { ...deviceProbes.value, [device]: { state: 'unhealthy', detail } }
      return
    }
    const data = await resp.json()
    deviceProbes.value = {
      ...deviceProbes.value,
      [device]: {
        state: data.healthy ? 'healthy' : 'unhealthy',
        detail: data.detail || ''
      }
    }
  } catch (e) {
    deviceProbes.value = {
      ...deviceProbes.value,
      [device]: { state: 'unhealthy', detail: e.message || t('ui.network_error') }
    }
  }
}

async function loadOptions(device) {
  if (!device) return
  try {
    const resp = await apiFetch(`/api/scan/options?device=${encodeURIComponent(device)}`, {}, () => emit('logout'))
    if (!resp.ok) return
    const data = await resp.json()
    const opts = data.options || {}
    // source 只有该设备真的有多种(如平板 + 送稿器)时才展示,否则保持默认单项。
    if (opts.source && opts.source.type === 'list' && opts.source.values?.length > 1) {
      deviceSources.value = opts.source.values
      form.value.source = opts.source.default || opts.source.values[0]
    } else {
      deviceSources.value = []
      form.value.source = ''
    }
    // 分辨率用设备上报档位(issue #114):list 型直接用枚举值,range 型取
    // min/default/常用值/max 生成;拿不到或解析失败时回落默认档位。
    const ro = opts.resolution
    if (ro && ro.type === 'list' && ro.values?.length) {
      deviceResolutionItems.value = ro.values.map((v) => ({
        value: v,
        isDefault: v === ro.default
      }))
    } else if (ro && ro.type === 'range') {
      const min = Number(ro.min)
      const max = Number(ro.max)
      const def = Number(ro.default)
      if (Number.isFinite(min) && Number.isFinite(max) && max > min) {
        const picks = [...new Set(
          [min, def, 150, 300, 600, max].filter((v) => Number.isFinite(v) && v >= min && v <= max)
        )].sort((a, b) => a - b)
        deviceResolutionItems.value = picks.map((v) => ({
          value: String(v),
          isDefault: v === def
        }))
      }
    } else {
      deviceResolutionItems.value = null
    }
    // 换设备后当前选中值可能不在新档位里,回落到设备默认或首项;自定义选择不受影响
    const values = (deviceResolutionItems.value || DEFAULT_RESOLUTION_ITEMS.value).map((i) => i.value)
    if (form.value.resolution !== 'custom' && !values.includes(form.value.resolution)) {
      form.value.resolution = (ro && values.includes(String(ro.default))) ? String(ro.default) : values[0]
    }
  } catch (e) {
    // 参数加载属可降级,静默失败即可
  }
}

async function loadRecords() {
  loadingRecords.value = true
  try {
    const resp = await apiFetch('/api/scan/records', {}, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    const data = await resp.json()
    records.value = data.records || []
  } catch (e) {
    toast.add({ title: t('ui.could_not_load_records'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    loadingRecords.value = false
  }
}

async function startScan() {
  if (!form.value.device) { toast.add({ title: t('ui.select_a_device'), color: 'warning' }); return }
  // 自定义 dpi:与后端 parseResolution 相同区间校验,非法时阻止提交(issue #114)
  let resolution = form.value.resolution
  if (resolution === 'custom') {
    const n = parseInt(customResolution.value, 10)
    if (!Number.isFinite(n) || n < 50 || n > 4800) {
      toast.add({ title: t('ui.invalid_custom_resolution'), description: t('ui.enter_a_whole_number_from_50_to_4800'), color: 'warning' })
      return
    }
    resolution = String(n)
  }
  starting.value = true
  try {
    const resp = await apiFetch('/api/scan/jobs', {
      method: 'POST',
      body: JSON.stringify({
        device: form.value.device,
        mode: form.value.mode,
        resolution,
        source: form.value.source,
        format: form.value.format,
        filename: form.value.filename
      })
    }, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    const data = await resp.json()
    // 立即建立本地占位视图,轮询会补齐真正的 status / log。
    currentJob.value = {
      id: data.jobId,
      device: form.value.device,
      mode: form.value.mode,
      resolution: parseInt(resolution, 10),
      format: form.value.format,
      filename: data.filename,
      status: 'running',
      log: '',
      error: '',
      sizeBytes: 0
    }
    pollJob(data.jobId)
    toast.add({ title: t('ui.scan_started'), color: 'success', icon: 'i-lucide-check-circle' })
  } catch (e) {
    toast.add({ title: t('ui.scan_failed'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    starting.value = false
  }
}

function pollJob(jobId) {
  clearTimer()
  const tick = async () => {
    try {
      const resp = await apiFetch(`/api/scan/jobs/${jobId}`, {}, () => emit('logout'))
      if (resp.ok) {
        currentJob.value = await resp.json()
        if (currentJob.value.status !== 'running') {
          clearTimer()
          await loadRecords()
          return
        }
      } else if (resp.status === 404) {
        clearTimer()
        return
      }
    } catch (e) {
      // 网络抖动,不停止轮询
    }
    pollTimer = setTimeout(tick, 1500)
  }
  pollTimer = setTimeout(tick, 500)
}

function clearTimer() {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
}

async function cancelCurrentJob() {
  if (!currentJob.value) return
  try {
    const resp = await apiFetch(`/api/scan/jobs/${currentJob.value.id}`, { method: 'DELETE' }, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    toast.add({ title: t('ui.cancellation_requested'), color: 'warning' })
  } catch (e) {
    toast.add({ title: t('ui.could_not_cancel'), description: e.message, color: 'error' })
  }
}

async function download(rec) {
  // 下载走 <a>,而不是 fetch,方便浏览器直接触发保存对话框。
  const a = document.createElement('a')
  a.href = `/api/scan/records/${rec.id}/file`
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
}

async function confirmDelete(rec) {
  if (!confirm(t('ui.delete_scan_the_file_and_record_will_be_permanently_removed', { p0: rec.filename }))) return
  try {
    const resp = await apiFetch(`/api/scan/records/${rec.id}`, { method: 'DELETE' }, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    toast.add({ title: t('ui.deleted'), color: 'success', icon: 'i-lucide-check-circle' })
    await loadRecords()
  } catch (e) {
    toast.add({ title: t('ui.could_not_delete'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  }
}

// 设备切换时刷新参数(可能改变可用的 source),并异步探一次可用性(issue #111 复测反馈)
watch(() => form.value.device, (dev) => {
  loadOptions(dev)
  probeDevice(dev)
})

onMounted(async () => {
  await Promise.all([loadDevices(), loadRecords()])
})
onBeforeUnmount(() => {
  clearTimer()
})
</script>
