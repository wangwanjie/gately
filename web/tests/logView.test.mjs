// 日志面板高度调整与贴底判断的单元测试：node --test tests/（Node 22.18+ 可直接加载 .ts）
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { BOTTOM_SLACK, KEY_STEP, MIN_HEIGHT, VIEWPORT_MARGIN, atBottom, clampHeight, heightKey, keyHeight, loadHeight, maxHeight, saveHeight } from '../src/logView.ts'

// 内存版 localStorage；broken 为真时每次读写都抛错，模拟隐私模式或存储被禁用
function fakeStorage(init = {}, broken = false) {
  const m = new Map(Object.entries(init))
  const guard = () => {
    if (broken) {
      throw new Error('SecurityError')
    }
  }
  return {
    getItem: (k) => (guard(), m.has(k) ? m.get(k) : null),
    setItem: (k, v) => (guard(), m.set(k, String(v))),
    removeItem: (k) => (guard(), m.delete(k)),
    map: m,
  }
}

test('maxHeight 按视口留出边距，视口很矮时不低于最小高度', () => {
  assert.equal(maxHeight(900), 900 - VIEWPORT_MARGIN)
  assert.equal(maxHeight(812.6), Math.floor(812.6 - VIEWPORT_MARGIN))
  assert.equal(maxHeight(100), MIN_HEIGHT)
  assert.equal(maxHeight(0), MIN_HEIGHT)
})

test('clampHeight 限制范围并取整，非有限数返回 null', () => {
  assert.equal(clampHeight(500, 900), 500)
  assert.equal(clampHeight(500.6, 900), 501)
  assert.equal(clampHeight(10, 900), MIN_HEIGHT)
  assert.equal(clampHeight(-300, 900), MIN_HEIGHT)
  assert.equal(clampHeight(5000, 900), 780)
  // 视口比最小高度还矮时仍保证最小高度
  assert.equal(clampHeight(5000, 50), MIN_HEIGHT)
  assert.equal(clampHeight(NaN, 900), null)
  assert.equal(clampHeight(Infinity, 900), null)
})

test('heightKey 区分页面卡片与弹窗', () => {
  assert.equal(heightKey('card'), 'gately.logs.height.card')
  assert.notEqual(heightKey('card'), heightKey('dialog'))
})

test('loadHeight 只接受合理范围内的整数', () => {
  const k = heightKey('card')
  assert.equal(loadHeight(fakeStorage({ [k]: '480' }), 'card'), 480)
  assert.equal(loadHeight(fakeStorage({ [k]: String(MIN_HEIGHT) }), 'card'), MIN_HEIGHT)
  assert.equal(loadHeight(fakeStorage(), 'card'), null)
  // 被篡改或损坏的值一律回退默认高度
  for (const bad of ['', 'abc', '12.5', '-200', '1e3', '99', '99999', '123456', ' 300', '300px', 'null']) {
    assert.equal(loadHeight(fakeStorage({ [k]: bad }), 'card'), null, bad)
  }
  // 另一个位置的值互不影响
  assert.equal(loadHeight(fakeStorage({ [k]: '480' }), 'dialog'), null)
  // 存储不可用时不抛错
  assert.equal(loadHeight(fakeStorage({ [k]: '480' }, true), 'card'), null)
  assert.equal(loadHeight(undefined, 'card'), null)
})

test('saveHeight 保存取整后的值，null 删除，存储不可用时不抛错', () => {
  const s = fakeStorage()
  saveHeight(s, 'card', 512.4)
  assert.equal(s.map.get(heightKey('card')), '512')
  assert.equal(loadHeight(s, 'card'), 512)
  // 越界的值保存时收进范围，读回后仍有效
  saveHeight(s, 'card', 5)
  assert.equal(loadHeight(s, 'card'), MIN_HEIGHT)
  saveHeight(s, 'card', 1e9)
  assert.equal(loadHeight(s, 'card'), 10000)
  saveHeight(s, 'card', null)
  assert.equal(s.map.has(heightKey('card')), false)
  assert.doesNotThrow(() => saveHeight(fakeStorage({}, true), 'card', 300))
  assert.doesNotThrow(() => saveHeight(undefined, 'card', 300))
})

test('atBottom 在底部余量内视为贴底，内容不足一屏也算贴底', () => {
  assert.equal(atBottom({ scrollTop: 700, scrollHeight: 1000, clientHeight: 300 }), true)
  assert.equal(atBottom({ scrollTop: 700 - BOTTOM_SLACK, scrollHeight: 1000, clientHeight: 300 }), true)
  assert.equal(atBottom({ scrollTop: 700 - BOTTOM_SLACK - 1, scrollHeight: 1000, clientHeight: 300 }), false)
  assert.equal(atBottom({ scrollTop: 0, scrollHeight: 1000, clientHeight: 300 }), false)
  assert.equal(atBottom({ scrollTop: 0, scrollHeight: 200, clientHeight: 300 }), true)
  // 小数像素（缩放后常见）不误判
  assert.equal(atBottom({ scrollTop: 699.5, scrollHeight: 1000, clientHeight: 300 }), true)
})

test('keyHeight 方向键、翻页键与 Home/End', () => {
  assert.equal(keyHeight('ArrowDown', 300, 900), 300 + KEY_STEP)
  assert.equal(keyHeight('ArrowUp', 300, 900), 300 - KEY_STEP)
  assert.equal(keyHeight('PageDown', 300, 900), 300 + KEY_STEP * 4)
  assert.equal(keyHeight('PageUp', 300, 900), 300 - KEY_STEP * 4)
  assert.equal(keyHeight('Home', 300, 900), MIN_HEIGHT)
  assert.equal(keyHeight('End', 300, 900), maxHeight(900))
  // 到边界后不再越界
  assert.equal(keyHeight('ArrowUp', MIN_HEIGHT, 900), MIN_HEIGHT)
  assert.equal(keyHeight('ArrowDown', maxHeight(900), 900), maxHeight(900))
  assert.equal(keyHeight('Enter', 300, 900), undefined)
  assert.equal(keyHeight('a', 300, 900), undefined)
})
