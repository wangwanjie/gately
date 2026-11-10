// 弹出层打开期间锁定页面滚动：拦截滚轮和触摸滑动，避免滚动穿透到下方页面。
// 不改 body 的 overflow，滚动条不消失，页面也不会因此左右抖动。
// 弹出层内部（allow 选择器匹配的元素）自身还能滚动时放行，滚到头后同样拦截，防止滚动链传到页面。
import { onBeforeUnmount } from 'vue'

// 当前加锁的弹出层：值为允许内部滚动的区域选择器，空字符串表示整页都不允许滚动
const locks = new Map<symbol, string>()
let touchY = 0
let touchX = 0

// canScroll 判断 el 在给定方向上是否还能滚动（dy/dx 为正表示向下/向右）。
function canScroll(el: Element, dx: number, dy: number) {
  const s = getComputedStyle(el)
  if (dy !== 0 && /(auto|scroll)/.test(s.overflowY) && el.scrollHeight > el.clientHeight) {
    if ((dy < 0 && el.scrollTop > 0) || (dy > 0 && el.scrollTop + el.clientHeight < el.scrollHeight - 1)) {
      return true
    }
  }
  if (dx !== 0 && /(auto|scroll)/.test(s.overflowX) && el.scrollWidth > el.clientWidth) {
    if ((dx < 0 && el.scrollLeft > 0) || (dx > 0 && el.scrollLeft + el.clientWidth < el.scrollWidth - 1)) {
      return true
    }
  }
  return false
}

// allowed 判断这次滚动是否发生在弹出层内部、且沿途有元素还能朝该方向滚动。
function allowed(target: EventTarget | null, dx: number, dy: number) {
  if (!(target instanceof Element)) {
    return false
  }
  for (const sel of locks.values()) {
    if (!sel) {
      continue
    }
    const root = target.closest(sel)
    if (!root) {
      continue
    }
    for (let el: Element | null = target; el; el = el.parentElement) {
      if (canScroll(el, dx, dy)) {
        return true
      }
      if (el === root) {
        break
      }
    }
  }
  return false
}

function onWheel(e: WheelEvent) {
  // 按住 Ctrl 的滚轮（含触摸板双指缩放）是页面缩放，不拦截
  if (e.ctrlKey) {
    return
  }
  if (!allowed(e.target, e.deltaX, e.deltaY)) {
    e.preventDefault()
  }
}

function onTouchStart(e: TouchEvent) {
  touchX = e.touches[0]?.clientX ?? 0
  touchY = e.touches[0]?.clientY ?? 0
}

function onTouchMove(e: TouchEvent) {
  // 多指手势（缩放）不拦截
  if (e.touches.length !== 1) {
    return
  }
  const t = e.touches[0]
  // 手指上滑相当于内容向下滚动，方向与滚轮相反
  const dx = touchX - t.clientX
  const dy = touchY - t.clientY
  touchX = t.clientX
  touchY = t.clientY
  if (!allowed(e.target, dx, dy) && e.cancelable) {
    e.preventDefault()
  }
}

function install() {
  document.addEventListener('wheel', onWheel, { passive: false, capture: true })
  document.addEventListener('touchstart', onTouchStart, { passive: true, capture: true })
  document.addEventListener('touchmove', onTouchMove, { passive: false, capture: true })
}

function uninstall() {
  document.removeEventListener('wheel', onWheel, { capture: true })
  document.removeEventListener('touchstart', onTouchStart, { capture: true })
  document.removeEventListener('touchmove', onTouchMove, { capture: true })
}

// joinSelectors 把一个或多个选择器合并成逗号分隔的选择器列表，closest() 会匹配其中最近的祖先。
export function joinSelectors(allow: string | string[]): string {
  const list = Array.isArray(allow) ? allow : [allow]
  return list.map((s) => s.trim()).filter((s) => s !== '').join(', ')
}

// useScrollLock 返回加锁开关：传 true 锁定页面滚动，false 解锁；组件卸载时自动解锁。
// allow 为弹出层的选择器，其内部可滚动的区域（如较长的下拉菜单）仍可正常滚动；
// 可传多个选择器（例如全屏面板本身与挂在 body 下的下拉弹出层），任一匹配即放行。
export function useScrollLock(allow: string | string[] = '') {
  const id = Symbol('scroll-lock')
  const sel = joinSelectors(allow)

  function setLocked(on: boolean) {
    if (on === locks.has(id)) {
      return
    }
    if (on) {
      if (locks.size === 0) {
        install()
      }
      locks.set(id, sel)
    } else {
      locks.delete(id)
      if (locks.size === 0) {
        uninstall()
      }
    }
  }

  onBeforeUnmount(() => {
    setLocked(false)
  })
  return setLocked
}
