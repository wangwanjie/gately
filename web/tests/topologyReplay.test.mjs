// 拓扑回放纯函数的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { bucketAt, formatClock, frameIdle, nextCursor, playTick, sameDay, spanParts, timeToX, timeline, xToTime } from '../src/topologyReplay.ts'

test('timeToX 与 xToTime 互为换算并对齐采样间隔', () => {
  assert.equal(timeToX(150, 100, 200, 100), 50)
  assert.equal(timeToX(50, 100, 200, 100), 0)
  assert.equal(timeToX(500, 100, 200, 100), 100)
  // 只有一个时刻时放在最右侧
  assert.equal(timeToX(100, 100, 100, 80), 80)
  assert.equal(xToTime(50, 100, 200, 100, 5), 150)
  assert.equal(xToTime(52, 100, 200, 100, 5), 150)
  assert.equal(xToTime(53, 100, 200, 100, 5), 155)
  assert.equal(xToTime(-10, 100, 200, 100, 5), 100)
  assert.equal(xToTime(999, 100, 200, 100, 5), 200)
  assert.equal(xToTime(10, 100, 100, 100, 5), 100)
})

test('timeline 面积图在空档处断开并标出异常', () => {
  const b = (t, qps, issue = false) => ({ t, qps, bps: 0, active: 0, ips: 0, issue })
  // 0..100 秒，桶宽 10，30..60 秒空档，第 70 秒的桶有异常
  const g = timeline([b(0, 1), b(10, 2), b(20, 4), b(60, 1), b(70, 2, true), b(80, 0), b(90, 1)], 0, 90, 10, 100, 40)
  assert.equal(g.peak, 4)
  assert.equal((g.area.match(/M/g) ?? []).length, 2, '空档两侧应为两段面积')
  assert.equal((g.area.match(/Z/g) ?? []).length, 2)
  assert.equal(g.gaps.length, 1)
  assert.equal(g.gaps[0].x, 30)
  assert.equal(g.gaps[0].w, 30)
  assert.deepEqual(g.issues, [{ x: 70, w: 10 }])
  // 峰值所在的桶贴近顶部，零值贴近底部
  assert.match(g.line, /L30\.0,2\.0/)
  assert.equal(timeline([], 0, 0, 0, 100, 40).area, '')
  // 全为 0 时画在底线上，不出现 NaN
  assert.doesNotMatch(timeline([b(0, 0)], 0, 0, 5, 100, 40).area, /NaN/)
})

test('bucketAt 二分查找所在的桶', () => {
  const bs = [0, 10, 20, 60].map((t) => ({ t, qps: t, bps: 0, active: 0, ips: 0 }))
  assert.equal(bucketAt(bs, 10, 15)?.t, 10)
  assert.equal(bucketAt(bs, 10, 60)?.t, 60)
  assert.equal(bucketAt(bs, 10, 69)?.t, 60)
  assert.equal(bucketAt(bs, 10, 40), undefined)
  assert.equal(bucketAt(bs, 10, 70), undefined)
  assert.equal(bucketAt([], 10, 0), undefined)
})

test('playTick 速度与前进步长', () => {
  assert.deepEqual(playTick(1, 5), { ms: 5000, advance: 5 })
  assert.deepEqual(playTick(10, 5), { ms: 500, advance: 5 })
  // 过快时合并多个采样，间隔不低于 250 毫秒
  assert.deepEqual(playTick(30, 5), { ms: 333, advance: 10 })
  assert.deepEqual(playTick(60, 5), { ms: 250, advance: 15 })
  assert.deepEqual(playTick(0, 0), { ms: 5000, advance: 5 })
})

test('nextCursor 播放到结尾停止、跳过空闲段', () => {
  const meta = { frameStart: 100, frameEnd: 200, end: 500, next: 105, step: 5 }
  assert.equal(nextCursor(100, meta, false, 5, true), 105)
  assert.equal(nextCursor(100, meta, false, 15, true), 115)
  // 空闲帧且所在段还有后续：跳到段后第一个采样
  assert.equal(nextCursor(100, meta, true, 5, true), 205)
  assert.equal(nextCursor(100, meta, true, 5, false), 105)
  // 下一个采样在空档之后：跳到 next
  assert.equal(nextCursor(200, { ...meta, next: 300 }, false, 5, true), 300)
  assert.equal(nextCursor(500, { ...meta, next: 0 }, false, 5, true), null)
  assert.equal(nextCursor(495, { ...meta, next: 500 }, false, 15, true), 500)
})

test('frameIdle', () => {
  const z = { qps: 0, bps: 0, qps60: 0 }
  assert.equal(frameIdle({ rate: z, active: 0, ipTotal: 0 }), true)
  assert.equal(frameIdle({ rate: { ...z, qps60: 0.1 }, active: 0, ipTotal: 0 }), false)
  assert.equal(frameIdle({ rate: z, active: 1, ipTotal: 0 }), false)
  assert.equal(frameIdle({ rate: z, active: 0, ipTotal: 2 }), false)
})

test('时间格式与时长拆分', () => {
  const t = new Date(2026, 10, 10, 9, 5, 7).getTime() / 1000
  assert.equal(formatClock(t), '09:05:07')
  assert.equal(formatClock(t, true), '11-10 09:05:07')
  assert.equal(sameDay(t, t + 3600), true)
  assert.equal(sameDay(t, t + 86400), false)
  assert.deepEqual(spanParts(90061), { d: 1, h: 1, m: 1, s: 1 })
  assert.deepEqual(spanParts(-5), { d: 0, h: 0, m: 0, s: 0 })
})
