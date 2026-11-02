// 副本名称与草稿的单元测试：node --test tests/
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { asciiCopyName, cloneItem, copyBase, copyName } from '../src/copyName.ts'

const zh = (name, n) => (n <= 1 ? `${name} 副本` : `${name} 副本 ${n}`)
const en = (name, n) => (n <= 1 ? `${name} (copy)` : `${name} (copy ${n})`)

test('首个副本加后缀，重复时递增序号', () => {
  assert.equal(copyName('Web', [], zh), 'Web 副本')
  assert.equal(copyName('Web', ['Web', 'Web 副本'], zh), 'Web 副本 2')
  assert.equal(copyName('Web', ['Web 副本', 'Web 副本 2', 'Web 副本 3'], zh), 'Web 副本 4')
  // 已有名称按去空白后比较
  assert.equal(copyName('  Web ', [' Web 副本 '], zh), 'Web 副本 2')
})

test('复制副本时去掉已有后缀，不叠加', () => {
  assert.equal(copyName('Web 副本', ['Web', 'Web 副本'], zh), 'Web 副本 2')
  assert.equal(copyName('Web 副本 2', ['Web', 'Web 副本', 'Web 副本 2'], zh), 'Web 副本 3')
  assert.equal(copyName('NAS (copy 3)', ['NAS (copy)'], en), 'NAS (copy 2)')
  assert.equal(copyName('svc-copy', ['svc', 'svc-copy'], asciiCopyName), 'svc-copy2')
  assert.equal(copyName('svc-copy12', [], asciiCopyName), 'svc-copy')
})

test('名称恰好等于后缀或包含正则特殊字符时保持原样', () => {
  assert.equal(copyBase(' 副本', zh), ' 副本')
  assert.equal(copyBase('副本', zh), '副本')
  assert.equal(copyBase('a.b (copy)', en), 'a.b')
  assert.equal(copyBase('a.b (copyX)', en), 'a.b (copyX)')
  assert.equal(copyName('[a]+(b)', [], en), '[a]+(b) (copy)')
})

test('模板没有名称时返回空串', () => {
  assert.equal(copyName('', ['x'], zh), '')
  assert.equal(copyName(undefined, [], zh), '')
  assert.equal(copyName('   ', [], zh), '')
})

test('cloneItem 深拷贝并去掉 key', () => {
  const src = { key: 'k1', name: 'a', list: [{ v: 1 }], nested: { x: [1, 2] } }
  const d = cloneItem(src)
  assert.equal(d.key, '')
  d.list[0].v = 2
  d.nested.x.push(3)
  assert.deepEqual(src, { key: 'k1', name: 'a', list: [{ v: 1 }], nested: { x: [1, 2] } })
})
