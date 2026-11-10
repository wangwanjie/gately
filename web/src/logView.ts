// 日志面板的显示辅助：日志区高度的范围限制、保存与读取，以及“是否贴底”的判断。
// 纯函数与存储读写放在这里，便于单元测试；组件只负责事件与渲染。

// 日志区最小高度（px）：至少能看到几行日志
export const MIN_HEIGHT = 120
// 日志区与视口上下留出的空间（px）：拖到最大时仍能看到工具栏与拖动条
export const VIEWPORT_MARGIN = 120
// 键盘调整的步长（px）
export const KEY_STEP = 40
// 距底部不超过该距离（px）即视为贴底：行高约 19px，留出一行左右的余量，避免小数像素误判
export const BOTTOM_SLACK = 24

// maxHeight 按视口高度计算允许的最大高度；视口很矮时不低于最小高度
export function maxHeight(viewport: number): number {
  return Math.max(MIN_HEIGHT, Math.floor(viewport - VIEWPORT_MARGIN))
}

// clampHeight 把高度限制在 [MIN_HEIGHT, maxHeight(viewport)] 内并取整；非有限数返回 null（表示用默认高度）
export function clampHeight(h: number, viewport: number): number | null {
  if (!Number.isFinite(h)) {
    return null
  }
  return Math.min(maxHeight(viewport), Math.max(MIN_HEIGHT, Math.round(h)))
}

// 保存的高度上限：防止被篡改的超大数字，同时足够覆盖大屏
const STORE_MAX = 10000

type Storage = Pick<globalThis.Storage, 'getItem' | 'setItem' | 'removeItem'>

// heightKey 返回保存高度用的键：页面里的日志卡片与弹窗里的日志分开记，互不影响
export function heightKey(scope: string): string {
  return `gately.logs.height.${scope}`
}

// loadHeight 读取保存的高度；没有、内容损坏或超出合理范围时返回 null
export function loadHeight(store: Storage | undefined, scope: string): number | null {
  try {
    const raw = store?.getItem(heightKey(scope))
    if (raw == null || !/^\d{1,5}$/.test(raw)) {
      return null
    }
    const v = Number(raw)
    return v >= MIN_HEIGHT && v <= STORE_MAX ? v : null
  } catch {
    // 存储不可用（隐私模式等）时使用默认高度
    return null
  }
}

// saveHeight 保存高度；传 null 表示恢复默认，删除保存的值
export function saveHeight(store: Storage | undefined, scope: string, h: number | null): void {
  try {
    if (h == null) {
      store?.removeItem(heightKey(scope))
    } else {
      store?.setItem(heightKey(scope), String(Math.min(STORE_MAX, Math.max(MIN_HEIGHT, Math.round(h)))))
    }
  } catch {
    // 存储不可用时只在本页生效
  }
}

// atBottom 判断滚动区是否已滚到底（或内容不足一屏）
export function atBottom(el: { scrollTop: number; scrollHeight: number; clientHeight: number }): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= BOTTOM_SLACK
}

// keyHeight 按键盘按键计算新的高度：上下方向键每次 KEY_STEP，PageUp/PageDown 每次四倍步长，
// Home/End 到最小/最大；其他按键返回 undefined（不处理）
export function keyHeight(key: string, cur: number, viewport: number): number | undefined {
  switch (key) {
    case 'ArrowUp':
      return clampHeight(cur - KEY_STEP, viewport) ?? undefined
    case 'ArrowDown':
      return clampHeight(cur + KEY_STEP, viewport) ?? undefined
    case 'PageUp':
      return clampHeight(cur - KEY_STEP * 4, viewport) ?? undefined
    case 'PageDown':
      return clampHeight(cur + KEY_STEP * 4, viewport) ?? undefined
    case 'Home':
      return MIN_HEIGHT
    case 'End':
      return maxHeight(viewport)
    default:
      return undefined
  }
}
