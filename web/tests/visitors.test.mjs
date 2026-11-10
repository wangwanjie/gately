// 访客审计页面辅助函数的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { latestOnly, shrinkImpact } from '../src/visitors.ts'

test('latestOnly 只认最后一次请求', () => {
  const seq = latestOnly()
  const a = seq.next()
  const b = seq.next()
  assert.equal(seq.isLatest(a), false)
  assert.equal(seq.isLatest(b), true)
  // 两个请求互相独立计数
  const other = latestOnly()
  assert.equal(other.isLatest(other.next()), true)
  assert.equal(seq.isLatest(b), true)
})

test('shrinkImpact 判断保存设置会删除哪些数据', () => {
  const cur = { days: 30, max: 10000, count: 5000 }
  assert.deepEqual(shrinkImpact(cur, { days: 30, max: 10000 }), [])
  assert.deepEqual(shrinkImpact(cur, { days: 7, max: 10000 }), [{ kind: 'days', days: 7 }])
  // 调小上限但仍不少于现有访客数时不删除
  assert.deepEqual(shrinkImpact(cur, { days: 30, max: 6000 }), [])
  assert.deepEqual(shrinkImpact(cur, { days: 30, max: 1000 }), [{ kind: 'max', max: 1000, n: 4000 }])
  assert.deepEqual(shrinkImpact(cur, { days: 1, max: 100 }), [
    { kind: 'days', days: 1 },
    { kind: 'max', max: 100, n: 4900 },
  ])
  // 调大或没有数据时不确认
  assert.deepEqual(shrinkImpact(cur, { days: 60, max: 20000 }), [])
  assert.deepEqual(shrinkImpact({ days: 30, max: 10000, count: 0 }, { days: 1, max: 100 }), [])
  assert.deepEqual(shrinkImpact(null, { days: 1, max: 100 }), [])
})

test('WebVisitors 设置调小最多访客数需确认、列表与详情丢弃过期响应', () => {
  const src = readFileSync(new URL('../src/views/WebVisitors.vue', import.meta.url), 'utf8')
  assert.match(src, /shrinkImpact\(info\.value, form\)/)
  assert.match(src, /visitors\.maxShrinkConfirm/)
  assert.match(src, /listSeq\.isLatest\(seq\)/)
  assert.match(src, /detailSeq\.isLatest\(seq\)/)
})

test('TopologyReplay 的 trackEl 监听在 setup 顶层注册', () => {
  const src = readFileSync(new URL('../src/components/TopologyReplay.vue', import.meta.url), 'utf8')
  const mounted = src.indexOf('onMounted(')
  const w = src.indexOf('watch(trackEl')
  assert.ok(w > 0 && mounted > 0 && w < mounted, '应在 onMounted 之前注册')
  // onMounted 回调内不再注册 watch
  const body = src.slice(mounted, src.indexOf('onBeforeUnmount(', mounted))
  assert.doesNotMatch(body, /\bwatch\(/)
})
