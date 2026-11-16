// 总览服务状态的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { firstAttention, serviceLevel } from '../src/overviewStatus.ts'

test('serviceLevel 取最严重的等级', () => {
  assert.equal(serviceLevel([{ label: '启用的证书', value: '2' }, { label: '最近到期', value: 'x 剩余 6 天', level: 'error' }]), 'error')
  assert.equal(serviceLevel([{ label: '封禁中', value: '0' }, { label: '防火墙', value: '未生效', level: 'warn' }]), 'warn')
  assert.equal(serviceLevel([{ label: '运行中的监听', value: '2 / 2', level: 'ok' }, { label: '站点', value: '4' }]), 'ok')
})

test('serviceLevel 没有配置或未启用时为 idle', () => {
  assert.equal(serviceLevel([{ label: '运行中', value: '0 / 0', level: 'ok' }, { label: '活动连接', value: '0' }]), 'idle')
  assert.equal(serviceLevel([{ label: '状态', value: '未启用', level: 'info' }]), 'idle')
  assert.equal(serviceLevel([{ label: '实例', value: '0' }, { label: '已拦截', value: '0', level: 'info' }]), 'idle')
  // 已配置但暂时没有连接仍算正常
  assert.equal(serviceLevel([{ label: '运行中', value: '1 / 1', level: 'ok' }, { label: '活动连接', value: '0' }]), 'ok')
})

test('firstAttention 先错误后警告', () => {
  assert.equal(firstAttention(['ok', 'warn', 'error']), 2)
  assert.equal(firstAttention(['ok', 'warn', 'idle']), 1)
  assert.equal(firstAttention(['ok', 'idle']), -1)
})
