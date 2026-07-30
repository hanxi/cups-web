<template>
  <UCard :ui="{ body: 'p-3 sm:p-4' }">
    <template #header>
      <div class="flex items-center gap-2 font-semibold">
        <UIcon name="i-lucide-id-card" class="w-5 h-5" />
        Загрузка ID-карты
      </div>
    </template>
    <div class="grid grid-cols-2 gap-3">
      <!-- 正面 -->
      <div
        class="border-2 border-dashed rounded-lg p-3 text-center cursor-pointer transition-colors min-h-[120px] flex flex-col items-center justify-center"
        :class="frontDragging ? 'border-primary bg-primary/5' : (front ? 'border-success' : 'border-muted hover:border-primary/50')"
        @dragover.prevent="frontDragging = true"
        @dragleave="frontDragging = false"
        @drop.prevent="onDropFront"
        @click="!front && frontInput.click()"
      >
        <input ref="frontInput" type="file" accept="image/*" class="hidden" @change="onFrontChange" />
        <template v-if="!front">
          <UIcon name="i-lucide-image-plus" class="w-8 h-8 text-muted mb-1" />
          <p class="text-sm text-muted">Лицевая сторона</p>
          <p class="text-xs text-muted">Нажмите или перетащите</p>
        </template>
        <template v-else>
          <img :src="frontPreview" class="max-h-[80px] max-w-full object-contain rounded mb-1" />
          <p class="text-xs text-muted truncate w-full">{{ front.name }}</p>
          <div class="flex gap-1 mt-1">
            <UButton variant="ghost" size="xs" icon="i-lucide-crop" @click.stop="reCropFront">Обрезать</UButton>
            <UButton variant="ghost" size="xs" color="error" icon="i-lucide-x" @click.stop="removeFront">Удалить</UButton>
          </div>
        </template>
      </div>
      <!-- 反面 -->
      <div
        class="border-2 border-dashed rounded-lg p-3 text-center cursor-pointer transition-colors min-h-[120px] flex flex-col items-center justify-center"
        :class="backDragging ? 'border-primary bg-primary/5' : (back ? 'border-success' : 'border-muted hover:border-primary/50')"
        @dragover.prevent="backDragging = true"
        @dragleave="backDragging = false"
        @drop.prevent="onDropBack"
        @click="!back && backInput.click()"
      >
        <input ref="backInput" type="file" accept="image/*" class="hidden" @change="onBackChange" />
        <template v-if="!back">
          <UIcon name="i-lucide-image-plus" class="w-8 h-8 text-muted mb-1" />
          <p class="text-sm text-muted">Оборотная сторона</p>
          <p class="text-xs text-muted">Нажмите или перетащите</p>
        </template>
        <template v-else>
          <img :src="backPreview" class="max-h-[80px] max-w-full object-contain rounded mb-1" />
          <p class="text-xs text-muted truncate w-full">{{ back.name }}</p>
          <div class="flex gap-1 mt-1">
            <UButton variant="ghost" size="xs" icon="i-lucide-crop" @click.stop="reCropBack">Обрезать</UButton>
            <UButton variant="ghost" size="xs" color="error" icon="i-lucide-x" @click.stop="removeBack">Удалить</UButton>
          </div>
        </template>
      </div>
    </div>

    <ImageCropModal
      :image-url="cropImageUrl"
      :open="showCropper"
      @cropped="onCropped"
      @close="showCropper = false"
    />
  </UCard>
</template>

<script setup>
import { ref } from 'vue'
import ImageCropModal from './ImageCropModal.vue'

defineProps({
  front: { type: [File, null], default: null },
  back: { type: [File, null], default: null },
  frontPreview: { type: String, default: '' },
  backPreview: { type: String, default: '' }
})

const emit = defineEmits(['update:front', 'update:back', 'clear'])

const frontDragging = ref(false)
const backDragging = ref(false)
const frontInput = ref(null)
const backInput = ref(null)

const showCropper = ref(false)
const cropImageUrl = ref('')
const cropTarget = ref(null)
const rawFront = ref(null)
const rawBack = ref(null)

function pickImage(files) {
  if (!files || files.length === 0) return null
  const f = files[0]
  if (f.type.startsWith('image/') || /\.(heic|heif)$/i.test(f.name)) return f
  return null
}

function openCropper(file, target) {
  if (cropImageUrl.value) { try { URL.revokeObjectURL(cropImageUrl.value) } catch (_) {} }
  cropImageUrl.value = URL.createObjectURL(file)
  cropTarget.value = target
  showCropper.value = true
}

function onCropped(blob) {
  const target = cropTarget.value
  const raw = target === 'front' ? rawFront.value : rawBack.value
  const name = raw ? raw.name.replace(/\.[^/.]+$/, '') + '_cropped.jpg' : 'cropped.jpg'
  const file = new File([blob], name, { type: 'image/jpeg', lastModified: Date.now() })
  if (target === 'front') {
    emit('update:front', file)
  } else {
    emit('update:back', file)
  }
  cropTarget.value = null
}

function handleUpload(file, target) {
  if (target === 'front') {
    rawFront.value = file
  } else {
    rawBack.value = file
  }
  openCropper(file, target)
}

function reCropFront() {
  if (rawFront.value) openCropper(rawFront.value, 'front')
}
function reCropBack() {
  if (rawBack.value) openCropper(rawBack.value, 'back')
}

function removeFront() {
  rawFront.value = null
  emit('update:front', null)
}
function removeBack() {
  rawBack.value = null
  emit('update:back', null)
}

function onDropFront(e) {
  frontDragging.value = false
  const f = pickImage(e.dataTransfer.files)
  if (f) handleUpload(f, 'front')
}
function onDropBack(e) {
  backDragging.value = false
  const f = pickImage(e.dataTransfer.files)
  if (f) handleUpload(f, 'back')
}
function onFrontChange(e) {
  const f = pickImage(e.target.files)
  if (f) handleUpload(f, 'front')
  if (frontInput.value) frontInput.value.value = ''
}
function onBackChange(e) {
  const f = pickImage(e.target.files)
  if (f) handleUpload(f, 'back')
  if (backInput.value) backInput.value.value = ''
}
</script>
