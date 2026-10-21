import { readPreference, writePreference } from '../locale-preference.js'

// 打印参数快照（issue #128）：「上次设置」存 localStorage，命名预置存服务端。
// 两者共用同一份字段白名单；套用前一律经 sanitizePrintSettings 过滤，旧版本或
// 被篡改的数据只会丢掉非法字段，不会让表单进入非法状态。
// 打印机、页码范围属于单次打印上下文，不进快照。

export const LAST_PRINT_SETTINGS_KEY = 'print_settings_last'

export const DEFAULT_PRINT_SETTINGS = Object.freeze({
  isColor: true,
  invert: false,
  compressImages: false,
  duplex: 'one-sided',
  orientation: 'portrait',
  copies: 1,
  paperSize: 'A4',
  paperType: 'plain',
  mediaSource: 'auto',
  printScaling: 'auto',
  scalePercent: 100,
  pageSet: 'all',
  mirror: false,
  photoDateWatermark: false,
  watermarkText: '',
  numberUp: 1,
  numberUpLayout: 'lrtb',
  pageBorder: 'none'
})

const oneOf = values => value => values.includes(value)
const isBool = value => typeof value === 'boolean'
const intIn = (min, max) => value => Number.isInteger(value) && value >= min && value <= max
const LEGACY_PAPER_SIZES = ['A5', 'A4', 'A3', 'A2', 'A1', '5inch', '6inch', '7inch', '8inch', '10inch', 'Letter', 'Legal']

const validators = {
  isColor: isBool,
  invert: isBool,
  compressImages: isBool,
  duplex: oneOf(['one-sided', 'two-sided-long-edge', 'two-sided-short-edge']),
  orientation: oneOf(['portrait', 'landscape']),
  copies: intIn(1, 99),
  // 驱动纸张以 custom:<宽>x<高> 表示，与 PrinterMediaOptions 的约定一致。
  paperSize: value => LEGACY_PAPER_SIZES.includes(value) || (typeof value === 'string' && /^custom:\d+(\.\d+)?x\d+(\.\d+)?$/.test(value)),
  paperType: oneOf(['plain', 'photo', 'glossy', 'matte', 'envelope', 'cardstock', 'labels', 'auto']),
  // 纸盒关键字由打印机上报，这里只做形状校验，可用性在套用时按当前打印机判断。
  mediaSource: value => typeof value === 'string' && /^[A-Za-z0-9._-]{1,64}$/.test(value),
  printScaling: oneOf(['auto', 'auto-fit', 'fit', 'fill', 'none', 'custom']),
  scalePercent: intIn(10, 400),
  pageSet: oneOf(['all', 'odd', 'even', 'even-reverse']),
  mirror: isBool,
  photoDateWatermark: isBool,
  watermarkText: value => typeof value === 'string' && value.length <= 1000,
  numberUp: oneOf([1, 2, 4, 6, 9, 16]),
  numberUpLayout: oneOf(['lrtb', 'rltb', 'tblr', 'tbrl']),
  pageBorder: oneOf(['single', 'none'])
}

const DRIVER_GROUPS = ['pageSize', 'mediaType', 'quality']

function sanitizeChoice(choice) {
  if (!choice || typeof choice !== 'object' || Array.isArray(choice)) return null
  if (typeof choice.attribute !== 'string' || !choice.attribute || typeof choice.value !== 'string') return null
  const clean = { attribute: choice.attribute, value: choice.value, label: typeof choice.label === 'string' ? choice.label : '' }
  for (const key of ['widthMM', 'heightMM']) {
    if (typeof choice[key] === 'number' && Number.isFinite(choice[key]) && choice[key] > 0) clean[key] = choice[key]
  }
  if (choice.borderless === true) clean.borderless = true
  if (Array.isArray(choice.margins) && choice.margins.length === 4 && choice.margins.every(Number.isInteger)) clean.margins = [...choice.margins]
  return clean
}

function sanitizeDriverOptions(options) {
  if (!options || typeof options !== 'object' || Array.isArray(options)) return {}
  const clean = {}
  for (const group of DRIVER_GROUPS) {
    const choice = sanitizeChoice(options[group])
    if (choice) clean[group] = choice
  }
  return clean
}

// sanitizePrintSettings 返回只含合法字段的部分设置；driverOptions 与其所属
// 打印机 printerUri 成对出现，缺一不可。
export function sanitizePrintSettings(raw) {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {}
  const clean = {}
  for (const [key, valid] of Object.entries(validators)) {
    if (key in raw && valid(raw[key])) clean[key] = raw[key]
  }
  if (typeof raw.printerUri === 'string' && raw.printerUri) {
    clean.printerUri = raw.printerUri
    clean.driverOptions = sanitizeDriverOptions(raw.driverOptions)
  }
  return clean
}

// snapshotPrintSettings 从表单当前值生成快照。includeWatermark=false 用于
// 「上次设置」：一次临时加的水印不应悄悄带到下一次打印。
export function snapshotPrintSettings(values, { includeWatermark = true } = {}) {
  const snapshot = {}
  for (const key of Object.keys(DEFAULT_PRINT_SETTINGS)) {
    if (key === 'watermarkText' && !includeWatermark) continue
    snapshot[key] = values[key]
  }
  snapshot.printerUri = values.printerUri || ''
  snapshot.driverOptions = values.driverOptions || {}
  return sanitizePrintSettings(snapshot)
}

export function readLastPrintSettings(browser = globalThis) {
  const raw = readPreference(LAST_PRINT_SETTINGS_KEY, browser)
  if (!raw) return null
  try { return sanitizePrintSettings(JSON.parse(raw)) } catch { return null }
}

export function writeLastPrintSettings(settings, browser = globalThis) {
  writePreference(LAST_PRINT_SETTINGS_KEY, JSON.stringify(settings), browser)
}
