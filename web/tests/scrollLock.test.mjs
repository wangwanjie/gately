// 滚动锁放行选择器的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { joinSelectors } from '../src/composables/useScrollLock.ts'

test('单个选择器原样使用，空值表示整页不放行', () => {
  assert.equal(joinSelectors('.log-viewer.full'), '.log-viewer.full')
  assert.equal(joinSelectors(''), '')
  assert.equal(joinSelectors([]), '')
})

test('多个选择器合并为选择器列表，忽略空项', () => {
  assert.equal(joinSelectors(['.log-viewer.full', ' .el-select__popper ', '', '.el-popper']), '.log-viewer.full, .el-select__popper, .el-popper')
})
