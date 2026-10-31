// 分组排序纯函数的单元测试：node --test tests/（Node 22.18+ 可直接加载 .ts）
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  buildSections,
  displayItems,
  dropTarget,
  flatten,
  groupChanges,
  groupSuggestions,
  moveInto,
  moveSection,
  moveStep,
  renameSection,
} from '../src/groupSort.ts'

const idOf = (it) => it.key
const groupOf = (it) => it.group ?? ''
const items = () => [
  { key: 'a', group: '家里' },
  { key: 'b', group: '' },
  { key: 'c', group: '公司' },
  { key: 'd', group: '家里' },
  { key: 'e' },
]
// layout 把分组列表转为便于比较的 "分组:id,id" 形式
const layout = (secs) => secs.map((s) => `${s.name}:${s.items.map(idOf).join(',')}`)

test('buildSections 按首次出现排列分组，未分组在最后', () => {
  assert.deepEqual(layout(buildSections(items(), groupOf)), ['家里:a,d', '公司:c', ':b,e'])
  assert.deepEqual(layout(buildSections([{ key: 'x' }], groupOf)), [':x'])
  assert.deepEqual(buildSections([], groupOf), [])
})

test('flatten 按显示顺序展开', () => {
  assert.deepEqual(flatten(buildSections(items(), groupOf)).map(idOf), ['a', 'd', 'c', 'b', 'e'])
})

test('moveStep 只在分组内移动，越界返回 null', () => {
  const secs = buildSections(items(), groupOf)
  assert.deepEqual(layout(moveStep(secs, idOf, 'd', -1)), ['家里:d,a', '公司:c', ':b,e'])
  assert.equal(moveStep(secs, idOf, 'a', -1), null)
  assert.equal(moveStep(secs, idOf, 'e', 1), null)
  assert.equal(moveStep(secs, idOf, 'zz', 1), null)
  // 原分组列表不被修改
  assert.deepEqual(layout(secs), ['家里:a,d', '公司:c', ':b,e'])
})

test('moveSection 只移动命名分组，未分组固定在最后', () => {
  const secs = buildSections(items(), groupOf)
  assert.deepEqual(layout(moveSection(secs, 1, -1)), ['公司:c', '家里:a,d', ':b,e'])
  assert.equal(moveSection(secs, 1, 1), null)
  assert.equal(moveSection(secs, 0, -1), null)
  assert.equal(moveSection(secs, 2, -1), null)
})

test('moveInto 多选按显示顺序移动，可插入到指定位置并新建分组', () => {
  const secs = buildSections(items(), groupOf)
  // 拖到另一分组的条目之前
  assert.deepEqual(layout(moveInto(secs, idOf, new Set(['e', 'a']), { group: '公司', beforeId: 'c' })), ['家里:d', '公司:a,e,c', ':b'])
  // 拖到条目之后
  assert.deepEqual(layout(moveInto(secs, idOf, new Set(['b']), { group: '家里', afterId: 'a' })), ['家里:a,b,d', '公司:c', ':e'])
  // 新分组放在未分组之前
  assert.deepEqual(layout(moveInto(secs, idOf, new Set(['b']), { group: '新' })), ['家里:a,d', '公司:c', '新:b', ':e'])
  // 移出分组放到末尾，移空的分组去掉
  assert.deepEqual(layout(moveInto(secs, idOf, new Set(['c']), { group: '' })), ['家里:a,d', ':b,e,c'])
  // 放到被移动的条目自身上不做改变
  assert.equal(moveInto(secs, idOf, new Set(['a', 'd']), { group: '家里', beforeId: 'd' }), null)
  assert.equal(moveInto(secs, idOf, new Set(), { group: '家里' }), null)
  assert.equal(moveInto(secs, idOf, new Set(['zz']), { group: '家里' }), null)
})

test('renameSection 原位改名，同名时合并到先出现的位置', () => {
  const secs = buildSections(items(), groupOf)
  assert.deepEqual(layout(renameSection(secs, '公司', '办公室')), ['家里:a,d', '办公室:c', ':b,e'])
  assert.deepEqual(layout(renameSection(secs, '公司', '家里')), ['家里:a,d,c', ':b,e'])
  assert.deepEqual(layout(renameSection(secs, '家里', '公司')), ['公司:a,d,c', ':b,e'])
  // 改为空名称即解散分组，未分组仍在最后
  assert.deepEqual(layout(renameSection(secs, '家里', '')), ['公司:c', ':a,d,b,e'])
  assert.equal(renameSection(secs, '家里', '家里'), null)
  assert.equal(renameSection(secs, '不存在', '新'), null)
})

test('groupChanges 只列出分组变化的条目', () => {
  const secs = buildSections(items(), groupOf)
  assert.equal(groupChanges(secs, idOf, groupOf).size, 0)
  const next = renameSection(secs, '公司', '家里')
  assert.deepEqual([...groupChanges(next, idOf, groupOf)], [['家里', ['c']]])
  const moved = moveInto(secs, idOf, new Set(['a', 'b']), { group: '新' })
  assert.deepEqual([...groupChanges(moved, idOf, groupOf)], [['新', ['a', 'b']]])
})

test('displayItems 生成分组标题与条目，处理收起、筛选与排序模式', () => {
  const secs = buildSections(items(), groupOf)
  const base = { query: '', sorting: false, matches: (it, q) => it.key.includes(q), isCollapsed: () => false, isSelected: () => false }
  const all = displayItems(secs, idOf, base)
  assert.deepEqual(
    all.map((x) => (x.kind === 'group' ? `#${x.name}` : x.id)),
    ['#家里', 'a', 'd', '#公司', 'c', '#', 'b', 'e'],
  )
  const head = all[0]
  assert.equal(head.canUp, false)
  assert.equal(head.canDown, true)
  // 未分组的标题不能移动
  assert.equal(all[5].canUp, false)
  assert.equal(all[5].canDown, false)
  // 组内第一项不能上移、最后一项不能下移
  assert.equal(all[1].canUp, false)
  assert.equal(all[2].canDown, false)

  // 收起的分组只显示标题；搜索时展开
  const folded = displayItems(secs, idOf, { ...base, isCollapsed: (n) => n === '家里' })
  assert.deepEqual(folded.map((x) => x.id), ['g:家里', 'g:公司', 'c', 'g:', 'b', 'e'])
  const searched = displayItems(secs, idOf, { ...base, query: 'D', isCollapsed: (n) => n === '家里' })
  assert.deepEqual(searched.map((x) => x.id), ['g:家里', 'd'])

  // 额外筛选在排序模式下不生效
  const visible = (it) => it.key !== 'c'
  assert.deepEqual(displayItems(secs, idOf, { ...base, visible }).filter((x) => x.kind === 'item').map((x) => x.id), ['a', 'd', 'b', 'e'])
  assert.equal(displayItems(secs, idOf, { ...base, visible, sorting: true, query: 'a' }).filter((x) => x.kind === 'item').length, 5)

  // 勾选状态
  const sel = displayItems(secs, idOf, { ...base, isSelected: (id) => id === 'a' })
  assert.equal(sel[0].indeterminate, true)
  assert.equal(sel[0].checked, false)

  // 没有命名分组时不显示标题
  const flat = displayItems(buildSections([{ key: 'x' }, { key: 'y' }], groupOf), idOf, base)
  assert.deepEqual(flat.map((x) => x.kind), ['item', 'item'])
  assert.equal(flat[0].grouped, false)
})

test('dropTarget 换算放置位置', () => {
  const secs = buildSections(items(), groupOf)
  assert.deepEqual(dropTarget(secs, idOf, { id: 'g:公司', after: false }), { group: '公司' })
  assert.deepEqual(dropTarget(secs, idOf, { id: 'c', after: true }), { group: '公司', afterId: 'c' })
  assert.deepEqual(dropTarget(secs, idOf, { id: 'b', after: false }), { group: '', beforeId: 'b' })
  assert.deepEqual(dropTarget(secs, idOf, { id: 'l1/g:x', after: false }, 'l1/'), { group: 'x' })
  assert.equal(dropTarget(secs, idOf, { id: 'zz', after: false }), null)
})

test('groupSuggestions 去重并按输入筛选', () => {
  assert.deepEqual(groupSuggestions(['家里', '', '公司', '家里'], ''), [{ value: '家里' }, { value: '公司' }])
  assert.deepEqual(groupSuggestions(['Home', 'Office'], 'off'), [{ value: 'Office' }])
})
