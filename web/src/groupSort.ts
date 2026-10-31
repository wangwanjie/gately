// 列表分组与排序的纯函数，所有支持分组排序的列表共用（见 composables/useGroupSort.ts 与 components/GroupedList.vue）。
// 分组只影响管理界面的展示：分组按首次出现的顺序排列，未分组的条目放在最后。
// 本文件不依赖 Vue，可直接用 node --test 测试（web/tests/groupSort.test.mjs）。

// MAX_GROUP_LEN 为分组名称的最大字符数，与后端 crud.MaxGroupLen 一致
export const MAX_GROUP_LEN = 64

export interface Section<T> {
  // name 为分组名称，空字符串为未分组
  name: string
  items: T[]
}

// MoveTarget 描述移动的目的地：beforeId / afterId 指定插入位置，都不提供时放到分组末尾
export interface MoveTarget {
  group: string
  beforeId?: string
  afterId?: string
}

// DropOver 为拖动时的悬停位置：id 为条目或分组标题，after 为 true 表示插到条目之后
export interface DropOver {
  id: string
  after: boolean
}

// SortEntry 为 SortDialog 中的一项：id 唯一标识，label / sub 为显示文字，off 表示已停用（置灰）
export interface SortEntry {
  id: string
  label: string
  sub?: string
  group: string
  off?: boolean
}

// buildSections 把条目按分组归类，保持各分组内的原有顺序
export function buildSections<T>(items: T[], groupOf: (it: T) => string): Section<T>[] {
  const map = new Map<string, T[]>()
  for (const it of items) {
    const g = groupOf(it)
    if (!map.has(g)) {
      map.set(g, [])
    }
    map.get(g)!.push(it)
  }
  const named = [...map.entries()].filter(([g]) => g).map(([name, list]) => ({ name, items: list }))
  const ungrouped = map.get('') ?? []
  return ungrouped.length ? [...named, { name: '', items: ungrouped }] : named
}

// flatten 按分组展示顺序展开为保存用的顺序
export function flatten<T>(sections: Section<T>[]): T[] {
  return sections.flatMap((s) => s.items)
}

// moveInto 把 moving 中的条目按当前显示顺序整体移到目标分组的指定位置，返回新的分组列表。
// 目标分组不存在时新建，放在未分组之前；移空的分组被去掉。插入位置本身是被移动的条目时不做任何改变。
export function moveInto<T>(sections: Section<T>[], idOf: (it: T) => string, moving: Set<string>, target: MoveTarget): Section<T>[] | null {
  if (!moving.size || (target.beforeId && moving.has(target.beforeId)) || (target.afterId && moving.has(target.afterId))) {
    return null
  }
  const picked = flatten(sections).filter((it) => moving.has(idOf(it)))
  if (!picked.length) {
    return null
  }
  const next = sections.map((s) => ({ name: s.name, items: s.items.filter((it) => !moving.has(idOf(it))) }))
  let sec = next.find((s) => s.name === target.group)
  if (!sec) {
    sec = { name: target.group, items: [] }
    const ui = next.findIndex((s) => !s.name)
    if (target.group && ui >= 0) {
      next.splice(ui, 0, sec)
    } else {
      next.push(sec)
    }
  }
  let at = sec.items.length
  if (target.beforeId) {
    const i = sec.items.findIndex((it) => idOf(it) === target.beforeId)
    at = i < 0 ? at : i
  } else if (target.afterId) {
    const i = sec.items.findIndex((it) => idOf(it) === target.afterId)
    at = i < 0 ? at : i + 1
  }
  sec.items.splice(at, 0, ...picked)
  return next.filter((s) => s.items.length)
}

// moveStep 把条目在所在分组内上移（d = -1）或下移（d = 1）一位
export function moveStep<T>(sections: Section<T>[], idOf: (it: T) => string, id: string, d: number): Section<T>[] | null {
  const next = sections.map((s) => ({ name: s.name, items: [...s.items] }))
  for (const s of next) {
    const i = s.items.findIndex((it) => idOf(it) === id)
    if (i < 0) {
      continue
    }
    const j = i + d
    if (j < 0 || j >= s.items.length) {
      return null
    }
    ;[s.items[i], s.items[j]] = [s.items[j], s.items[i]]
    return next
  }
  return null
}

// moveSection 整体移动一个命名分组；未分组始终在最后
export function moveSection<T>(sections: Section<T>[], index: number, d: number): Section<T>[] | null {
  const j = index + d
  if (j < 0 || j >= sections.length || !sections[index]?.name || !sections[j].name) {
    return null
  }
  const next = [...sections]
  ;[next[index], next[j]] = [next[j], next[index]]
  return next
}

// renameSection 把分组 from 改名为 to，位置与组内顺序不变；to 已存在时两个分组的条目合在一起，
// 放在两者中先出现的位置。to 为空表示解散分组，条目并入未分组（放在最后）。名称未变或分组不存在时返回 null
export function renameSection<T>(sections: Section<T>[], from: string, to: string): Section<T>[] | null {
  if (from === to || !sections.some((s) => s.name === from)) {
    return null
  }
  const renamed = sections.map((s) => (s.name === from ? { name: to, items: s.items } : s))
  const merged: Section<T>[] = []
  for (const sec of renamed) {
    const hit = merged.find((m) => m.name === sec.name)
    if (hit) {
      hit.items.push(...sec.items)
    } else {
      merged.push({ name: sec.name, items: [...sec.items] })
    }
  }
  // 未分组始终在最后
  const ui = merged.findIndex((s) => !s.name)
  if (ui >= 0 && ui < merged.length - 1) {
    merged.push(...merged.splice(ui, 1))
  }
  return merged
}

// groupChanges 比较新的分组列表与条目原有的分组，返回 目标分组 → 需要改到该分组的条目 id
export function groupChanges<T>(sections: Section<T>[], idOf: (it: T) => string, groupOf: (it: T) => string): Map<string, string[]> {
  const changes = new Map<string, string[]>()
  for (const s of sections) {
    for (const it of s.items) {
      if (groupOf(it) !== s.name) {
        changes.set(s.name, [...(changes.get(s.name) ?? []), idOf(it)])
      }
    }
  }
  return changes
}

// groupSuggestions 生成分组输入框的候选项：已有分组名称中包含输入内容的
export function groupSuggestions(names: Iterable<string>, query: string) {
  const q = query.trim().toLowerCase()
  const set = new Set<string>()
  for (const n of names) {
    if (n && (!q || n.toLowerCase().includes(q))) {
      set.add(n)
    }
  }
  return [...set].map((value) => ({ value }))
}

// GroupItem 是分组列表中依次显示的一项：分组标题或条目
export type GroupItem<T> =
  | {
      kind: 'group'
      id: string
      name: string
      count: number
      collapsed: boolean
      index: number
      canUp: boolean
      canDown: boolean
      // 排序模式下分组标题的勾选状态：全部选中或部分选中
      checked: boolean
      indeterminate: boolean
      ids: string[]
    }
  | { kind: 'item'; id: string; item: T; grouped: boolean; canUp: boolean; canDown: boolean }

// displayItems 生成依次显示的分组标题与条目。没有任何命名分组时不显示标题；筛选与排序时展开全部分组。
// 排序模式下不做任何筛选，保证调整的是全部条目的顺序
export function displayItems<T>(
  sections: Section<T>[],
  idOf: (it: T) => string,
  opts: {
    query: string
    sorting: boolean
    matches: (it: T, q: string) => boolean
    isCollapsed: (name: string) => boolean
    isSelected: (id: string) => boolean
    // visible 为搜索之外的额外筛选（如按实例筛选），返回 false 的条目不显示
    visible?: (it: T) => boolean
    // groupIdPrefix 加在分组标题 id 前，同一页面有多个分组列表时区分各列表的分组
    groupIdPrefix?: string
  },
): GroupItem<T>[] {
  const q = opts.sorting ? '' : opts.query.trim().toLowerCase()
  const visible = opts.sorting ? undefined : opts.visible
  const filtering = !!q || !!visible
  const grouped = sections.some((s) => s.name)
  const namedCount = sections.filter((s) => s.name).length
  const out: GroupItem<T>[] = []
  sections.forEach((sec, si) => {
    const rows = filtering ? sec.items.filter((it) => (!visible || visible(it)) && (!q || opts.matches(it, q))) : sec.items
    if (!rows.length) {
      return
    }
    const collapsed = grouped && !q && !opts.sorting && opts.isCollapsed(sec.name)
    if (grouped) {
      const ids = rows.map(idOf)
      const n = ids.filter((id) => opts.isSelected(id)).length
      out.push({
        kind: 'group',
        id: `${opts.groupIdPrefix ?? ''}g:${sec.name}`,
        name: sec.name,
        count: rows.length,
        collapsed,
        index: si,
        canUp: !!sec.name && si > 0,
        canDown: !!sec.name && si < namedCount - 1,
        checked: n > 0 && n === ids.length,
        indeterminate: n > 0 && n < ids.length,
        ids,
      })
    }
    if (collapsed) {
      return
    }
    rows.forEach((it, ri) => out.push({ kind: 'item', id: idOf(it), item: it, grouped, canUp: ri > 0, canDown: ri < rows.length - 1 }))
  })
  return out
}

// dropTarget 把悬停位置换算为移动目标：放在分组标题上时移到该分组末尾
export function dropTarget<T>(sections: Section<T>[], idOf: (it: T) => string, over: DropOver, groupIdPrefix = ''): MoveTarget | null {
  const g = `${groupIdPrefix}g:`
  if (over.id.startsWith(g)) {
    return { group: over.id.slice(g.length) }
  }
  const sec = sections.find((s) => s.items.some((it) => idOf(it) === over.id))
  if (!sec) {
    return null
  }
  return over.after ? { group: sec.name, afterId: over.id } : { group: sec.name, beforeId: over.id }
}
