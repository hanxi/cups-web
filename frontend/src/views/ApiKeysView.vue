<template>
  <div class="p-3 sm:p-4 md:p-6 max-w-5xl mx-auto space-y-4">
    <div class="flex items-center justify-between gap-2 flex-wrap">
      <h1 class="text-lg font-bold flex items-center gap-2">
        <UIcon name="i-lucide-key-round" class="w-5 h-5 text-primary" /> {{ t('ui.api_keys_2') }} </h1>
      <div class="flex items-center gap-2">
        <UButton
          variant="ghost"
          size="xs"
          icon="i-lucide-refresh-cw"
          :loading="loading"
          @click="loadKeys"
        >{{ t('ui.refresh') }}</UButton>
        <UButton
          size="sm"
          color="primary"
          icon="i-lucide-plus"
          @click="openCreate"
        >{{ t('ui.new_key') }}</UButton>
      </div>
    </div>

    <UAlert
      color="neutral"
      variant="soft"
      icon="i-lucide-info"
      :title="t('ui.usage')"
      :description="t('ui.keys_call_the_api_as_their_owner_and_allow_third')"
    />

    <div v-if="loading" class="flex items-center justify-center py-10 text-muted gap-2">
      <UIcon name="i-lucide-loader-circle" class="w-5 h-5 animate-spin" /> {{ t('ui.loading') }} </div>
    <div v-else-if="!keys.length" class="text-center py-16 text-muted">
      <UIcon name="i-lucide-key-off" class="w-10 h-10 mx-auto mb-2 opacity-40" />
      <div>{{ t('ui.no_api_keys_yet') }}</div>
    </div>
    <div v-else class="space-y-3">
      <UCard v-for="k in keys" :key="k.id" class="overflow-hidden">
        <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
          <div class="min-w-0 space-y-1">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="font-medium truncate">{{ k.name }}</span>
              <UBadge color="neutral" variant="outline" size="sm">
                <code class="font-mono">{{ k.prefix }}</code>
              </UBadge>
              <UBadge
                v-if="k.expiresAt"
                :color="isExpired(k) ? 'error' : 'warning'"
                variant="soft"
                size="sm"
              >
                {{ isExpired(k) ? t('ui.expired') : t('ui.expires', { p0: formatDate(k.expiresAt) }) }}
              </UBadge>
              <UBadge v-else color="success" variant="soft" size="sm">{{ t('ui.never_expires') }}</UBadge>
            </div>
            <div class="text-xs text-muted space-y-0.5">
              <div>{{ t('ui.created_at', { date: formatDate(k.createdAt) }) }}</div>
              <div v-if="k.lastUsedAt"> {{ t('ui.last_used_at', { date: formatDate(k.lastUsedAt) }) }}
                <span v-if="k.lastUsedIp" class="ml-1 text-default/50">{{ t('ui.from_address', { address: k.lastUsedIp }) }}</span>
              </div>
              <div v-else class="text-default/50">{{ t('ui.not_used_yet') }}</div>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <UButton
              size="xs"
              variant="outline"
              color="error"
              icon="i-lucide-trash-2"
              @click="confirmDelete(k)"
            >{{ t('ui.delete') }}</UButton>
          </div>
        </div>
      </UCard>
    </div>

    <!-- 新建密钥表单 -->
    <UModal :title="t('ui.new_api_key')" v-model:open="showCreate">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold flex items-center gap-2">
            <UIcon name="i-lucide-key-round" class="w-5 h-5 text-primary" /> {{ t('ui.new_api_key') }} </h3>
          <div class="space-y-3">
            <div>
              <label class="block text-sm font-medium mb-1">{{ t('ui.description') }}</label>
              <UInput v-model="createForm.name" :placeholder="t('ui.for_example_chatbot_home_printing_script')" maxlength="64" />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1">{{ t('ui.validity') }}</label>
              <USelect
                v-model="createForm.expiresInDays"
                :items="expiryOptions"
                value-key="value"
                label-key="label"
              />
              <p class="text-xs text-muted mt-1">{{ t('ui.the_key_stops_working_when_it_expires_choose_never_expires') }}</p>
            </div>
          </div>
          <div class="flex justify-end gap-2 pt-2">
            <UButton variant="ghost" @click="showCreate = false">{{ t('ui.cancel') }}</UButton>
            <UButton
              color="primary"
              :loading="creating"
              :disabled="!createForm.name.trim() || creating"
              @click="submitCreate"
            >{{ t('ui.create') }}</UButton>
          </div>
        </div>
      </template>
    </UModal>

    <!-- 明文密钥展示（仅一次） -->
    <UModal :title="t('ui.key_created_visible_only_once')" v-model:open="showReveal" :dismissible="false">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold flex items-center gap-2 text-warning">
            <UIcon name="i-lucide-alert-triangle" class="w-5 h-5" /> {{ t('ui.key_created_visible_only_once') }} </h3>
          <p class="text-sm text-muted"> {{ t('ui.copy_and_store_it_securely_now_the_complete_key_cannot') }} </p>
          <div class="p-3 rounded bg-elevated/60 border border-default">
            <div class="flex items-center gap-2">
              <code class="font-mono text-sm break-all flex-1 select-all">{{ revealedKey }}</code>
              <UButton
                size="xs"
                variant="outline"
                icon="i-lucide-copy"
                @click="copyKey"
              >{{ t('ui.copy') }}</UButton>
            </div>
          </div>
          <div class="text-xs text-muted space-y-1">
            <div>{{ t('ui.example_request') }}</div>
            <pre class="p-2 rounded bg-elevated/60 border border-default overflow-x-auto"><code>curl -H "Authorization: Bearer {{ revealedKey }}" \
     {{ origin }}/api/printers</code></pre>
          </div>
          <div class="flex justify-end">
            <UButton color="primary" @click="closeReveal">{{ t('ui.i_have_saved_it') }}</UButton>
          </div>
        </div>
      </template>
    </UModal>

    <UModal :title="t('ui.confirm_deletion')" v-model:open="showDelete">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">{{ t('ui.confirm_deletion') }}</h3>
          <p>{{ t('ui.delete_key_confirm', { name: pendingDelete?.name, prefix: pendingDelete?.prefix }) }}</p>
          <p class="text-sm text-muted">{{ t('ui.requests_using_this_key_will_be_rejected_immediately_this_cannot') }}</p>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="showDelete = false">{{ t('ui.cancel') }}</UButton>
            <UButton color="error" :loading="deleting" @click="executeDelete">{{ t('ui.confirm_deletion') }}</UButton>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted } from 'vue'
import { apiFetch, readError } from '../utils/api'

const { t, locale } = useI18n()

const emit = defineEmits(['logout'])
const toast = useToast()

const loading = ref(false)
const keys = ref([])
const showCreate = ref(false)
const creating = ref(false)
const createForm = ref({ name: '', expiresInDays: 0 })
const showReveal = ref(false)
const revealedKey = ref('')
const showDelete = ref(false)
const deleting = ref(false)
const pendingDelete = ref(null)

const origin = computed(() => (typeof window !== 'undefined' ? window.location.origin : ''))

const expiryOptions = computed(() => ([
  { label: t('ui.never_expires'), value: 0 },
  { label: t('ui.option_7_days'), value: 7 },
  { label: t('ui.option_30_days'), value: 30 },
  { label: t('ui.option_90_days'), value: 90 },
  { label: t('ui.one_year'), value: 365 }
]))

function formatDate(v) {
  if (!v) return ''
  try {
    const d = new Date(v)
    if (Number.isNaN(d.getTime())) return v
    return d.toLocaleString(locale.value)
  } catch {
    return v
  }
}

function isExpired(k) {
  if (!k.expiresAt) return false
  const d = new Date(k.expiresAt)
  if (Number.isNaN(d.getTime())) return false
  return d.getTime() < Date.now()
}

async function loadKeys() {
  loading.value = true
  try {
    const resp = await apiFetch('/api/api-keys', {}, () => emit('logout'))
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: t('ui.could_not_load'), description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      return
    }
    keys.value = await resp.json()
  } finally {
    loading.value = false
  }
}

function openCreate() {
  createForm.value = { name: '', expiresInDays: 0 }
  showCreate.value = true
}

async function submitCreate() {
  const name = createForm.value.name.trim()
  if (!name) return
  creating.value = true
  try {
    const resp = await apiFetch('/api/api-keys', {
      method: 'POST',
      body: JSON.stringify({
        name,
        expiresInDays: Number(createForm.value.expiresInDays) || 0
      })
    }, () => emit('logout'))
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: t('ui.could_not_create'), description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      return
    }
    const data = await resp.json()
    revealedKey.value = data.key || ''
    showCreate.value = false
    showReveal.value = true
    await loadKeys()
  } finally {
    creating.value = false
  }
}

async function copyKey() {
  try {
    await navigator.clipboard.writeText(revealedKey.value)
    toast.add({ title: t('ui.copied'), color: 'success', icon: 'i-lucide-check-circle' })
  } catch {
    toast.add({ title: t('ui.could_not_copy'), description: t('ui.please_select_and_copy_manually'), color: 'warning', icon: 'i-lucide-alert-triangle' })
  }
}

function closeReveal() {
  revealedKey.value = ''
  showReveal.value = false
}

function confirmDelete(k) {
  pendingDelete.value = k
  showDelete.value = true
}

async function executeDelete() {
  const k = pendingDelete.value
  if (!k) return
  deleting.value = true
  try {
    const resp = await apiFetch(`/api/api-keys/${k.id}`, { method: 'DELETE' }, () => emit('logout'))
    if (!resp.ok) {
      const msg = await readError(resp)
      toast.add({ title: t('ui.could_not_delete'), description: msg, color: 'error', icon: 'i-lucide-x-circle' })
      return
    }
    toast.add({ title: t('ui.deleted_successfully'), color: 'success', icon: 'i-lucide-check-circle' })
    await loadKeys()
  } finally {
    deleting.value = false
    showDelete.value = false
    pendingDelete.value = null
  }
}

onMounted(loadKeys)
</script>
