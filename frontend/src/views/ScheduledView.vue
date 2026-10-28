<template>
  <div class="p-3 sm:p-4 md:p-6 max-w-5xl mx-auto space-y-4">
    <div class="flex items-center justify-between gap-2 flex-wrap">
      <h1 class="text-lg font-bold flex items-center gap-2">
        <UIcon name="i-lucide-calendar-clock" class="w-5 h-5 text-primary" /> {{ t('ui.scheduled_printing') }} </h1>
      <div class="flex items-center gap-2">
        <UButton
          variant="ghost"
          size="xs"
          icon="i-lucide-refresh-cw"
          :loading="loading"
          @click="loadRecords()"
        >{{ t('ui.refresh') }}</UButton>
        <UButton
          size="sm"
          color="primary"
          icon="i-lucide-plus"
          @click="openCreate"
        >{{ t('ui.new_scheduled_job') }}</UButton>
      </div>
    </div>

    <UAlert
      color="neutral"
      variant="soft"
      icon="i-lucide-info"
      :title="t('ui.usage')"
      :description="t('ui.schedules_run_in_the_server_s_local_time_zone_history')"
    />

    <div v-if="loading" class="flex items-center justify-center py-10 text-muted gap-2">
      <UIcon name="i-lucide-loader-circle" class="w-5 h-5 animate-spin" /> {{ t('ui.loading') }} </div>
    <div v-else-if="!records.length" class="text-center py-16 text-muted">
      <UIcon name="i-lucide-calendar-off" class="w-10 h-10 mx-auto mb-2 opacity-40" />
      <div>{{ t('ui.no_scheduled_jobs_yet') }}</div>
    </div>
    <div v-else class="space-y-3">
      <UCard v-for="rec in records" :key="rec.id" class="overflow-hidden">
        <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
          <div class="min-w-0 space-y-1">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="font-medium truncate">{{ rec.name || rec.filename }}</span>
              <UBadge
                :color="rec.enabled || rec.completed ? 'success' : rec.running ? 'primary' : 'neutral'"
                :variant="rec.enabled ? 'subtle' : 'outline'"
                size="sm"
              >{{ rec.running ? t('ui.submitting') : rec.completed ? t('ui.completed') : rec.enabled ? t('ui.enabled') : t('ui.paused') }}</UBadge>
              <UBadge color="primary" variant="outline" size="sm">
                {{ scheduleLabel(rec) }}
              </UBadge>
              <UBadge
                v-if="rec.lastStatus && !rec.running"
                :color="lastStatusColor(rec.lastStatus)"
                variant="soft"
                size="sm"
              >{{ lastStatusLabel(rec.lastStatus) }}</UBadge>
            </div>
            <div class="text-xs text-muted flex flex-wrap gap-x-4 gap-y-0.5">
              <span>{{ t('ui.file_3') }}{{ rec.files?.length ? t('ui.photo_count', { count: rec.files.length }) : rec.filename }}</span>
              <span>{{ t('ui.printer_2') }}{{ shortPrinter(rec.printerUri) }}</span>
              <span>{{ t('ui.next_run') }}{{ rec.completed ? '—' : formatTime(rec.nextRunAt) }}</span>
              <span v-if="rec.lastRunAt">{{ t('ui.last_run') }}{{ formatTime(rec.lastRunAt) }}</span>
            </div>
            <div v-if="rec.lastError && !rec.running" class="text-xs text-error truncate"> {{ t('ui.last_error') }}{{ translateError(rec.lastError) }}
            </div>
          </div>
          <div class="flex items-center gap-1 flex-shrink-0">
            <UButton size="xs" variant="ghost" icon="i-lucide-play" :loading="rec.running || rec._running" :disabled="rec.completed || rec.running" @click="runNow(rec)">{{ rec.files?.length ? t('ui.print_next_batch') : t('ui.run_now') }}</UButton>
            <UButton size="xs" variant="ghost" :icon="rec.enabled ? 'i-lucide-pause' : 'i-lucide-play-circle'" :disabled="rec.completed || rec.running" @click="toggleEnabled(rec)">
              {{ rec.enabled ? t('ui.pause') : t('ui.resume') }}
            </UButton>
            <UButton size="xs" variant="ghost" icon="i-lucide-pencil" :disabled="rec.running" @click="openEdit(rec)">{{ t('ui.edit') }}</UButton>
            <UButton size="xs" variant="ghost" color="error" icon="i-lucide-trash-2" :disabled="rec.running" @click="confirmDelete(rec)">{{ t('ui.delete') }}</UButton>
          </div>
        </div>
        <div v-if="rec.files?.length" class="mt-4 space-y-3">
          <div class="flex items-center justify-between text-sm">
            <span>{{ t(rec.loop ? 'ui.queue_cycle_progress' : 'ui.queue_progress', { submitted: rec.nextFileIndex, count: rec.files.length }) }} <span v-if="rec.loop" class="text-muted"> · {{ t('ui.queue_total', { count: rec.submittedCount }) }}</span>
            </span>
            <span class="text-muted">{{ Math.round(rec.nextFileIndex / rec.files.length * 100) }}%</span>
          </div>
          <div class="h-2 rounded-full bg-elevated overflow-hidden" role="progressbar"
            :aria-valuenow="rec.nextFileIndex" :aria-valuemax="rec.files.length" aria-valuemin="0" :aria-label="t('ui.photo_submission_progress_this_cycle')">
            <div class="h-full bg-primary rounded-full transition-all" :style="{ width: (rec.nextFileIndex / rec.files.length * 100) + '%' }" />
          </div>
          <div class="flex gap-2 overflow-x-auto pb-1">
            <div v-for="(photo, index) in rec.files" :key="photo.thumbnailUrl" class="w-24 shrink-0">
              <div class="relative rounded-lg overflow-hidden border-2"
                :class="index === rec.nextFileIndex && !rec.completed ? 'border-primary' : 'border-transparent'">
                <img :src="photo.thumbnailUrl" :alt="photo.filename" loading="lazy" class="w-full h-20 object-cover bg-elevated"
                  :class="index < rec.nextFileIndex ? 'opacity-60' : ''" />
                <span v-if="index < rec.nextFileIndex" class="absolute bottom-1 right-1 rounded-full bg-primary text-white p-0.5 flex">
                  <UIcon name="i-lucide-check" class="w-3 h-3" />
                </span>
                <span v-else-if="index === rec.nextFileIndex" class="absolute bottom-1 left-1 text-xs rounded bg-primary text-white px-1">{{ t('ui.next_photo') }}</span>
              </div>
              <div class="text-xs text-muted truncate mt-1" :title="photo.filename">{{ photo.filename }}</div>
            </div>
          </div>
          <div class="rounded-lg bg-elevated/50 px-3 py-2 text-xs text-muted space-y-1">
            <div v-if="!rec.completed">{{ t('ui.next_photo_2') }}{{ rec.files[rec.nextFileIndex]?.filename }}</div>
            <div>{{ rec.loop ? t('ui.after_submitting_all_photos_start_again_from_the_first') : t('ui.the_schedule_ends_after_all_photos_have_been_submitted') }} {{ t('ui.run_photo_summary', { photos: t('ui.photo_count', rec.photosPerRun || 1), copies: t('ui.copy_count', rec.copies) }) }}</div>
          </div>
          <details v-if="rec.history?.length" class="text-sm">
            <summary class="cursor-pointer text-muted">{{ t('ui.recent_run_count', { count: rec.history.length }) }}</summary>
            <div class="mt-2 divide-y divide-default">
              <div v-for="(run, index) in rec.history" :key="index" class="flex items-start gap-3 py-2 text-xs">
                <span class="text-muted shrink-0">{{ formatTime(run.runAt) }}</span>
                <span class="min-w-0 flex-1 truncate" :title="run.filename">{{ run.filename }}</span>
                <span :class="run.status === 'failed' ? 'text-error' : 'text-muted'" :title="translateError(run.error)">
                  {{ run.status === 'printed' ? t('ui.submitted') : run.status === 'skipped' ? t('ui.skipped') : t('ui.failed') }}
                </span>
              </div>
            </div>
          </details>
        </div>
      </UCard>
    </div>

    <!-- 新建 / 编辑抽屉 -->
    <UModal v-model:open="modalOpen" :title="editing ? t('ui.edit_scheduled_job') : t('ui.new_scheduled_job')" :ui="{ content: 'sm:max-w-3xl' }">
      <template #body>
        <div class="space-y-3">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <UFormField v-if="!editing" :label="t('ui.schedule_type')">
              <USelect v-model="form.mode" :items="modeItems" value-key="value" label-key="label" class="w-full" />
            </UFormField>
            <UFormField :label="t('ui.job_name_optional')">
              <UInput class="w-full" v-model="form.name" :placeholder="t('ui.for_example_weekly_meeting_minutes')" />
            </UFormField>
          </div>

          <UFormField v-if="form.mode === 'queue'" :label="t('ui.photo_queue')" required>
            <div class="flex items-center justify-between text-xs text-muted mb-2">
              <span>{{ t('ui.selected_photo_count', { count: editing ? form.existingFiles.length : form.photos.length }) }}</span>
              <label v-if="!editing" class="cursor-pointer text-primary hover:underline"> {{ t('ui.add_photos') }} <input type="file" multiple accept="image/jpeg,image/png,image/gif" class="sr-only" @change="onPhotoPick" />
              </label>
            </div>
            <div v-if="!editing" class="grid grid-cols-3 sm:grid-cols-5 gap-2">
              <div v-for="(photo, index) in form.photos" :key="photo.key" draggable="true"
                class="rounded-lg border border-default p-1 cursor-grab"
                @dragstart="dragIndex = index" @dragover.prevent @drop.prevent="movePhoto(dragIndex, index)" @dragend="dragIndex = null">
                <div class="relative">
                  <img :src="photo.url" :alt="photo.file.name" class="w-full h-20 object-cover rounded bg-elevated" />
                  <span class="absolute bottom-1 left-1 bg-black/60 text-white text-xs px-1 rounded">{{ index + 1 }}</span>
                </div>
                <div class="text-xs truncate mt-1" :title="photo.file.name">{{ photo.file.name }}</div>
                <div class="flex justify-between mt-1">
                  <UButton size="xs" variant="ghost" icon="i-lucide-chevron-left" :disabled="index === 0" :aria-label="t('ui.move_earlier') + photo.file.name" @click="movePhoto(index, index - 1)" />
                  <UButton size="xs" variant="ghost" icon="i-lucide-chevron-right" :disabled="index === form.photos.length - 1" :aria-label="t('ui.move_later') + photo.file.name" @click="movePhoto(index, index + 1)" />
                  <UButton size="xs" variant="ghost" color="error" icon="i-lucide-x" :aria-label="t('ui.remove') + photo.file.name" @click="removePhoto(index)" />
                </div>
              </div>
            </div>
            <div v-else class="flex gap-2 overflow-x-auto">
              <div v-for="photo in form.existingFiles" :key="photo.thumbnailUrl" class="w-24 shrink-0">
                <img :src="photo.thumbnailUrl" :alt="photo.filename" class="w-full h-20 object-cover rounded" />
                <div class="text-xs truncate mt-1">{{ photo.filename }}</div>
              </div>
            </div>
            <template #help>{{ editing ? t('ui.create_a_new_schedule_to_change_photos_or_their_order') : t('ui.supports_up_to_100_jpg_png_gif_photos_drag_or') }}</template>
          </UFormField>
          <UFormField v-else-if="!editing" :label="t('ui.file')" required>
            <input type="file"
              class="block w-full text-sm text-default file:mr-3 file:py-1.5 file:px-3 file:rounded-md file:border-0 file:bg-primary/10 file:text-primary hover:file:bg-primary/20"
              @change="onFilePick" />
            <div v-if="form.file" class="text-xs text-muted mt-1 truncate"> {{ t('ui.selected_2') }}{{ form.file.name }}（{{ formatSize(form.file.size) }}）
            </div>
          </UFormField>
          <UFormField v-else :label="t('ui.file')">
            <div class="text-sm text-muted truncate">{{ form.filename }}{{ t('ui.create_a_new_job_to_replace_the_file') }}</div>
          </UFormField>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <UFormField :label="t('ui.printer')" required>
              <USelect class="w-full"
                v-model="form.printerUri"
                :items="printerItems"
                value-key="value"
                label-key="label"
                :placeholder="t('ui.select_a_printer')"
                icon="i-lucide-printer"
              />
            </UFormField>

            <UFormField :label="t('ui.schedule_type_2')" required>
              <USelect class="w-full"
                v-model="form.scheduleType"
                :items="form.mode === 'queue' ? scheduleTypeItems.filter(item => item.value !== 'once') : scheduleTypeItems"
                value-key="value"
                label-key="label"
              />
            </UFormField>

            <UFormField v-if="form.scheduleType === 'once'" :label="t('ui.execution_time')" required>
              <UInput class="w-full" v-model="form.runAtLocal" type="datetime-local" />
            </UFormField>
            <template v-else>
              <UFormField :label="t('ui.start_date')">
                <UInput class="w-full" v-model="form.startDate" type="date" />
                <template #help>{{ t('ui.runs_in_the_server_s_time_zone') }}</template>
              </UFormField>
              <UFormField :label="t('ui.execution_time')" required>
                <UInput class="w-full" v-model="form.scheduleTime" type="time" />
              </UFormField>
              <UFormField v-if="form.scheduleType === 'weekly'" :label="t('ui.day_of_the_week')" required>
                <USelect class="w-full"
                  v-model.number="form.scheduleWeekday"
                  :items="weekdayItems"
                  value-key="value"
                  label-key="label"
                />
              </UFormField>
              <UFormField v-if="form.scheduleType === 'monthly'" :label="t('ui.day_of_the_month')" required>
                <UInput class="w-full" v-model.number="form.scheduleDay" type="number" min="1" max="31" />
                <template #help>{{ t('ui.if_the_day_exceeds_the_number_of_days_in_the') }}</template>
              </UFormField>
            </template>


          </div>
          <PrinterMediaOptions :key="editing?.id || 'new'" :printer-uri="form.printerUri"
            v-model:driverOptions="form.driverOptions" v-model:paperSize="form.paperSize" v-model:paperType="form.paperType"
            :paper-size-items="paperSizeItems" :paper-type-items="paperTypeItems" />
          <UFormField v-if="form.mode === 'queue'" :label="t('ui.photos_per_run')">
            <UInput class="w-full" v-model.number="form.photosPerRun" type="number" min="1" max="100" />
            <template #help>{{ t('ui.prints_in_queue_order_copies_per_photo_are_configured_separately') }}</template>
          </UFormField>
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <UFormField :label="t('ui.scaling')">
              <USelect class="w-full" v-model="form.printScaling" :items="scalingItems" value-key="value" label-key="label" />
            </UFormField>
            <UFormField :label="t('ui.orientation')">
              <USelect class="w-full" v-model="form.orientation" :items="orientationItems" value-key="value" label-key="label" />
              <template v-if="form.orientation === 'auto'" #help>{{ t('ui.orientation_follows_each_photo_s_dimensions_square_photos_use_portrait') }}</template>
            </UFormField>
            <UFormField :label="t('ui.duplex')">
              <USelect class="w-full" v-model="form.duplex" :items="duplexItems" value-key="value" label-key="label" />
            </UFormField>
            <UFormField :label="t('ui.colour')">
              <USelect class="w-full" v-model="form.color" :items="colorItems" value-key="value" label-key="label" />
            </UFormField>
            <UFormField :label="t('ui.copies')">
              <UInput class="w-full" v-model.number="form.copies" type="number" min="1" max="99" />
            </UFormField>
          </div>

          <UFormField v-if="form.mode === 'queue'" :label="t('ui.after_all_photos_are_submitted')">
            <USelect :model-value="form.loop ? 'loop' : 'stop'" @update:model-value="form.loop = $event === 'loop'" :items="endModeItems" value-key="value" label-key="label" class="w-full" />
            <template #help>{{ t('ui.a_failure_stops_the_batch_and_preserves_successful_submissions_the') }}</template>
          </UFormField>
          <div v-if="form.mode === 'queue' && !editing" class="rounded-lg bg-primary/10 px-3 py-2 text-sm text-primary">
            {{ t('ui.queue_form_summary', { schedule: scheduleLabel(form), photos: t('ui.photo_count', form.photosPerRun), copies: t('ui.copy_count', form.copies), ending: form.loop ? t('ui.repeat_in_queue_order') : t('ui.queue_stop_summary', { count: form.photos.length }) }) }}
          </div>
          <UFormField v-if="form.mode === 'queue' || /\.(jpe?g|png|gif)$/i.test(form.file?.name || form.filename || '')" :label="t('ui.photo_date')" :help="t('ui.print_the_exif_capture_date_at_the_bottom_right_of')">
            <USwitch v-model="form.photoDateWatermark" :label="t('ui.print_photo_date')" />
          </UFormField>
          <UFormField :label="t('ui.watermark_text_optional')">
            <UInput class="w-full" v-model="form.watermarkText" :placeholder="t('ui.leave_blank_for_no_watermark')" />
          </UFormField>
        </div>
      </template>
      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <UButton variant="ghost" @click="modalOpen = false">{{ t('ui.cancel') }}</UButton>
          <UButton color="primary" :loading="submitting" @click="submit">
            {{ editing ? t('ui.save') : t('ui.create') }}
          </UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import PrinterMediaOptions from '../components/print/PrinterMediaOptions.vue'
import { apiFetch, readError } from '../utils/api'
import { translateError } from '../utils/translate-error.js'

const { t, locale, n } = useI18n()

const emit = defineEmits(['logout'])
const toast = useToast()

const loading = ref(false)
const records = ref([])
const printers = ref([])

const modalOpen = ref(false)
const editing = ref(null)
const submitting = ref(false)

const form = ref(emptyForm())
const dragIndex = ref(null)
let photoKey = 0
let refreshTimer
let unmounted = false
let refreshing = 0
let recordsRequestVersion = 0
const modeItems = computed(() => ([
  { value: 'queue', label: t('ui.photo_queue_2') },
  { value: 'single', label: t('ui.single_file_schedule') }
]))
const endModeItems = computed(() => ([
  { value: 'stop', label: t('ui.stop_after_all_photos') },
  { value: 'loop', label: t('ui.loop_repeat') }
]))
const paperTypeItems = computed(() => ([
  { value: 'plain', label: t('ui.plain_paper') }, { value: 'photo', label: t('ui.photo_paper') },
  { value: 'glossy', label: t('ui.glossy_photo_paper') }, { value: 'matte', label: t('ui.matt_photo_paper') },
  { value: 'auto', label: t('ui.automatic_selection') }
]))
const scalingItems = computed(() => ([
  { value: 'fit', label: t('ui.fit_to_paper') }, { value: 'fill', label: t('ui.fill_paper') },
  { value: 'auto', label: t('ui.automatic') }, { value: 'none', label: t('ui.no_scaling') }
]))
watch(() => form.value.mode, (mode) => {
  if (editing.value) return
  clearPhotoPreviews()
  form.value.file = null
  form.value.scheduleType = mode === 'queue' ? 'daily' : 'once'
  form.value.paperType = mode === 'queue' ? 'photo' : 'plain'
  form.value.paperSize = mode === 'queue' ? '6inch' : 'A4'
  if (mode === 'single' && form.value.orientation === 'auto') form.value.orientation = 'portrait'
})
watch(modalOpen, (open) => { if (!open) clearPhotoPreviews() })

function emptyForm() {
  return {
    id: null,
    name: '',
    mode: 'queue', photos: [], existingFiles: [], loop: false, photosPerRun: 1, driverOptions: {},
    startDate: defaultRunAtLocal().slice(0, 10), paperType: 'photo', printScaling: 'fit',
    file: null,
    filename: '',
    printerUri: '',
    scheduleType: 'daily',
    runAtLocal: defaultRunAtLocal(),
    scheduleTime: '09:00',
    scheduleWeekday: 1,
    scheduleDay: 1,
    copies: 1,
    duplex: 'one-sided',
    color: 'color',
    paperSize: '6inch',
    orientation: 'auto',
    photoDateWatermark: false,
    watermarkText: ''
  }
}

// 默认新任务时间：明天上午 9 点，避免用户不改时间就直接创建导致立即触发。
function defaultRunAtLocal() {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  d.setHours(9, 0, 0, 0)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

const scheduleTypeItems = computed(() => ([
  { value: 'once', label: t('ui.once') },
  { value: 'daily', label: t('ui.daily') },
  { value: 'weekly', label: t('ui.weekly') },
  { value: 'monthly', label: t('ui.monthly') }
]))

const weekdayItems = computed(() => ([
  { value: 0, label: t('ui.sunday') },
  { value: 1, label: t('ui.monday') },
  { value: 2, label: t('ui.tuesday') },
  { value: 3, label: t('ui.wednesday') },
  { value: 4, label: t('ui.thursday') },
  { value: 5, label: t('ui.friday') },
  { value: 6, label: t('ui.saturday') }
]))

const paperSizeItems = computed(() => ([
  { value: 'A4', label: 'A4' },
  { value: 'A3', label: 'A3' },
  { value: 'Letter', label: 'Letter' },
  { value: '5inch', label: t('ui.option_3_5_5_in_photo_paper') },
  { value: '6inch', label: t('ui.option_4_6_in_photo_paper') },
  { value: '7inch', label: t('ui.option_5_7_in_photo_paper') },
  { value: '8inch', label: t('ui.option_6_8_in_photo_paper') },
  { value: '10inch', label: t('ui.option_8_10_in_photo_paper') }
]))
const orientationItems = computed(() => [
  ...(form.value.mode === 'queue' ? [{ value: 'auto', label: t('ui.automatic') }] : []),
  { value: 'portrait', label: t('ui.portrait') },
  { value: 'landscape', label: t('ui.landscape') }
])
const duplexItems = computed(() => ([
  { value: 'one-sided', label: t('ui.single_sided') },
  { value: 'two-sided-long-edge', label: t('ui.double_sided_long_edge') },
  { value: 'two-sided-short-edge', label: t('ui.double_sided_short_edge_binding') }
]))
const colorItems = computed(() => ([
  { value: 'color', label: t('ui.colour_2') },
  { value: 'monochrome', label: t('ui.black_and_white') }
]))

const printerItems = computed(() =>
  printers.value.map((p) => ({ value: p.uri || p.deviceUri || p.name, label: p.name || p.uri }))
)

function shortPrinter(uri) {
  if (!uri) return ''
  const idx = uri.lastIndexOf('/')
  return idx >= 0 ? uri.slice(idx + 1) : uri
}

function formatTime(iso) {
  if (!iso) return '—'
  try {
    const date = new Date(iso)
    return Number.isNaN(date.getTime()) ? iso : date.toLocaleString(locale.value)
  } catch (e) {
    return iso
  }
}

function formatSize(bytes) {
  if (!bytes) return ''
  const units = ['B', 'KB', 'MB', 'GB']
  let size = bytes
  let i = 0
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++ }
  return `${n(size, { minimumFractionDigits: 1, maximumFractionDigits: 1 })} ${units[i]}`
}

function scheduleLabel(rec) {
  switch (rec.scheduleType) {
    case 'once': return t('ui.once')
    case 'daily': return t('ui.every_day_at', { p0: rec.scheduleTime })
    case 'weekly': return t('ui.every_at', { p0: weekdayLabel(rec.scheduleWeekday), p1: rec.scheduleTime })
    case 'monthly': return t('ui.day_of_each_month_at', { p0: rec.scheduleDay, p1: rec.scheduleTime })
    default: return rec.scheduleType
  }
}
function weekdayLabel(w) {
  return [t('ui.sunday'), t('ui.monday'), t('ui.tuesday'), t('ui.wednesday'), t('ui.thursday'), t('ui.friday'), t('ui.saturday')][((w % 7) + 7) % 7]
}
function lastStatusLabel(s) {
  if (s === 'running') return t('ui.result_awaiting_confirmation')
  if (s === 'printed') return t('ui.last_run_submitted')
  if (s === 'failed') return t('ui.last_run_failed')
  if (s === 'skipped') return t('ui.missed_window')
  return s
}
function lastStatusColor(s) {
  if (s === 'printed') return 'success'
  if (s === 'running') return 'warning'
  if (s === 'failed') return 'error'
  if (s === 'skipped') return 'warning'
  return 'neutral'
}

async function loadRecords(silent = false) {
  const version = ++recordsRequestVersion
  refreshing++
  if (!silent) loading.value = true
  try {
    const resp = await apiFetch('/api/scheduled-prints', {}, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    const data = await resp.json()
    if (!unmounted && version === recordsRequestVersion) records.value = data
  } catch (e) {
    if (!silent && !unmounted && version === recordsRequestVersion) toast.add({ title: t('ui.could_not_load'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    refreshing--
    if (version === recordsRequestVersion) loading.value = false
  }
}

async function loadPrinters() {
  try {
    const resp = await apiFetch('/api/printers', {}, () => emit('logout'))
    if (!resp.ok) return
    printers.value = await resp.json()
  } catch (e) {
    // 静默失败：定时任务表单没有打印机数据时会给出必填提示
  }
}

function openCreate() {
  // 清除上一操作的提示，避免提示浮层挡住弹窗按钮。
  toast.clear()
  clearPhotoPreviews()
  editing.value = null
  form.value = emptyForm()
  if (printerItems.value.length) {
    form.value.printerUri = printerItems.value[0].value
  }
  modalOpen.value = true
}

function openEdit(rec) {
  toast.clear()
  clearPhotoPreviews()
  editing.value = rec
  form.value = {
    id: rec.id,
    mode: rec.files?.length ? 'queue' : 'single',
    photos: [], existingFiles: rec.files || [], loop: rec.loop || false,
    startDate: rec.startDate || '', paperType: rec.paperType || 'plain', printScaling: rec.printScaling || 'fit',
    name: rec.name || '',
    file: null,
    filename: rec.filename,
    printerUri: rec.printerUri,
    scheduleType: rec.scheduleType,
    runAtLocal: rec.runAt ? toLocalInput(rec.runAt) : defaultRunAtLocal(),
    scheduleTime: rec.scheduleTime || '09:00',
    scheduleWeekday: rec.scheduleWeekday,
    scheduleDay: rec.scheduleDay || 1,
    copies: rec.copies || 1,
    duplex: rec.sides || (rec.isDuplex ? 'two-sided-long-edge' : 'one-sided'),
    color: rec.isColor ? 'color' : 'monochrome',
    paperSize: rec.paperSize || 'A4',
    driverOptions: rec.driverOptions || {}, photosPerRun: rec.photosPerRun || 1,
    orientation: rec.orientation || 'portrait',
    photoDateWatermark: !!rec.photoDateWatermark,
    watermarkText: rec.watermarkText || ''
  }
  modalOpen.value = true
}

// toLocalInput 把服务器返回的 UTC RFC3339 转成 <input type="datetime-local"> 需要的
// 本地时区 YYYY-MM-DDTHH:mm 字符串。用户在本地看到的日期时间与实际触发对齐。
function toLocalInput(iso) {
  const d = new Date(iso)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function clearPhotoPreviews() {
  for (const photo of form.value.photos || []) URL.revokeObjectURL(photo.url)
  form.value.photos = []
}

function onPhotoPick(e) {
  const selected = Array.from(e.target.files || [])
  if (form.value.photos.length + selected.length > 100) {
    toast.add({ title: t('ui.select_up_to_100_photos'), color: 'warning' })
    e.target.value = ''
    return
  }
  if (selected.some(file => !/\.(jpe?g|png|gif)$/i.test(file.name))) {
    toast.add({ title: t('ui.select_jpg_png_or_gif_images'), color: 'warning' })
    e.target.value = ''
    return
  }
  for (const file of selected) {
    form.value.photos.push({ key: ++photoKey, file, url: URL.createObjectURL(file) })
  }
  e.target.value = ''
}

function removePhoto(index) {
  const [photo] = form.value.photos.splice(index, 1)
  if (photo) URL.revokeObjectURL(photo.url)
}

function movePhoto(from, to) {
  if (from == null || from === to || from < 0 || to < 0 || from >= form.value.photos.length || to >= form.value.photos.length) return
  const [photo] = form.value.photos.splice(from, 1)
  form.value.photos.splice(to, 0, photo)
}

function onFilePick(e) {
  const files = e.target?.files
  form.value.file = files && files.length ? files[0] : null
}

async function submit() {
  const v = form.value
  if (!v.printerUri) { toast.add({ title: t('ui.select_a_printer_2'), color: 'warning' }); return }
  if (!editing.value && (v.mode === 'queue' ? !v.photos.length : !v.file)) { toast.add({ title: t('ui.select_a_file'), color: 'warning' }); return }
  if (v.scheduleType === 'once' && !v.runAtLocal) { toast.add({ title: t('ui.choose_an_execution_time'), color: 'warning' }); return }

  submitting.value = true
  try {
    if (editing.value) {
      await saveEdit(v)
    } else {
      await createNew(v)
    }
    modalOpen.value = false
    await loadRecords()
  } catch (e) {
    toast.add({ title: t('ui.could_not_save'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    submitting.value = false
  }
}

async function createNew(v) {
  const fd = new FormData()
  if (v.mode === 'queue') {
    if (!Number.isInteger(v.photosPerRun) || v.photosPerRun < 1 || v.photosPerRun > 100) throw new Error(t('ui.photos_per_run_must_be_from_1_to_100'))
    for (const photo of v.photos) fd.append('files', photo.file, photo.file.name)
    fd.append('loop', String(v.loop))
  } else {
    fd.append('file', v.file, v.file.name)
  }
  fd.append('start_date', v.scheduleType === 'once' ? '' : v.startDate)
  fd.append('name', v.name)
  fd.append('printer', v.printerUri)
  fd.append('copies', String(v.copies))
  fd.append('sides', v.duplex)
  fd.append('duplex', v.duplex !== 'one-sided' ? 'true' : 'false')
  fd.append('color', v.color === 'color' ? 'true' : 'false')
  fd.append('orientation', v.orientation)
  fd.append('paper_size', v.paperSize)
  fd.append('paper_type', v.paperType)
  fd.append('driver_options', JSON.stringify(v.driverOptions))
  fd.append('photos_per_run', String(v.photosPerRun))
  fd.append('media_source', 'auto')
  fd.append('print_scaling', v.printScaling)
  fd.append('page_set', 'all')
  fd.append('number_up', '1')
  fd.append('number_up_layout', 'lrtb')
  fd.append('page_border', 'none')
  fd.append('photo_date_watermark', String(v.photoDateWatermark))
  if (v.watermarkText) fd.append('watermark_text', v.watermarkText)
  fd.append('schedule_type', v.scheduleType)
  if (v.scheduleType === 'once') {
    fd.append('run_at', new Date(v.runAtLocal).toISOString())
  } else {
    fd.append('schedule_time', v.scheduleTime)
    fd.append('schedule_weekday', String(v.scheduleWeekday))
    fd.append('schedule_day', String(v.scheduleDay))
  }
  const resp = await apiFetch('/api/scheduled-prints', { method: 'POST', body: fd }, () => emit('logout'))
  if (!resp.ok) throw new Error(await readError(resp))
  toast.add({ title: t('ui.scheduled_job_created'), color: 'success', icon: 'i-lucide-check-circle' })
}

async function saveEdit(v) {
  if (v.mode === 'queue' && (!Number.isInteger(v.photosPerRun) || v.photosPerRun < 1 || v.photosPerRun > 100)) throw new Error(t('ui.photos_per_run_must_be_from_1_to_100'))
  const payload = buildPayloadFromRecord(editing.value, {
    name: v.name,
    loop: v.loop,
    startDate: v.scheduleType === 'once' ? '' : v.startDate,
    printerUri: v.printerUri,
    sides: v.duplex,
    isDuplex: v.duplex !== 'one-sided',
    isColor: v.color === 'color',
    copies: v.copies,
    orientation: v.orientation,
    paperSize: v.paperSize,
    driverOptions: v.driverOptions, photosPerRun: v.photosPerRun,
    paperType: v.paperType,
    printScaling: v.printScaling,
    photoDateWatermark: v.photoDateWatermark,
    watermarkText: v.watermarkText,
    scheduleType: v.scheduleType,
    scheduleTime: v.scheduleTime,
    scheduleWeekday: v.scheduleWeekday,
    scheduleDay: v.scheduleDay,
    runAt: v.scheduleType === 'once' ? new Date(v.runAtLocal).toISOString() : ''
  })
  // 编辑参数时保留服务端最新启用状态；状态切换由暂停/恢复按钮处理。
  delete payload.enabled
  const resp = await apiFetch(`/api/scheduled-prints/${v.id}`, {
    method: 'PUT',
    body: JSON.stringify(payload)
  }, () => emit('logout'))
  if (!resp.ok) throw new Error(await readError(resp))
  toast.add({ title: t('ui.updated'), color: 'success', icon: 'i-lucide-check-circle' })
}

async function runNow(rec) {
  rec._running = true
  try {
    const resp = await apiFetch(`/api/scheduled-prints/${rec.id}/run`, { method: 'POST' }, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    toast.add({ title: t('ui.submitting_to_printer'), color: 'success', icon: 'i-lucide-check-circle' })
    await loadRecords()
  } catch (e) {
    toast.add({ title: t('ui.could_not_run_now'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  } finally {
    rec._running = false
  }
}

async function toggleEnabled(rec) {
  try {
    const resp = await apiFetch(`/api/scheduled-prints/${rec.id}`, {
      method: 'PUT',
      body: JSON.stringify(buildPayloadFromRecord(rec, { enabled: !rec.enabled }))
    }, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    await loadRecords()
  } catch (e) {
    toast.add({ title: t('ui.operation_failed'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  }
}

async function confirmDelete(rec) {
  if (!confirm(t('ui.delete_job_its_source_file_will_also_be_permanently_removed', { p0: rec.name || rec.filename }))) return
  try {
    const resp = await apiFetch(`/api/scheduled-prints/${rec.id}`, { method: 'DELETE' }, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    toast.add({ title: t('ui.deleted'), color: 'success', icon: 'i-lucide-check-circle' })
    await loadRecords()
  } catch (e) {
    toast.add({ title: t('ui.could_not_delete'), description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
  }
}

function buildPayloadFromRecord(rec, overrides = {}) {
  return {
    name: rec.name || '',
    loop: rec.loop || false, startDate: rec.startDate || '',
    printerUri: rec.printerUri,
    enabled: rec.enabled,
    sides: rec.sides || (rec.isDuplex ? 'two-sided-long-edge' : 'one-sided'),
    isDuplex: rec.isDuplex,
    isColor: rec.isColor,
    copies: rec.copies || 1,
    orientation: rec.orientation || 'portrait',
    paperSize: rec.paperSize || 'A4',
    driverOptions: rec.driverOptions || {}, photosPerRun: rec.photosPerRun || 1,
    paperType: rec.paperType || 'plain',
    mediaSource: rec.mediaSource || 'auto',
    printScaling: rec.printScaling || 'fit',
    pageRange: rec.pageRange || '',
    pageSet: rec.pageSet || 'all',
    mirror: !!rec.mirror,
    photoDateWatermark: !!rec.photoDateWatermark,
    watermarkText: rec.watermarkText || '',
    numberUp: rec.numberUp || 1,
    numberUpLayout: rec.numberUpLayout || 'lrtb',
    pageBorder: rec.pageBorder || 'none',
    scheduleType: rec.scheduleType,
    scheduleTime: rec.scheduleTime || '09:00',
    scheduleWeekday: rec.scheduleWeekday || 0,
    scheduleDay: rec.scheduleDay || 1,
    runAt: rec.runAt || '',
    ...overrides
  }
}

onMounted(async () => {
  await Promise.all([loadRecords(), loadPrinters()])
  if (unmounted) return
  refreshTimer = setInterval(() => {
    if (!document.hidden && !refreshing) loadRecords(true)
  }, 5000)
})
onUnmounted(() => {
  unmounted = true
  clearInterval(refreshTimer)
  clearPhotoPreviews()
})
</script>
