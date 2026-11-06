// 终端选择模式文本拼接的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { edgePoint, joinRows } from '../src/components/terminal/selectText.ts'

const rows = ['$ ls', 'a.txt  b.txt', 'long-line-part1', '-part2', '', '$ ']
// 第 3 行（下标 3）是第 2 行的软换行续行
const wrapped = [false, false, false, true, false, false]

test('同一行内取子串', () => {
  assert.equal(joinRows(rows, wrapped, { r: 1, o: 0 }, { r: 1, o: 5 }), 'a.txt')
})

test('跨行时逻辑行之间插入换行，软换行续行直接相连', () => {
  assert.equal(joinRows(rows, wrapped, { r: 1, o: 7 }, { r: 3, o: 6 }), 'b.txt\nlong-line-part1-part2')
})

test('起点收拢到首行开头时包含屏幕外的全部前序行（向上拖过一屏）', () => {
  const s = edgePoint(rows, 'start')
  assert.deepEqual(s, { r: 0, o: 0 })
  assert.equal(joinRows(rows, wrapped, s, { r: 1, o: 5 }), '$ ls\na.txt')
})

test('终点收拢到末行结尾，去掉末尾空行带来的换行', () => {
  const e = edgePoint(rows, 'end')
  assert.deepEqual(e, { r: 5, o: 2 })
  assert.equal(joinRows(rows, wrapped, { r: 3, o: 0 }, { r: 4, o: 0 }), '-part2')
  assert.equal(joinRows(rows, wrapped, { r: 0, o: 0 }, e), '$ ls\na.txt  b.txt\nlong-line-part1-part2\n\n$ ')
})

test('整个选区都在行外（起点终点都收拢到同一端）时为空', () => {
  assert.equal(joinRows(rows, wrapped, edgePoint(rows, 'start'), edgePoint(rows, 'start')), '')
  assert.equal(joinRows(rows, wrapped, edgePoint(rows, 'end'), edgePoint(rows, 'end')), '')
})

test('越界的行号与偏移收拢到边界，起点在终点之后时为空', () => {
  assert.equal(joinRows(rows, wrapped, { r: -3, o: 9 }, { r: 0, o: 99 }), '$ ls')
  assert.equal(joinRows(rows, wrapped, { r: 5, o: 0 }, { r: 99, o: 0 }), '$ ')
  assert.equal(joinRows(rows, wrapped, { r: 2, o: 4 }, { r: 1, o: 0 }), '')
  assert.equal(joinRows([], [], { r: 0, o: 0 }, { r: 0, o: 1 }), '')
})
