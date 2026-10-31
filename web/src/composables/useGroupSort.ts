// 列表分组、排序模式、多选与拖动的状态，所有支持分组排序的列表共用。
// 页面一般只需调用 useGroupedList，再把返回值交给 components/GroupedList.vue 渲染；纯函数在 ../groupSort.ts。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '../api'
import {
  MAX_GROUP_LEN,
  buildSections,
  displayItems,
  dropTarget,
  flatten,
  groupChanges,
  moveInto,
  moveSection,
  moveStep,
  renameSection,
  type DropOver,
  type Section,
} from '../groupSort'
import { useCollapsed } from './useCollapsed'

// 兼容旧的导入位置
export { displayItems, dropTarget, type DropOver, type GroupItem } from '../groupSort'

// useSortSelect 管理排序模式、选中项与拖动状态。选中多项后拖动其中任一项，所有选中项一起移动。
export function useSortSelect() {
  const sortMode = ref(false)
  const selected = ref(new Set<string>())
  // dragIds 为正在拖动的条目（按显示顺序），over 为当前悬停的放置位置
  const dragIds = ref<string[]>([])
  const over = ref<DropOver | null>(null)

  function isSelected(id: string) {
    return selected.value.has(id)
  }

  function setSelected(ids: string[], v: boolean) {
    const next = new Set(selected.value)
    for (const id of ids) {
      if (v) {
        next.add(id)
      } else {
        next.delete(id)
      }
    }
    selected.value = next
  }

  function clearSelection() {
    selected.value = new Set()
  }

  function resetDrag() {
    dragIds.value = []
    over.value = null
  }

  function toggleSortMode() {
    sortMode.value = !sortMode.value
    clearSelection()
    resetDrag()
  }

  // dragStart 开始拖动：拖动的是选中项时带上同一范围内全部选中项，ordered 为该范围内按显示顺序排列的条目
  function dragStart(id: string, ordered: string[], e: DragEvent) {
    dragIds.value = isSelected(id) ? ordered.filter((x) => isSelected(x)) : [id]
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData('text/plain', id)
      if (dragIds.value.length > 1) {
        setCountImage(e.dataTransfer, dragIds.value.length)
      }
    }
  }

  // dragOver 记录悬停位置：鼠标在条目下半部分时插到它之后；allowAfter 为 false 时（分组标题）只记录目标
  function dragOver(id: string, e: DragEvent, allowAfter = true) {
    if (!dragIds.value.length) {
      return
    }
    let after = false
    if (allowAfter) {
      const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
      after = e.clientY > rect.top + rect.height / 2
    }
    if (over.value?.id !== id || over.value.after !== after) {
      over.value = { id, after }
    }
  }

  function dragLeave(id: string) {
    if (over.value?.id === id) {
      over.value = null
    }
  }

  // takeDrop 取出本次拖动的条目与放置位置并清空拖动状态；放到被拖动的条目自身上时返回 null
  function takeDrop(): { ids: Set<string>; over: DropOver } | null {
    const ids = new Set(dragIds.value)
    const o = over.value
    resetDrag()
    if (!ids.size || !o || ids.has(o.id)) {
      return null
    }
    return { ids, over: o }
  }

  // dropClass 返回条目在拖动中的样式：被拖动、插入线在上方或下方
  function dropClass(id: string) {
    return {
      dragging: dragIds.value.includes(id),
      'drop-before': over.value?.id === id && !over.value.after && !dragIds.value.includes(id),
      'drop-after': over.value?.id === id && over.value.after && !dragIds.value.includes(id),
    }
  }

  return { sortMode, selected, dragIds, over, isSelected, setSelected, clearSelection, toggleSortMode, dragStart, dragOver, dragLeave, takeDrop, resetDrag, dropClass }
}

// setCountImage 拖动多项时用“N 项”小标签作为拖动图像，提示正在一起移动
function setCountImage(dt: DataTransfer, count: number) {
  const el = document.createElement('div')
  el.textContent = `× ${count}`
  el.style.cssText =
    'position:fixed;top:-100px;left:-100px;padding:4px 10px;border-radius:12px;background:var(--el-color-primary);color:#fff;font:600 13px/1.4 sans-serif'
  document.body.appendChild(el)
  dt.setDragImage(el, 10, 10)
  window.setTimeout(() => el.remove(), 0)
}

// useGroupRename 返回询问分组新名称的函数：新名称与现有的另一个分组同名时先确认合并。
// 返回 null 表示取消或名称未变
export function useGroupRename() {
  const { t } = useI18n()
  return async function askName(current: string, existing: string[]): Promise<string | null> {
    let name: string
    try {
      const r = await ElMessageBox.prompt(t('groupSort.renamePrompt'), t('groupSort.renameTitle', { name: current }), {
        inputValue: current,
        confirmButtonText: t('common.save'),
        cancelButtonText: t('common.cancel'),
        inputValidator: (v: string) => {
          const n = (v ?? '').trim()
          if (!n) {
            return t('groupSort.nameRequired')
          }
          if ([...n].length > MAX_GROUP_LEN) {
            return t('groupSort.nameTooLong', { max: MAX_GROUP_LEN })
          }
          return true
        },
      })
      name = r.value.trim()
    } catch {
      return null
    }
    if (name === current) {
      return null
    }
    if (existing.includes(name)) {
      try {
        await ElMessageBox.confirm(t('groupSort.mergeConfirm', { from: current, to: name }), t('groupSort.renameTitle', { name: current }), { type: 'warning' })
      } catch {
        return null
      }
    }
    return name
  }
}

// GroupedListOptions 为 useGroupedList 的参数。保存方式二选一：
//   - base：通用 CRUD 接口路径（如 /api/stun/rules），分组用 <base>-group 批量保存，顺序用 <base>-order 保存，都不会重启运行中的实例；
//   - save：自定义保存函数，收到调整后的完整分组列表，用于嵌套列表（如 DDNS 任务内的解析记录）或对话框内的本地排序。
export interface GroupedListOptions<T> {
  // storageKey 为分组收起状态的存储键
  storageKey: string
  items: () => T[]
  // idOf / groupOf 取条目 id 与分组名称，默认为 key 与 group 字段
  idOf?: (it: T) => string
  groupOf?: (it: T) => string
  // matches 判断条目是否符合搜索内容，q 已转为小写
  matches: (it: T, q: string) => boolean
  // visible 为搜索之外的额外筛选（如按 frpc 实例筛选代理），排序模式下不生效
  visible?: (it: T) => boolean
  base?: string
  save?: (next: Section<T>[]) => Promise<void>
  // reload 在保存后重新加载列表；本地排序时可为空函数
  reload: () => Promise<void>
}

// fieldOf 读取条目的字符串字段，缺省为空字符串
function fieldOf(it: unknown, name: string) {
  const v = (it as Record<string, unknown>)[name]
  return typeof v === 'string' ? v : ''
}

// useGroupedList 为条目列表提供搜索、分组、排序与批量操作，返回值交给 GroupedList 组件渲染
export function useGroupedList<T>(opts: GroupedListOptions<T>) {
  const sel = useSortSelect()
  const filterText = ref('')
  const fold = useCollapsed(opts.storageKey)
  const idOf = opts.idOf ?? ((it: T) => fieldOf(it, 'key'))
  const groupOf = opts.groupOf ?? ((it: T) => fieldOf(it, 'group'))
  const sections = computed(() => buildSections(opts.items(), groupOf))
  const groupNames = computed(() => sections.value.map((s) => s.name).filter(Boolean))
  const items = computed(() =>
    displayItems(sections.value, idOf, {
      query: filterText.value,
      sorting: sel.sortMode.value,
      matches: opts.matches,
      visible: opts.visible,
      isCollapsed: (name) => fold.isCollapsed(name),
      isSelected: sel.isSelected,
    }),
  )
  const orderedIds = computed(() => flatten(sections.value).map(idOf))
  const total = computed(() => orderedIds.value.length)
  const busy = ref(false)

  // saveCrud 通过通用 CRUD 接口保存：先按目标分组批量修改分组，再保存整体顺序
  async function saveCrud(next: Section<T>[]) {
    for (const [group, keys] of groupChanges(next, idOf, groupOf)) {
      await api.put(`${opts.base}-group`, { keys, group })
    }
    const order = flatten(next).map(idOf)
    if (order.join('\n') !== orderedIds.value.join('\n')) {
      await api.put(`${opts.base}-order`, { keys: order })
    }
  }

  // persist 保存新的分组与顺序，然后重新加载列表；next 为 null 表示没有变化
  async function persist(next: Section<T>[] | null) {
    if (!next || busy.value) {
      return false
    }
    busy.value = true
    let ok = true
    try {
      if (opts.save) {
        await opts.save(next)
      } else {
        await saveCrud(next)
      }
    } catch (e) {
      ok = false
      ElMessage.error((e as Error).message)
    } finally {
      busy.value = false
    }
    await opts.reload()
    return ok
  }

  function toggleGroup(name: string) {
    fold.setCollapsed(name, !fold.isCollapsed(name))
  }

  function moveItem(id: string, d: number) {
    return persist(moveStep(sections.value, idOf, id, d))
  }

  function moveGroup(index: number, d: number) {
    return persist(moveSection(sections.value, index, d))
  }

  // setGroup 把选中项移到指定分组末尾，留空为移出分组
  function setGroup(name: string) {
    return persist(moveInto(sections.value, idOf, new Set(sel.selected.value), { group: name.trim() }))
  }

  function dragStart(id: string, e: DragEvent) {
    sel.dragStart(id, orderedIds.value, e)
  }

  function drop() {
    const d = sel.takeDrop()
    if (!d) {
      return
    }
    const target = dropTarget(sections.value, idOf, d.over)
    if (target) {
      persist(moveInto(sections.value, idOf, d.ids, target))
    }
  }

  function selectAll() {
    sel.setSelected(orderedIds.value, true)
  }

  // renameGroup 重命名分组：组内条目一起改为新名称，位置与顺序不变；新名称已存在时并入该分组，收起状态随名称保留
  const askName = useGroupRename()
  async function renameGroup(name: string) {
    const to = await askName(name, groupNames.value)
    if (to === null) {
      return
    }
    const wasCollapsed = fold.isCollapsed(name)
    if (await persist(renameSection(sections.value, name, to))) {
      fold.setCollapsed(to, wasCollapsed || fold.isCollapsed(to))
      fold.setCollapsed(name, false)
    }
  }

  return {
    ...sel,
    filterText,
    sections,
    groupNames,
    items,
    orderedIds,
    total,
    busy,
    toggleGroup,
    moveItem,
    moveGroup,
    setGroup,
    dragStart,
    drop,
    selectAll,
    renameGroup,
  }
}

// GroupedListState 为 useGroupedList 的返回值类型，供 GroupedList 等组件声明属性
export type GroupedListState<T> = ReturnType<typeof useGroupedList<T>>
