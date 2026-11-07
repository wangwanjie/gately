// 列表前端分页的单元测试：node --test tests/（Node 22.18+ 可直接加载 .ts）
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { registerHooks } from 'node:module'
import { DEFAULT_PAGE_SIZE, PAGE_SIZES, clampPage, needsPager, normalizePageSize, pageCount, pageSlice } from '../src/paging.ts'

// usePager 按 Vite 习惯省略 .ts 扩展名导入，测试时补上扩展名再解析
registerHooks({
  resolve(specifier, context, next) {
    if (specifier.startsWith('.') && !/\.\w+$/.test(specifier) && context.parentURL?.includes('/src/')) {
      return next(specifier + '.ts', context)
    }
    return next(specifier, context)
  },
})
const { nextTick, ref } = await import('vue')
const { usePager } = await import('../src/composables/usePager.ts')

const range = (n) => Array.from({ length: n }, (_, i) => i + 1)

// 内存版 localStorage，测试每页条数的保存与被篡改时的回退
function fakeStorage(init = {}) {
  const m = new Map(Object.entries(init))
  return {
    getItem: (k) => (m.has(k) ? m.get(k) : null),
    setItem: (k, v) => m.set(k, String(v)),
    map: m,
  }
}

test('总页数：空列表算 1 页，非法参数不抛错', () => {
  assert.equal(pageCount(0, 50), 1)
  assert.equal(pageCount(1, 50), 1)
  assert.equal(pageCount(50, 50), 1)
  assert.equal(pageCount(51, 50), 2)
  assert.equal(pageCount(3813, 50), 77)
  assert.equal(pageCount(10, 0), 1)
  assert.equal(pageCount(NaN, 50), 1)
  assert.equal(pageCount(10, -5), 1)
})

test('页码收拢到有效范围', () => {
  assert.equal(clampPage(1, 0, 50), 1)
  assert.equal(clampPage(3, 120, 50), 3)
  assert.equal(clampPage(4, 120, 50), 3)
  assert.equal(clampPage(0, 120, 50), 1)
  assert.equal(clampPage(-2, 120, 50), 1)
  assert.equal(clampPage(NaN, 120, 50), 1)
  assert.equal(clampPage(Infinity, 120, 50), 1)
  assert.equal(clampPage(2.7, 120, 50), 2)
})

test('切片只返回当前页，越界页码取最后一页', () => {
  const list = range(120)
  assert.deepEqual(pageSlice(list, 1, 50), range(50))
  assert.deepEqual(pageSlice(list, 3, 50), range(20).map((i) => i + 100))
  assert.deepEqual(pageSlice(list, 9, 50), range(20).map((i) => i + 100))
  assert.deepEqual(pageSlice([], 1, 50), [])
  // 不修改原数组
  assert.equal(list.length, 120)
})

test('每页条数只接受可选值，被改过的存储值回到默认', () => {
  assert.equal(normalizePageSize(100), 100)
  assert.equal(normalizePageSize('200'), 200)
  assert.equal(normalizePageSize('1e9'), DEFAULT_PAGE_SIZE)
  assert.equal(normalizePageSize('0'), DEFAULT_PAGE_SIZE)
  assert.equal(normalizePageSize('-50'), DEFAULT_PAGE_SIZE)
  assert.equal(normalizePageSize('abc'), DEFAULT_PAGE_SIZE)
  assert.equal(normalizePageSize(null), DEFAULT_PAGE_SIZE)
  assert.equal(normalizePageSize({}), DEFAULT_PAGE_SIZE)
  assert.equal(normalizePageSize(30, [10, 30]), 30)
  // 默认值不在可选范围内时取第一个
  assert.equal(normalizePageSize('x', [10, 30], 50), 10)
  assert.ok(PAGE_SIZES.includes(DEFAULT_PAGE_SIZE))
})

test('条数不超过最小每页条数时不显示分页条', () => {
  assert.equal(needsPager(0), false)
  assert.equal(needsPager(20), false)
  assert.equal(needsPager(21), true)
  assert.equal(needsPager(5, [5, 10]), false)
  assert.equal(needsPager(6, [5, 10]), true)
})

test('usePager：只渲染当前页，列表变短时收拢页码', async () => {
  globalThis.localStorage = fakeStorage()
  const list = ref(range(3813))
  const p = usePager(() => list.value)
  assert.equal(p.size.value, DEFAULT_PAGE_SIZE)
  assert.equal(p.total.value, 3813)
  assert.equal(p.rows.value.length, 50)
  p.page.value = 77
  assert.deepEqual(p.rows.value, range(13).map((i) => i + 3800))
  // 解封最后一页的全部条目后回到新的最后一页
  list.value = range(3800)
  await nextTick()
  assert.equal(p.page.value, 76)
  assert.equal(p.rows.value.length, 50)
  // 每页条数变大后页码收拢
  p.size.value = 200
  await nextTick()
  assert.equal(p.page.value, 19)
  list.value = []
  await nextTick()
  assert.equal(p.page.value, 1)
  assert.deepEqual(p.rows.value, [])
})

test('usePager：搜索词或目录变化时回到第 1 页，刷新同一列表保持当前页', async () => {
  globalThis.localStorage = fakeStorage()
  const list = ref(range(500))
  const q = ref('')
  const p = usePager(() => list.value.filter((n) => String(n).includes(q.value)), { resetOn: q })
  p.page.value = 5
  // 定时刷新返回同样数量的新数组
  list.value = range(500)
  await nextTick()
  assert.equal(p.page.value, 5)
  q.value = '1'
  await nextTick()
  assert.equal(p.page.value, 1)
  // 多个触发源
  const dir = ref('/')
  const sortAsc = ref(true)
  const p2 = usePager(() => list.value, { resetOn: [dir, sortAsc] })
  p2.page.value = 3
  sortAsc.value = false
  await nextTick()
  assert.equal(p2.page.value, 1)
  p2.page.value = 4
  dir.value = '/data'
  await nextTick()
  assert.equal(p2.page.value, 1)
})

test('usePager：每页条数按键名保存，存储值非法或不可用时用默认值', async () => {
  const s = fakeStorage({ 'gately.pageSize.guard.bans': '100', 'gately.pageSize.evil': '999999' })
  globalThis.localStorage = s
  const list = ref(range(1000))
  const a = usePager(() => list.value, { storageKey: 'guard.bans' })
  assert.equal(a.size.value, 100)
  a.size.value = 20
  await nextTick()
  assert.equal(s.map.get('gately.pageSize.guard.bans'), '20')
  // 篡改成超大值不能让页面一次渲染全部条目
  const b = usePager(() => list.value, { storageKey: 'evil' })
  assert.equal(b.size.value, DEFAULT_PAGE_SIZE)
  assert.equal(b.rows.value.length, DEFAULT_PAGE_SIZE)
  // 自定义默认值
  const c = usePager(() => list.value, { storageKey: 'access.bans', defaultSize: 20 })
  assert.equal(c.size.value, 20)
  // 隐私模式下读写存储抛错
  globalThis.localStorage = {
    getItem() {
      throw new Error('denied')
    },
    setItem() {
      throw new Error('denied')
    },
  }
  const d = usePager(() => list.value, { storageKey: 'x' })
  assert.equal(d.size.value, DEFAULT_PAGE_SIZE)
  d.size.value = 100
  await nextTick()
  assert.equal(d.rows.value.length, 100)
  delete globalThis.localStorage
})
