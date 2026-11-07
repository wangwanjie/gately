// 列表前端分页：只渲染当前页的行，避免数千条数据一次性渲染导致页面卡顿。
// 每页条数按页面键名保存到 localStorage；列表变短（删除、刷新、筛选）时自动收拢页码。
import { computed, ref, watch, type Ref, type WatchSource } from 'vue'
import { DEFAULT_PAGE_SIZE, PAGE_SIZES, clampPage, normalizePageSize, pageSlice } from '../paging'

const prefix = 'gately.pageSize.'

export interface PagerOptions {
  // storageKey 用于保存每页条数，留空时不保存
  storageKey?: string
  defaultSize?: number
  sizes?: readonly number[]
  // resetOn 变化时回到第 1 页（例如搜索词、当前目录）
  resetOn?: WatchSource | WatchSource[]
}

export interface Pager<T> {
  page: Ref<number>
  size: Ref<number>
  sizes: readonly number[]
  total: Ref<number>
  rows: Ref<T[]>
}

export function usePager<T>(source: () => readonly T[], opts: PagerOptions = {}): Pager<T> {
  const sizes = opts.sizes ?? PAGE_SIZES
  const fallback = opts.defaultSize ?? DEFAULT_PAGE_SIZE
  let stored: string | null = null
  if (opts.storageKey) {
    try {
      stored = localStorage.getItem(prefix + opts.storageKey)
    } catch {
      // 隐私模式等场景下无法读取存储，使用默认值
    }
  }
  const page = ref(1)
  const size = ref(normalizePageSize(stored ?? fallback, sizes, fallback))
  const total = computed(() => source().length)
  const rows = computed(() => pageSlice(source(), page.value, size.value))

  // 列表变短或每页条数变大后，当前页可能超出范围，收拢到最后一页
  watch([total, size], () => {
    const p = clampPage(page.value, total.value, size.value)
    if (p !== page.value) {
      page.value = p
    }
  })
  watch(size, (v) => {
    if (!opts.storageKey) {
      return
    }
    try {
      localStorage.setItem(prefix + opts.storageKey, String(v))
    } catch {
      // 写入失败时仅本次会话生效
    }
  })
  if (opts.resetOn) {
    watch(opts.resetOn, () => {
      page.value = 1
    })
  }
  return { page, size, sizes, total, rows }
}
