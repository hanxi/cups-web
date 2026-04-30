<template>
  <div ref="container" class="w-full h-full flex items-center justify-center">
    <div v-if="loading" class="text-center text-muted">
      <UIcon name="i-lucide-loader-circle" class="w-6 h-6 animate-spin" />
    </div>
    <div v-else-if="error" class="text-center text-muted text-xs p-3 leading-relaxed">
      <p>PDF 预览加载失败</p>
      <p class="mt-1 text-[10px] opacity-80">不影响打印，仍可点击"开始打印"</p>
    </div>
    <div v-show="!loading && !error" class="relative w-full h-full flex items-center justify-center">
      <canvas ref="canvas" class="max-w-full max-h-full" />
      <div v-if="totalPages > 1 && !loading && !error" class="absolute bottom-2 left-1/2 -translate-x-1/2 flex flex-nowrap items-center gap-1 sm:gap-2 px-2 sm:px-3 py-1 bg-black/40 rounded-full w-max max-w-[90%] whitespace-nowrap" style="backdrop-filter: blur(4px)">
        <UButton size="xs" variant="ghost" color="white" icon="i-lucide-chevron-left" :disabled="currentPage <= 1" class="flex-shrink-0" @click="prevPage" />
        <span class="text-xs text-white whitespace-nowrap flex-shrink-0">{{ currentPage }} / {{ totalPages }}</span>
        <UButton size="xs" variant="ghost" color="white" icon="i-lucide-chevron-right" :disabled="currentPage >= totalPages" class="flex-shrink-0" @click="nextPage" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, watch, onUnmounted, nextTick } from 'vue'
import * as pdfjsLib from 'pdfjs-dist'
import pdfjsWorker from 'pdfjs-dist/build/pdf.worker.min.mjs?url'

pdfjsLib.GlobalWorkerOptions.workerSrc = pdfjsWorker

const props = defineProps({
  src: { type: String, required: true }
})

// 预览失败时通知父组件，便于在外层展示"不影响打印"的提示
const emit = defineEmits(['preview-failed'])

const canvas = ref(null)
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

    const viewport = page.getViewport({ scale: 1 })
    const scaleX = containerWidth / viewport.width
    const scaleY = containerHeight / viewport.height
    const scale = Math.min(scaleX, scaleY, 2)

    const scaledViewport = page.getViewport({ scale })

    const ctx = canvas.value.getContext('2d')
    canvas.value.width = scaledViewport.width
    canvas.value.height = scaledViewport.height

    renderTask = page.render({
      canvasContext: ctx,
      viewport: scaledViewport
    })

    await renderTask.promise
    renderTask = null
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

  try {
    if (renderTask) {
      renderTask.cancel()
      renderTask = null
    }
    if (pdfDoc) {
      pdfDoc.destroy()
      pdfDoc = null
    }

    // 关键参数说明：
    // - cMapUrl / cMapPacked：支持 UniGB-UCS2-H 等中文外部 CMap，修复 CJK 预览空白
    // - standardFontDataUrl + useSystemFonts：预览未嵌入字体的 PDF 时用系统兜底字体
    // - disableFontFace=false：允许用 @font-face 注入字体（默认就是 false，显式保留以防升级回归）
    // - isEvalSupported=false：CSP/严格 Worker 环境下避免 eval 被拦截
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
    console.error('PDF render error:', e)
    error.value = true
    loading.value = false
    emit('preview-failed', e)
  }
}

function prevPage() {
  if (currentPage.value <= 1) return
  currentPage.value--
  renderPage(currentPage.value)
}

function nextPage() {
  if (currentPage.value >= totalPages.value) return
  currentPage.value++
  renderPage(currentPage.value)
}

watch(() => props.src, () => {
  if (props.src) {
    nextTick(() => renderPdf())
  }
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
      if (pdfDoc && currentPage.value > 0 && !loading.value) {
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
})
</script>
