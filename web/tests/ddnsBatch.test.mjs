// DDNS 批量添加解析的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { isIPv6, parseBatch, splitDomain } from '../src/ddnsBatch.ts'

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

test('AAAA 记录值严格校验，与后端 netip.ParseAddr 一致', () => {
  const res = parseBatch('a 1:2\nb :::\nc AAAA fe80::1%eth0\nd 2001:db8::1\ne AAAA ::ffff:1.2.3.4', { ...base, zone: 'example.com' })
  assert.deepEqual(res.errors.map((e) => `${e.line} ${e.reason}`), ['1 address', '2 address', '3 address'])
  assert.deepEqual(brief(res), ['AAAA d.example.com 2001:db8::1', 'AAAA e.example.com ::ffff:1.2.3.4'])
  assert.equal(isIPv6('2001:db8::1'), true)
  assert.equal(isIPv6('1.2.3.4'), false)
})

test('已有记录的主机记录两端带空白时，与批量输入的同名记录视为重复', () => {
  const res = parseBatch('www', { ...base, zone: 'example.com', existing: [{ sub: ' WWW ', zone: 'Example.com', type: 'a', line: '' }] })
  assert.deepEqual(res.records, [])
  assert.deepEqual(res.duplicates.map((d) => d.line), [1])
})

test('同一行生成 A 与 AAAA 时只跳过已存在的那种，不计入重复', () => {
  const res = parseBatch('www', { ...base, zone: 'example.com', types: ['A', 'AAAA'], existing: [{ sub: 'www', zone: 'example.com', type: 'A', line: '' }] })
  assert.deepEqual(brief(res), ['AAAA www.example.com {ipv6Addr}'])
  assert.deepEqual(res.duplicates, [])
})

test('一种类型都没勾选时，没写类型的行提示勾选类型', () => {
  const res = parseBatch('www.example.com\napi.example.com CNAME www.example.com', { ...base, types: [] })
  assert.deepEqual(res.errors.map((e) => `${e.line} ${e.reason}`), ['1 types'])
  assert.deepEqual(brief(res), ['CNAME api.example.com www.example.com'])
})

test('只有公共后缀时不能作为主域名', () => {
  assert.equal(splitDomain('com.cn', '', []), null)
  const res = parseBatch('com.cn\nco.uk', base)
  assert.deepEqual(res.errors.map((e) => e.reason), ['domain', 'domain'])
})
