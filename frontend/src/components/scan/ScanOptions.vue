<template>
  <UCard>
    <template #header>
      <div class="flex items-center gap-2 font-semibold">
        <UIcon name="i-lucide-settings-2" class="w-5 h-5" />
        扫描参数
      </div>
    </template>
    <div class="space-y-4">
      <!-- 分辨率 -->
      <UFormField label="分辨率 (DPI)">
        <USelect
          :model-value="resolution"
          :items="resolutionItems"
          value-key="value"
          label-key="label"
          class="w-full"
          @update:model-value="$emit('update:resolution', $event)"
        />
      </UFormField>

      <!-- 颜色模式 -->
      <UFormField label="颜色模式">
        <div class="flex rounded-lg border border-muted overflow-hidden">
          <label v-for="item in colorModeItems" :key="item.value"
            class="flex-1 flex items-center justify-center gap-1.5 py-2 px-2 cursor-pointer text-sm transition"
            :class="colorMode === item.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'">
            <input type="radio" :value="item.value" :checked="colorMode === item.value" class="sr-only" @change="$emit('update:colorMode', item.value)" />
            <UIcon :name="item.icon" class="w-3.5 h-3.5 shrink-0" />
            <span class="text-xs whitespace-nowrap">{{ item.label }}</span>
          </label>
        </div>
      </UFormField>

      <!-- 纸张大小 -->
      <UFormField label="纸张大小">
        <USelect
          :model-value="paperSize"
          :items="paperSizeItems"
          value-key="value"
          label-key="label"
          class="w-full"
          @update:model-value="$emit('update:paperSize', $event)"
        />
      </UFormField>

      <!-- 扫描区域 -->
      <UFormField label="扫描区域" hint="留空使用默认纸张大小">
        <UInput
          :model-value="scanArea"
          placeholder="如：210mm x 297mm"
          class="w-full"
          @update:model-value="$emit('update:scanArea', $event)"
        />
      </UFormField>
    </div>
  </UCard>
</template>

<script setup>
const props = defineProps({
  resolution: { type: Number, default: 300 },
  colorMode: { type: String, default: 'color' },
  paperSize: { type: String, default: 'A4' },
  scanArea: { type: String, default: '' }
})

defineEmits(['update:resolution', 'update:colorMode', 'update:paperSize', 'update:scanArea'])

const resolutionItems = [
  { label: '72 DPI (草稿)', value: 72 },
  { label: '150 DPI (标准)', value: 150 },
  { label: '300 DPI (高质量)', value: 300 },
  { label: '600 DPI (超高)', value: 600 },
  { label: '1200 DPI (专业)', value: 1200 }
]

const colorModeItems = [
  { label: '彩色', value: 'color', icon: 'i-lucide-palette' },
  { label: '灰度', value: 'gray', icon: 'i-lucide-contrast' },
  { label: '黑白', value: 'lineart', icon: 'i-lucide-circle' }
]

const paperSizeItems = [
  { label: 'A4 (210×297mm)', value: 'A4' },
  { label: 'A3 (297×420mm)', value: 'A3' },
  { label: 'Letter (8.5×11in)', value: 'Letter' },
  { label: 'Legal (8.5×14in)', value: 'Legal' },
  { label: '自定义', value: 'custom' }
]
</script>