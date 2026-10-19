import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import { parse, compileScript, compileTemplate } from '@vue/compiler-sfc'
import { baseParse } from '@vue/compiler-dom'

const root = new URL('./', import.meta.url)
for (const file of readdirSync(root, { recursive: true }).filter(f => f.endsWith('.vue'))) {
  test(`${file}: template bindings and localized accessible controls`, () => {
    const { descriptor } = parse(readFileSync(new URL(file, root), 'utf8'))
    const script = compileScript(descriptor, { id: file })
    const template = compileTemplate({ source: descriptor.template.content, filename: file, id: file, compilerOptions: { bindingMetadata: script.bindings } })
    assert.deepEqual(template.errors, [])
    const globals = new Set(['$emit', '$event', '$slots', '$attrs'])
    for (const [, binding] of template.code.matchAll(/_ctx\.([\w$]+)/g)) assert.ok(globals.has(binding), `Missing setup binding: ${binding}`)
    const visit = node => {
      if (node.type === 2) assert.doesNotMatch(node.content, /[\u3400-\u9fff]/, 'Untranslated template text')
      if (node.type === 1) {
        for (const prop of node.props) if (prop.type === 6 && prop.value) assert.doesNotMatch(prop.value.content, /[\u3400-\u9fff]/, 'Untranslated attribute')
        if (node.tag === 'img') assert.ok(node.props.some(p => p.name === 'alt' || p.arg?.content === 'alt'), 'Preview images need an accessible name')
        if (node.tag === 'UModal') assert.ok(node.props.some(p => p.arg?.content === 'title'), 'Dialogs need a localized accessible title')
        if (node.tag === 'UButton' && !node.children.length) assert.ok(node.props.some(p => p.name === 'aria-label' || p.arg?.content === 'aria-label'), 'Icon-only button needs a localized accessible label')
      }
      for (const child of node.children || []) visit(child)
    }
    visit(baseParse(descriptor.template.content))
  })
}
