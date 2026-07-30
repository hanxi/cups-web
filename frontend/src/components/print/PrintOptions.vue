<template>
  <UCard>
    <template #header>
      <div class="flex items-center gap-2 font-semibold">
        <UIcon name="i-lucide-settings-2" class="w-5 h-5" />
        Параметры печати
      </div>
    </template>
    <div class="space-y-4">
      <!-- ═══ Основные опции ═══ -->
      <!-- Цвет -->
      <UFormField label="Цвет" :hint="isColor ? undefined : 'Цветной контент будет напечатан в оттенках серого'">
        <div class="flex rounded-lg border border-muted overflow-hidden">
          <label v-for="item in colorItems" :key="String(item.value)"
            class="flex-1 flex items-center justify-center gap-1.5 py-2 px-2 cursor-pointer text-sm transition"
            :class="isColor === item.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'">
            <input type="radio" :value="item.value" :checked="isColor === item.value" class="sr-only" @change="$emit('update:isColor', item.value)" />
            <UIcon :name="item.icon" class="w-3.5 h-3.5 shrink-0" />
            <span class="text-xs whitespace-nowrap">{{ item.label }}</span>
          </label>
        </div>
      </UFormField>

      <!-- Двусторонняя печать + Копии -->
      <div class="grid grid-cols-2 gap-3">
        <UFormField label="Двусторонняя печать">
          <USelect :model-value="duplex" :items="duplexItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:duplex', $event)" />
        </UFormField>

        <UFormField label="Копии">
          <UInput
            :model-value="copies"
            type="number"
            :min="1"
            :max="99"
            class="w-full"
            @update:model-value="$emit('update:copies', Number($event))"
          />
        </UFormField>
      </div>

      <!-- ═══ Расширенные настройки ═══ -->
      <div class="border-t border-default pt-2">
        <button
          type="button"
          class="flex items-center gap-1.5 w-full text-xs sm:text-sm text-primary hover:text-primary/80 transition cursor-pointer py-1"
          @click="showAdvanced = !showAdvanced"
        >
          <UIcon
            name="i-lucide-chevron-right"
            class="w-3.5 h-3.5 transition-transform duration-200 shrink-0"
            :class="showAdvanced ? 'rotate-90' : ''"
          />
          <span class="font-medium">Расширенные настройки</span>
          <span v-if="!showAdvanced" class="text-[11px] sm:text-xs text-muted ml-1 truncate">{{ advancedSummary }}</span>
        </button>

        <div
          class="overflow-hidden transition-all duration-300 ease-in-out"
          :style="{ maxHeight: showAdvanced ? '1000px' : '0px', opacity: showAdvanced ? 1 : 0, visibility: showAdvanced ? 'visible' : 'hidden' }"
        >
          <div class="space-y-4 pt-3">
            <!-- Размер бумаги + Тип бумаги -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <UFormField label="Размер бумаги">
                <USelect :model-value="paperSize" :items="paperSizeItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:paperSize', $event)" />
              </UFormField>
              <UFormField label="Тип бумаги">
                <USelect :model-value="paperType" :items="paperTypeItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:paperType', $event)" />
              </UFormField>
            </div>

            <!-- Лоток -->
            <UFormField v-if="mediaSourceItems.length > 1" label="Лоток" hint="Выберите лоток; «Авто» — выбор принтером">
              <USelect :model-value="mediaSource" :items="mediaSourceItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:mediaSource', $event)" />
            </UFormField>

            <!-- Масштаб + Страницы -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <UFormField label="Масштаб">
                <USelect :model-value="printScaling" :items="scalingItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:printScaling', $event)" />
              </UFormField>
              <UFormField label="Страницы" :hint="pageRangeError || 'Например: 1-5 8'">
                <UInput
                  :model-value="pageRange"
                  placeholder="Пусто = все"
                  class="w-full"
                  :color="pageRangeError ? 'error' : undefined"
                  @update:model-value="onPageRangeInput"
                />
              </UFormField>
            </div>

            <!-- Страниц на листе (N-up) -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <UFormField label="Страниц на листе" hint="Разместить несколько страниц на одном листе">
                <USelect :model-value="numberUp" :items="numberUpItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:numberUp', Number($event))" />
              </UFormField>
              <UFormField v-if="numberUp > 1" label="Порядок страниц">
                <USelect :model-value="numberUpLayout" :items="numberUpLayoutItems" value-key="value" label-key="label" class="w-full" @update:model-value="$emit('update:numberUpLayout', $event)" />
              </UFormField>
            </div>
            <UFormField v-if="numberUp > 1" label="Рамка">
              <label class="flex items-center gap-2 p-2 border rounded-lg cursor-pointer transition hover:bg-elevated w-fit"
                :class="pageBorder === 'single' ? 'border-primary bg-primary/5' : 'border-muted'">
                <UCheckbox :model-value="pageBorder === 'single'" @update:model-value="$emit('update:pageBorder', $event ? 'single' : 'none')" />
                <UIcon name="i-lucide-square" class="w-4 h-4" />
                <span class="text-sm">Добавить рамку для каждой страницы</span>
              </label>
            </UFormField>

            <!-- Подмножество страниц -->
            <UFormField label="Подмножество" hint="Для ручной двусторонней печати: сначала нечетные, затем четные">
              <div class="flex rounded-lg border border-muted overflow-hidden">
                <label
                  v-for="item in pageSetItems"
                  :key="item.value"
                  class="flex-1 flex items-center justify-center gap-1.5 py-2 px-2 cursor-pointer text-sm transition"
                  :class="pageSet === item.value ? 'bg-primary text-white font-medium' : 'hover:bg-elevated'"
                >
                  <input type="radio" :value="item.value" :checked="pageSet === item.value" class="sr-only" @change="$emit('update:pageSet', item.value)" />
                  <UIcon :name="item.icon" class="w-3.5 h-3.5 shrink-0" />
                  <span class="text-xs whitespace-nowrap">{{ item.label }}</span>
                </label>
              </div>
            </UFormField>

            <!-- Зеркальная печать -->
            <UFormField label="Зеркальная печать">
              <label class="flex items-center gap-2 p-2 border rounded-lg cursor-pointer transition hover:bg-elevated w-fit"
                :class="mirror ? 'border-primary bg-primary/5' : 'border-muted'">
                <UCheckbox :model-value="mirror" @update:model-value="$emit('update:mirror', $event)" />
                <UIcon name="i-lucide-flip-horizontal" class="w-4 h-4" />
                <span class="text-sm">Отразить по горизонтали</span>
              </label>
            </UFormField>

            <!-- Водяной знак -->
            <UFormField label="Водяной знак" hint="Пусто = без знака; Например: КОПИЯ">
              <UInput
                :model-value="watermarkText"
                placeholder="Пусто = без знака"
                class="w-full"
                @update:model-value="$emit('update:watermarkText', $event)"
              />
            </UFormField>
          </div>
        </div>
      </div>

    </div>
  </UCard>
</template>

<script setup>
import { ref, computed, watch } from 'vue'

const props = defineProps({
  isColor: { type: Boolean, default: true },
  duplex: { type: String, default: 'one-sided' },
  copies: { type: Number, default: 1 },
  paperSize: { type: String, default: 'A4' },
  paperType: { type: String, default: 'plain' },
  mediaSource: { type: String, default: 'auto' },
  mediaSourceSupported: { type: Array, default: () => [] },
  printScaling: { type: String, default: 'fit' },
  pageRange: { type: String, default: '' },
  pageSet: { type: String, default: 'all' },
  mirror: { type: Boolean, default: false },
  watermarkText: { type: String, default: '' },
  numberUp: { type: Number, default: 1 },
  numberUpLayout: { type: String, default: 'lrtb' },
  pageBorder: { type: String, default: 'none' },
  printing: { type: Boolean, default: false }
})

const emit = defineEmits([
  'update:isColor', 'update:duplex', 'update:copies',
  'update:paperSize', 'update:paperType', 'update:mediaSource', 'update:printScaling', 'update:pageRange',
  'update:pageSet', 'update:mirror', 'update:watermarkText',
  'update:numberUp', 'update:numberUpLayout', 'update:pageBorder'
])

const showAdvanced = ref(localStorage.getItem('print_options_expanded') === '1')
watch(showAdvanced, (val) => { localStorage.setItem('print_options_expanded', val ? '1' : '0') })
const pageRangeError = ref('')

// IPP media-source keyword → 中文名映射。不同打印机上报的纸盒关键字差异很大，
// 未命中的关键字（如 tray-3）会走 mediaSourceLabel 的通用规则或原样显示。
const mediaSourceNames = {
  'auto': 'Автоматически',
  'auto-select': 'Автоматически',
  'main': 'Основной лоток',
  'alternate': 'Доп. лоток',
  'large-capacity': 'Лоток большой емкости',
  'manual': 'Ручная подача',
  'bypass': 'Обходной лоток',
  'by-pass-tray': 'Обходной лоток',
  'multipurpose': 'Универсальный лоток',
  'envelope': 'Лоток для конвертов',
  'top': 'Верхний лоток',
  'middle': 'Средний лоток',
  'bottom': 'Нижний лоток',
  'left': 'Левый лоток',
  'right': 'Правый лоток',
  'center': 'Центральный лоток',
  'rear': 'Задний лоток',
  'side': 'Боковой лоток',
  'photo': 'Лоток для фото',
  'hagaki': 'Лоток для открыток',
  'disc': 'Лоток для дисков'
}

function mediaSourceLabel(key) {
  if (mediaSourceNames[key]) return mediaSourceNames[key]
  // tray-1 / tray-2 ... → Лоток 1 / Лоток 2
  const m = /^tray-?(\d+)$/i.exec(key)
  if (m) return `Лоток ${m[1]}`
  return key
}

// 供 USelect 使用的纸盒选项：始终含「自动选择」，其余来自打印机上报的 media-source-supported。
const mediaSourceItems = computed(() => {
  const items = [{ label: 'Автоматически', value: 'auto' }]
  for (const key of props.mediaSourceSupported) {
    if (key === 'auto' || key === 'auto-select') continue
    items.push({ label: mediaSourceLabel(key), value: key })
  }
  return items
})

const advancedSummary = computed(() => {
  const sizeLabel = paperSizeItems.find(i => i.value === props.paperSize)?.label?.split(' ')[0] || props.paperSize
  const typeLabel = paperTypeItems.find(i => i.value === props.paperType)?.label || props.paperType
  const scaleLabel = scalingItems.find(i => i.value === props.printScaling)?.label || props.printScaling
  const parts = [sizeLabel, typeLabel, scaleLabel]
  if (props.mediaSource && props.mediaSource !== 'auto') parts.push(mediaSourceLabel(props.mediaSource))
  if (props.pageRange) parts.push(`Стр: ${props.pageRange}`)
  const pageSetLabel = pageSetItems.find(i => i.value === props.pageSet)?.label
  if (props.pageSet && props.pageSet !== 'all' && pageSetLabel) parts.push(pageSetLabel)
  if (props.numberUp > 1) {
    parts.push(`${props.numberUp} стр/лист`)
    if (props.pageBorder === 'single') parts.push('с рамкой')
  }
  if (props.mirror) parts.push('зеркало')
  if (props.watermarkText) parts.push(`водяной знак: ${props.watermarkText}`)
  return parts.join(' / ')
})

const colorItems = [
  { label: 'Цветная', value: true, icon: 'i-lucide-palette' },
  { label: 'Черно-белая', value: false, icon: 'i-lucide-contrast' }
]

const duplexItems = [
  { label: 'Односторонняя', value: 'one-sided' },
  { label: 'Двусторонняя (длинный край)', value: 'two-sided-long-edge' },
  { label: 'Двусторонняя (короткий край)', value: 'two-sided-short-edge' }
]

const paperSizeItems = [
  { label: 'A5 (148×210мм)', value: 'A5' },
  { label: 'A4 (210×297мм)', value: 'A4' },
  { label: 'A3 (297×420мм)', value: 'A3' },
  { label: 'A2 (420×594мм)', value: 'A2' },
  { label: 'A1 (594×841мм)', value: 'A1' },
  { label: '5" (89×127мм)', value: '5inch' },
  { label: '6" (102×152мм)', value: '6inch' },
  { label: '7" (127×178мм)', value: '7inch' },
  { label: '8" (152×203мм)', value: '8inch' },
  { label: '10" (203×254мм)', value: '10inch' },
  { label: 'Letter (8.5×11in)', value: 'Letter' },
  { label: 'Legal (8.5×14in)', value: 'Legal' }
]

const paperTypeItems = [
  { label: 'Обычная бумага', value: 'plain' },
  { label: 'Фотобумага', value: 'photo' },
  { label: 'Глянцевая', value: 'glossy' },
  { label: 'Матовая', value: 'matte' },
  { label: 'Конверт', value: 'envelope' },
  { label: 'Картон', value: 'cardstock' },
  { label: 'Этикетки', value: 'labels' },
  { label: 'Автоматически', value: 'auto' }
]

const scalingItems = [
  { label: 'Авто', value: 'auto' },
  { label: 'Автоподбор', value: 'auto-fit' },
  { label: 'По размеру', value: 'fit' },
  { label: 'Заполнить', value: 'fill' },
  { label: 'Без масштаба', value: 'none' }
]

const pageSetItems = [
  { label: 'Все', value: 'all', icon: 'i-lucide-copy' },
  { label: 'Нечетные', value: 'odd', icon: 'i-lucide-list-ordered' },
  { label: 'Четные', value: 'even', icon: 'i-lucide-list-ordered' },
  { label: 'Четные (обратно)', value: 'even-reverse', icon: 'i-lucide-arrow-down-up' }
]

const numberUpItems = [
  { label: '1 стр/лист', value: 1 },
  { label: '2 стр/лист', value: 2 },
  { label: '4 стр/лист', value: 4 },
  { label: '6 стр/лист', value: 6 },
  { label: '9 стр/лист', value: 9 },
  { label: '16 стр/лист', value: 16 }
]

const numberUpLayoutItems = [
  { label: 'Горизонтально Z (слева направо)', value: 'lrtb' },
  { label: 'Горизонтально Z (справа налево)', value: 'rltb' },
  { label: 'Вертикально N (сверху вниз)', value: 'tblr' },
  { label: 'Вертикально N (сверху вниз, справа налево)', value: 'tbrl' }
]

function onPageRangeInput(val) {
  emit('update:pageRange', val)
  validatePageRange(val)
}

function validatePageRange(val) {
  if (typeof val !== 'string') val = ''
  val = val.trim()
  if (!val) { pageRangeError.value = ''; return }

  const normalizedVal = val
    .replace(/[－—–―]/g, '-')
    .replace(/\s*-\s*/g, '-')
    .replace(/[，,]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()

  if (normalizedVal !== val) {
    emit('update:pageRange', normalizedVal)
    val = normalizedVal
  }

  const pattern = /^(\d+(-\d+)?)(\s+\d+(-\d+)?)*$/
  pageRangeError.value = pattern.test(val) ? '' : 'Неверный формат, например: 1-5 8 10-12'
}
</script>
