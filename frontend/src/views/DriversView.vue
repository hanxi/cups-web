<template>
  <div class="p-3 sm:p-4 md:p-6 space-y-4 md:space-y-6">
    <!-- 自动检测打印机 -->
    <UCard>
      <template #header>
        <h2 class="text-xl font-bold flex items-center gap-2">
          <UIcon name="i-lucide-scan-search" class="w-5 h-5" />
          Автоматический поиск
        </h2>
      </template>
      <div class="space-y-4">
        <UButton
          icon="i-lucide-scan-search"
          :loading="scanning"
          :disabled="scanning"
          @click="detectPrinters"
        >
          Сканировать
        </UButton>
        <div v-if="scanning" class="flex items-center gap-2 text-sm text-muted">
          <UIcon name="i-lucide-loader-circle" class="w-4 h-4 animate-spin" />
          Сканирование, пожалуйста, подождите...
        </div>
        <div v-if="detected.length" class="overflow-x-auto">
          <UTable :columns="detectColumns" :data="detected">
            <template #connection-cell="{ row }">
              <div class="flex items-center gap-1">
                <UIcon
                  :name="row.original.connection === 'usb' ? 'i-lucide-usb' : 'i-lucide-wifi'"
                  class="w-4 h-4"
                />
                <span>{{ row.original.connection === 'usb' ? 'USB' : 'Сеть' }}</span>
              </div>
            </template>
            <template #printer-cell="{ row }">
              <div>{{ printerLabel(row.original) }}</div>
              <div class="text-xs text-muted truncate max-w-xs">{{ row.original.deviceUri }}</div>
            </template>
            <template #driverStatus-cell="{ row }">
              <div class="flex items-center gap-1 flex-wrap">
                <UBadge v-if="row.original.existingQueue" color="success" variant="subtle" size="sm">
                  Добавлено: {{ row.original.existingQueue }}
                </UBadge>
                <UBadge v-if="row.original.driverState === 'ready'" color="success" size="sm">
                  <UTooltip :text="row.original.topCandidate?.makeAndModel || ''">Драйвер готов</UTooltip>
                </UBadge>
                <UBadge v-else-if="row.original.driverState === 'driverless'" color="primary" size="sm">
                  IPP Everywhere
                </UBadge>
                <UBadge v-else-if="row.original.driverState === 'needsVendorDriver'" color="warning" size="sm">
                  Нужен драйвер: {{ row.original.driverMatch?.displayName || 'Драйвер' }}
                </UBadge>
                <UBadge v-else-if="row.original.driverState === 'unmatched'" color="error" variant="subtle" size="sm">
                  Драйвер не найден
                </UBadge>
                <!-- 兼容旧后端（无 driverState 字段时退回三态） -->
                <template v-if="!row.original.driverState">
                  <UBadge v-if="row.original.hasDriver" color="success" size="sm">Готов</UBadge>
                  <UBadge v-else-if="row.original.driverMatch" color="warning" size="sm">
                    Рекомендуется: {{ row.original.driverMatch.displayName }}
                  </UBadge>
                  <UBadge v-else color="neutral" size="sm">Неизвестно</UBadge>
                </template>
              </div>
            </template>
            <template #actions-cell="{ row }">
              <!-- 已添加队列：禁用操作 -->
              <UTooltip v-if="row.original.existingQueue" text="Для перенастройки удалите очередь в CUPS">
                <UButton size="sm" icon="i-lucide-check" color="neutral" variant="outline" disabled>
                  Добавлено
                </UButton>
              </UTooltip>
              <!-- 推荐的驱动在当前架构上不可用 -->
              <UTooltip
                v-else-if="row.original.driverMatch && !row.original.hasDriver && !archSupported(row.original.driverMatch.arch)"
                :text="`Архитектура ${currentArch} не поддерживается`"
              >
                <UButton size="sm" icon="i-lucide-ban" color="neutral" variant="outline" disabled>
                  Архитектура!
                </UButton>
              </UTooltip>
              <!-- 需安装厂商驱动 -->
              <UButton
                v-else-if="row.original.driverMatch && !row.original.hasDriver"
                size="sm"
                icon="i-lucide-download"
                :loading="settingUp === row.original.deviceUri"
                :disabled="busy"
                @click="openPPDModal(row.original)"
              >
                Установить и добавить
              </UButton>
              <!-- 未匹配到驱动：手动选择 -->
              <UButton
                v-else-if="row.original.driverState === 'unmatched'"
                size="sm"
                variant="outline"
                icon="i-lucide-search"
                :loading="settingUp === row.original.deviceUri"
                :disabled="busy"
                @click="openPPDModal(row.original)"
              >
                Выбрать вручную
              </UButton>
              <!-- 已就绪 / driverless：直接添加 -->
              <UButton
                v-else
                size="sm"
                variant="outline"
                icon="i-lucide-plus"
                :loading="settingUp === row.original.deviceUri"
                :disabled="busy"
                @click="openPPDModal(row.original)"
              >
                Добавить принтер
              </UButton>
            </template>
          </UTable>
        </div>
        <div v-else-if="scanDone && !detected.length" class="text-sm text-muted">
          Принтеры не найдены. Проверьте подключение.
        </div>
      </div>
    </UCard>

    <!-- 后台任务进度：安装/卸载/一键设置都是异步任务，这里展示实时日志 -->
    <UCard v-if="jobTitle">
      <template #header>
        <div class="flex items-center justify-between gap-2">
          <h2 class="text-base font-semibold flex items-center gap-2">
            <UIcon
              :name="jobRunning ? 'i-lucide-loader-circle' : (jobFailed ? 'i-lucide-x-circle' : 'i-lucide-check-circle')"
              :class="['w-5 h-5', jobRunning && 'animate-spin', jobFailed && 'text-error']"
            />
            {{ jobTitle }}
          </h2>
          <UButton
            size="xs"
            variant="ghost"
            :icon="jobLogOpen ? 'i-lucide-chevron-up' : 'i-lucide-chevron-down'"
            @click="jobLogOpen = !jobLogOpen"
          >
            {{ jobLogOpen ? 'Скрыть логи' : 'Показать логи' }}
          </UButton>
        </div>
      </template>
      <div class="space-y-2">
        <p v-if="jobRunning" class="text-sm text-muted">
          Сборка драйвера может занять несколько минут. Не закрывайте страницу.
        </p>
        <pre
          v-if="jobLogOpen"
          class="text-xs bg-elevated rounded p-3 max-h-64 overflow-auto whitespace-pre-wrap break-all"
        >{{ jobLog || '(нет данных)' }}</pre>
      </div>
    </UCard>

    <!-- 驱动管理 -->
    <UCard>
      <template #header>
        <div class="flex items-center justify-between gap-2 flex-wrap">
          <h2 class="text-xl font-bold flex items-center gap-2">
            <UIcon name="i-lucide-puzzle" class="w-5 h-5" />
            Управление драйверами
          </h2>
          <UBadge v-if="currentArch" color="neutral" variant="subtle" size="sm">
            Архитектура: {{ currentArch }}
          </UBadge>
        </div>
      </template>
      <div class="overflow-x-auto">
        <UTable :columns="driverColumns" :data="drivers">
          <template #description-cell="{ row }">
            <span>{{ row.original.description }}</span>
            <UBadge v-if="row.original.needCompile" color="warning" size="xs" class="ml-1">Нужна сборка</UBadge>
          </template>
          <template #arch-cell="{ row }">
            {{ (row.original.arch || []).join(', ') }}
          </template>
          <template #status-cell="{ row }">
            <div class="space-y-1">
              <UBadge v-if="row.original.installed" color="success" size="sm">
                Установлено {{ row.original.installedAt ? formatDate(row.original.installedAt) : '' }}
              </UBadge>
              <UBadge v-else color="neutral" size="sm">Не установлено</UBadge>
              <!-- 驱动数据是挂载卷，换机器时可能与当前架构不符，必须提示重装 -->
              <UBadge
                v-if="row.original.installed && row.original.installedArch && currentArch && row.original.installedArch !== currentArch"
                color="warning"
                size="xs"
              >
                Установлено для {{ row.original.installedArch }}, не совпадает с текущей, рекомендуется переустановить
              </UBadge>
            </div>
          </template>
          <template #actions-cell="{ row }">
            <UTooltip
              v-if="!row.original.installed && row.original.supported === false"
              :text="`Архитектура ${currentArch} не поддерживается`"
            >
              <UButton size="sm" icon="i-lucide-ban" color="neutral" variant="outline" disabled>
                Установить
              </UButton>
            </UTooltip>
            <UTooltip
              v-else-if="!row.original.installed && row.original.hasScript === false"
              text="Отсутствует скрипт установки"
            >
              <UButton size="sm" icon="i-lucide-ban" color="neutral" variant="outline" disabled>
                Установить
              </UButton>
            </UTooltip>
            <UButton
              v-else-if="!row.original.installed"
              size="sm"
              icon="i-lucide-download"
              :loading="installingDriver === row.original.name"
              :disabled="busy"
              @click="confirmInstall(row.original)"
            >
              Установить
            </UButton>
            <UButton
              v-else
              size="sm"
              variant="outline"
              color="error"
              icon="i-lucide-trash-2"
              :loading="removingDriver === row.original.name"
              :disabled="busy"
              @click="confirmRemove(row.original)"
            >
              Удалить
            </UButton>
          </template>
        </UTable>
      </div>
    </UCard>

    <!-- 上传自定义驱动 -->
    <UCard>
      <template #header>
        <h2 class="text-xl font-bold flex items-center gap-2">
          <UIcon name="i-lucide-upload" class="w-5 h-5" />
          Загрузить драйвер
        </h2>
      </template>
      <div class="space-y-3">
        <p class="text-sm text-muted">Поддерживаются PPD (.ppd) и Debian (.deb)</p>
        <div class="flex flex-wrap items-center gap-3">
          <UButton variant="outline" icon="i-lucide-file-up" @click="triggerFileInput">
            Выбрать файл
          </UButton>
          <span v-if="uploadFile" class="text-sm text-muted truncate max-w-xs">{{ uploadFile.name }}</span>
          <input
            ref="fileInputRef"
            type="file"
            accept=".ppd,.deb"
            class="hidden"
            @change="onFileSelected"
          />
        </div>
        <UButton
          v-if="uploadFile"
          color="primary"
          icon="i-lucide-upload"
          :loading="uploading"
          :disabled="uploading || busy"
          @click="uploadDriver"
        >
          Загрузить и установить
        </UButton>

        <!-- .deb 无法自动恢复，必须显式告知，不能静默丢失 -->
        <UAlert
          v-if="customDebs.length"
          color="warning"
          variant="subtle"
          icon="i-lucide-triangle-alert"
          title="Загруженные .deb пакеты (нужно переустановить после перезапуска)"
        >
          <template #description>
            <p class="mb-1">{{ customDebNotice }}</p>
            <ul class="list-disc pl-5">
              <li v-for="pkg in customDebs" :key="pkg.filename">
                {{ pkg.filename }}
                <span v-if="pkg.installedAt" class="text-muted">（{{ formatDate(pkg.installedAt) }}）</span>
              </li>
            </ul>
          </template>
        </UAlert>
      </div>
    </UCard>

    <!-- 安装确认弹窗 -->
    <UModal v-model:open="showInstallModal">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">Подтверждение установки</h3>
          <p>Установить драйвер <strong>{{ pendingDriver?.displayName }}</strong>?</p>
          <p class="text-sm text-muted">Сборка драйвера может занять несколько минут. Не закрывайте страницу.</p>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="showInstallModal = false">Отмена</UButton>
            <UButton color="primary" :loading="!!installingDriver" @click="installDriver">Подтвердить</UButton>
          </div>
        </div>
      </template>
    </UModal>

    <!-- 卸载确认弹窗 -->
    <UModal v-model:open="showRemoveModal">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">Подтверждение удаления</h3>
          <p>Вы уверены, что хотите удалить драйвер <strong>{{ pendingDriver?.displayName }}</strong>?</p>
          <p class="text-sm text-muted">После удаления принтеры с этим драйвером могут перестать работать.</p>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="showRemoveModal = false">Отмена</UButton>
            <UButton color="error" :loading="!!removingDriver" @click="removeDriver">Удалить</UButton>
          </div>
        </div>
      </template>
    </UModal>

    <!-- PPD 候选选择弹窗 -->
    <UModal v-model:open="showPPDModal" :ui="{ width: 'max-w-lg' }">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">Выбор драйвера</h3>
          <!-- 设备摘要 -->
          <div class="text-sm">
            <div class="font-medium">{{ printerLabel(ppdModalPrinter) }}</div>
            <div class="text-xs text-muted truncate">{{ ppdModalPrinter?.deviceUri }}</div>
            <details v-if="ppdModalPrinter?.deviceId" class="mt-1">
              <summary class="text-xs text-muted cursor-pointer">Device ID (для отладки)</summary>
              <code class="text-xs break-all">{{ ppdModalPrinter.deviceId }}</code>
            </details>
          </div>

          <!-- 已有队列警告 -->
          <UAlert
            v-if="ppdModalData?.existingQueue"
            color="warning"
            icon="i-lucide-alert-triangle"
            :title="`Это устройство уже добавлено как ${ppdModalData.existingQueue}, удалите его перед повторным добавлением`"
          />

          <!-- 候选查询失败降级 -->
          <UAlert
            v-if="ppdModalError"
            color="warning"
            icon="i-lucide-alert-triangle"
            title="Ошибка поиска, будет использован автоподбор"
          />

          <!-- 候选加载中 -->
          <div v-if="ppdModalLoading" class="space-y-2">
            <USkeleton v-for="i in 3" :key="i" class="h-12 w-full" />
          </div>

          <!-- 候选列表 -->
          <div v-else-if="ppdModalCandidates.length" class="space-y-1">
            <p class="text-xs text-muted">
              Точное совпадение = совпадает модель; Возможное = частичное совпадение; Универсальный = базовые функции.
            </p>
            <URadioGroup v-model="selectedPPD" :items="ppdRadioItems">
              <template #label="{ item }">
                <div class="flex items-center gap-2 flex-wrap">
                  <span>{{ item.label }}</span>
                  <UBadge v-if="item.raw?.recommended" color="primary" size="xs">Рекомендовано</UBadge>
                  <UBadge
                    :color="item.raw?.confidence === 'high' ? 'success' : item.raw?.confidence === 'medium' ? 'warning' : 'neutral'"
                    size="xs"
                  >
                    {{ item.raw?.confidence === 'high' ? 'Точное' : item.raw?.confidence === 'medium' ? 'Возможное' : 'Универсальный' }}
                  </UBadge>
                  <UBadge v-if="item.raw?.driverdRank >= 1" color="primary" variant="subtle" size="xs">Рекомендовано CUPS</UBadge>
                </div>
                <div class="text-xs text-muted">{{ item.raw?.reason }} · {{ item.raw?.makeAndModel }}</div>
              </template>
            </URadioGroup>

            <!-- IPP Everywhere 选项 -->
            <div class="border-t pt-2 mt-2">
              <UTooltip
                v-if="!ppdModalData?.driverless?.available"
                :text="ppdModalData?.driverless?.reason || 'Не поддерживается'"
              >
                <div class="opacity-50 cursor-not-allowed text-sm">
                  <input type="radio" disabled class="mr-2" />IPP Everywhere (автоматически)
                </div>
              </UTooltip>
              <label v-else class="flex items-center gap-2 text-sm cursor-pointer">
                <input
                  type="radio"
                  :checked="selectedPPD === 'everywhere'"
                  @change="selectedPPD = 'everywhere'"
                />
                IPP Everywhere (автоматически)
              </label>
            </div>

            <!-- 高级选项：raw 队列 -->
            <details class="border-t pt-2 mt-2">
              <summary class="text-xs text-muted cursor-pointer">Расширенные настройки</summary>
              <label class="flex items-center gap-2 text-sm cursor-pointer mt-1">
                <input
                  type="radio"
                  :checked="selectedPPD === '__raw__'"
                  @change="selectedPPD = '__raw__'"
                />
                Без драйвера (raw очередь)
              </label>
              <UAlert
                v-if="selectedPPD === '__raw__'"
                color="error"
                icon="i-lucide-alert-triangle"
                title="Настройки будут недоступны, возможна печать мусора"
                description="Используйте, только если принтер понимает PDF/PostScript напрямую"
                class="mt-2"
              />
            </details>
          </div>

          <!-- 队列名 -->
          <div v-if="!ppdModalData?.existingQueue">
            <label class="text-sm font-medium">Имя очереди</label>
            <UInput v-model="ppdQueueName" class="mt-1" />
          </div>

          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="showPPDModal = false">Отмена</UButton>
            <UButton
              color="primary"
              :disabled="!!ppdModalData?.existingQueue || busy"
              :loading="settingUp === ppdModalPrinter?.deviceUri"
              @click="submitPPDSelection"
            >
              {{ ppdModalError ? 'Продолжить (авто)' : 'Подтвердить и добавить' }}
            </UButton>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { apiFetch, readError } from '../utils/api'

defineProps({ session: Object })
const emit = defineEmits(['logout'])
const toast = useToast()

// 提交请求本身很快（后端立刻 202 返回 jobId），给个短超时即可
const SUBMIT_TIMEOUT = 30000
// 轮询间隔与总时长上限：后端任务硬超时是 30 分钟，前端留一点余量
const POLL_INTERVAL = 2000
const POLL_MAX_MS = 35 * 60 * 1000

// --- 后台任务（安装 / 卸载 / 一键设置统一走 /api/admin/drivers/jobs/{id} 轮询）---
const jobTitle = ref('')
const jobLog = ref('')
const jobLogOpen = ref(true)
const jobRunning = ref(false)
const jobFailed = ref(false)

let pollTimer = null
let unmounted = false

function startJobPanel(title) {
  jobTitle.value = title
  jobLog.value = ''
  jobRunning.value = true
  jobFailed.value = false
  jobLogOpen.value = true
}

function delay(ms) {
  return new Promise((resolve) => {
    pollTimer = setTimeout(resolve, ms)
  })
}

// 轮询任务直到 status !== 'running'，期间持续刷新日志；超时或组件卸载时抛错
async function pollDriverJob(jobId) {
  const deadline = Date.now() + POLL_MAX_MS
  while (Date.now() < deadline) {
    await delay(POLL_INTERVAL)
    if (unmounted) throw new Error('Вы покинули страницу, задача продолжается в фоне')
    const resp = await apiFetch(`/api/admin/drivers/jobs/${jobId}`, {}, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    const job = await resp.json()
    jobLog.value = job.log || ''
    if (job.status !== 'running') return job
  }
  throw new Error('Таймаут (более 35 минут), проверьте логи контейнера')
}
</toolcall_result>

// 提交一个驱动任务并等待其结束，返回最终任务对象；提交失败/任务失败均抛错
async function runDriverJob(url, payload) {
  const resp = await apiFetch(url, {
    method: 'POST',
    body: JSON.stringify(payload),
    signal: AbortSignal.timeout(SUBMIT_TIMEOUT)
  }, () => emit('logout'))

  if (!resp.ok) {
    const msg = await readError(resp)
    // 409：已有驱动任务在跑（apt/dpkg 有全局锁，后端只允许一个任务）
    throw new Error(msg)
  }
  const data = await resp.json()
  if (!data.jobId) throw new Error('服务端未返回任务 ID')

  const job = await pollDriverJob(data.jobId)
  if (job.status !== 'succeeded') {
    throw new Error(job.error || '任务执行失败')
  }
  return job
}

function finishJobPanel(ok) {
  jobRunning.value = false
  jobFailed.value = !ok
}

// --- 自动检测打印机 ---
const scanning = ref(false)
const scanDone = ref(false)
const detected = ref([])
const settingUp = ref(null)

const detectColumns = [
  { id: 'connection', header: '连接方式' },
  { id: 'printer', header: '打印机' },
  { id: 'driverStatus', header: '驱动状态' },
  { id: 'actions', header: '操作' }
]

function printerLabel(printer) {
  const label = `${printer.manufacturer || ''} ${printer.model || ''}`.trim()
  return label || '未知型号'
}

async function detectPrinters() {
  scanning.value = true
  scanDone.value = false
  detected.value = []
  candidatesByUri.value = {} // 重扫后旧候选一定失效
  try {
    const resp = await apiFetch('/api/admin/drivers/detect', {}, () => emit('logout'))
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: '扫描失败', description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      return
    }
    detected.value = (await resp.json()) || []
  } catch (e) {
    toast.add({ title: '扫描失败', description: String(e), color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    scanning.value = false
    scanDone.value = true
  }
}

// 请求体字段名与后端 adminSetupPrinterHandler 严格对齐。
async function setupPrinter(printer, opts = {}) {
  settingUp.value = printer.deviceUri
  startJobPanel(`正在设置打印机 ${printerLabel(printer)}`)
  try {
    const job = await runDriverJob('/api/admin/drivers/setup', {
      deviceUri: printer.deviceUri,
      driverName: printer.hasDriver ? '' : (printer.driverMatch?.name || ''),
      manufacturer: printer.manufacturer || '',
      model: printer.model || '',
      deviceId: printer.deviceId || '',
      ppdUri: opts.ppdUri || '',
      printerName: opts.printerName || '',
      allowRaw: opts.allowRaw || false
    })
    finishJobPanel(true)
    toast.add({
      title: '设置成功',
      description: `打印机 ${job.result?.printerName || printerLabel(printer)} 已添加`,
      color: 'success',
      icon: 'i-lucide-check-circle'
    })
    await Promise.all([detectPrinters(), loadDrivers()])
  } catch (e) {
    finishJobPanel(false)
    toast.add({ title: '设置失败', description: e.message || String(e), color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    settingUp.value = null
  }
}

// --- PPD 候选选择 Modal ---
const showPPDModal = ref(false)
const ppdModalPrinter = ref(null)
const ppdModalData = ref(null)
const ppdModalLoading = ref(false)
const ppdModalError = ref('')
const ppdModalCandidates = ref([])
const selectedPPD = ref('')
const ppdQueueName = ref('')
// 独立状态 map，不挂 row.original（detected 是 ref 数组，给元素加属性易踩响应性坑）
const candidatesByUri = ref({})

const ppdRadioItems = computed(() =>
  ppdModalCandidates.value.map((c) => ({
    value: c.ppd,
    label: c.makeAndModel,
    raw: c
  }))
)

async function openPPDModal(printer) {
  ppdModalPrinter.value = printer
  ppdModalData.value = null
  ppdModalCandidates.value = []
  ppdModalError.value = ''
  selectedPPD.value = ''
  ppdQueueName.value = printer.suggestedName || ''
  showPPDModal.value = true

  // 同一 deviceUri 在本次扫描周期内只查一次
  if (candidatesByUri.value[printer.deviceUri]) {
    applyCandidateData(printer.deviceUri, candidatesByUri.value[printer.deviceUri])
    return
  }

  ppdModalLoading.value = true
  try {
    const params = new URLSearchParams({
      deviceUri: printer.deviceUri,
      deviceId: printer.deviceId || '',
      manufacturer: printer.manufacturer || '',
      model: printer.model || '',
      limit: '8'
    })
    const resp = await apiFetch(
      `/api/admin/drivers/ppds?${params}`,
      { signal: AbortSignal.timeout(20000) },
      () => emit('logout')
    )
    if (!resp.ok) {
      const msg = await readError(resp)
      if (resp.status === 429) {
        ppdModalError.value = '候选查询繁忙，请稍后重试'
      } else {
        ppdModalError.value = msg
      }
      return
    }
    const data = await resp.json()
    candidatesByUri.value[printer.deviceUri] = data
    applyCandidateData(printer.deviceUri, data)
  } catch (e) {
    ppdModalError.value = String(e)
  } finally {
    ppdModalLoading.value = false
  }
}

function applyCandidateData(uri, data) {
  ppdModalData.value = data
  ppdModalCandidates.value = data.candidates || []
  ppdQueueName.value = data.suggestedName || ppdQueueName.value
  // 默认选中 recommended 项，没有则选第一条
  const rec = ppdModalCandidates.value.find((c) => c.recommended)
  selectedPPD.value = rec ? rec.ppd : (ppdModalCandidates.value[0]?.ppd || '')
}

async function submitPPDSelection() {
  const printer = ppdModalPrinter.value
  if (!printer) return
  showPPDModal.value = false

  const opts = {
    ppdUri: ppdModalError.value ? '' : selectedPPD.value,
    printerName: ppdQueueName.value,
    allowRaw: selectedPPD.value === '__raw__'
  }
  await setupPrinter(printer, opts)
}

// --- 驱动管理 ---
const drivers = ref([])
const currentArch = ref('')
const customDebs = ref([])
const customDebNotice = ref('')
const installingDriver = ref(null)
const removingDriver = ref(null)
const pendingDriver = ref(null)
const showInstallModal = ref(false)
const showRemoveModal = ref(false)

// 任何一个驱动任务在跑时禁用所有触发按钮，防止重复点击（后端也会回 409）
const busy = computed(() => !!settingUp.value || !!installingDriver.value || !!removingDriver.value)

const driverColumns = [
  { accessorKey: 'displayName', header: '驱动名称' },
  { id: 'description', header: '说明' },
  { id: 'arch', header: '架构' },
  { id: 'status', header: '状态' },
  { id: 'actions', header: '操作' }
]

// 检测表格里的推荐驱动只有 arch 数组（DriverMeta），按当前架构自行判断是否可装
function archSupported(arch) {
  if (!arch || !arch.length) return true
  return arch.includes('all') || arch.includes(currentArch.value)
}

function formatDate(dateStr) {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

async function loadDrivers() {
  try {
    const resp = await apiFetch('/api/admin/drivers', {}, () => emit('logout'))
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: '加载驱动列表失败', description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      return
    }
    const data = await resp.json()
    drivers.value = data.drivers || []
    currentArch.value = data.currentArch || ''
    customDebs.value = data.customDebs || []
    customDebNotice.value = data.customDebNotice || ''
  } catch (e) {
    toast.add({ title: '加载驱动列表失败', description: String(e), color: 'error', icon: 'i-lucide-x-circle' })
  }
}

function confirmInstall(driver) {
  pendingDriver.value = driver
  showInstallModal.value = true
}

function confirmRemove(driver) {
  pendingDriver.value = driver
  showRemoveModal.value = true
}

async function installDriver() {
  const driver = pendingDriver.value
  if (!driver) return
  installingDriver.value = driver.name
  showInstallModal.value = false
  startJobPanel(`正在安装驱动 ${driver.displayName}`)
  try {
    await runDriverJob('/api/admin/drivers/install', { name: driver.name })
    finishJobPanel(true)
    toast.add({ title: '安装成功', description: `驱动 ${driver.displayName} 已安装`, color: 'success', icon: 'i-lucide-check-circle' })
    await loadDrivers()
  } catch (e) {
    finishJobPanel(false)
    toast.add({ title: '安装失败', description: e.message || String(e), color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    installingDriver.value = null
    pendingDriver.value = null
  }
}

async function removeDriver() {
  const driver = pendingDriver.value
  if (!driver) return
  removingDriver.value = driver.name
  showRemoveModal.value = false
  startJobPanel(`正在卸载驱动 ${driver.displayName}`)
  try {
    await runDriverJob('/api/admin/drivers/remove', { name: driver.name })
    finishJobPanel(true)
    toast.add({ title: '卸载成功', description: `驱动 ${driver.displayName} 已卸载`, color: 'success', icon: 'i-lucide-check-circle' })
    await loadDrivers()
  } catch (e) {
    finishJobPanel(false)
    toast.add({ title: '卸载失败', description: e.message || String(e), color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    removingDriver.value = null
    pendingDriver.value = null
  }
}

// --- 上传自定义驱动 ---
const fileInputRef = ref(null)
const uploadFile = ref(null)
const uploading = ref(false)

function triggerFileInput() {
  fileInputRef.value?.click()
}

function onFileSelected(e) {
  const file = e.target.files?.[0]
  if (file) {
    uploadFile.value = file
  }
}

async function uploadDriver() {
  if (!uploadFile.value) return
  uploading.value = true
  try {
    const formData = new FormData()
    formData.append('file', uploadFile.value)
    const resp = await apiFetch('/api/admin/drivers/upload', {
      method: 'POST',
      body: formData,
      signal: AbortSignal.timeout(300000)
    }, () => emit('logout'))
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: '上传失败', description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      return
    }
    const data = await resp.json()
    toast.add({
      title: '上传成功',
      // .deb 不会随容器重启自动恢复，后端会在 warning 里说明，必须透传给用户
      description: data.warning
        ? `驱动文件 ${uploadFile.value.name} 已安装。${data.warning}`
        : `驱动文件 ${uploadFile.value.name} 已安装`,
      color: data.warning ? 'warning' : 'success',
      icon: data.warning ? 'i-lucide-triangle-alert' : 'i-lucide-check-circle'
    })
    if (data.log) {
      jobTitle.value = `上传安装 ${uploadFile.value.name}`
      jobLog.value = data.log
      jobRunning.value = false
      jobFailed.value = false
    }
    uploadFile.value = null
    if (fileInputRef.value) fileInputRef.value.value = ''
    await loadDrivers()
  } catch (e) {
    toast.add({ title: '上传失败', description: String(e), color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    uploading.value = false
  }
}

onMounted(() => {
  loadDrivers()
})

// 组件卸载时停掉轮询定时器，避免离开页面后仍在打接口
onUnmounted(() => {
  unmounted = true
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
})
</script>
