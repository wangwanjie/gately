// IP 归属地显示辅助函数的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { countryName, geoParts, lookupKey } from '../src/ipGeo.ts'

test('lookupKey 取出要查询的地址', () => {
  assert.equal(lookupKey('1.2.3.4'), '1.2.3.4')
  assert.equal(lookupKey(' 1.2.3.4:5678 '), '1.2.3.4')
  assert.equal(lookupKey('[2001:db8::1]:443'), '2001:db8::1')
  assert.equal(lookupKey('[2001:db8::1]'), '2001:db8::1')
  assert.equal(lookupKey('2001:db8::1'), '2001:db8::1')
  assert.equal(lookupKey('fe80::1%eth0'), 'fe80::1')
  assert.equal(lookupKey('203.0.113.0/24'), '203.0.113.0/24')
  assert.equal(lookupKey('2001:db8::/64'), '2001:db8::/64')
  // 主机名、空串等不查询
  assert.equal(lookupKey('example.com'), '')
  assert.equal(lookupKey(''), '')
  assert.equal(lookupKey('-'), '')
  assert.equal(lookupKey('1234'), '')
})

test('geoParts 拼出国家、省、城市', () => {
  assert.deepEqual(geoParts(undefined, 'zh-CN', '内网'), [])
  assert.deepEqual(geoParts({ country: 'LAN' }, 'zh-CN', '内网'), ['内网'])
  assert.deepEqual(geoParts({ country: 'CN', region: '广东', city: '广州' }, 'zh-CN', '内网'), ['中国', '广东', '广州'])
  assert.deepEqual(geoParts({ country: 'US' }, 'en', 'LAN'), ['US'])
  // 香港的城市名与地区名相同时只保留一个
  assert.deepEqual(geoParts({ country: 'HK', region: 'Kowloon', city: 'Hong Kong' }, 'en', 'LAN'), ['Hong Kong', 'Kowloon'])
  assert.deepEqual(geoParts({ country: 'HK', city: 'Hong Kong' }, 'en', 'LAN'), ['Hong Kong'])
  assert.deepEqual(geoParts({ country: 'HK', city: '香港' }, 'zh-CN', '内网'), ['香港'])
})

test('countryName 使用短名称，无法识别时返回代码', () => {
  assert.equal(countryName('HK', 'zh-CN'), '香港')
  assert.equal(countryName('CN', 'ja'), '中国')
  assert.equal(countryName('ZZZ', 'en'), 'ZZZ')
})
