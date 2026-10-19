import { test } from 'node:test'
import assert from 'node:assert/strict'
import { computed } from 'vue'
import { setLanguage } from '../i18n.js'
import { translateError } from './translate-error.js'

// Exact application-owned messages from scheduled_queue.go, scan_registry.go,
// scan_devices.go, driver_handlers.go and ppd_match.go (not external logs).
const diagnostic = 'IPP 0x0500 /扫描/照片 @ {name}.pdf: 设备离线\nfilter failed'
const cases = [
  ['上次提交结果未确认，请核实打印机后再恢复任务', 'The previous submission result is unconfirmed. Check the printer before resuming the task.'],
  ['批次未完成，请核实打印机后再恢复任务', 'The batch is incomplete. Check the printer before resuming the task.'],
  [`提交结果未确认，请核实打印机后再恢复任务：${diagnostic}`, `The submission result is unconfirmed. Check the printer before resuming the task: ${diagnostic}`],
  [`提交结果未确认，请核实打印机后再恢复任务：IPP 提交结果未确认：${diagnostic}`, `The submission result is unconfirmed. Check the printer before resuming the task: IPP submission result is unconfirmed: ${diagnostic}`],
  ['任务已取消', 'Task cancelled'],
  ['探测超时(>12s)', 'Probe timed out (>12s)'],
  [`探测超时: ${diagnostic}`, `Probe timed out: ${diagnostic}`],
  ['上传的 .deb 包已归档，容器重启时会自动重新安装；若某个包始终装不上，可在宿主的 ./.drivers/custom-deb/packages/ 下删除它。', 'Uploaded .deb packages have been archived and will be reinstalled automatically when the container restarts. If a package repeatedly fails to install, delete it from ./.drivers/custom-deb/packages/ on the host.'],
  ['PostScript 驱动（依赖打印机 PS 解释器，老机型可能不稳定）', 'PostScript driver (depends on the printer’s PS interpreter; older models may be unstable)'],
  ['该 .deb 安装成功但归档失败，容器重启后需要重新上传安装（且列表中不会显示该包）。', 'The .deb package was installed but could not be archived. Upload it again after a restart; it will not appear in the package list.']
]

for (const [raw, english] of cases) test(`known app message reacts to language changes: ${raw}`, () => {
  setLanguage('en', {})
  const rendered = computed(() => translateError(raw))
  assert.equal(rendered.value, english)
  setLanguage('zh-CN', {})
  assert.equal(rendered.value, raw)
  assert.equal(translateError(english), raw, 'an already displayed message can switch back')
  setLanguage('en', {})
  assert.equal(rendered.value, english)
})

test('unknown diagnostics and app-like text inside external output stay byte-for-byte intact', () => {
  const diagnostics = [
    diagnostic,
    'scanimage: 任务已取消',
    'vendor output: 探测超时(>12s)',
    '上次提交结果未确认，请核实打印机后再恢复任务 - vendor extension',
    'PostScript 驱动（厂商提供的未知说明）',
    '上传的 .deb 包已归档，unknown vendor instructions'
  ]
  for (const language of ['en', 'zh-CN']) {
    setLanguage(language, {})
    for (const raw of diagnostics) assert.equal(translateError(raw), raw)
  }
  setLanguage('en', {})
})
