<template>
  <div class="p-3 sm:p-4 md:p-6 space-y-4 md:space-y-6">
    <UCard>
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-2">
            <UIcon name="i-lucide-type" class="w-5 h-5 text-primary" />
            <h2 class="text-xl font-bold">{{ t('ui.font_management') }}</h2>
          </div>
          <div class="flex items-center gap-2">
            <UButton
              size="sm"
              variant="outline"
              icon="i-lucide-refresh-cw"
              :loading="refreshingFonts"
              :disabled="refreshingFonts"
              @click="refreshFonts"
            >
              {{ t('ui.refresh_font_cache') }}
            </UButton>
            <UButton
              size="sm"
              color="primary"
              icon="i-lucide-upload"
              :loading="uploadingFont"
              :disabled="uploadingFont"
              @click="triggerFontUpload"
            >
              {{ t('ui.upload_font') }}
            </UButton>
            <input
              ref="fontFileInput"
              type="file"
              class="hidden"
              accept=".ttf,.ttc,.otf"
              multiple
              @change="onFontFileSelected"
            />
          </div>
        </div>
      </template>

      <div class="space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-2 text-sm text-muted bg-elevated/40 p-3 rounded-lg border border-default">
          <div class="flex items-center gap-2 flex-wrap">
            <span>{{ t('ui.font_management_desc') }}</span>
            <span class="font-mono text-xs bg-elevated px-2 py-0.5 rounded border border-default">{{ fontData.fontDir || '/data/fonts' }}</span>
          </div>
          <div class="text-xs font-medium">
            {{ t('ui.system_recognized_families', { count: fontData.totalSystemFamilies || 0 }) }}
          </div>
        </div>

        <div class="flex items-center justify-between gap-3">
          <UInput
            v-model="searchQuery"
            :placeholder="t('ui.search_fonts_placeholder')"
            icon="i-lucide-search"
            class="max-w-xs w-full"
          />
        </div>

        <div class="overflow-x-auto">
          <UTable :columns="fontColumns" :data="filteredFonts">
            <template #name-cell="{ row }">
              <div class="flex items-start gap-2.5 py-1">
                <UIcon name="i-lucide-type" class="w-5 h-5 text-primary shrink-0 mt-0.5" />
                <div class="min-w-0">
                  <div class="font-medium text-sm sm:text-base flex items-center gap-1.5 flex-wrap">
                    <span class="truncate">{{ row.original.displayName || row.original.name }}</span>
                    <UBadge
                      v-if="row.original.subfamily && row.original.subfamily.toLowerCase() !== 'regular'"
                      color="neutral"
                      variant="subtle"
                      size="xs"
                    >
                      {{ row.original.subfamily }}
                    </UBadge>
                    <UBadge
                      v-if="row.original.format"
                      color="primary"
                      variant="subtle"
                      size="xs"
                    >
                      {{ row.original.format }}
                    </UBadge>
                  </div>
                  <div class="text-xs text-muted font-mono flex items-center gap-1 mt-0.5">
                    <UIcon name="i-lucide-file" class="w-3.5 h-3.5 shrink-0" />
                    <span>{{ row.original.name }}</span>
                  </div>
                </div>
              </div>
            </template>
            <template #sizeBytes-cell="{ row }">
              {{ formatFileSize(row.original.sizeBytes) }}
            </template>
            <template #updatedAt-cell="{ row }">
              {{ formatTime(row.original.updatedAt) }}
            </template>
            <template #actions-cell="{ row }">
              <UButton
                size="xs"
                variant="outline"
                color="error"
                icon="i-lucide-trash-2"
                @click="confirmDeleteFont(row.original)"
              >
                {{ t('ui.delete') }}
              </UButton>
            </template>
          </UTable>
          <div v-if="!filteredFonts.length" class="text-center py-10 text-sm text-muted">
            {{ searchQuery ? (t('ui.no_matching_driver') || '无匹配字体') : t('ui.no_custom_fonts') }}
          </div>
        </div>
      </div>
    </UCard>

    <UModal :title="t('ui.confirm_delete_font')" v-model:open="showDeleteFontModal">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">{{ t('ui.confirm_delete_font') }}</h3>
          <p>
            {{
              pendingDeleteFont?.displayName && pendingDeleteFont?.displayName !== pendingDeleteFont?.name
                ? t('ui.delete_font_confirm_detail', { name: pendingDeleteFont?.displayName, file: pendingDeleteFont?.name })
                : t('ui.delete_font_confirm', { name: pendingDeleteFont?.name })
            }}
          </p>
          <p class="text-sm text-muted">{{ t('ui.this_cannot_be_undone') }}</p>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="showDeleteFontModal = false">{{ t('ui.cancel') }}</UButton>
            <UButton color="error" :loading="deletingFont" @click="executeDeleteFont">{{ t('ui.confirm_deletion') }}</UButton>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted } from 'vue'
import { getCSRF, readError } from '../utils/api'
import { formatTime, formatFileSize } from '../utils/format'

const { t } = useI18n()
const toast = useToast()
const emit = defineEmits(['logout'])

const fontData = ref({ fontDir: '', customFonts: [], totalSystemFamilies: 0 })
const searchQuery = ref('')
const uploadingFont = ref(false)
const refreshingFonts = ref(false)
const deletingFont = ref(false)
const pendingDeleteFont = ref(null)
const showDeleteFontModal = ref(false)
const fontFileInput = ref(null)

const fontColumns = computed(() => ([
  { accessorKey: 'name', header: t('ui.font_name') },
  { accessorKey: 'sizeBytes', header: t('ui.file_size') },
  { accessorKey: 'updatedAt', header: t('ui.updated_at') },
  { id: 'actions', header: t('ui.actions') }
]))

const filteredFonts = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  const list = fontData.value.customFonts || []
  if (!query) {
    return list
  }
  return list.filter(f => {
    return (
      (f.displayName && f.displayName.toLowerCase().includes(query)) ||
      (f.name && f.name.toLowerCase().includes(query)) ||
      (f.family && f.family.toLowerCase().includes(query))
    )
  })
})

async function loadFonts() {
  try {
    const resp = await fetch('/api/admin/fonts', { credentials: 'include' })
    if (!resp.ok) {
      if (resp.status === 401) emit('logout')
      return
    }
    fontData.value = await resp.json()
  } catch (err) {
    console.error('Failed to load fonts:', err)
  }
}

function triggerFontUpload() {
  fontFileInput.value?.click()
}

async function onFontFileSelected(event) {
  const files = event.target.files
  if (!files || files.length === 0) return

  const formData = new FormData()
  for (let i = 0; i < files.length; i++) {
    const file = files[i]
    const ext = file.name.slice(file.name.lastIndexOf('.')).toLowerCase()
    if (!['.ttf', '.ttc', '.otf'].includes(ext)) {
      toast.add({
        title: t('ui.font_format_unsupported'),
        description: file.name,
        color: 'error',
        icon: 'i-lucide-x-circle'
      })
      event.target.value = ''
      return
    }
    formData.append('files', file)
  }

  uploadingFont.value = true
  try {
    const resp = await fetch('/api/admin/fonts/upload', {
      method: 'POST',
      credentials: 'include',
      headers: {
        'X-CSRF-Token': getCSRF()
      },
      body: formData
    })
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: t('ui.upload_failed'), description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      if (resp.status === 401) emit('logout')
      return
    }
    toast.add({ title: t('ui.font_upload_success'), color: 'success', icon: 'i-lucide-check-circle' })
    await loadFonts()
  } finally {
    uploadingFont.value = false
    event.target.value = ''
  }
}

function confirmDeleteFont(font) {
  pendingDeleteFont.value = font
  showDeleteFontModal.value = true
}

async function executeDeleteFont() {
  if (!pendingDeleteFont.value) return
  deletingFont.value = true
  try {
    const resp = await fetch(`/api/admin/fonts/${encodeURIComponent(pendingDeleteFont.value.name)}`, {
      method: 'DELETE',
      credentials: 'include',
      headers: {
        'X-CSRF-Token': getCSRF()
      }
    })
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: t('ui.task_failed'), description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      if (resp.status === 401) emit('logout')
      return
    }
    toast.add({ title: t('ui.font_delete_success'), color: 'success', icon: 'i-lucide-check-circle' })
    showDeleteFontModal.value = false
    pendingDeleteFont.value = null
    await loadFonts()
  } finally {
    deletingFont.value = false
  }
}

async function refreshFonts() {
  refreshingFonts.value = true
  try {
    const resp = await fetch('/api/admin/fonts/refresh', {
      method: 'POST',
      credentials: 'include',
      headers: {
        'X-CSRF-Token': getCSRF()
      }
    })
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: t('ui.task_failed'), description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      if (resp.status === 401) emit('logout')
      return
    }
    toast.add({ title: t('ui.font_refresh_success'), color: 'success', icon: 'i-lucide-check-circle' })
    await loadFonts()
  } finally {
    refreshingFonts.value = false
  }
}

onMounted(() => {
  loadFonts()
})
</script>
