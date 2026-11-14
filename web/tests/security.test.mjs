// 管理端安全评分前端规则的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { bannerLevel, failingItems, fixTarget, isWeakCode, localWeakCode, snoozeKey, strengthBars, SEC_SNOOZE_MS, WEAK_CODES } from '../src/security.ts'

const it = (id, status, points, max = points) => ({ id, status, points, max })
const mk = (level, items) => ({ score: 70, level, exposed: true, passwordIssue: '', items })
const now = 1_000_000_000_000

test('failingItems 只返回未满分项，按扣分从多到少，同分按固定顺序', () => {
  const s = mk('warn', [it('username', 'ok', 0, 5), it('https', 'warn', 10), it('entry', 'warn', 15), it('twoFactor', 'warn', 10), it('password', 'risk', 35)])
  assert.deepEqual(failingItems(s).map((x) => x.id), ['password', 'entry', 'twoFactor', 'https'])
  assert.deepEqual(failingItems(undefined), [])
  // 状态不是 ok 但没有扣分的项不显示
  assert.deepEqual(failingItems(mk('good', [it('entry', 'warn', 0, 15)])), [])
})

test('良好或无数据时不提示；risk 一直提示且不能暂缓', () => {
  assert.equal(bannerLevel(undefined, { snooze: '', now }), '')
  assert.equal(bannerLevel(mk('good', []), { snooze: '', now }), '')
  const risk = mk('risk', [it('password', 'risk', 35)])
  const snooze = JSON.stringify({ key: snoozeKey(risk), until: now + 1000 })
  assert.equal(bannerLevel(risk, { snooze, now }), 'risk')
})

test('warn 级别可暂缓，问题变化或到期后重新提示', () => {
  const warn = mk('warn', [it('entry', 'warn', 15), it('https', 'warn', 10)])
  const key = snoozeKey(warn)
  assert.equal(key, 'entry,https')
  assert.equal(bannerLevel(warn, { snooze: '', now }), 'warn')
  assert.equal(bannerLevel(warn, { snooze: JSON.stringify({ key, until: now + 1000 }), now }), '')
  // 已到期
  assert.equal(bannerLevel(warn, { snooze: JSON.stringify({ key, until: now - 1 }), now }), 'warn')
  // 出现了新的问题
  const worse = mk('warn', [it('entry', 'warn', 15), it('https', 'warn', 10), it('twoFactor', 'warn', 20)])
  assert.equal(bannerLevel(worse, { snooze: JSON.stringify({ key, until: now + 1000 }), now }), 'warn')
  // 暂缓期被篡改得超过上限：忽略
  assert.equal(bannerLevel(warn, { snooze: JSON.stringify({ key, until: now + SEC_SNOOZE_MS + 5000 }), now }), 'warn')
  // 记录损坏
  assert.equal(bannerLevel(warn, { snooze: '{bad', now }), 'warn')
})

test('修复入口覆盖全部评分项', () => {
  assert.deepEqual(fixTarget('entry'), { tab: 'access', anchor: 'sec-entry' })
  assert.deepEqual(fixTarget('twoFactor'), { tab: 'account', anchor: 'sec-twofactor' })
  assert.deepEqual(fixTarget('lockout'), { tab: 'account', anchor: 'sec-loginprotect' })
  assert.deepEqual(fixTarget('globalLimit'), { tab: 'account', anchor: 'sec-loginprotect' })
  for (const id of ['password', 'username', 'https', 'unknown']) {
    assert.deepEqual(fixTarget(id), { tab: 'account', anchor: 'sec-account' })
  }
})

test('本地长度判断：按字符数算下限、按 UTF-8 字节数算上限', () => {
  assert.equal(localWeakCode('short'), 'tooShort')
  assert.equal(localWeakCode('123456789'), 'tooShort')
  assert.equal(localWeakCode('1234567890'), '')
  // 中文每字 3 字节：24 字 = 72 字节通过，25 字 = 75 字节过长
  assert.equal(localWeakCode('我'.repeat(24)), '')
  assert.equal(localWeakCode('我'.repeat(25)), 'tooLong')
  assert.equal(localWeakCode('a'.repeat(72)), '')
  assert.equal(localWeakCode('a'.repeat(73)), 'tooLong')
  assert.equal(localWeakCode('abc\ndefghijk'), 'control')
})

test('后端原因代码集合与判断', () => {
  assert.equal(WEAK_CODES.length, 9)
  assert.ok(isWeakCode('common'))
  assert.ok(!isWeakCode('nope'))
  assert.ok(!isWeakCode(undefined))
})

test('强度条：空为 0，不合格封顶 1，合格按长度与种类递增', () => {
  assert.equal(strengthBars('', undefined), 0)
  assert.equal(strengthBars('short', undefined), 1)
  assert.equal(strengthBars('Zm3$kq9Lr!vT', false), 1)
  assert.equal(strengthBars('kjxhzpqwmvnb', true), 2)
  assert.equal(strengthBars('Zm3kq9LrvTab', true), 3)
  assert.equal(strengthBars('Zm3$kq9Lr!vTxy', true), 4)
  assert.equal(strengthBars('kjxhzpqwmvnbtrgd', true), 3)
})
