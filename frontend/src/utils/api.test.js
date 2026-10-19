import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readError } from './api.js'
import { setLanguage } from '../i18n.js'

test('readError consumes plain-text and JSON responses once and preserves unknown diagnostics', async () => {
  setLanguage('en', {})
  const plain = new Response('CUPS filter failed: vendor code 0x42', { status: 502 })
  assert.equal(await readError(plain), 'CUPS filter failed: vendor code 0x42')
  assert.equal(plain.bodyUsed, true)
  assert.equal(await readError(new Response(JSON.stringify({ error: 'invalid credentials' }))), 'Incorrect username or password')
  assert.equal(await readError(new Response(JSON.stringify({ message: 'external diagnostic 中文' }))), 'external diagnostic 中文')
  setLanguage('zh-CN', {})
  assert.equal(await readError(new Response(JSON.stringify({ error: 'invalid credentials' }))), '用户名或密码错误')
  assert.equal(await readError(new Response('unknown-name.pdf: broken device')), 'unknown-name.pdf: broken device')
})
