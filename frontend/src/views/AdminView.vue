<template>
  <div class="p-3 sm:p-4 md:p-6 space-y-4 md:space-y-6">
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 md:gap-6">
      <UCard>
        <template #header>
          <h2 class="text-xl font-bold flex items-center gap-2">
            <UIcon name="i-lucide-users" class="w-5 h-5" /> {{ t('ui.user_management') }} </h2>
        </template>
        <UForm @submit="saveUser" :state="form" class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div>
            <UInput :aria-label="t('ui.username')" v-model="form.username" :disabled="form.protected" :placeholder="t('ui.username')" :color="formErrors.username ? 'error' : undefined" />
            <p v-if="formErrors.username" class="text-xs text-error mt-1">{{ t(formErrors.username) }}</p>
          </div>
          <div>
            <UInput :aria-label="t('ui.password')" type="password" v-model="form.password" :placeholder="isEditing ? t('ui.leave_blank_to_keep_the_current_password') : t('ui.password')" :color="formErrors.password ? 'error' : undefined" />
            <p v-if="formErrors.password" class="text-xs text-error mt-1">{{ t(formErrors.password) }}</p>
          </div>
          <USelect
            v-model="form.role"
            :disabled="form.protected"
            :items="roleItems"
            value-key="value"
            label-key="label"
          />
          <UInput :aria-label="t('ui.contact_name')" v-model="form.contactName" :placeholder="t('ui.contact_name')" />
          <UInput :aria-label="t('ui.phone')" v-model="form.phone" :placeholder="t('ui.contact_telephone')" />
          <div>
            <UInput :aria-label="t('ui.email')" v-model="form.email" :placeholder="t('ui.email')" :color="formErrors.email ? 'error' : undefined" />
            <p v-if="formErrors.email" class="text-xs text-error mt-1">{{ t(formErrors.email) }}</p>
          </div>
          <div class="flex gap-2 md:col-span-2">
            <UButton type="submit" color="primary" :loading="savingUser" :disabled="savingUser">{{ isEditing ? t('ui.save') : t('ui.add_user') }}</UButton>
            <UButton type="button" variant="ghost" @click="resetForm">{{ t('ui.reset') }}</UButton>
          </div>
        </UForm>

        <div class="overflow-x-auto mt-4">
          <UTable :columns="userColumns" :data="users">
            <template #role-cell="{ row }">{{ row.original.role === 'admin' ? t('ui.administrator') : row.original.role === 'user' ? t('ui.standard_user') : row.original.role }}</template>
            <template #actions-cell="{ row }">
              <div class="flex gap-2">
                <UButton size="sm" variant="ghost" icon="i-lucide-pencil" @click="editUser(row.original)">{{ t('ui.edit') }}</UButton>
                <UButton size="sm" variant="outline" color="error" icon="i-lucide-trash-2" :disabled="row.original.username === 'admin'" @click="confirmDelete(row.original)">{{ t('ui.delete') }}</UButton>
              </div>
            </template>
          </UTable>
        </div>
      </UCard>

      <UCard>
        <template #header>
          <h2 class="text-xl font-bold flex items-center gap-2">
            <UIcon name="i-lucide-file-text" class="w-5 h-5" /> {{ t('ui.print_history') }} </h2>
        </template>
        <div class="flex flex-wrap gap-3 items-end mb-4">
          <UInput :aria-label="t('ui.username')" v-model="printFilters.username" :placeholder="t('ui.username_2')" />
          <UInput :aria-label="t('accessibility.start_date')" type="date" v-model="printFilters.start" />
          <UInput :aria-label="t('accessibility.end_date')" type="date" v-model="printFilters.end" />
          <UButton variant="outline" @click="loadPrintRecords" icon="i-lucide-search">{{ t('ui.search') }}</UButton>
        </div>
        <div class="overflow-x-auto">
          <UTable :columns="printColumns" :data="printRecords">
            <template #createdAt-cell="{ row }">{{ formatTime(row.original.createdAt) }}</template>
            <template #status-cell="{ row }">{{ statusText(row.original.status) }}</template>
            <template #download-cell="{ row }">
              <UButton size="xs" variant="ghost" icon="i-lucide-download" @click="downloadFile(row.original.id)">{{ t('ui.download') }}</UButton>
            </template>
          </UTable>
        </div>
      </UCard>
    </div>

    <UCard>
      <template #header>
        <h2 class="text-xl font-bold flex items-center gap-2">
          <UIcon name="i-lucide-settings" class="w-5 h-5" /> {{ t('ui.system_settings') }} </h2>
      </template>
      <div class="grid grid-cols-1 md:grid-cols-4 gap-3 items-end">
        <div>
          <label class="block text-sm font-medium mb-1">{{ t('ui.automatically_delete_after_days') }}</label>
          <UInput :aria-label="t('ui.automatically_delete_after_days')" type="number" step="1" v-model="settings.retentionDays" :placeholder="t('ui.for_example_30')" />
        </div>
        <div>
          <label class="block text-sm font-medium mb-1">{{ t('ui.maximum_pages_per_job') }}</label>
          <UInput :aria-label="t('ui.maximum_pages_per_job')" type="number" step="1" min="0" v-model="settings.maxPagesPerJob" :placeholder="t('ui.option_0_means_unlimited')" />
        </div>
        <div>
          <label class="block text-sm font-medium mb-1">{{ t('ui.maximum_file_size_mb') }}</label>
          <UInput :aria-label="t('ui.maximum_file_size_mb')" type="number" step="1" min="0" v-model="settings.maxUploadMB" :placeholder="t('ui.option_0_means_unlimited')" />
        </div>
        <div>
          <label class="flex items-center gap-2 cursor-pointer h-9">
            <UCheckbox v-model="settings.saveHistory" />
            <span class="text-sm">{{ t('ui.save_print_history') }}</span>
          </label>
        </div>
        <div class="md:col-span-4">
          <label class="flex items-center gap-2 cursor-pointer">
            <UCheckbox v-model="settings.guestMode" />
            <span class="text-sm">{{ t('ui.enable_guest_mode_no_sign_in_required') }}</span>
          </label>
          <p class="text-xs text-muted mt-1 ml-6">{{ t('ui.visitors_automatically_enter_the_print_page_using_the_built_in') }}</p>
        </div>
        <div class="flex items-end gap-2 md:col-span-4">
          <UButton color="primary" @click="saveSettings" icon="i-lucide-save" :loading="savingSettings" :disabled="savingSettings">{{ t('ui.save_settings') }}</UButton>
          <UButton variant="outline" @click="showCleanupConfirm = true" icon="i-lucide-trash-2" :loading="cleaningUp" :disabled="cleaningUp">{{ t('ui.clean_up_now') }}</UButton>
        </div>
      </div>
      <div class="text-sm text-muted mt-2">{{ t('ui.automatic_clean_up_removes_expired_print_records_and_files_after') }}</div>

      <div class="mt-4 pt-4 border-t border-default">
        <label class="block text-sm font-medium mb-1">{{ t('ui.custom_css') }}</label>
        <UTextarea
          v-model="settings.customCss"
          :rows="6"
          :placeholder="t('ui.for_example_change_theme_colours_font_sizes_or_button_gradients')"
          class="w-full font-mono text-xs"
          :maxlength="32768"
        />
        <div class="text-xs text-muted mt-1">{{ t('ui.applies_immediately_to_the_login_page_and_main_interface_maximum') }}</div>
      </div>
    </UCard>

    <UModal :title="t('ui.confirm_deletion')" v-model:open="showDeleteModal">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">{{ t('ui.confirm_deletion') }}</h3>
          <p>{{ t('ui.delete_user_confirm', { name: pendingDeleteUser?.username }) }}</p>
          <p class="text-sm text-muted">{{ t('ui.this_cannot_be_undone') }}</p>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="showDeleteModal = false">{{ t('ui.cancel') }}</UButton>
            <UButton color="error" :loading="!!deletingUserId" @click="executeDelete">{{ t('ui.confirm_deletion') }}</UButton>
          </div>
        </div>
      </template>
    </UModal>

    <UModal :title="t('ui.confirm_clean_up')" v-model:open="showCleanupConfirm">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">{{ t('ui.confirm_clean_up') }}</h3>
          <p>{{ t('ui.cleanup_confirm') }}</p>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="showCleanupConfirm = false">{{ t('ui.cancel') }}</UButton>
            <UButton color="error" :loading="cleaningUp" @click="triggerCleanup">{{ t('ui.confirm_clean_up') }}</UButton>
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
import { formatTime, statusText } from '../utils/format'

const { t } = useI18n()

const toast = useToast()
const emit = defineEmits(['logout'])

const users = ref([])
const form = ref({
  id: null,
  username: '',
  password: '',
  role: 'user',
  protected: false,
  contactName: '',
  phone: '',
  email: ''
})
const printFilters = ref({ username: '', start: '', end: '' })
const printRecords = ref([])
const settings = ref({ retentionDays: '', saveHistory: true, maxPagesPerJob: '0', maxUploadMB: '0', customCss: '', guestMode: false })
const showCleanupConfirm = ref(false)

const savingUser = ref(false)
const savingSettings = ref(false)
const cleaningUp = ref(false)
const deletingUserId = ref(null)
const pendingDeleteUser = ref(null)
const showDeleteModal = ref(false)
const formErrors = ref({})

const isEditing = computed(() => !!form.value.id)

const roleItems = computed(() => ([
  { label: t('ui.standard_user'), value: 'user' },
  { label: t('ui.administrator'), value: 'admin' }
]))

const userColumns = computed(() => ([
  { accessorKey: 'id', header: 'ID' },
  { accessorKey: 'username', header: t('ui.username') },
  { accessorKey: 'role', header: t('ui.role') },
  { accessorKey: 'contactName', header: t('ui.contact_name') },
  { accessorKey: 'phone', header: t('ui.phone') },
  { accessorKey: 'email', header: t('ui.email') },
  { id: 'actions', header: t('ui.actions') }
]))

const printColumns = computed(() => ([
  { accessorKey: 'createdAt', header: t('ui.time') },
  { accessorKey: 'username', header: t('ui.user') },
  { accessorKey: 'filename', header: t('ui.file') },
  { accessorKey: 'pages', header: t('ui.pages') },
  { accessorKey: 'status', header: t('ui.status') },
  { id: 'download', header: t('ui.download') }
]))

function validateForm() {
  formErrors.value = {}
  if (!form.value.username.trim()) {
    formErrors.value.username = 'ui.username_is_required'
  }
  if (!isEditing.value && !form.value.password) {
    formErrors.value.password = 'ui.a_new_user_must_have_a_password'
  }
  if (form.value.email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.value.email)) {
    formErrors.value.email = 'ui.invalid_email_address'
  }
  return Object.keys(formErrors.value).length === 0
}

function resetForm() {
  form.value = {
    id: null,
    username: '',
    password: '',
    role: 'user',
    protected: false,
    contactName: '',
    phone: '',
    email: ''
  }
  formErrors.value = {}
}

function editUser(user) {
  form.value = {
    id: user.id,
    username: user.username,
    password: '',
    role: user.role,
    protected: user.username === 'admin',
    contactName: user.contactName || '',
    phone: user.phone || '',
    email: user.email || ''
  }
  formErrors.value = {}
}

async function loadUsers() {
  const resp = await fetch('/api/admin/users', { credentials: 'include' })
  if (!resp.ok) {
    if (resp.status === 401) emit('logout')
    return
  }
  users.value = await resp.json()
}

async function saveUser() {
  if (!validateForm()) return
  savingUser.value = true
  try {
    const payload = {
      username: form.value.username,
      password: form.value.password,
      role: form.value.role,
      contactName: form.value.contactName,
      phone: form.value.phone,
      email: form.value.email
    }
    const url = isEditing.value ? `/api/admin/users/${form.value.id}` : '/api/admin/users'
    const method = isEditing.value ? 'PUT' : 'POST'
    const resp = await fetch(url, {
      method,
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCSRF()
      },
      body: JSON.stringify(payload)
    })
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: t('ui.could_not_save'), description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      if (resp.status === 401) emit('logout')
      return
    }
    toast.add({ title: isEditing.value ? t('ui.updated_successfully') : t('ui.created_successfully'), description: t('ui.user_saved', { p0: form.value.username }), color: 'success', icon: 'i-lucide-check-circle' })
    await loadUsers()
    resetForm()
  } finally {
    savingUser.value = false
  }
}

function confirmDelete(user) {
  pendingDeleteUser.value = user
  showDeleteModal.value = true
}

async function executeDelete() {
  const user = pendingDeleteUser.value
  if (!user) return
  deletingUserId.value = user.id
  try {
    const resp = await fetch(`/api/admin/users/${user.id}`, {
      method: 'DELETE',
      credentials: 'include',
      headers: { 'X-CSRF-Token': getCSRF() }
    })
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: t('ui.could_not_delete'), description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      if (resp.status === 401) emit('logout')
      return
    }
    toast.add({ title: t('ui.deleted_successfully'), description: t('ui.user_deleted', { p0: user.username }), color: 'success', icon: 'i-lucide-check-circle' })
    await loadUsers()
  } finally {
    deletingUserId.value = null
    showDeleteModal.value = false
    pendingDeleteUser.value = null
  }
}

function downloadFile(id) {
  window.open(`/api/print-records/${id}/file`, '_blank')
}

async function loadPrintRecords() {
  const params = new URLSearchParams()
  if (printFilters.value.username) params.set('username', printFilters.value.username)
  if (printFilters.value.start) params.set('start', printFilters.value.start)
  if (printFilters.value.end) params.set('end', printFilters.value.end)
  const resp = await fetch(`/api/admin/print-records?${params.toString()}`, { credentials: 'include' })
  if (!resp.ok) {
    if (resp.status === 401) emit('logout')
    return
  }
  printRecords.value = await resp.json()
}

async function loadSettings() {
  const resp = await fetch('/api/admin/settings', { credentials: 'include' })
  if (!resp.ok) {
    if (resp.status === 401) emit('logout')
    return
  }
  const data = await resp.json()
  settings.value.retentionDays = String(data.retentionDays || 0)
  settings.value.saveHistory = data.saveHistory !== false
  settings.value.maxPagesPerJob = String(data.maxPagesPerJob || 0)
  // 后端以字节存储，前端 UI 用 MB 展示（1 MB = 1024*1024 B）。
  const bytes = Number(data.maxUploadBytes || 0)
  settings.value.maxUploadMB = String(bytes > 0 ? Math.round(bytes / (1024 * 1024)) : 0)
  settings.value.customCss = typeof data.customCss === 'string' ? data.customCss : ''
  settings.value.guestMode = data.guestMode === true
}

async function triggerCleanup() {
  showCleanupConfirm.value = false
  cleaningUp.value = true
  try {
    const resp = await fetch('/api/admin/cleanup', {
      method: 'POST',
      credentials: 'include',
      headers: { 'X-CSRF-Token': getCSRF() }
    })
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: t('ui.could_not_clean_up'), description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      if (resp.status === 401) emit('logout')
      return
    }
    const data = await resp.json()
    const count = data.deleted || 0
    toast.add({
      title: t('ui.clean_up_complete'),
      description: count > 0 ? t('ui.deleted_print_records_and_associated_files', { p0: count }) : t('ui.no_records_need_cleaning_up'),
      color: 'success',
      icon: 'i-lucide-check-circle'
    })
    await loadPrintRecords()
  } finally {
    cleaningUp.value = false
  }
}

async function saveSettings() {
  savingSettings.value = true
  try {
    const maxPages = Math.max(0, parseInt(settings.value.maxPagesPerJob || '0', 10) || 0)
    const maxMB = Math.max(0, parseInt(settings.value.maxUploadMB || '0', 10) || 0)
    const payload = {
      retentionDays: parseInt(settings.value.retentionDays || '0', 10),
      saveHistory: settings.value.saveHistory,
      maxPagesPerJob: maxPages,
      maxUploadBytes: maxMB * 1024 * 1024,
      customCss: settings.value.customCss || '',
      guestMode: settings.value.guestMode === true
    }
    const resp = await fetch('/api/admin/settings', {
      method: 'PUT',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCSRF()
      },
      body: JSON.stringify(payload)
    })
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: t('ui.could_not_save'), description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      if (resp.status === 401) emit('logout')
      return
    }
    toast.add({ title: t('ui.saved_successfully'), description: t('ui.system_settings_updated'), color: 'success', icon: 'i-lucide-check-circle' })
    await loadSettings()
    // 立即把新的自定义 CSS 应用到当前页面，避免管理员必须手动 reload（Issue #57）。
    const el = document.getElementById('app-custom-css')
    if (el || settings.value.customCss) {
      const style = el || Object.assign(document.createElement('style'), { id: 'app-custom-css' })
      if (!el) document.head.appendChild(style)
      style.textContent = settings.value.customCss || ''
    }
  } finally {
    savingSettings.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadUsers(), loadPrintRecords(), loadSettings()])
})
</script>
