// 错误页工具函数的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { checkCodes, decodeUTF8, formatSize } from '../src/errorPageUtil.ts'

test('拦截状态码：合法写法通过', () => {
  for (const s of ['404', '404, 500-599', '5xx', '4XX，502；503、504', ' 500-599\n404 ']) {
    assert.equal(checkCodes(s), null, s)
  }
})

test('拦截状态码：空与非法写法给出原因', () => {
  assert.deepEqual(checkCodes('  , '), { empty: true })
  for (const [s, bad] of [
    ['399', '399'],
    ['404, 600', '600'],
    ['500-450', '500-450'],
    ['3xx', '3xx'],
    ['abc', 'abc'],
    ['404-', '404-'],
    ['200', '200'],
  ]) {
    assert.deepEqual(checkCodes(s), { bad }, s)
  }
})

test('上传文件严格按 UTF-8 解码', () => {
  assert.equal(decodeUTF8(new TextEncoder().encode('<h1>出错了</h1>')), '<h1>出错了</h1>')
  assert.equal(decodeUTF8(new Uint8Array([0x3c, 0x68, 0x31, 0x3e, 0xb4, 0xed, 0xce, 0xf3])), null)
})

test('大小格式化', () => {
  assert.equal(formatSize(512), '512 B')
  assert.equal(formatSize(1536), '1.5 KB')
  assert.equal(formatSize(256 * 1024), '256 KB')
})
