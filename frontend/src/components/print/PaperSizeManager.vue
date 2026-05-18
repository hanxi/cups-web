<template>
  <UModal v-model:open="showDialog" :ui="{ content: 'max-w-lg w-full' }">
    <UButton size="xs" variant="soft" icon="i-lucide-settings-2" @click="showDialog = true">
      管理自定义大小
    </UButton>
    <template #content>
      <div class="p-5 space-y-4">
        <div class="flex items-center justify-between">
          <h3 class="text-base font-semibold">自定义纸张大小</h3>
          <UButton variant="ghost" icon="i-lucide-x" size="xs" @click="showDialog = false" />
        </div>

        <!-- 已保存列表 -->
        <div v-if="loading" class="text-sm text-muted text-center py-4">
          <UIcon name="i-lucide-loader-2" class="w-4 h-4 animate-spin inline-block" />
          加载中...
        </div>
        <div v-else-if="presets.length > 0" class="space-y-1.5 max-h-48 overflow-y-auto">
          <div
            v-for="preset in presets"
            :key="preset.id"
            class="flex items-center gap-2 px-3 py-2 rounded-lg border cursor-pointer transition text-sm"
            :class="selectedId === preset.id
              ? 'border-primary bg-primary/5'
              : 'border-default hover:bg-elevated'"
            @click="selectPreset(preset)"
          >
            <div class="flex-1 min-w-0">
              <div class="font-medium truncate">{{ preset.name || '未命名' }}</div>
              <div class="text-xs text-muted">
                {{ preset.width }}×{{preset.height}}mm
                · 页边距 {{ preset.marginTop }}/{{ preset.marginRight }}/{{ preset.marginBottom }}/{{ preset.marginLeft }}mm
              </div>
            </div>
            <UButton
              icon="i-lucide-trash-2"
              size="xs"
              variant="ghost"
              color="error"
              :loading="deletingId === preset.id"
              @click.stop="deletePreset(preset.id)"
            />
          </div>
        </div>
        <div v-else class="text-sm text-muted text-center py-4">
          暂无自定义纸张大小
        </div>

        <!-- 编辑表单 -->
        <div class="border-t border-default pt-4 space-y-3">
          <p class="text-xs text-muted font-medium uppercase tracking-wide">
            {{ selectedId ? '编辑' : '新建' }}
          </p>
          <UFormField label="名称">
            <UInput v-model="form.name" placeholder="如：收银小票" class="w-full" />
          </UFormField>
          <div class="grid grid-cols-2 gap-3">
            <UFormField label="宽度 (mm)">
              <UInput v-model.number="form.width" type="number" :min="1" :max="2000" placeholder="210" class="w-full" />
            </UFormField>
            <UFormField label="高度 (mm)">
              <UInput v-model.number="form.height" type="number" :min="1" :max="2000" placeholder="297" class="w-full" />
            </UFormField>
          </div>
          <div class="space-y-2">
            <p class="text-xs text-muted">页边距 (mm)</p>
            <div class="grid grid-cols-4 gap-2">
              <UFormField label="上" :ui="{ label: 'text-[11px]' }">
                <UInput v-model.number="form.marginTop" type="number" :min="0" :max="200" class="w-full" size="sm" />
              </UFormField>
              <UFormField label="右" :ui="{ label: 'text-[11px]' }">
                <UInput v-model.number="form.marginRight" type="number" :min="0" :max="200" class="w-full" size="sm" />
              </UFormField>
              <UFormField label="下" :ui="{ label: 'text-[11px]' }">
                <UInput v-model.number="form.marginBottom" type="number" :min="0" :max="200" class="w-full" size="sm" />
              </UFormField>
              <UFormField label="左" :ui="{ label: 'text-[11px]' }">
                <UInput v-model.number="form.marginLeft" type="number" :min="0" :max="200" class="w-full" size="sm" />
              </UFormField>
            </div>
          </div>

          <!-- 纸张预览 -->
          <div v-if="form.width > 0 && form.height > 0" class="flex justify-center py-2">
            <div
              class="border-2 border-dashed border-muted rounded-sm relative bg-white"
              :style="previewStyle"
            >
              <div
                class="absolute inset-0 border border-primary/30 rounded-sm"
                :style="marginPreviewStyle"
              />
              <div class="absolute inset-0 flex items-center justify-center text-[10px] text-muted select-none">
                {{ form.width }}×{{ form.height }}
              </div>
            </div>
          </div>

          <div class="flex justify-between gap-2 pt-1">
            <UButton
              v-if="selectedId"
              variant="ghost"
              size="sm"
              @click="resetForm"
            >
              取消编辑
            </UButton>
            <div v-else />
            <div class="flex gap-2">
              <UButton
                variant="soft"
                size="sm"
                icon="i-lucide-save"
                :loading="saving"
                :disabled="!form.width || !form.height"
                @click="savePreset"
              >
                {{ selectedId ? '更新' : '添加' }}
              </UButton>
            </div>
          </div>
        </div>
      </div>
    </template>
  </UModal>
</template>

<script setup>
import { ref, reactive, computed, watch } from 'vue'
import { apiFetch, readError } from '../../utils/api'

const emit = defineEmits(['select', 'logout'])

const showDialog = ref(false)
const presets = ref([])
const selectedId = ref(null)
const loading = ref(false)
const saving = ref(false)
const deletingId = ref(null)

const form = reactive({
  name: '',
  width: 210,
  height: 297,
  marginTop: 10,
  marginRight: 10,
  marginBottom: 10,
  marginLeft: 10
})

const previewStyle = computed(() => {
  const maxH = 100
  const ratio = form.width / form.height
  const h = maxH
  const w = Math.round(h * ratio)
  return {
    width: `${Math.min(w, 140)}px`,
    height: `${Math.min(h, 100)}px`
  }
})

const marginPreviewStyle = computed(() => {
  const maxH = 100
  const ratio = form.width / form.height
  const h = maxH
  const w = Math.min(Math.round(h * ratio), 140)
  const scale = h / form.height
  return {
    top: `${form.marginTop * scale}px`,
    right: `${form.marginRight * scale}px`,
    bottom: `${form.marginBottom * scale}px`,
    left: `${form.marginLeft * scale}px`
  }
})

async function loadPresets() {
  loading.value = true
  try {
    const resp = await apiFetch('/api/paper-sizes', {}, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    presets.value = await resp.json()
  } catch (e) {
    console.error('加载自定义纸张预设失败:', e)
  } finally {
    loading.value = false
  }
}

function selectPreset(preset) {
  selectedId.value = preset.id
  form.name = preset.name
  form.width = preset.width
  form.height = preset.height
  form.marginTop = preset.marginTop
  form.marginRight = preset.marginRight
  form.marginBottom = preset.marginBottom
  form.marginLeft = preset.marginLeft
}

function resetForm() {
  selectedId.value = null
  form.name = ''
  form.width = 210
  form.height = 297
  form.marginTop = 10
  form.marginRight = 10
  form.marginBottom = 10
  form.marginLeft = 10
}

async function savePreset() {
  if (!form.width || !form.height) return
  saving.value = true
  try {
    const body = {
      name: form.name.trim(),
      width: Number(form.width),
      height: Number(form.height),
      marginTop: Number(form.marginTop) || 0,
      marginRight: Number(form.marginRight) || 0,
      marginBottom: Number(form.marginBottom) || 0,
      marginLeft: Number(form.marginLeft) || 0
    }
    let resp
    if (selectedId.value) {
      resp = await apiFetch(`/api/paper-sizes/${selectedId.value}`, {
        method: 'PUT',
        body: JSON.stringify(body)
      }, () => emit('logout'))
    } else {
      resp = await apiFetch('/api/paper-sizes', {
        method: 'POST',
        body: JSON.stringify(body)
      }, () => emit('logout'))
    }
    if (!resp.ok) throw new Error(await readError(resp))
    const saved = await resp.json()
    if (selectedId.value) {
      const idx = presets.value.findIndex(p => p.id === selectedId.value)
      if (idx >= 0) presets.value[idx] = saved
    } else {
      presets.value.push(saved)
    }
    resetForm()
  } catch (e) {
    console.error('保存纸张预设失败:', e)
  } finally {
    saving.value = false
  }
}

async function deletePreset(id) {
  deletingId.value = id
  try {
    const resp = await apiFetch(`/api/paper-sizes/${id}`, { method: 'DELETE' }, () => emit('logout'))
    if (!resp.ok) throw new Error(await readError(resp))
    presets.value = presets.value.filter(p => p.id !== id)
    if (selectedId.value === id) resetForm()
  } catch (e) {
    console.error('删除纸张预设失败:', e)
  } finally {
    deletingId.value = null
  }
}

function selectForPrint(preset) {
  emit('select', {
    value: `custom_${preset.width}x${preset.height}mm`,
    margins: {
      top: preset.marginTop,
      right: preset.marginRight,
      bottom: preset.marginBottom,
      left: preset.marginLeft
    }
  })
}

watch(showDialog, (val) => {
  if (val) loadPresets()
})

loadPresets()

defineExpose({ presets, loadPresets, selectForPrint })
</script>
