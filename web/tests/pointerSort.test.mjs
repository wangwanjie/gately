// 拖动排序自动滚动速度的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { edgeSpeed } from '../src/composables/usePointerSort.ts'

test('edgeSpeed 只在可见区域上下边缘滚动，越靠边越快', () => {
  // 可见区域 0~800，边缘 56，最快 22
  assert.equal(edgeSpeed(400, 0, 800), 0)
  assert.equal(edgeSpeed(56, 0, 800), 0)
  assert.equal(edgeSpeed(744, 0, 800), 0)
  assert.ok(edgeSpeed(40, 0, 800) < 0)
  assert.ok(edgeSpeed(760, 0, 800) > 0)
  assert.ok(edgeSpeed(5, 0, 800) < edgeSpeed(40, 0, 800))
  assert.ok(edgeSpeed(795, 0, 800) > edgeSpeed(760, 0, 800))
  // 指针移出可见区域时按最快速度，不会无限加速
  assert.equal(edgeSpeed(-300, 0, 800), -22)
  assert.equal(edgeSpeed(2000, 0, 800), 22)
  // 可见区域上边不在 0（对话框内的滚动容器）
  assert.equal(edgeSpeed(130, 100, 600), -Math.ceil((26 / 56) * 22))
})

test('edgeSpeed 区域很矮时缩小边缘，中间仍可停住', () => {
  assert.equal(edgeSpeed(50, 0, 100), 0)
  assert.ok(edgeSpeed(5, 0, 100) < 0)
  assert.ok(edgeSpeed(95, 0, 100) > 0)
})
