<template>
  <div class="space-y-2">
    <div class="flex items-center justify-between text-xs text-muted">
      <span>{{ loading ? t('ui.loading_printer_options') : translateError(error) || (options?.source === 'ppd' ? t('ui.options_provided_by_the_printer_driver') : options ? t('ui.options_provided_by_the_printer') : '') }}</span>
      <UButton v-if="printerUri" size="xs" variant="ghost" icon="i-lucide-refresh-cw" :loading="loading" @click="load(true)">{{ t('ui.refresh_options') }}</UButton>
    </div>
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
      <UFormField :label="t('ui.paper_size')">
        <USelect :model-value="selectedValue('pageSize', paperSize)" :items="items('pageSize', paperSizeItems)" value-key="value" label-key="label" class="w-full" :disabled="loading" @update:model-value="select('pageSize', $event)" />
      </UFormField>
      <UFormField :label="t('ui.media_type')">
        <USelect :model-value="selectedValue('mediaType', paperType)" :items="items('mediaType', paperTypeItems)" value-key="value" label-key="label" class="w-full" :disabled="loading" @update:model-value="select('mediaType', $event)" />
      </UFormField>
      <UFormField v-if="choices('quality').length || driverOptions.quality" :label="t('ui.print_quality')">
        <USelect :model-value="selectedValue('quality', '')" :items="items('quality', [{ label: t('ui.printer_default'), value: '' }])" value-key="value" label-key="label" class="w-full" :disabled="loading" @update:model-value="select('quality', $event)" />
      </UFormField>
    </div>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, watch, onUnmounted } from 'vue'
import { apiFetch, readError } from '../../utils/api'
import { translateError } from '../../utils/translate-error.js'
import { paperDimensions, printerChoiceLabel } from '../../utils/printer-options'

const { t } = useI18n()

const props = defineProps({
  printerUri: { type: String, default: '' },
  driverOptions: { type: Object, default: () => ({}) },
  paperSize: { type: String, default: 'A4' },
  paperType: { type: String, default: 'plain' },
  paperSizeItems: { type: Array, default: () => [] },
  paperTypeItems: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:driverOptions', 'update:paperSize', 'update:paperType'])
const options = ref(null)
const loading = ref(false)
const error = ref('')
let requestVersion = 0
const fields = { pageSize: 'paperSizes', mediaType: 'mediaTypes', quality: 'qualities' }
function choiceID(choice) { return `${choice.attribute}:${choice.value}` }
function choices(group) { return options.value?.[fields[group]] || [] }
function selectedValue(group, legacy) {
  const choice = props.driverOptions[group]
  return choice ? choiceID(choice) : `legacy:${legacy}`
}
function items(group, fallback) {
  const available = choices(group)
  let result = available.length ? available.map(c => ({ label: printerChoiceLabel(c), value: choiceID(c) })) : fallback.map(c => ({ ...c, value: `legacy:${c.value}` }))
  if (group === 'quality' && available.length) result.unshift({ label: t('ui.printer_default'), value: 'legacy:' })
  if (group === 'mediaType' && available.length) result.unshift({ label: t('ui.printer_default'), value: 'legacy:auto' })
  const saved = props.driverOptions[group]
  if (saved && !result.some(c => c.value === choiceID(saved))) result.push({ label: t('ui.saved_not_yet_verified', { p0: printerChoiceLabel(saved) }), value: choiceID(saved) })
  const value = selectedValue(group, group === 'pageSize' ? props.paperSize : group === 'mediaType' ? props.paperType : '')
  if (!result.some(c => c.value === value) && !saved) {
    const legacy = group === 'pageSize' ? props.paperSize : props.paperType
    result.push({ label: fallback.find(c => c.value === legacy)?.label || legacy || t('ui.printer_default'), value })
  }
  return result
}
function select(group, value) {
  const next = { ...props.driverOptions }
  if (value.startsWith('legacy:')) {
    delete next[group]
    if (group === 'pageSize') emit('update:paperSize', value.slice(7))
    if (group === 'mediaType') emit('update:paperType', value.slice(7))
  } else {
    const choice = choices(group).find(c => choiceID(c) === value)
    if (!choice) return
    next[group] = choice
    if (group === 'pageSize') emit('update:paperSize', `custom:${choice.widthMM}x${choice.heightMM}`)
  }
  emit('update:driverOptions', next)
}
async function load(force = false, reset = false) {
  const version = ++requestVersion
  const uri = props.printerUri
  options.value = null
  error.value = ''
  if (!uri) { loading.value = false; return }
  loading.value = true
  try {
    const resp = await apiFetch(`/api/printer-options?uri=${encodeURIComponent(uri)}${force ? '&force=1' : ''}`)
    if (!resp.ok) throw new Error(await readError(resp))
    const data = await resp.json()
    if (version !== requestVersion) return
    options.value = data
    const next = reset ? {} : { ...props.driverOptions }
    for (const [group, field] of Object.entries(fields)) {
      const saved = next[group]
      if (saved) next[group] = data[field]?.find(c => choiceID(c) === choiceID(saved)) || saved
    }
    // Preserve saved selections, including choices removed by a changed driver.
    if (!next.pageSize && data.paperSizes?.length) {
      const dim = paperDimensions(props.paperSize)
      next.pageSize = data.paperSizes.find(c => !c.borderless && dim && Math.abs(c.widthMM - dim.width) < 0.5 && Math.abs(c.heightMM - dim.height) < 0.5) || data.defaults?.pageSize || data.paperSizes[0]
    }
    if (!next.mediaType && data.mediaTypes?.length && props.paperType !== 'auto') {
      const aliases = { plain: 'stationery', photo: 'photographic', glossy: 'photographic-glossy', matte: 'photographic-matte' }
      const match = data.mediaTypes.find(c => c.value === aliases[props.paperType] || c.value.toLowerCase() === props.paperType.toLowerCase())
      if (match) next.mediaType = match
    }
    if (next.pageSize) emit('update:paperSize', `custom:${next.pageSize.widthMM}x${next.pageSize.heightMM}`)
    emit('update:driverOptions', next)
  } catch (e) {
    if (version === requestVersion) error.value = e.message || t('ui.could_not_load_refresh_to_try_again')
  } finally {
    if (version === requestVersion) loading.value = false
  }
}
watch(() => props.printerUri, (uri, previous) => {
  const reset = !!previous && uri !== previous
  if (reset) {
    emit('update:driverOptions', {})
    // A previous queue's dimensions must not become this queue's fallback.
    if (props.paperSize.startsWith('custom:')) emit('update:paperSize', 'A4')
  }
  load(false, reset)
}, { immediate: true })
watch(() => props.paperSize, (size) => {
  const saved = props.driverOptions.pageSize
  const dim = paperDimensions(size)
  if (!saved || !dim || (Math.abs(saved.widthMM - dim.width) < 0.01 && Math.abs(saved.heightMM - dim.height) < 0.01)) return
  const next = { ...props.driverOptions }
  next.pageSize = choices('pageSize').find(c => !c.borderless && Math.abs(c.widthMM - dim.width) < 0.5 && Math.abs(c.heightMM - dim.height) < 0.5)
  emit('update:driverOptions', next)
})
onUnmounted(() => { requestVersion++ })
</script>
