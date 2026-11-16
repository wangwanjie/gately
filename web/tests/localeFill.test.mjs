// 构建期补齐缺失文案键：目标语言缺少的键取回退语言的文案，已有的键不动。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { fillMissing } from '../src/i18n/localeFill.ts'

test('补齐缺失的叶子键与整段子树', () => {
  const target = { a: '甲', nested: { x: 'X' } }
  const base = { a: 'A', b: 'B', nested: { x: 'x', y: 'Y' }, group: { p: 'P', q: { r: 'R' } } }
  const n = fillMissing(target, base)
  assert.equal(n, 4)
  assert.deepEqual(target, { a: '甲', b: 'B', nested: { x: 'X', y: 'Y' }, group: { p: 'P', q: { r: 'R' } } })
})

test('完整时不改动且返回 0', () => {
  const target = { a: '甲', n: { x: 'X' } }
  assert.equal(fillMissing(target, { a: 'A', n: { x: 'x' } }), 0)
  assert.deepEqual(target, { a: '甲', n: { x: 'X' } })
})

test('类型不一致时保持目标语言原样，空串与数组也视为已有', () => {
  const target = { a: 'text', e: '', list: ['1'] }
  assert.equal(fillMissing(target, { a: { sub: 'S' }, e: 'E', list: ['x', 'y'] }), 0)
  assert.deepEqual(target, { a: 'text', e: '', list: ['1'] })
})
