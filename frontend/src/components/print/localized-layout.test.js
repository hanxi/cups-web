import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'
import { baseParse } from '@vue/compiler-dom'

function templateElements(file) {
  const { descriptor } = parse(readFileSync(new URL(file, import.meta.url), 'utf8'))
  const elements = []
  function visit(node, parent) {
    if (node.type === 1) elements.push({ node, parent })
    for (const child of node.children || []) visit(child, node)
  }
  visit(baseParse(descriptor.template.content))
  return elements
}

function classes(node) {
  return new Set(node.props.find(prop => prop.name === 'class')?.value?.content.split(/\s+/))
}

// Node has no layout engine: guard the wrapping/shrinking contract here;
// verify actual element bounds in browser QA at 390px in both locales.
test('crop controls wrap as a group while the fine rotation slider can shrink', () => {
  const elements = templateElements('./ImageCropModal.vue')
  const { node: slider, parent: fineControls } = elements.find(({ node }) =>
    node.tag === 'input' && node.props.some(prop => prop.name === 'type' && prop.value?.content === 'range'))
  assert.ok(classes(slider).has('min-w-0'), 'The range input must not impose its intrinsic minimum width')
  assert.ok(classes(fineControls).has('min-w-0'))
  assert.ok(classes(fineControls).has('basis-48'), 'Keep room for the slider, value and reset as one wrapping group')
  const { parent: toolbar } = elements.find(({ node }) => node === fineControls)
  assert.ok(classes(toolbar).has('flex-wrap'), 'Long rotation labels must not push fine controls outside the dialog')
  assert.ok(fineControls.children.some(node => node.tag === 'span'), 'Keep the angle with its slider')
  assert.ok(fineControls.children.some(node => node.tag === 'UButton'), 'Keep reset with its slider')
})

test('PPD chooser uses the modal scrollable body and separate reachable footer', () => {
  const elements = templateElements('../../views/DriversView.vue')
  const { node: modal } = elements.find(({ node }) => node.tag === 'UModal' &&
    node.props.some(prop => prop.name === 'model' && prop.exp?.content === 'showPPDModal'))
  const slots = modal.children.filter(node => node.tag === 'template').map(node =>
    node.props.find(prop => prop.name === 'slot')?.arg?.content)
  assert.ok(slots.includes('body'), 'Use Nuxt UI’s bounded, scrollable modal body')
  assert.ok(slots.includes('footer'), 'Keep actions outside the overflowing candidate list')
  assert.ok(!slots.includes('content'), 'Do not bypass the modal scroll/flex layout')
})

test('page subsets use two shrinkable columns on narrow dialogs, four on wider screens', () => {
  const elements = templateElements('./PrintOptions.vue')
  const { node: label, parent: group } = elements.find(({ node }) =>
    node.tag === 'label' && node.props.some(prop => prop.name === 'for' && prop.exp?.content === 'item in pageSetItems'))
  assert.ok(classes(group).has('grid'), 'A single non-wrapping flex row clips translated labels')
  assert.ok(classes(group).has('grid-cols-2'))
  assert.ok(classes(group).has('sm:grid-cols-4'))
  assert.ok(classes(label).has('min-w-0'), 'Labels must be allowed to shrink within the grid')
})
