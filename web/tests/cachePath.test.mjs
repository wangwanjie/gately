// 缓存规则路径前端校验的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cachePathError } from '../src/cachePath.ts'

test('合法路径与未填写的路径不报错', () => {
  for (const p of ['', '  ', '/', '/api', '/api/products/*', '/static/*.js', '/中文/*', ' /a ']) {
    assert.equal(cachePathError(p), '', p)
  }
})

test('含 % 的路径提示填写解码后的路径', () => {
  for (const p of ['/a%2Fb', '/%E4%B8%AD', '/a%']) {
    assert.equal(cachePathError(p), 'percent', p)
  }
})

test('格式与路径段错误与后端一致', () => {
  for (const p of ['api', '/a b', '/a?x=1', '/a#x']) {
    assert.equal(cachePathError(p), 'format', p)
  }
  for (const p of ['/a//b', '/a/../b', '/a/.', '/./a']) {
    assert.equal(cachePathError(p), 'segment', p)
  }
})
