import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync, existsSync } from 'node:fs'
import { baseCompile } from '@intlify/message-compiler'

const src = new URL('../', import.meta.url)
const read = file => readFileSync(new URL(file, src), 'utf8')
const flatten = (object, prefix = '') => Object.fromEntries(Object.entries(object).flatMap(([key, value]) => typeof value === 'string' ? [[prefix + key, value]] : Object.entries(flatten(value, prefix + key + '.'))))

test('locale catalogs have descriptive keys, identical placeholders and valid messages', () => {
  assert.ok(existsSync(new URL('locales/en.json', src)), 'English catalog exists')
  const en = flatten(JSON.parse(read('locales/en.json')))
  const zh = flatten(JSON.parse(read('locales/zh-CN.json')))
  assert.deepEqual(Object.keys(en).sort(), Object.keys(zh).sort())
  for (const [key, message] of Object.entries(en)) {
    assert.match(key, /^[a-zA-Z][\w]*(\.[a-zA-Z][\w]*)+$/)
    assert.doesNotMatch(key, /\.m[0-9a-f]{10}$/)
    assert.ok(message.trim() && zh[key].trim(), key)
    // Plural branches may repeat a placeholder in only one locale.
    const params = text => [...new Set([...text.matchAll(/\{(\w+)\}/g)].map(m => m[1]))].sort()
    assert.deepEqual(params(message), params(zh[key]), key)
    assert.doesNotMatch(message, /[\u3400-\u9fff]/, key)
    for (const text of [message, zh[key]]) baseCompile(text, { onError: error => assert.fail(`${key}: ${error.message}`) })
  }
  for (const file of readdirSync(src, { recursive: true }).filter(f => /\.(vue|js)$/.test(f) && !f.endsWith('.test.js'))) {
    for (const match of read(file).matchAll(/\bt\(['"]([^'"]+)['"]/g)) assert.ok(en[match[1]], `${file}: missing ${match[1]}`)
  }
})
