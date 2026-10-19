import { t } from '../i18n.js'

export function printerChoiceLabel(choice) {
  if (!choice) return ''
  const quality = { 3: 'ui.quality_draft', 4: 'ui.quality_standard', 5: 'ui.quality_high' }
  if (choice.attribute === 'print-quality' && quality[choice.value]) return t(quality[choice.value])
  // This suffix is added by cups-web. PPD/vendor-provided names stay verbatim.
  const label = choice.label || choice.value || ''
  return label.endsWith('（无边距）') ? t('ui.borderless_label', { label: label.slice(0, -5) }) : label
}

export function paperDimensions(size) {
  const match = /^custom:([0-9.]+)x([0-9.]+)$/.exec(size || '')
  if (match) {
    const width = Number(match[1]), height = Number(match[2])
    if (width > 0 && height > 0 && width <= 2000 && height <= 2000) return { width, height }
  }
  const sizes = {
    A5: [148, 210], A4: [210, 297], A3: [297, 420], A2: [420, 594], A1: [594, 841],
    '5inch': [89, 127], '6inch': [102, 152], '7inch': [127, 178], '8inch': [152, 203], '10inch': [203, 254],
    Letter: [216, 279], Legal: [216, 356]
  }
  const pair = sizes[size]
  return pair ? { width: pair[0], height: pair[1] } : null
}
