// 端口转发与动态域名空状态场景模板的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import { DDNS_TEMPLATES, PORTFORWARD_TEMPLATES } from '../src/emptyTemplates.ts'

test('端口转发模板：协议与端口有效', () => {
  const protocols = ['tcp', 'udp', 'tcp4', 'udp4', 'tcp6', 'udp6']
  for (const tp of PORTFORWARD_TEMPLATES) {
    assert.ok(tp.protocols.length && tp.protocols.every((p) => protocols.includes(p)), tp.id)
    for (const p of [tp.listenPorts, tp.targetPorts]) {
      assert.ok(Number(p) > 0 && Number(p) < 65536, tp.id)
    }
  }
})

test('动态域名模板：服务商存在，记录类型与 IPv4 / IPv6 开关一致', () => {
  // 服务商名称取自后端 DNS 服务商实现
  const dir = new URL('../../internal/dnsprovider/', import.meta.url)
  const src = readdirSync(dir).filter((f) => f.endsWith('.go')).map((f) => readFileSync(new URL(f, dir), 'utf8')).join('\n')
  for (const tp of DDNS_TEMPLATES) {
    assert.ok(src.includes(`"${tp.provider}"`), tp.provider)
    assert.equal(tp.types.includes('A'), tp.ipv4, tp.id)
    assert.equal(tp.types.includes('AAAA'), tp.ipv6, tp.id)
  }
})
