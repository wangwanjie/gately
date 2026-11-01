// 终端粘贴文本过滤的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { sanitizePaste } from '../src/components/terminal/pasteText.ts'

test('去掉内嵌的括号粘贴结束序列，防止提前结束括号粘贴', () => {
  assert.equal(sanitizePaste('echo hi\x1b[201~\nrm -rf ~\n'), 'echo hi[201~\nrm -rf ~\n')
  assert.ok(!sanitizePaste('\x1b[200~x\x1b[201~').includes('\x1b'))
})

test('去掉 C0 控制字符与 DEL，保留 Tab、回车与换行', () => {
  assert.equal(sanitizePaste('a\x0fb\x03c\x04d\x7fe'), 'abcde')
  assert.equal(sanitizePaste('a\tb\r\nc\nd\re'), 'a\tb\r\nc\nd\re')
})

test('普通文本与多字节字符原样保留', () => {
  const s = 'ls -la /tmp && echo 中文 ✓  '
  assert.equal(sanitizePaste(s), s)
})
