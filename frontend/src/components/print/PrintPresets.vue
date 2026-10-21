<template>
  <div>
    <UDropdownMenu :items="menuItems" :content="{ align: 'end' }">
      <UButton variant="ghost" color="neutral" size="xs" icon="i-lucide-bookmark" trailing-icon="i-lucide-chevron-down" :loading="loading">
        {{ t('ui.print_presets') }}
      </UButton>
    </UDropdownMenu>

    <!-- 保存为预置 -->
    <UModal :title="t('ui.save_current_settings_as_preset')" v-model:open="showSave">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold flex items-center gap-2">
            <UIcon name="i-lucide-bookmark-plus" class="w-5 h-5 text-primary" /> {{ t('ui.save_current_settings_as_preset') }} </h3>
          <UFormField :label="t('ui.preset_name')" :hint="sameNamePreset ? t('ui.a_preset_with_this_name_exists_saving_will_overwrite_it') : undefined">
            <UInput v-model="saveName" :placeholder="t('ui.for_example_black_and_white_double_sided')" maxlength="64" class="w-full" autofocus @keydown.enter="savePreset" />
          </UFormField>
          <div class="flex justify-end gap-2 pt-2">
            <UButton variant="ghost" @click="showSave = false">{{ t('ui.cancel') }}</UButton>
            <UButton color="primary" :loading="saving" :disabled="!saveName.trim()" @click="savePreset">
              {{ sameNamePreset ? t('ui.overwrite') : t('ui.save') }}
            </UButton>
          </div>
        </div>
      </template>
    </UModal>

    <!-- 管理预置 -->
    <UModal :title="t('ui.manage_presets')" v-model:open="showManage">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold flex items-center gap-2">
            <UIcon name="i-lucide-bookmark" class="w-5 h-5 text-primary" /> {{ t('ui.manage_presets') }} </h3>
          <p v-if="presets.length === 0" class="text-sm text-muted">{{ t('ui.no_saved_presets') }}</p>
          <ul v-else class="divide-y divide-default border border-default rounded-lg">
            <li v-for="p in presets" :key="p.id" class="flex items-center gap-2 p-2">
              <UInput v-if="renamingId === p.id" v-model="renameValue" maxlength="64" size="sm" class="flex-1" autofocus
                @keydown.enter="renamePreset(p)" @keydown.esc="renamingId = 0" />
              <span v-else class="flex-1 text-sm truncate" :title="p.name">{{ p.name }}</span>
              <template v-if="renamingId === p.id">
                <UButton size="xs" color="primary" icon="i-lucide-check" :aria-label="t('ui.save')" :loading="busyId === p.id" :disabled="!renameValue.trim()" @click="renamePreset(p)" />
                <UButton size="xs" variant="ghost" icon="i-lucide-x" :aria-label="t('ui.cancel')" @click="renamingId = 0" />
              </template>
              <template v-else>
                <UButton size="xs" variant="ghost" icon="i-lucide-pencil" :title="t('ui.rename_preset')" :aria-label="t('ui.rename_preset')" :disabled="busyId !== 0" @click="startRename(p)" />
                <UButton size="xs" variant="ghost" icon="i-lucide-save" :title="t('ui.update_preset_with_current_settings')" :aria-label="t('ui.update_preset_with_current_settings')" :loading="busyId === p.id" :disabled="busyId !== 0" @click="overwritePreset(p)" />
                <UButton size="xs" variant="ghost" color="error" icon="i-lucide-trash-2" :title="t('ui.delete')" :aria-label="t('ui.delete')" :disabled="busyId !== 0" @click="deletePreset(p)" />
              </template>
            </li>
          </ul>
          <div class="flex justify-end pt-2">
            <UButton variant="ghost" @click="showManage = false">{{ t('ui.close') }}</UButton>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted } from 'vue'
import { apiFetch, readError } from '../../utils/api'

// 打印预置入口（issue #128）：命名预置存服务端，按用户隔离。
// guest 是多人共享账号，canManage=false 时只保留「恢复默认」。
const { t } = useI18n()
const toast = useToast()

const props = defineProps({
  canManage: { type: Boolean, default: false },
  // 返回当前表单的打印参数快照（含水印文字）
  snapshot: { type: Function, required: true }
})
const emit = defineEmits(['apply', 'reset', 'logout'])

const presets = ref([])
const loading = ref(false)
const showSave = ref(false)
const saveName = ref('')
const saving = ref(false)
const showManage = ref(false)
const renamingId = ref(0)
const renameValue = ref('')
const busyId = ref(0)

const sameNamePreset = computed(() => {
  const name = saveName.value.trim()
  return name ? presets.value.find(p => p.name === name) : null
})

const menuItems = computed(() => {
  const groups = []
  if (props.canManage) {
    groups.push(presets.value.length
      ? presets.value.map(p => ({ label: p.name, icon: 'i-lucide-bookmark-check', onSelect: () => applyPreset(p) }))
      : [{ label: t('ui.no_saved_presets'), disabled: true }])
    const actions = [{ label: t('ui.save_current_settings_as_preset'), icon: 'i-lucide-bookmark-plus', onSelect: openSave }]
    if (presets.value.length) actions.push({ label: t('ui.manage_presets'), icon: 'i-lucide-list', onSelect: () => { renamingId.value = 0; showManage.value = true } })
    groups.push(actions)
  }
  groups.push([{ label: t('ui.restore_default_settings'), icon: 'i-lucide-rotate-ccw', onSelect: restoreDefaults }])
  return groups
})

async function request(url, options, failureTitle) {
  try {
    const resp = await apiFetch(url, options, () => emit('logout'))
    if (!resp.ok) {
      if (resp.status !== 401) toast.add({ title: failureTitle, description: await readError(resp), color: 'error', icon: 'i-lucide-x-circle' })
      return null
    }
    return await resp.json()
  } catch (e) {
    toast.add({ title: failureTitle, description: e.message, color: 'error', icon: 'i-lucide-x-circle' })
    return null
  }
}

async function loadPresets() {
  if (!props.canManage) return
  loading.value = true
  const list = await request('/api/print-presets', {}, t('ui.could_not_load_print_presets'))
  if (list) presets.value = list
  loading.value = false
}

function jsonBody(method, body) {
  return { method, body: JSON.stringify(body) }
}

function applyPreset(p) {
  emit('apply', p.settings)
  toast.add({ title: t('ui.preset_applied', { p0: p.name }), color: 'success', icon: 'i-lucide-check-circle' })
}

function restoreDefaults() {
  emit('reset')
  toast.add({ title: t('ui.default_settings_restored'), color: 'success', icon: 'i-lucide-check-circle' })
}

function openSave() {
  saveName.value = ''
  showSave.value = true
}

async function savePreset() {
  const name = saveName.value.trim()
  if (!name || saving.value) return
  saving.value = true
  const existing = sameNamePreset.value
  const settings = props.snapshot()
  const saved = existing
    ? await request(`/api/print-presets/${existing.id}`, jsonBody('PUT', { settings }), t('ui.could_not_save_print_preset'))
    : await request('/api/print-presets', jsonBody('POST', { name, settings }), t('ui.could_not_save_print_preset'))
  saving.value = false
  if (!saved) return
  showSave.value = false
  toast.add({ title: t('ui.preset_saved', { p0: saved.name }), color: 'success', icon: 'i-lucide-check-circle' })
  await loadPresets()
}

function startRename(p) {
  renamingId.value = p.id
  renameValue.value = p.name
}

async function renamePreset(p) {
  const name = renameValue.value.trim()
  if (!name || busyId.value) return
  if (name === p.name) { renamingId.value = 0; return }
  busyId.value = p.id
  const saved = await request(`/api/print-presets/${p.id}`, jsonBody('PUT', { name }), t('ui.could_not_save_print_preset'))
  busyId.value = 0
  if (!saved) return
  renamingId.value = 0
  await loadPresets()
}

async function overwritePreset(p) {
  if (busyId.value) return
  busyId.value = p.id
  const saved = await request(`/api/print-presets/${p.id}`, jsonBody('PUT', { settings: props.snapshot() }), t('ui.could_not_save_print_preset'))
  busyId.value = 0
  if (!saved) return
  toast.add({ title: t('ui.preset_saved', { p0: saved.name }), color: 'success', icon: 'i-lucide-check-circle' })
  await loadPresets()
}

async function deletePreset(p) {
  if (busyId.value || !confirm(t('ui.delete_print_preset_confirm', { p0: p.name }))) return
  busyId.value = p.id
  const ok = await request(`/api/print-presets/${p.id}`, { method: 'DELETE' }, t('ui.could_not_delete_print_preset'))
  busyId.value = 0
  if (ok) await loadPresets()
}

onMounted(loadPresets)
</script>
