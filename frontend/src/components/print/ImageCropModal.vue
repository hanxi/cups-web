<template>
  <UModal v-model:open="isOpen">
    <template #content>
      <div class="p-4 space-y-4">
        <h3 class="text-lg font-semibold flex items-center gap-2">
          <UIcon name="i-lucide-crop" class="w-5 h-5" />
          Обрезка ID-карты
        </h3>
        <div class="relative bg-elevated rounded-lg overflow-hidden" style="max-height: 60vh;">
          <img ref="imgRef" :src="imageUrl" class="max-w-full block" />
        </div>
        <div class="flex items-center gap-2">
          <div class="flex gap-1 shrink-0">
            <UButton variant="outline" size="sm" icon="i-lucide-rotate-ccw" @click="rotateLeft">Влево</UButton>
            <UButton variant="outline" size="sm" icon="i-lucide-rotate-cw" @click="rotateRight">Вправо</UButton>
          </div>
          <input
            type="range"
            :min="-45"
            :max="45"
            step="0.5"
            v-model.number="fineAngle"
            class="flex-1 h-1.5 accent-[var(--ui-primary)]"
            @input="onFineRotate"
          />
          <span class="text-xs text-muted tabular-nums w-12 text-right shrink-0">{{ fineAngle.toFixed(1) }}°</span>
          <UButton v-if="fineAngle !== 0" variant="ghost" size="xs" icon="i-lucide-undo-2" @click="resetFine" />
        </div>
        <div class="flex justify-end gap-2">
          <UButton variant="ghost" @click="cancel">Отмена</UButton>
          <UButton color="primary" icon="i-lucide-check" @click="confirm">Обрезать</UButton>
        </div>
      </div>
    </template>
  </UModal>
</template>

<script setup>
import { ref, watch, nextTick, onBeforeUnmount } from 'vue'
import Cropper from 'cropperjs'
import 'cropperjs/dist/cropper.min.css'

const props = defineProps({
  imageUrl: { type: String, default: '' },
  open: { type: Boolean, default: false },
  aspectRatio: { type: Number, default: 85.6 / 54 }
})

const emit = defineEmits(['cropped', 'close'])

const isOpen = ref(false)
const imgRef = ref(null)
const fineAngle = ref(0)
const baseAngle = ref(0)
let cropper = null

watch(() => props.open, (val) => { isOpen.value = val })

watch(isOpen, (val) => {
  if (val) {
    fineAngle.value = 0
    baseAngle.value = 0
    nextTick(() => initCropper())
  } else {
    destroyCropper()
    emit('close')
  }
})

function initCropper() {
  destroyCropper()
  if (!imgRef.value) return
  nextTick(() => {
    cropper = new Cropper(imgRef.value, {
      aspectRatio: props.aspectRatio,
      viewMode: 1,
      autoCrop: false,
      responsive: true,
      guides: true,
      highlight: true,
      background: true,
      ready() {
        const region = detectCardRegion(imgRef.value)
        if (region) {
          cropper.setData(region)
        } else {
          cropper.setData({
            x: 0, y: 0,
            width: imgRef.value.naturalWidth,
            height: imgRef.value.naturalWidth / props.aspectRatio
          })
        }
        cropper.crop()
      }
    })
  })
}

function detectCardRegion(imgEl) {
  const nw = imgEl.naturalWidth
  const nh = imgEl.naturalHeight
  if (nw < 50 || nh < 50) return null

  const scale = Math.min(1, 600 / Math.max(nw, nh))
  const sw = Math.round(nw * scale)
  const sh = Math.round(nh * scale)

  const canvas = document.createElement('canvas')
  canvas.width = sw
  canvas.height = sh
  const ctx = canvas.getContext('2d')
  ctx.drawImage(imgEl, 0, 0, sw, sh)
  const { data } = ctx.getImageData(0, 0, sw, sh)

  const cs = Math.max(3, Math.round(Math.min(sw, sh) * 0.06))
  const corners = [
    [0, 0], [sw - cs, 0], [0, sh - cs], [sw - cs, sh - cs]
  ]
  let sumR = 0, sumG = 0, sumB = 0, cnt = 0
  for (const [rx, ry] of corners) {
    for (let y = ry; y < ry + cs && y < sh; y++) {
      for (let x = rx; x < rx + cs && x < sw; x++) {
        const i = (y * sw + x) * 4
        sumR += data[i]; sumG += data[i + 1]; sumB += data[i + 2]
        cnt++
      }
    }
  }
  const bgR = sumR / cnt, bgG = sumG / cnt, bgB = sumB / cnt

  const rowFg = new Int32Array(sh)
  const colFg = new Int32Array(sw)
  for (let y = 0; y < sh; y++) {
    for (let x = 0; x < sw; x++) {
      const i = (y * sw + x) * 4
      const dr = data[i] - bgR, dg = data[i + 1] - bgG, db = data[i + 2] - bgB
      if (dr * dr + dg * dg + db * db > 35 * 35) {
        rowFg[y]++
        colFg[x]++
      }
    }
  }

  const rowTh = sw * 0.08
  const colTh = sh * 0.08
  let top = 0, bottom = sh - 1, left = 0, right = sw - 1
  while (top < sh && rowFg[top] < rowTh) top++
  while (bottom > 0 && rowFg[bottom] < rowTh) bottom--
  while (left < sw && colFg[left] < colTh) left++
  while (right > 0 && colFg[right] < colTh) right--

  const cw = right - left, ch = bottom - top
  if (cw < sw * 0.25 || ch < sh * 0.25 || cw <= 0 || ch <= 0) return null

  const margin = Math.round(Math.min(sw, sh) * 0.01)
  top = Math.max(0, top - margin)
  bottom = Math.min(sh - 1, bottom + margin)
  left = Math.max(0, left - margin)
  right = Math.min(sw - 1, right + margin)

  return {
    x: Math.round(left / scale),
    y: Math.round(top / scale),
    width: Math.round((right - left) / scale),
    height: Math.round((bottom - top) / scale)
  }
}

function destroyCropper() {
  if (cropper) { cropper.destroy(); cropper = null }
}

function confirm() {
  if (!cropper) return
  cropper.getCroppedCanvas({
    maxWidth: 3000, maxHeight: 3000, imageSmoothingQuality: 'high'
  }).toBlob((blob) => {
    if (blob) emit('cropped', blob)
    isOpen.value = false
  }, 'image/jpeg', 0.92)
}

function rotateLeft() {
  if (!cropper) return
  baseAngle.value -= 90
  cropper.rotateTo(baseAngle.value + fineAngle.value)
}
function rotateRight() {
  if (!cropper) return
  baseAngle.value += 90
  cropper.rotateTo(baseAngle.value + fineAngle.value)
}
function onFineRotate() {
  if (cropper) cropper.rotateTo(baseAngle.value + fineAngle.value)
}
function resetFine() {
  fineAngle.value = 0
  onFineRotate()
}

function cancel() { isOpen.value = false }

onBeforeUnmount(() => destroyCropper())
</script>
