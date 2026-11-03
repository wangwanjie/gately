// 页面头部吸顶计算的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { HEAD_PAD_TOP, headLayout, isStuck } from '../src/stickyHead.ts'

test('没有操作栏的头部不吸顶', () => {
  assert.equal(headLayout({ headHeight: 70, mainTop: 10, mainHeight: 44, actionsTop: null }), null)
})

test('操作栏与标题同一行时整行吸住', () => {
  // 宽屏：操作栏垂直居中，顶部略低于标题区顶部，但仍在标题区内
  assert.deepEqual(headLayout({ headHeight: 66, mainTop: 10, mainHeight: 44, actionsTop: 16 }), { offset: 0, visible: 66 })
  // 操作栏比标题高（多行按钮在同一行右侧），顶部在标题区上方
  assert.deepEqual(headLayout({ headHeight: 90, mainTop: 23, mainHeight: 44, actionsTop: 10 }), { offset: 0, visible: 90 })
})

test('操作栏换行到标题下方时只露出操作栏', () => {
  // 标题区 10~54，行间距 12，操作栏从 66 开始，高 32，下内边距 12
  const l = headLayout({ headHeight: 110, mainTop: 10, mainHeight: 44, actionsTop: 66 })
  assert.equal(l.offset, 66 - HEAD_PAD_TOP)
  // 露出部分：上方留白 + 操作栏 + 下内边距
  assert.equal(l.visible, HEAD_PAD_TOP + 32 + 12)
})

test('移动端操作栏折成多行时露出全部操作栏', () => {
  const l = headLayout({ headHeight: 10 + 60 + 12 + 3 * 32 + 2 * 8 + 12, mainTop: 10, mainHeight: 60, actionsTop: 82 })
  assert.equal(l.offset, 72)
  assert.equal(l.visible, 10 + 3 * 32 + 2 * 8 + 12)
})

test('小数像素四舍五入，偏移不为负', () => {
  const l = headLayout({ headHeight: 109.6, mainTop: 10, mainHeight: 43.7, actionsTop: 65.6 })
  assert.equal(l.offset, 56)
  assert.equal(l.visible, 54)
  // 标题区底部与操作栏顶部只差亚像素，也算换行
  assert.equal(headLayout({ headHeight: 60, mainTop: 0, mainHeight: 30, actionsTop: 29.5 }).offset, 20)
  assert.equal(headLayout({ headHeight: 40, mainTop: 0, mainHeight: 4, actionsTop: 4 }).offset, 0)
})

test('吸住判断：顶端到达「顶栏高度 - 偏移」即为吸住', () => {
  // 桌面顶栏 56，整行吸住（offset 0）
  assert.equal(isStuck(66, 56, 0), false)
  assert.equal(isStuck(56.4, 56, 0), true)
  assert.equal(isStuck(56, 56, 0), true)
  // 移动端顶栏 52，只露操作栏（offset 76）：吸住后顶端停在 -24
  assert.equal(isStuck(-23, 52, 76), false)
  assert.equal(isStuck(-24, 52, 76), true)
})

test('操作栏为空（插槽内容全被 v-if 隐藏）时只露出上下留白', () => {
  // 窄屏纵向排列：标题区 10~54，空操作栏紧接在下方（高 0），下内边距 12
  const l = headLayout({ headHeight: 10 + 44 + 12 + 0 + 12, mainTop: 10, mainHeight: 44, actionsTop: 66 })
  assert.equal(l.offset, 56)
  assert.equal(l.visible, HEAD_PAD_TOP + 12)
})

test('吸住判断在偏移为 0 与亚像素边界上稳定', () => {
  assert.equal(isStuck(56.5, 56, 0), true)
  assert.equal(isStuck(56.51, 56, 0), false)
  // 顶栏高度变化（断点切换）后按新高度判断
  assert.equal(isStuck(52, 52, 0), true)
  assert.equal(isStuck(54, 52, 0), false)
})
