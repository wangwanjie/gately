// 目录与配置目录关系的前端判断：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { configDirRelation } from '../src/configDir.ts'

const conf = '/vol1/1000/docker/gately'

test('就是配置目录或位于其中', () => {
  for (const p of [conf, conf + '/', conf + '/history', '/vol1/1000/media/../docker/gately', '/vol1//1000/./docker/gately/x']) {
    assert.equal(configDirRelation(p, conf, false), 'inside', p)
  }
})

test('包含配置目录', () => {
  for (const p of ['/', '/vol1', '/vol1/', '/vol1/1000', '/vol1/1000/docker', ' /vol1 ']) {
    assert.equal(configDirRelation(p, conf, false), 'contains', p)
  }
})

test('无关目录、前缀相同的目录与未填完的输入不提示', () => {
  for (const p of ['', '/vol1/1000/media', '/vol1/1000/docker/gately-old', '/vol2', 'vol1', 'relative/path']) {
    assert.equal(configDirRelation(p, conf, false), '', p)
  }
  assert.equal(configDirRelation('/vol1', '', false), '')
})

test('大小写：只在不区分大小写的系统上视为同一目录', () => {
  assert.equal(configDirRelation('/VOL1/1000/Docker/GATELY', conf, true), 'inside')
  assert.equal(configDirRelation('/VOL1', conf, true), 'contains')
  assert.equal(configDirRelation('/VOL1', conf, false), '')
})

test('Windows 路径', () => {
  const win = 'C:\\ProgramData\\Gately\\data'
  assert.equal(configDirRelation('c:\\programdata', win, true), 'contains')
  assert.equal(configDirRelation('C:/ProgramData/Gately/data/x', win, true), 'inside')
  assert.equal(configDirRelation('D:\\', win, true), '')
})
