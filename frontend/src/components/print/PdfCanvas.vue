<template>
  <div ref="container" class="w-full h-full flex items-center justify-center">
    <div v-if="loading" class="text-center text-muted">
      <UIcon name="i-lucide-loader-circle" class="w-6 h-6 animate-spin" />
    </div>
    <div v-else-if="error" class="text-center text-muted text-xs p-3 leading-relaxed">
      <p>{{ t('ui.could_not_load_pdf_preview') }}</p>
      <p class="mt-1 text-[10px] opacity-80">{{ t('ui.printing_is_still_available_select_start_printing') }}</p>
    </div>
    <div v-show="!loading && !error" class="relative w-full h-full flex items-center justify-center">
      <!-- 自定义缩放只作用于画布本身，翻页控件留在外层不跟着缩小 -->
      <div class="relative w-full h-full flex items-center justify-center transition-transform duration-200" :style="contentScaleStyle">
        <canvas v-show="!fallbackMode" ref="canvas" class="max-w-full max-h-full" />
        <img v-if="fallbackMode && fallbackImgUrl" ref="fallbackImg" :src="fallbackImgUrl" :alt="t('accessibility.preview')" class="max-w-full max-h-full" @load="drawWatermark" />
        <canvas ref="watermarkCanvas" class="absolute top-0 left-0 pointer-events-none" />
      </div>
      <div v-if="totalPages > 1 && !loading && !error" class="absolute bottom-2 left-1/2 -translate-x-1/2 flex flex-nowrap items-center gap-1 sm:gap-2 px-2 sm:px-3 py-1 bg-black/40 rounded-full w-max max-w-[90%] whitespace-nowrap" style="backdrop-filter: blur(4px)">
        <UButton :aria-label="t('accessibility.previous_page')" size="xs" variant="ghost" color="white" icon="i-lucide-chevron-left" :disabled="currentPage <= 1" class="flex-shrink-0" @click="prevPage" />
        <span class="text-xs text-white whitespace-nowrap flex-shrink-0">{{ currentPage }} / {{ totalPages }}</span>
        <UButton :aria-label="t('accessibility.next_page')" size="xs" variant="ghost" color="white" icon="i-lucide-chevron-right" :disabled="currentPage >= totalPages" class="flex-shrink-0" @click="nextPage" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted, watch, onUnmounted, nextTick } from 'vue'
// legacy 构建内置 core-js polyfill，兼容 iOS 旧版 Safari、Kiwi 等旧内核浏览器（issue #41/#86/#89）。
import * as pdfjsLib from 'pdfjs-dist/legacy/build/pdf.mjs'
import pdfjsWorker from 'pdfjs-dist/legacy/build/pdf.worker.min.mjs?url'
import { apiFetch } from '../../utils/api.js'

const { t } = useI18n()

pdfjsLib.GlobalWorkerOptions.workerSrc = pdfjsWorker

const props = defineProps({
  src: { type: String, required: true },
  watermarkText: { type: String, default: '' },
  // 自定义百分比缩放的预览倍率（1 = 不缩放）。用 CSS transform 展示，
  // 不改渲染分辨率：canvas 仍按容器满尺寸渲染，缩小后反而更清晰。
  contentScale: { type: Number, default: 1 }
})

const contentScaleStyle = computed(() =>
  props.contentScale === 1 ? {} : { transform: `scale(${props.contentScale})` }
)

// contentScale > 1 时 CSS 放大会让画布发虚，按整数档提高渲染分辨率补偿。
// 取整数档（1/2/3）而不是直接用 contentScale，是为了让用户连续调百分比时最多只重渲两次。
const superSample = computed(() => Math.min(3, Math.max(1, Math.ceil(props.contentScale))))

// 预览失败时通知父组件，便于在外层展示"不影响打印"的提示
const emit = defineEmits(['preview-failed'])

const canvas = ref(null)
const watermarkCanvas = ref(null)
const container = ref(null)
const loading = ref(true)
const error = ref(false)
const currentPage = ref(1)
const totalPages = ref(0)
let pdfDoc = null
let renderTask = null
let resizeObserver = null
let lastWidth = 0
let lastHeight = 0

// requestToken 用于区分多次并发的 renderPdf 调用。
// 场景：父组件（PrintView）在 PDF 分支会"先用本地 blob 出预览，再异步用后端标准化 blob 替换"，
// 两次 props.src 变化会在极短时间内触发两次 renderPdf。第一次的 blob 一旦被 URL.revokeObjectURL
// 立即吊销，pdf.js 的 fetch 会 abort 并 reject，若此时直接写入 error/loading，会污染掉第二次成功请求
// 的状态（尤其在 iPhone Safari 下更容易踩到，见 issue 截图中红色的 blob 请求）。
// 规则：renderPdf 入口自增 token，仅当捕获异常时 token 仍为当前值才写状态；过期请求静默丢弃。
let requestToken = 0

// pdf.js 加载失败时改由后端 /api/preview-page 把单页渲染成 PNG 显示（issue #41）。
const fallbackMode = ref(false)
const fallbackImgUrl = ref('')
const fallbackImg = ref(null)
let fallbackBlob = null
let fallbackPages = new Map()

function resetFallback() {
  fallbackMode.value = false
  fallbackImgUrl.value = ''
  fallbackBlob = null
  for (const url of fallbackPages.values()) URL.revokeObjectURL(url)
  fallbackPages = new Map()
}

async function fetchFallbackPage(pageNum) {
  const form = new FormData()
  form.append('file', fallbackBlob, 'preview.pdf')
  form.append('page', String(pageNum))
  const resp = await apiFetch('/api/preview-page', { method: 'POST', body: form })
  if (!resp.ok) throw new Error(`preview-page HTTP ${resp.status}`)
  const count = Number(resp.headers.get('X-Page-Count')) || 0
  return { url: URL.createObjectURL(await resp.blob()), count }
}

async function showFallbackPage(pageNum, token) {
  let url = fallbackPages.get(pageNum)
  if (!url) {
    const res = await fetchFallbackPage(pageNum)
    if (token !== requestToken) {
      URL.revokeObjectURL(res.url)
      return
    }
    // 快速来回翻页时同一页可能并发请求两次，保留先到的那份
    if (fallbackPages.has(pageNum)) {
      URL.revokeObjectURL(res.url)
    } else {
      fallbackPages.set(pageNum, res.url)
    }
    url = fallbackPages.get(pageNum)
  }
  if (token === requestToken && currentPage.value === pageNum) fallbackImgUrl.value = url
}

async function startFallback(token) {
  const srcResp = await fetch(props.src)
  if (!srcResp.ok) throw new Error(`fetch preview source HTTP ${srcResp.status}`)
  const blob = await srcResp.blob()
  if (token !== requestToken) return

  fallbackBlob = blob
  const res = await fetchFallbackPage(1)
  if (token !== requestToken) {
    URL.revokeObjectURL(res.url)
    return
  }
  fallbackPages.set(1, res.url)
  totalPages.value = res.count || 1
  currentPage.value = 1
  fallbackMode.value = true
  fallbackImgUrl.value = res.url
  loading.value = false
}

function drawWatermark() {
  const wc = watermarkCanvas.value
  if (!wc) return

  let cssW, cssH
  if (fallbackMode.value) {
    // clientWidth 不受 contentScale 的 CSS transform 影响，和画布分支的 style 尺寸口径一致
    cssW = fallbackImg.value?.clientWidth
    cssH = fallbackImg.value?.clientHeight
  } else {
    if (!canvas.value) return
    cssW = parseFloat(canvas.value.style.width)
    cssH = parseFloat(canvas.value.style.height)
  }
  if (!cssW || !cssH) return

  const dpr = window.devicePixelRatio || 1
  wc.width = cssW * dpr
  wc.height = cssH * dpr
  wc.style.width = cssW + 'px'
  wc.style.height = cssH + 'px'

  const ctx = wc.getContext('2d')
  ctx.clearRect(0, 0, wc.width, wc.height)

  if (!props.watermarkText) return

  ctx.save()
  ctx.scale(dpr, dpr)
  ctx.globalAlpha = 0.15
  ctx.fillStyle = '#b4b4b4'
  const fontSize = Math.max(14, cssW * 0.06)
  ctx.font = `${fontSize}px sans-serif`

  const textWidth = ctx.measureText(props.watermarkText).width
  const stepX = textWidth + 30
  const stepY = fontSize * 2.5
  const diag = Math.sqrt(cssW * cssW + cssH * cssH)

  ctx.translate(cssW / 2, cssH / 2)
  ctx.rotate(-45 * Math.PI / 180)

  for (let y = -diag; y < diag; y += stepY) {
    for (let x = -diag; x < diag; x += stepX) {
      ctx.fillText(props.watermarkText, x, y)
    }
  }
  ctx.restore()
}

async function renderPage(pageNum) {
  if (!pdfDoc || !canvas.value) return

  try {
    if (renderTask) {
      renderTask.cancel()
      renderTask = null
    }

    const page = await pdfDoc.getPage(pageNum)

    const containerEl = container.value
    if (!containerEl) return

    const containerWidth = containerEl.clientWidth
    const containerHeight = containerEl.clientHeight

    // 容器尺寸无效时不渲染
    if (containerWidth <= 0 || containerHeight <= 0) return

    const dpr = window.devicePixelRatio || 1

    const viewport = page.getViewport({ scale: 1 })
    const scaleX = containerWidth / viewport.width
    const scaleY = containerHeight / viewport.height
    // Match the displayed paper even for small photo formats. Capping the
    // scale at 2 leaves an artificial white frame around a borderless PDF.
    // The container dimensions still bound the actual canvas pixel count.
    const baseScale = Math.min(scaleX, scaleY)

    // 高清渲染：scale 乘以 DPR（放大预览时再乘 superSample），canvas 实际像素更大，CSS 尺寸保持正常
    const pixelRatio = dpr * superSample.value
    const scaledViewport = page.getViewport({ scale: baseScale * pixelRatio })

    const ctx = canvas.value.getContext('2d')
    canvas.value.width = scaledViewport.width
    canvas.value.height = scaledViewport.height
    canvas.value.style.width = (scaledViewport.width / pixelRatio) + 'px'
    canvas.value.style.height = (scaledViewport.height / pixelRatio) + 'px'

    renderTask = page.render({
      canvasContext: ctx,
      viewport: scaledViewport
    })

    await renderTask.promise
    renderTask = null
    drawWatermark()
  } catch (e) {
    if (e?.name === 'RenderingCancelledException') return
    throw e
  }
}

async function renderPdf() {
  if (!props.src || !canvas.value) return

  // 每次进入都分配一个独立 token；并发场景下只有最后一次调用的 token 等于 requestToken
  const myToken = ++requestToken

  loading.value = true
  error.value = false
  resetFallback()

  try {
    if (renderTask) {
      renderTask.cancel()
      renderTask = null
    }
    if (pdfDoc) {
      pdfDoc.destroy()
      pdfDoc = null
    }

    const doc = await pdfjsLib.getDocument({
      url: props.src,
      cMapUrl: '/pdfjs/cmaps/',
      cMapPacked: true,
      standardFontDataUrl: '/pdfjs/standard_fonts/',
      disableFontFace: false,
      useSystemFonts: true,
      isEvalSupported: false
    }).promise

    // 加载过程中又被更新的请求超车了（如父组件很快换了 src），直接丢弃当前结果
    if (myToken !== requestToken) {
      doc.destroy()
      return
    }

    pdfDoc = doc
    totalPages.value = pdfDoc.numPages
    currentPage.value = 1

    // 等待 DOM 布局完成
    await nextTick()
    await new Promise(resolve => requestAnimationFrame(resolve))

    if (myToken !== requestToken) return

    await renderPage(1)
    if (myToken !== requestToken) return
    loading.value = false
  } catch (e) {
    if (e?.name === 'RenderingCancelledException') return
    // 过期请求的失败不污染最新状态（常见于 blob URL 在第一次 fetch 进行中被 revoke）
    if (myToken !== requestToken) return
    console.error('PDF render error, falling back to server rendering:', e)
    try {
      await startFallback(myToken)
      return
    } catch (fallbackErr) {
      if (myToken !== requestToken) return
      console.error('Server preview fallback failed:', fallbackErr)
    }
    resetFallback()
    error.value = true
    loading.value = false
    emit('preview-failed', e)
  }
}

function goToPage(pageNum) {
  currentPage.value = pageNum
  if (!fallbackMode.value) {
    renderPage(pageNum)
    return
  }
  showFallbackPage(pageNum, requestToken).catch(e => console.error('Server preview page failed:', e))
}

function prevPage() {
  if (currentPage.value <= 1) return
  goToPage(currentPage.value - 1)
}

function nextPage() {
  if (currentPage.value >= totalPages.value) return
  goToPage(currentPage.value + 1)
}

watch(() => props.src, () => {
  if (props.src) {
    nextTick(() => renderPdf())
  }
})

watch(() => props.watermarkText, () => {
  drawWatermark()
})

watch(superSample, () => {
  if (!fallbackMode.value && pdfDoc && currentPage.value > 0 && !loading.value) renderPage(currentPage.value)
})

onMounted(() => {
  if (props.src) renderPdf()

  // 监听容器大小变化
  resizeObserver = new ResizeObserver((entries) => {
    const entry = entries[0]
    const { width, height } = entry.contentRect
    // 只在尺寸真正变化时重新渲染
    if (Math.abs(width - lastWidth) > 1 || Math.abs(height - lastHeight) > 1) {
      lastWidth = width
      lastHeight = height
      if (fallbackMode.value) {
        drawWatermark()
      } else if (pdfDoc && currentPage.value > 0 && !loading.value) {
        renderPage(currentPage.value)
      }
    }
  })
  if (container.value) {
    resizeObserver.observe(container.value)
  }
})

onUnmounted(() => {
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  if (renderTask) {
    renderTask.cancel()
    renderTask = null
  }
  if (pdfDoc) {
    pdfDoc.destroy()
    pdfDoc = null
  }
  // 让仍在进行中的兜底请求作废，避免卸载后再写入状态或泄漏 object URL
  requestToken++
  resetFallback()
})
</script>
