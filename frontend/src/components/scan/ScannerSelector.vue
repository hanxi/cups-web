<template>
  <UCard>
    <template #header>
      <div class="flex items-center gap-2 font-semibold">
        <UIcon name="i-lucide-scan" class="w-5 h-5" />
        扫描仪
      </div>
    </template>
    <UFormField label="选择扫描仪">
      <USelect
        :model-value="modelValue"
        :items="scannerItems"
        value-key="value"
        label-key="label"
        class="w-full"
        @update:model-value="onSelect"
      />
    </UFormField>
    <div v-if="loading" class="mt-2 text-sm text-gray-500">
      正在搜索扫描仪...
    </div>
    <div v-else-if="scanners.length === 0" class="mt-2 text-sm text-gray-500">
      未发现可用的扫描仪
    </div>
  </UCard>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'

const props = defineProps({
  modelValue: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue', 'change'])

const scanners = ref([])
const loading = ref(false)

const scannerItems = computed(() =>
  scanners.value.map(s => ({ 
    label: `${s.name} — ${s.description || s.vendor + ' ' + s.model}`, 
    value: s.device 
  }))
)

function onSelect(val) {
  emit('update:modelValue', val)
  emit('change')
}

async function fetchScanners() {
  loading.value = true
  try {
    const response = await fetch('/api/scanners')
    if (response.ok) {
      scanners.value = await response.json()
    } else {
      console.error('Failed to fetch scanners')
    }
  } catch (error) {
    console.error('Error fetching scanners:', error)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchScanners()
})
</script>