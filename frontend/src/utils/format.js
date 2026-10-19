import { t, locale, formatNumber } from '../i18n.js'
export function formatFileSize(bytes) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return formatNumber(bytes) + ' B'
  const options = { minimumFractionDigits: 1, maximumFractionDigits: 1 }
  if (bytes < 1024 * 1024) return formatNumber(bytes / 1024, options) + ' KB'
  return formatNumber(bytes / (1024 * 1024), options) + ' MB'
}

export function formatTime(iso) {
  if (!iso) return ''
  try {
    const date = new Date(iso)
    if (Number.isNaN(date.getTime())) return iso
    return date.toLocaleString(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
  } catch { return iso }
}

export function formatPrinterName(uri) {
  if (!uri) return ''
  const parts = uri.split('/')
  return parts[parts.length - 1] || uri
}

// printerLabel 拼打印机下拉的主标签：队列名 + CUPS 描述（issue #101）。
// CUPS 建队列时若没指定 -D，printer-info 会默认等于队列名，这时不重复显示。
export function printerLabel(printer) {
  if (!printer) return ''
  const name = printer.name || formatPrinterName(printer.uri)
  const info = (printer.info || '').trim()
  if (info && info.toLowerCase() !== name.toLowerCase()) return `${name} — ${info}`
  return name
}

// printerDescription 拼下拉项的副标题：型号 · 位置 · URI。
// 描述也重名时靠这一行区分（同型号多台、或多台机器共用一个描述）。
export function printerDescription(printer) {
  if (!printer) return ''
  return [
    (printer.makeAndModel || '').trim(),
    (printer.location || '').trim(),
    printer.uri
  ].filter(Boolean).join(' · ')
}

export function formatDurationSeconds(totalSeconds) {
  if (!totalSeconds || totalSeconds < 0) return t('ui.unknown')
  const d = Math.floor(totalSeconds / 86400)
  const h = Math.floor((totalSeconds % 86400) / 3600)
  const m = Math.floor((totalSeconds % 3600) / 60)
  if (d > 0) return t('ui.duration_days_hours', { p0: d, p1: h })
  if (h > 0) return t('ui.duration_hours_minutes', { p0: h, p1: m })
  if (m > 0) return t('ui.duration_minutes', { p0: m })
  return t('ui.duration_seconds', { p0: totalSeconds })
}

export function statusColor(status) {
  const map = { queued: 'info', printed: 'success', failed: 'error', cancelled: 'neutral' }
  return map[status] || 'neutral'
}

export function statusText(status) {
  const map = { queued: t('ui.queued'), printed: t('ui.printed'), failed: t('ui.failed'), cancelled: t('ui.cancelled') }
  return map[status] || status
}

export function printerStateColor(state) {
  const map = { idle: 'success', processing: 'warning', stopped: 'error' }
  return map[state] || 'neutral'
}

export function printerStateText(state) {
  const map = { idle: t('ui.idle'), processing: t('ui.printing'), stopped: t('ui.stopped') }
  return map[state] || state || t('ui.unknown')
}

export function jobStateText(job) {
  const keys = { 3: 'job.pending', 4: 'job.held', 5: 'job.processing', 6: 'job.stopped', 7: 'job.cancelled', 8: 'job.aborted', 9: 'job.completed' }
  return keys[job.state] ? t(keys[job.state]) : job.stateText
}

export function markerLevelColor(level) {
  if (level === undefined || level === null) return 'text-muted'
  if (level <= 10) return 'text-error font-bold'
  if (level <= 25) return 'text-warning font-medium'
  return 'text-success'
}

export function markerBarColor(level) {
  if (level === undefined || level === null) return 'bg-muted'
  if (level <= 10) return 'bg-error'
  if (level <= 25) return 'bg-warning'
  return 'bg-success'
}
