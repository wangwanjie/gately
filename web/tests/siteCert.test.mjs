// HTTPS 站点证书状态的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { certRequestDomains, siteCertState } from '../src/siteCert.ts'

const now = Date.parse('2026-11-16T00:00:00Z')
const inDays = (d) => new Date(now + d * 86400000 + 3600000).toISOString()

test('siteCertState 没有可检查域名时为 null', () => {
  assert.equal(siteCertState(undefined, now), null)
  assert.equal(siteCertState([], now), null)
})

test('siteCertState 全部覆盖时取最早到期', () => {
  const st = siteCertState([{ domain: 'a', name: 'a', notAfter: inDays(80) }, { domain: 'b', name: '*.x', notAfter: inDays(30) }], now)
  assert.equal(st.kind, 'ok')
  assert.equal(st.days, 30)
  assert.deepEqual(st.names, ['a', '*.x'])
})

test('siteCertState 临近到期与已过期', () => {
  assert.equal(siteCertState([{ domain: 'a', name: 'a', notAfter: inDays(5) }], now).kind, 'expiring')
  const ex = siteCertState([{ domain: 'a', name: 'a', notAfter: new Date(now - 86400000 * 2).toISOString() }], now)
  assert.equal(ex.kind, 'expired')
  assert.ok(ex.days < 0)
})

test('siteCertState 缺证书优先，并给出申请域名', () => {
  const st = siteCertState([{ domain: 'a', name: 'a', notAfter: inDays(5) }, { domain: 'b.example.com' }, { domain: '*.example.org' }], now)
  assert.equal(st.kind, 'missing')
  assert.deepEqual(st.missing, ['b.example.com', '*.example.org'])
  assert.deepEqual(certRequestDomains(st), ['b.example.com', '*.example.org'])
})
