export function formatFileSize(bytes) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

export function formatTime(iso) {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleString('ru-RU', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
  } catch { return iso }
}

export function formatPrinterName(uri) {
  if (!uri) return ''
  const parts = uri.split('/')
  return parts[parts.length - 1] || uri
}

export function formatDurationSeconds(totalSeconds) {
  if (!totalSeconds || totalSeconds < 0) return 'Неизвестно'
  const d = Math.floor(totalSeconds / 86400)
  const h = Math.floor((totalSeconds % 86400) / 3600)
  const m = Math.floor((totalSeconds % 3600) / 60)
  if (d > 0) return `${d} д. ${h} ч.`
  if (h > 0) return `${h} ч. ${m} мин.`
  if (m > 0) return `${m} мин.`
  return `${totalSeconds} сек.`
}

// formatStateDuration вычисляет сколько времени прошло с указанной даты (ISO) до текущего момента
export function formatStateDuration(isoStr) {
  if (!isoStr) return 'Неизвестно'
  const past = new Date(isoStr)
  if (isNaN(past.getTime())) return 'Неизвестно'
  const diffMs = Date.now() - past.getTime()
  if (diffMs < 0) return 'Неизвестно'
  const totalSeconds = Math.floor(diffMs / 1000)
  const d = Math.floor(totalSeconds / 86400)
  const h = Math.floor((totalSeconds % 86400) / 3600)
  const m = Math.floor((totalSeconds % 3600) / 60)
  if (d > 0) return `${d} д. ${h} ч.`
  if (h > 0) return `${h} ч. ${m} мин.`
  if (m > 0) return `${m} мин.`
  return `${totalSeconds} сек.`
}

export function statusColor(status) {
  const map = { queued: 'info', printed: 'success', failed: 'error', cancelled: 'neutral' }
  return map[status] || 'neutral'
}

export function statusText(status) {
  const map = { queued: 'В очереди', printed: 'Готово', failed: 'Ошибка', cancelled: 'Отменено' }
  return map[status] || status
}

export function printerStateColor(state) {
  const map = { idle: 'success', processing: 'warning', stopped: 'error' }
  return map[state] || 'neutral'
}

export function printerStateText(state) {
  const map = { idle: 'Свободен', processing: 'Печать...', stopped: 'Остановлен' }
  return map[state] || state || 'Неизвестно'
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
