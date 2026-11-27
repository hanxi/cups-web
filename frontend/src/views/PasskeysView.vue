<template>
  <div class="p-3 sm:p-4 md:p-6 max-w-3xl mx-auto space-y-4">
    <div class="flex items-center justify-between gap-2 flex-wrap">
      <h1 class="text-lg font-bold flex items-center gap-2">
        <UIcon name="i-lucide-fingerprint" class="w-5 h-5 text-primary" /> {{ t('ui.passkeys') }}
      </h1>
      <UButton size="sm" color="primary" icon="i-lucide-plus" :disabled="!supported" @click="openAdd">
        {{ t('ui.add_passkey') }}
      </UButton>
    </div>

    <UAlert color="neutral" variant="soft" icon="i-lucide-info" :description="t('ui.passkey_description')" />
    <UAlert v-if="!supported" color="warning" variant="soft" icon="i-lucide-triangle-alert" :title="t('ui.passkey_unsupported')" />

    <div v-if="loading" class="flex items-center justify-center py-10 text-muted gap-2">
      <UIcon name="i-lucide-loader-circle" class="w-5 h-5 animate-spin" /> {{ t('ui.loading') }}
    </div>
    <div v-else-if="!items.length" class="text-center py-12 text-muted">
      <UIcon name="i-lucide-fingerprint" class="w-10 h-10 mx-auto mb-2 opacity-40" />
      <div>{{ t('ui.no_passkeys_yet') }}</div>
    </div>
    <div v-else class="space-y-3">
      <UCard v-for="p in items" :key="p.id">
        <div class="flex items-center justify-between gap-3">
          <div class="min-w-0 space-y-1">
            <div class="font-medium truncate">{{ p.name }}</div>
            <div class="text-xs text-muted">
              <div>{{ t('ui.created_at', { date: formatDate(p.createdAt) }) }}</div>
              <div v-if="p.lastUsedAt">{{ t('ui.last_used_at', { date: formatDate(p.lastUsedAt) }) }}</div>
              <div v-else>{{ t('ui.not_used_yet') }}</div>
            </div>
          </div>
          <UButton size="xs" variant="outline" color="error" icon="i-lucide-trash-2" @click="confirmDelete(p)">
            {{ t('ui.delete') }}
          </UButton>
        </div>
      </UCard>
    </div>

    <UModal :title="t('ui.add_passkey')" v-model:open="showAdd">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">{{ t('ui.add_passkey') }}</h3>
          <div>
            <label class="block text-sm font-medium mb-1">{{ t('ui.passkey_name') }}</label>
            <UInput v-model="newName" :placeholder="t('ui.passkey_name_placeholder')" maxlength="64" class="w-full" />
          </div>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" @click="showAdd = false">{{ t('ui.cancel') }}</UButton>
            <UButton color="primary" :loading="adding" @click="submitAdd">{{ t('ui.create') }}</UButton>
          </div>
        </div>
      </template>
    </UModal>

    <UModal :title="t('ui.confirm_deletion')" v-model:open="showDelete">
      <template #content>
        <div class="p-6 space-y-4">
          <h3 class="text-lg font-semibold">{{ t('ui.confirm_deletion') }}</h3>
          <p>{{ t('ui.passkey_delete_confirm', { name: pending?.name }) }}</p>
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
import { ref, onMounted } from 'vue'
import { browserSupportsWebAuthn, startRegistration } from '@simplewebauthn/browser'
import { apiFetch, readError } from '../utils/api'

const { t, locale } = useI18n()
const emit = defineEmits(['logout'])
const toast = useToast()

const supported = browserSupportsWebAuthn()
const loading = ref(false)
const items = ref([])
const showAdd = ref(false)
const adding = ref(false)
const newName = ref('')
const showDelete = ref(false)
const deleting = ref(false)
const pending = ref(null)

function formatDate(v) {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString(locale.value)
}

function fail(title, description) {
  toast.add({ title, description, color: 'error', icon: 'i-lucide-x-circle' })
}

async function load() {
  loading.value = true
  try {
    const resp = await apiFetch('/api/passkeys', {}, () => emit('logout'))
    if (!resp.ok) return fail(t('ui.could_not_load'), await readError(resp))
    items.value = await resp.json()
  } finally {
    loading.value = false
  }
}

function openAdd() {
  newName.value = ''
  showAdd.value = true
}

async function submitAdd() {
  adding.value = true
  try {
    const begin = await apiFetch('/api/passkeys/register/begin', { method: 'POST' }, () => emit('logout'))
    if (!begin.ok) return fail(t('ui.could_not_create'), await readError(begin))
    const { sessionKey, options } = await begin.json()
    let attestation
    try {
      // options 即 go-webauthn 的 CredentialCreation，真正的参数在 publicKey 下。
      attestation = await startRegistration({ optionsJSON: options.publicKey })
    } catch (e) {
      if (e?.name === 'NotAllowedError' || e?.name === 'AbortError') return fail(t('ui.passkey_cancelled'))
      return fail(t('ui.could_not_create'), e?.message)
    }
    const name = newName.value.trim() || 'Passkey'
    const resp = await fetch(`/api/passkeys/register/finish?name=${encodeURIComponent(name)}`, {
      method: 'POST',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
        'X-Passkey-Session': sessionKey,
        'X-CSRF-Token': (document.cookie.match('(^|;)\\s*csrf_token\\s*=\\s*([^;]+)') || []).pop() || ''
      },
      body: JSON.stringify(attestation)
    })
    if (!resp.ok) return fail(t('ui.could_not_create'), await readError(resp))
    toast.add({ title: t('ui.passkey_added'), color: 'success', icon: 'i-lucide-check-circle' })
    showAdd.value = false
    await load()
  } finally {
    adding.value = false
  }
}

function confirmDelete(p) {
  pending.value = p
  showDelete.value = true
}

async function executeDelete() {
  if (!pending.value) return
  deleting.value = true
  try {
    const resp = await apiFetch(`/api/passkeys/${pending.value.id}`, { method: 'DELETE' }, () => emit('logout'))
    if (!resp.ok) return fail(t('ui.could_not_delete'), await readError(resp))
    toast.add({ title: t('ui.deleted_successfully'), color: 'success', icon: 'i-lucide-check-circle' })
    await load()
  } finally {
    deleting.value = false
    showDelete.value = false
    pending.value = null
  }
}

onMounted(load)
</script>
