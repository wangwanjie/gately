// 监听地址判断的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { forwardsBeyondLocal, listensBeyondLocal, maskProxyPassword } from '../src/netaddr.ts'

test('listensBeyondLocal 本机地址与留空不提醒', () => {
  for (const v of ['', '  ', '127.0.0.1:1080', '127.1.2.3:1080', 'localhost:1055', 'LOCALHOST:1', '[::1]:1080']) {
    assert.equal(listensBeyondLocal(v), false, v)
  }
})

test('listensBeyondLocal 全部地址与局域网地址提醒', () => {
  for (const v of [':1080', '0.0.0.0:1080', '[::]:1080', '192.168.1.2:1080', 'nas.lan:1080', '[fd00::1]:1080', '1080']) {
    assert.equal(listensBeyondLocal(v), true, v)
  }
})

test('maskProxyPassword 隐藏代理密码', () => {
  assert.equal(maskProxyPassword('http://u:pw@proxy:3128'), 'http://u:***@proxy:3128')
  assert.equal(maskProxyPassword('SOCKS5H://u:pw@proxy:1080'), 'SOCKS5H://u:***@proxy:1080')
  // 只有用户名、没有用户信息、不是 URL 时原样返回
  assert.equal(maskProxyPassword('socks5://u@proxy:1080'), 'socks5://u@proxy:1080')
  assert.equal(maskProxyPassword('http://proxy:3128'), 'http://proxy:3128')
  assert.equal(maskProxyPassword('proxy:3128'), 'proxy:3128')
  // 密码含未编码的 @、/、: 时整段隐藏，不露出一部分
  assert.equal(maskProxyPassword('http://u:p@ss@proxy:3128'), 'http://u:***@proxy:3128')
  assert.equal(maskProxyPassword('http://u:a/b:c@proxy:3128'), 'http://u:***@proxy:3128')
  // 空密码同样替换，不暴露是否设置过密码
  assert.equal(maskProxyPassword('http://u:@proxy:3128'), 'http://u:***@proxy:3128')
})

test('forwardsBeyondLocal 只在绑定到本机以外时提醒', () => {
  assert.equal(forwardsBeyondLocal(''), false)
  assert.equal(forwardsBeyondLocal('# 注释\n\ntcp://127.0.0.1:2222/10.126.126.1:22\nudp://[::1]:53/10.126.126.1:53'), false)
  assert.equal(forwardsBeyondLocal('tcp://localhost:2222/10.126.126.1:22'), false)
  // 目标地址是组网内的 IP，不影响判断
  assert.equal(forwardsBeyondLocal('tcp://127.0.0.1:2222/192.168.1.5:22'), false)
  assert.equal(forwardsBeyondLocal('tcp://0.0.0.0:2222/10.126.126.1:22'), true)
  assert.equal(forwardsBeyondLocal('tcp://127.0.0.1:1/10.0.0.1:1\ntcp://192.168.1.2:2222/10.126.126.1:22'), true)
  assert.equal(forwardsBeyondLocal('udp://[::]:53/10.126.126.1:53'), true)
})
