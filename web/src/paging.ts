// 列表前端分页的纯函数：页码收拢、切片与每页条数校验，供 usePager 与单元测试共用。

// PAGE_SIZES 是可选的每页条数
export const PAGE_SIZES = [20, 50, 100, 200] as const

// DEFAULT_PAGE_SIZE 是默认每页条数
export const DEFAULT_PAGE_SIZE = 50

// pageCount 返回总页数，空列表也算 1 页
export function pageCount(total: number, size: number): number {
  if (!Number.isFinite(total) || total <= 0 || !Number.isFinite(size) || size <= 0) {
    return 1
  }
  return Math.ceil(total / size)
}

// clampPage 把页码收拢到 [1, 总页数]，非法值回到第 1 页
export function clampPage(page: number, total: number, size: number): number {
  if (!Number.isFinite(page) || page < 1) {
    return 1
  }
  return Math.min(Math.floor(page), pageCount(total, size))
}

// pageSlice 返回第 page 页的元素（页码先收拢）
export function pageSlice<T>(list: readonly T[], page: number, size: number): T[] {
  const p = clampPage(page, list.length, size)
  const start = (p - 1) * size
  return list.slice(start, start + size)
}

// normalizePageSize 校验每页条数（可能来自被改过的 localStorage），不在可选范围内时用默认值
export function normalizePageSize(v: unknown, sizes: readonly number[] = PAGE_SIZES, fallback = DEFAULT_PAGE_SIZE): number {
  const n = typeof v === 'string' ? Number(v) : v
  if (typeof n === 'number' && sizes.includes(n)) {
    return n
  }
  return sizes.includes(fallback) ? fallback : sizes[0]
}

// needsPager 判断是否需要显示分页条：条数不超过最小可选每页条数时不显示
export function needsPager(total: number, sizes: readonly number[] = PAGE_SIZES): boolean {
  return total > Math.min(...sizes)
}
