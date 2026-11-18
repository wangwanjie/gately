// 拖动排序状态流转的单元测试（鼠标与触屏共用的 Pointer 事件逻辑）：node --test tests/
// 用一个最小的假 DOM 驱动 usePointerSort：window 事件、elementFromPoint 与放置目标元素
import { test, beforeEach } from 'node:test'
import assert from 'node:assert/strict'
import { ref } from 'vue'

// listeners 记录 window 上注册的事件处理函数，dispatch 模拟浏览器派发
const listeners = new Map()
function dispatch(type, ev) {
  for (const fn of [...(listeners.get(type) ?? [])]) {
    fn(ev)
  }
}

// targets 为假列表里的放置目标：每个 48px 高，按 id 顺序自上而下排列；group 为分组标题
let targets = []
function makeTarget(id, top, kind) {
  const el = {
    dataset: kind ? { dropId: id, dropKind: kind } : { dropId: id },
    closest: () => el,
    getBoundingClientRect: () => ({ top, bottom: top + 48, height: 48, left: 0, right: 300, width: 300 }),
  }
  return el
}

const rootClasses = new Set()
globalThis.window = {
  innerHeight: 800,
  addEventListener(type, fn) {
    if (!listeners.has(type)) {
      listeners.set(type, new Set())
    }
    listeners.get(type).add(fn)
  },
  removeEventListener(type, fn) {
    listeners.get(type)?.delete(fn)
  },
}
globalThis.document = {
  // elementFromPoint 返回纵坐标所在的放置目标，列表外返回 null
  elementFromPoint: (_x, y) => targets.find((t) => {
    const r = t.getBoundingClientRect()
    return y >= r.top && y < r.bottom
  }) ?? null,
  documentElement: { classList: { add: (c) => rootClasses.add(c), remove: (c) => rootClasses.delete(c) } },
  scrollingElement: { scrollTop: 0, scrollHeight: 800, clientHeight: 800 },
}
globalThis.getComputedStyle = () => ({ overflowY: 'visible' })
// 自动滚动只在边缘触发，这里的指针都在中间区域，帧回调不需要真正执行
globalThis.requestAnimationFrame = () => 1
globalThis.cancelAnimationFrame = () => {}

const { usePointerSort } = await import('../src/composables/usePointerSort.ts')

// setup 创建一个拖动排序实例；组件外调用 onBeforeUnmount 时 Vue 会警告，测试里静音
function setup() {
  const drops = []
  const root = { parentElement: null, contains: (el) => targets.includes(el) }
  const warn = console.warn
  console.warn = () => {}
  const s = usePointerSort({ root: ref(root), onDrop: (ids, over) => drops.push({ ids, over }) })
  console.warn = warn
  return { s, drops }
}

function pe(type, x, y, extra = {}) {
  return { type, clientX: x, clientY: y, pointerId: 1, pointerType: 'touch', button: 0, preventDefault() {}, ...extra }
}

beforeEach(() => {
  listeners.clear()
  rootClasses.clear()
  // a: 0~48，b: 48~96，c: 96~144，分组 g: 144~192
  targets = [makeTarget('a', 0), makeTarget('b', 48), makeTarget('c', 96), makeTarget('g', 144, 'group')]
})

test('拖到目标下半部分插到其后，上半部分插到其前，松手后清理状态', () => {
  const { s, drops } = setup()
  s.start(pe('pointerdown', 10, 20), ['a'], 'A')
  dispatch('pointermove', pe('pointermove', 10, 90))
  assert.deepEqual(s.dragIds.value, ['a'])
  assert.equal(s.ghost.show, true)
  assert.equal(s.ghost.label, 'A')
  assert.ok(rootClasses.has('g-pointer-sorting'))
  assert.deepEqual(s.over.value, { id: 'b', after: true })
  assert.deepEqual(s.dropClass('b'), { dragging: false, 'drop-before': false, 'drop-after': true })
  assert.equal(s.dropClass('a').dragging, true)
  dispatch('pointermove', pe('pointermove', 10, 100))
  assert.deepEqual(s.over.value, { id: 'c', after: false })
  dispatch('pointerup', pe('pointerup', 10, 100))
  assert.deepEqual(drops, [{ ids: ['a'], over: { id: 'c', after: false } }])
  assert.deepEqual(s.dragIds.value, [])
  assert.equal(s.over.value, null)
  assert.equal(s.ghost.show, false)
  assert.ok(!rootClasses.has('g-pointer-sorting'))
  // 结束后不再留有 window 监听
  for (const set of listeners.values()) {
    assert.equal(set.size, 0)
  }
})

test('点一下把手（移动不到阈值）不进入拖动，也不触发放置', () => {
  const { s, drops } = setup()
  s.start(pe('pointerdown', 10, 20), ['a'], 'A')
  dispatch('pointermove', pe('pointermove', 12, 22))
  assert.deepEqual(s.dragIds.value, [])
  assert.equal(s.ghost.show, false)
  dispatch('pointerup', pe('pointerup', 12, 22))
  assert.equal(drops.length, 0)
})

test('分组标题只记录目标，不区分前后；拖到自身或列表外没有放置位置', () => {
  const { s, drops } = setup()
  s.start(pe('pointerdown', 10, 20), ['a', 'b'], '2')
  dispatch('pointermove', pe('pointermove', 10, 185))
  assert.deepEqual(s.over.value, { id: 'g', after: false })
  dispatch('pointermove', pe('pointermove', 10, 60))
  assert.equal(s.over.value, null, '拖动中的条目自身不是放置位置')
  dispatch('pointermove', pe('pointermove', 10, 500))
  assert.equal(s.over.value, null, '列表外')
  dispatch('pointerup', pe('pointerup', 10, 500))
  assert.equal(drops.length, 0)
})

test('Esc 与系统取消放弃拖动，Esc 不再传给对话框', () => {
  const { s, drops } = setup()
  s.start(pe('pointerdown', 10, 20), ['a'], 'A')
  dispatch('pointermove', pe('pointermove', 10, 90))
  let stopped = false
  dispatch('keydown', { key: 'Escape', preventDefault() {}, stopPropagation() { stopped = true } })
  assert.ok(stopped)
  assert.deepEqual(s.dragIds.value, [])
  dispatch('pointerup', pe('pointerup', 10, 90))
  assert.equal(drops.length, 0)

  s.start(pe('pointerdown', 10, 20), ['a'], 'A')
  dispatch('pointermove', pe('pointermove', 10, 90))
  dispatch('pointercancel', pe('pointercancel', 10, 90))
  assert.deepEqual(s.dragIds.value, [])
  assert.equal(drops.length, 0)
})

test('只跟踪开始拖动的那根手指；鼠标右键与空列表不开始拖动', () => {
  const { s, drops } = setup()
  s.start(pe('pointerdown', 10, 20), ['a'], 'A')
  dispatch('pointermove', pe('pointermove', 10, 90, { pointerId: 2 }))
  assert.deepEqual(s.dragIds.value, [], '另一根手指的移动不算')
  dispatch('pointermove', pe('pointermove', 10, 90))
  dispatch('pointerup', pe('pointerup', 10, 90, { pointerId: 2 }))
  assert.deepEqual(s.dragIds.value, ['a'], '另一根手指抬起不结束拖动')
  dispatch('pointerup', pe('pointerup', 10, 90))
  assert.equal(drops.length, 1)

  s.start(pe('pointerdown', 10, 20, { pointerType: 'mouse', button: 2 }), ['a'], 'A')
  assert.equal(listeners.get('pointermove')?.size ?? 0, 0)
  s.start(pe('pointerdown', 10, 20), [], '')
  assert.equal(listeners.get('pointermove')?.size ?? 0, 0)
})

test('鼠标左键拖动与触屏相同；拖动中再次按下会放弃前一次', () => {
  const { s, drops } = setup()
  const mouse = { pointerType: 'mouse', button: 0 }
  s.start(pe('pointerdown', 10, 20, mouse), ['a'], 'A')
  dispatch('pointermove', pe('pointermove', 10, 90, mouse))
  s.start(pe('pointerdown', 10, 110, mouse), ['c'], 'C')
  assert.deepEqual(s.dragIds.value, [], '前一次拖动已放弃')
  dispatch('pointermove', pe('pointermove', 10, 30, mouse))
  dispatch('pointerup', pe('pointerup', 10, 30, mouse))
  assert.deepEqual(drops, [{ ids: ['c'], over: { id: 'a', after: true } }])
})
