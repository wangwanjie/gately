// 监听地址判断的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { listensBeyondLocal } from '../src/netaddr.ts'

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
