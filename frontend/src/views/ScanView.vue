<template>
  <div class="p-3 sm:p-4 md:p-6 max-w-5xl mx-auto space-y-4">
    <div class="flex items-center justify-between gap-2 flex-wrap">
      <h1 class="text-lg font-bold flex items-center gap-2">
        <UIcon name="i-lucide-scan-line" class="w-5 h-5 text-primary" />
        扫描
      </h1>
      <div class="flex items-center gap-2">
        <UButton
          variant="ghost"
          size="xs"
          icon="i-lucide-refresh-cw"
          :loading="loadingDevices"
          @click="loadDevices(true)"
        >刷新设备</UButton>
      </div>
    </div>

    <UAlert
      color="neutral"
      variant="soft"
      icon="i-lucide-info"
      title="使用说明"
      description="AIO 镜像自带 scanimage 与多种 SANE 后端。zeroconf 类零配置后端(如 hpljm1005: / escl:)通常可直接工作;同一台设备若同时列出 hpaio: 与 hpljm1005:,优先选后者。选中设备后系统会做一次快速可用性探测,若打开失败请换其它后端再试。PDF 会先扫成 PNG,再用 img2pdf 无损嵌入合成。"
    />

    <UCard>
      <template #header>
        <div class="flex items-center gap-2 text-sm font-medium">
          <UIcon name="i-lucide-settings-2" class="w-4 h-4" />
          扫描参数
        </div>
      </template>
      <div class="space-y-3">
        <UFormField label="设备" required>
          <div class="flex gap-2">
            <USelect
              v-model="form.device"
              :items="deviceItems"
              value-key="value"
              label-key="label"
              placeholder="请先刷新设备列表"
              class="flex-1"
              icon="i-lucide-scanner"
            />
          </div>
          <template #hint>
            <span v-if="!devices.length && !loadingDevices" class="text-warning">
              未检测到设备。请确认扫描仪已开机、USB 已连接,或宿主机已挂 --device=/dev/bus/usb。
            </span>
            <span v-else-if="loadingDevices">检测中…</span>
            <span v-else-if="currentProbe?.state === 'probing'" class="text-muted">
              正在探测该后端是否可打开…
            </span>
            <span v-else-if="currentProbe?.state === 'healthy'" class="text-success">
              ✓ 该后端可打开
            </span>
            <span v-else-if="currentProbe?.state === 'unhealthy'" class="text-error">
              该后端无法打开{{ currentProbe.detail ? ':' + currentProbe.detail : '' }}。请换其它后端(如 hpljm1005: / escl:)再试。
            </span>
          </template>
        </UFormField>

        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <UFormField label="模式">
            <USelect v-model="form.mode" :items="modeItems" value-key="value" label-key="label" />
          </UFormField>
          <UFormField label="分辨率(dpi)">
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
          <UFormField label="输出格式">
            <USelect v-model="form.format" :items="formatItems" value-key="value" label-key="label" />
          </UFormField>
        </div>

        <UFormField v-if="sourceItems.length > 1" label="来源">
          <USelect v-model="form.source" :items="sourceItems" value-key="value" label-key="label" />
          <template #hint>MVP 单页扫描,ADF/双面暂未支持</template>
        </UFormField>

        <UFormField label="文件名(可选)">
          <UInput v-model="form.filename" placeholder="scan(留空自动生成)" />
          <template #hint>最终为 <code>&lt;文件名&gt;-&lt;时间戳&gt;.&lt;扩展名&gt;</code>,以避免覆盖</template>
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
            {{ currentJob?.status === 'running' ? '扫描中…' : '开始扫描' }}
          </UButton>
          <UButton
            v-if="currentJob?.status === 'running'"
            size="sm"
            color="error"
            variant="ghost"
            icon="i-lucide-x"
            @click="cancelCurrentJob"
          >取消</UButton>
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
          <span class="text-sm font-medium">当前扫描任务</span>
          <UBadge :color="jobBadgeColor(currentJob.status)" variant="subtle" size="sm">
            {{ jobStatusLabel(currentJob.status) }}
          </UBadge>
        </div>
      </template>
      <div class="space-y-2">
        <div class="text-xs text-muted flex flex-wrap gap-x-4 gap-y-0.5">
          <span>ID:{{ currentJob.id }}</span>
          <span>设备:{{ currentJob.device }}</span>
          <span>{{ currentJob.mode }} / {{ currentJob.resolution }}dpi / {{ currentJob.format }}</span>
          <span v-if="currentJob.filename">文件:{{ currentJob.filename }}</span>
          <span v-if="currentJob.sizeBytes">大小:{{ formatSize(currentJob.sizeBytes) }}</span>
        </div>
        <div v-if="currentJob.error" class="text-sm text-error">{{ currentJob.error }}</div>
        <pre v-if="currentJob.log" class="text-xs bg-elevated/60 border border-default rounded p-2 max-h-48 overflow-auto whitespace-pre-wrap">{{ currentJob.log }}</pre>
      </div>
    </UCard>

    <!-- 历史记录 -->
    <div class="flex items-center justify-between gap-2 pt-2">
      <div class="text-sm font-medium flex items-center gap-2">
        <UIcon name="i-lucide-history" class="w-4 h-4" />
        扫描历史
      </div>
      <UButton variant="ghost" size="xs" icon="i-lucide-refresh-cw" :loading="loadingRecords" @click="loadRecords">刷新</UButton>
    </div>

    <div v-if="loadingRecords" class="flex items-center justify-center py-8 text-muted gap-2">
      <UIcon name="i-lucide-loader-circle" class="w-5 h-5 animate-spin" />
      加载中…
    </div>
    <div v-else-if="!records.length" class="text-center py-12 text-muted">
      <UIcon name="i-lucide-scan-line" class="w-10 h-10 mx-auto mb-2 opacity-40" />
      <div>还没有扫描记录</div>
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
            <div v-if="rec.errMsg" class="text-xs text-error truncate">错误:{{ rec.errMsg }}</div>
          </div>
          <div class="flex items-center gap-1 flex-shrink-0">
            <UButton
              v-if="rec.status === 'succeeded'"
              size="xs"
              variant="ghost"
              icon="i-lucide-download"
              @click="download(rec)"
            >下载</UButton>
            <UButton
              size="xs"
              variant="ghost"
              color="error"
              icon="i-lucide-trash-2"
              @click="confirmDelete(rec)"
            >删除</UButton>
          </div>
        </div>
      </UCard>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { apiFetch, readError } from '../utils/api'

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

const modeItems = [
  { value: 'Color', label: '彩色 (Color)' },
  { value: 'Gray', label: '灰度 (Gray)' },
  { value: 'Lineart', label: '黑白线稿 (Lineart)' }
]
// 分辨率默认档位:/api/scan/options 拉取失败或设备未上报 resolution 时兜底。
const DEFAULT_RESOLUTION_ITEMS = [
  { value: '75', label: '75 dpi' },
  { value: '100', label: '100 dpi' },
  { value: '150', label: '150 dpi' },
  { value: '200', label: '200 dpi' },
  { value: '300', label: '300 dpi (推荐)' },
  { value: '600', label: '600 dpi (高质量)' }
]
// 设备上报档位优先(issue #114):list 型直接用枚举值(如 M1005 含 1200),
// range 型由 min/default/max 生成;末尾始终追加「自定义…」。
const deviceResolutionItems = ref(DEFAULT_RESOLUTION_ITEMS)
const resolutionSelectItems = computed(() => [
  ...deviceResolutionItems.value,
  { value: 'custom', label: '自定义…' }
])
// 选「自定义…」时的数值输入;前端按后端 parseResolution 同样的 50..4800 校验。
const customResolution = ref('')
const formatItems = [
  { value: 'png', label: 'PNG(位图,无损)' },
  { value: 'jpeg', label: 'JPEG(位图,较小)' },
  { value: 'pdf', label: 'PDF(单页,无损嵌入)' }
]
// 动态从 /api/scan/options 拉;后端返回不足时用默认单项 Flatbed 兜底。
const sourceItems = ref([{ value: '', label: '(默认)' }])

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
  let n = bytes
  let i = 0
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(1)} ${units[i]}`
}
function formatTime(iso) {
  if (!iso) return '—'
  try { return new Date(iso).toLocaleString() } catch { return iso }
}
function jobStatusLabel(s) {
  return {
    running: '扫描中',
    succeeded: '已完成',
    failed: '失败',
    cancelled: '已取消'
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
    toast.add({ title: '设备加载失败', description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
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
      [device]: { state: 'unhealthy', detail: e.message || '网络异常' }
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
      sourceItems.value = opts.source.values.map((v) => ({ value: v, label: v }))
      form.value.source = opts.source.default || opts.source.values[0]
    } else {
      sourceItems.value = [{ value: '', label: '(默认)' }]
      form.value.source = ''
    }
    // 分辨率用设备上报档位(issue #114):list 型直接用枚举值,range 型取
    // min/default/常用值/max 生成;拿不到或解析失败时回落默认档位。
    const ro = opts.resolution
    if (ro && ro.type === 'list' && ro.values?.length) {
      deviceResolutionItems.value = ro.values.map((v) => ({
        value: v,
        label: v === ro.default ? `${v} dpi (设备默认)` : `${v} dpi`
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
          label: v === def ? `${v} dpi (设备默认)` : `${v} dpi`
        }))
      }
    } else {
      deviceResolutionItems.value = DEFAULT_RESOLUTION_ITEMS
    }
    // 换设备后当前选中值可能不在新档位里,回落到设备默认或首项;自定义选择不受影响
    const values = deviceResolutionItems.value.map((i) => i.value)
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
    toast.add({ title: '记录加载失败', description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    loadingRecords.value = false
  }
}

async function startScan() {
  if (!form.value.device) { toast.add({ title: '请选择设备', color: 'warning' }); return }
  // 自定义 dpi:与后端 parseResolution 相同区间校验,非法时阻止提交(issue #114)
  let resolution = form.value.resolution
  if (resolution === 'custom') {
    const n = parseInt(customResolution.value, 10)
    if (!Number.isFinite(n) || n < 50 || n > 4800) {
      toast.add({ title: '自定义 dpi 非法', description: '请输入 50 - 4800 之间的整数', color: 'warning' })
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
    toast.add({ title: '扫描已开始', color: 'success', icon: 'i-lucide-check-circle' })
  } catch (e) {
    toast.add({ title: '扫描失败', description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
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
    toast.add({ title: '已请求取消', color: 'warning' })
  } catch (e) {
    toast.add({ title: '取消失败', description: e.message, color: 'error' })
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
  if (!confirm(`确认删除扫描件「${rec.filename}」?文件与记录都会被清理,无法恢复。`)) return
  try {
    const resp = await apiFetch(`/api/scan/records/${rec.id}`, { method: 'DELETE' }, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    toast.add({ title: '已删除', color: 'success', icon: 'i-lucide-check-circle' })
    await loadRecords()
  } catch (e) {
    toast.add({ title: '删除失败', description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
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
