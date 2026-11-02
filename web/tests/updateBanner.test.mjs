// 顶部升级提示规则的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { bannerKind, securityKey, SNOOZE_MS } from '../src/updateBanner.ts'

const base = { current: 'v0.6.0', latest: 'v0.6.3', hasUpdate: true, security: false, advisories: [], securityReleases: [] }
const sec = { ...base, security: true, advisories: [{ id: 'GHSA-m6h7-mm5r-r53q' }], securityReleases: [{ version: 'v0.6.2' }, { version: 'v0.6.1' }] }
const now = 1_000_000_000_000
const st = (o = {}) => ({ snooze: '', skipped: '', now, ...o })

test('没有检查结果或已是最新时不提示', () => {
  assert.equal(bannerKind(undefined, st()), '')
  assert.equal(bannerKind({ ...base, latest: '' }, st()), '')
  assert.equal(bannerKind({ ...base, hasUpdate: false }, st()), '')
})

test('普通新版本可以忽略该版本，下一个版本重新提示', () => {
  assert.equal(bannerKind(base, st()), 'update')
  assert.equal(bannerKind(base, st({ skipped: 'v0.6.3' })), '')
  assert.equal(bannerKind({ ...base, latest: 'v0.6.4' }, st({ skipped: 'v0.6.3' })), 'update')
})

test('安全问题的标识与顺序无关', () => {
  assert.equal(securityKey(sec), 'GHSA-m6h7-mm5r-r53q,v0.6.1,v0.6.2')
  assert.equal(securityKey({ advisories: [], securityReleases: [{ version: 'v0.6.2' }, { version: 'v0.6.1' }] }), 'v0.6.1,v0.6.2')
})

test('安全提示不受忽略版本影响，只能暂缓 24 小时', () => {
  assert.equal(bannerKind(sec, st({ skipped: 'v0.6.3' })), 'security')
  const snooze = JSON.stringify({ key: securityKey(sec), until: now + SNOOZE_MS })
  assert.equal(bannerKind(sec, st({ snooze })), '')
  // 到期后重新提示
  assert.equal(bannerKind(sec, st({ snooze, now: now + SNOOZE_MS + 1 })), 'security')
  // 出现新的安全问题时立即提示
  const more = { ...sec, advisories: [...sec.advisories, { id: 'GHSA-2345-6789-cfgh' }] }
  assert.equal(bannerKind(more, st({ snooze })), 'security')
  // 手改 localStorage 写入过长的暂缓时间无效
  assert.equal(bannerKind(sec, st({ snooze: JSON.stringify({ key: securityKey(sec), until: now + 10 * SNOOZE_MS }) })), 'security')
  // 记录损坏时照常提示
  assert.equal(bannerKind(sec, st({ snooze: '{bad' })), 'security')
})

test('已是最新版本仍受影响（尚无修复）时也提示', () => {
  assert.equal(bannerKind({ ...sec, hasUpdate: false }, st()), 'security')
})
