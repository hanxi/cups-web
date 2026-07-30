<template>
  <div class="p-3 sm:p-4 md:p-6 space-y-4 md:space-y-6">
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 md:gap-6">
      <UCard>
        <template #header>
          <h2 class="text-xl font-bold flex items-center gap-2">
            <UIcon name="i-lucide-users" class="w-5 h-5" />
            Управление пользователями
          </h2>
        </template>
        <UForm @submit="saveUser" :state="form" class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div>
            <UInput v-model="form.username" :disabled="form.protected" placeholder="Логин" :color="formErrors.username ? 'error' : undefined" />
            <p v-if="formErrors.username" class="text-xs text-error mt-1">{{ formErrors.username }}</p>
          </div>
          <div>
            <UInput type="password" v-model="form.password" :placeholder="isEditing ? 'Пусто = не менять' : 'Пароль'" :color="formErrors.password ? 'error' : undefined" />
            <p v-if="formErrors.password" class="text-xs text-error mt-1">{{ formErrors.password }}</p>
          </div>
          <USelect
            v-model="form.role"
            :disabled="form.protected"
            :items="roleItems"
            value-key="value"
            label-key="label"
          />
          <UInput v-model="form.contactName" placeholder="Контактное лицо" />
          <UInput v-model="form.phone" placeholder="Телефон" />
          <div>
            <UInput v-model="form.email" placeholder="Email" :color="formErrors.email ? 'error' : undefined" />
            <p v-if="formErrors.email" class="text-xs text-error mt-1">{{ formErrors.email }}</p>
          </div>
          <div class="flex gap-2 md:col-span-2">
            <UButton type="submit" color="primary" :loading="savingUser" :disabled="savingUser">{{ isEditing ? 'Сохранить' : 'Добавить' }}</UButton>
            <UButton type="button" variant="ghost" @click="resetForm">Сбросить</UButton>
          </div>
        </UForm>

        <div class="overflow-x-auto mt-4">
          <UTable :columns="userColumns" :data="users">
            <template #actions-cell="{ row }">
              <div class="flex gap-2">
                <UButton size="sm" variant="ghost" icon="i-lucide-pencil" @click="editUser(row.original)">Изменить</UButton>
                <UButton size="sm" variant="outline" color="error" icon="i-lucide-trash-2" :disabled="row.original.username === 'admin'" @click="confirmDelete(row.original)">Удалить</UButton>
              </div>
            </template>
          </UTable>
        </div>
      </UCard>

      <UCard>
        <template #header>
          <h2 class="text-xl font-bold flex items-center gap-2">
            <UIcon name="i-lucide-file-text" class="w-5 h-5" />
            История печати
          </h2>
        </template>
        <div class="flex flex-wrap gap-3 items-end mb-4">
          <UInput v-model="printFilters.username" placeholder="Имя пользователя" />
          <UInput type="date" v-model="printFilters.start" />
          <UInput type="date" v-model="printFilters.end" />
          <UButton variant="outline" @click="loadPrintRecords" icon="i-lucide-search">Поиск</UButton>
        </div>
        <div class="overflow-x-auto">
          <UTable :columns="printColumns" :data="printRecords">
            <template #download-cell="{ row }">
              <UButton size="xs" variant="ghost" icon="i-lucide-download" @click="downloadFile(row.original.id)">Скачать</UButton>
            </template>
          </UTable>
        </div>
      </UCard>
    </div>

    <UCard>
      <template #header>
        <h2 class="text-xl font-bold flex items-center gap-2">
          <UIcon name="i-lucide-settings" class="w-5 h-5" />
          Системные настройки
        </h2>
      </template>
      <div class="grid grid-cols-1 md:grid-cols-4 gap-3 items-end">
        <div>
          <label class="block text-sm font-medium mb-1">Автоочистка (дни)</label>
          <UInput type="number" step="1" v-model="settings.retentionDays" placeholder="Например: 30" />
        </div>
        <div>
          <label class="flex items-center gap-2 cursor-pointer h-9">
            <UCheckbox v-model="settings.saveHistory" />
            <span class="text-sm">Сохранять историю</span>
          </label>
        </div>
        <div class="flex items-end gap-2 md:col-span-2">
          <UButton color="primary" @click="saveSettings" icon="i-lucide-save" :loading="savingSettings" :disabled="savingSettings">Сохранить</UButton>
          <UButton variant="outline" @click="showCleanupConfirm = true" icon="i-lucide-trash-2" :loading="cleaningUp" :disabled="cleaningUp">Очистить сейчас</UButton>
        </div>
      </div>
      <div class="text-sm text-muted mt-2">Автоочистка удаляет старые записи и файлы через указанное количество дней. "Очистить сейчас" удалит ВСЕ записи и файлы. Если отключить сохранение истории, новые задачи не будут записываться.</div>
    </UCard>

    <UModal v-model:open="showDeleteModal">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">Подтверждение удаления</h3>
          <p>Вы уверены, что хотите удалить пользователя <strong>{{ pendingDeleteUser?.username }}</strong>?</p>
          <p class="text-sm text-muted">Это действие нельзя отменить.</p>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="showDeleteModal = false">Отмена</UButton>
            <UButton color="error" :loading="!!deletingUserId" @click="executeDelete">Удалить</UButton>
          </div>
        </div>
      </template>
    </UModal>

    <UModal v-model:open="showCleanupConfirm">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">Подтверждение очистки</h3>
          <p>Это действие <strong>удалит ВСЕ записи и файлы</strong>. Это нельзя отменить.</p>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="showCleanupConfirm = false">Отмена</UButton>
            <UButton color="error" :loading="cleaningUp" @click="triggerCleanup">Очистить</UButton>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { getCSRF, readError } from '../utils/api'

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
const settings = ref({ retentionDays: '', saveHistory: true })
const showCleanupConfirm = ref(false)

const savingUser = ref(false)
const savingSettings = ref(false)
const cleaningUp = ref(false)
const deletingUserId = ref(null)
const pendingDeleteUser = ref(null)
const showDeleteModal = ref(false)
const formErrors = ref({})

const isEditing = computed(() => !!form.value.id)

const roleItems = [
  { label: 'Пользователь', value: 'user' },
  { label: 'Администратор', value: 'admin' }
]

const userColumns = [
  { accessorKey: 'id', header: 'ID' },
  { accessorKey: 'username', header: 'Логин' },
  { accessorKey: 'role', header: 'Роль' },
  { accessorKey: 'contactName', header: 'Контакт' },
  { accessorKey: 'phone', header: 'Телефон' },
  { accessorKey: 'email', header: 'Email' },
  { id: 'actions', header: 'Действия' }
]

const printColumns = [
  { accessorKey: 'createdAt', header: 'Время' },
  { accessorKey: 'username', header: 'Пользователь' },
  { accessorKey: 'filename', header: 'Файл' },
  { accessorKey: 'pages', header: 'Стр.' },
  { accessorKey: 'status', header: 'Статус' },
  { id: 'download', header: 'Скачать' }
]

function validateForm() {
  formErrors.value = {}
  if (!form.value.username.trim()) {
    formErrors.value.username = 'Имя пользователя не может быть пустым'
  }
  if (!isEditing.value && !form.value.password) {
    formErrors.value.password = 'Пароль обязателен для нового пользователя'
  }
  if (form.value.email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.value.email)) {
    formErrors.value.email = 'Неверный формат Email'
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
      toast.add({ title: 'Ошибка сохранения', description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      if (resp.status === 401) emit('logout')
      return
    }
    toast.add({ title: isEditing.value ? 'Обновлено' : 'Создано', description: `Пользователь ${form.value.username} сохранен`, color: 'success', icon: 'i-lucide-check-circle' })
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
      toast.add({ title: 'Ошибка удаления', description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      if (resp.status === 401) emit('logout')
      return
    }
    toast.add({ title: 'Удалено', description: `Пользователь ${user.username} удален`, color: 'success', icon: 'i-lucide-check-circle' })
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
      toast.add({ title: 'Ошибка очистки', description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      if (resp.status === 401) emit('logout')
      return
    }
    const data = await resp.json()
    const count = data.deleted || 0
    toast.add({
      title: 'Очистка завершена',
      description: count > 0 ? `Удалено ${count} записей и файлов` : 'Нет записей для удаления',
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
    const payload = {
      retentionDays: parseInt(settings.value.retentionDays || '0', 10),
      saveHistory: settings.value.saveHistory
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
      toast.add({ title: 'Ошибка сохранения', description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      if (resp.status === 401) emit('logout')
      return
    }
    toast.add({ title: 'Сохранено', description: 'Системные настройки обновлены', color: 'success', icon: 'i-lucide-check-circle' })
    await loadSettings()
  } finally {
    savingSettings.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadUsers(), loadPrintRecords(), loadSettings()])
})
</script>
