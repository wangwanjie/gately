// DDNS 批量添加解析的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { parseBatch, splitDomain } from '../src/ddnsBatch.ts'

const base = { zone: '', types: ['A'], knownZones: [], existing: [], line: '' }
const brief = (res) => res.records.map((r) => `${r.type} ${r.sub}.${r.zone} ${r.value}`)

test('完整域名按已知主域名拆分，取最长匹配', () => {
  assert.deepEqual(splitDomain('a.b.example.com', '', ['example.com', 'b.example.com']), { sub: 'a', zone: 'b.example.com' })
  assert.deepEqual(splitDomain('example.com', 'example.com', []), { sub: '@', zone: 'example.com' })
  assert.deepEqual(splitDomain('*.example.com', 'example.com', []), { sub: '*', zone: 'example.com' })
})

test('没有已知主域名时按公共后缀猜测', () => {
  assert.deepEqual(splitDomain('nas.home.example.com', '', []), { sub: 'nas.home', zone: 'example.com' })
  assert.deepEqual(splitDomain('nas.example.com.cn', '', []), { sub: 'nas', zone: 'example.com.cn' })
  assert.deepEqual(splitDomain('example.co.uk', '', []), { sub: '@', zone: 'example.co.uk' })
})

test('不带点的主机记录挂在默认主域名下，没有默认主域名时报错', () => {
  assert.deepEqual(splitDomain('nas', 'Example.COM.', []), { sub: 'nas', zone: 'example.com' })
  assert.equal(splitDomain('nas', '', []), null)
  const res = parseBatch('nas', base)
  assert.deepEqual(res.errors.map((e) => e.reason), ['zone'])
})

test('每行一个域名，按勾选类型生成，忽略空行与注释', () => {
  const res = parseBatch('# 注释\n\nwww.example.com\r\nhttps://NAS.example.com:8443/path\nemby.\n', {
    ...base,
    zone: 'example.com',
    types: ['A', 'AAAA'],
  })
  assert.deepEqual(brief(res), [
    'A www.example.com {ipv4Addr}',
    'AAAA www.example.com {ipv6Addr}',
    'A nas.example.com {ipv4Addr}',
    'AAAA nas.example.com {ipv6Addr}',
    'A emby.example.com {ipv4Addr}',
    'AAAA emby.example.com {ipv6Addr}',
  ])
  assert.equal(res.errors.length, 0)
})

test('行内可写类型与记录值，TXT 值可含空格，地址可省略类型', () => {
  const res = parseBatch(
    'v6.example.com aaaa\ncdn.example.com CNAME target.example.net\n_ssh.example.com TXT port={STUN_ssh_PORT} x\nfix.example.com 1.2.3.4\nfix6.example.com 2001:db8::1',
    base,
  )
  assert.deepEqual(brief(res), [
    'AAAA v6.example.com {ipv6Addr}',
    'CNAME cdn.example.com target.example.net',
    'TXT _ssh.example.com port={STUN_ssh_PORT} x',
    'A fix.example.com 1.2.3.4',
    'AAAA fix6.example.com 2001:db8::1',
  ])
})

test('无效行给出行号与原因', () => {
  const res = parseBatch('bad_domain!.com\nwww.example.com MX\ncdn.example.com CNAME\nip.example.com A 300.1.1.1\nip6.example.com AAAA 1.2.3.4\nnas', base)
  assert.deepEqual(
    res.errors.map((e) => [e.line, e.reason]),
    [
      [1, 'domain'],
      [2, 'type'],
      [3, 'value'],
      [4, 'address'],
      [5, 'address'],
      [6, 'zone'],
    ],
  )
  assert.equal(res.records.length, 0)
})

test('与已有记录或前面的行重复时跳过，线路不同不算重复', () => {
  const existing = [{ sub: '@', zone: 'example.com', type: 'A', line: '' }, { sub: 'www', zone: 'example.com', type: 'A', line: '电信' }]
  const res = parseBatch('example.com\nWWW.example.com\nwww.example.com\nnew.example.com', {
    ...base,
    types: ['A', 'AAAA'],
    existing,
  })
  assert.deepEqual(brief(res), [
    'AAAA @.example.com {ipv6Addr}',
    'A www.example.com {ipv4Addr}',
    'AAAA www.example.com {ipv6Addr}',
    'A new.example.com {ipv4Addr}',
    'AAAA new.example.com {ipv6Addr}',
  ])
  assert.deepEqual(res.duplicates, [{ line: 3, text: 'www.example.com' }])
  // 同一线路下才算重复
  const same = parseBatch('www.example.com', { ...base, existing, line: '电信' })
  assert.equal(same.records.length, 0)
  assert.equal(same.duplicates.length, 1)
})
