// 用 Pointer 事件实现的拖动排序，鼠标与触屏共用一套逻辑（HTML5 拖放在手机浏览器上基本不可用）。
// 用法：可放置的元素带 data-drop-id（分组标题再加 data-drop-kind="group"），拖动把手的 pointerdown 调用 start；
// 拖动中靠近滚动容器上下边缘时自动滚动，松手时把拖动的 id 与放置位置交给 onDrop，按 Esc 或系统取消时放弃。
import { computed, onBeforeUnmount, reactive, ref, type Ref } from 'vue'
import type { DropOver } from '../groupSort'

// THRESHOLD 为开始拖动前需要移动的距离（像素），避免点一下把手就进入拖动
const THRESHOLD = 4
// EDGE 为自动滚动的触发区域高度，MAX_SPEED 为每帧最多滚动的像素
const EDGE = 56
const MAX_SPEED = 22

// scrollParent 找到元素最近的可纵向滚动的祖先，找不到时用页面本身
function scrollParent(el: HTMLElement | null): HTMLElement {
  for (let p = el?.parentElement; p; p = p.parentElement) {
    const oy = getComputedStyle(p).overflowY
    if ((oy === 'auto' || oy === 'scroll') && p.scrollHeight > p.clientHeight) {
      return p
    }
  }
  return (document.scrollingElement as HTMLElement) ?? document.documentElement
}

// edgeSpeed 计算自动滚动速度：y 越接近可见区域上（下）边缘，向上（下）滚得越快；不在边缘区域时为 0
export function edgeSpeed(y: number, top: number, bottom: number, edge = EDGE, max = MAX_SPEED): number {
  if (bottom - top < edge * 2) {
    edge = Math.max(8, (bottom - top) / 4)
  }
  if (y < top + edge) {
    return -Math.ceil((Math.min(top + edge - y, edge) / edge) * max)
  }
  if (y > bottom - edge) {
    return Math.ceil((Math.min(y - (bottom - edge), edge) / edge) * max)
  }
  return 0
}

export function usePointerSort(opts: { root: Ref<HTMLElement | null>; onDrop: (ids: string[], over: DropOver) => void }) {
  // dragIds 为正在拖动的条目，over 为当前悬停的放置位置；ghost 为跟随指针的提示标签
  const dragIds = ref<string[]>([])
  const over = ref<DropOver | null>(null)
  const ghost = reactive({ show: false, x: 0, y: 0, label: '' })

  let pending: { ids: string[]; label: string; pointerId: number; x0: number; y0: number } | null = null
  let active = false
  let lastX = 0
  let lastY = 0
  let scroller: HTMLElement | null = null
  let raf = 0

  // hitTest 找出指针下的放置目标：条目下半部分为插到其后，分组标题只记录目标；拖动的条目自身与列表外不算
  function hitTest() {
    const hit = document.elementFromPoint(lastX, lastY)?.closest<HTMLElement>('[data-drop-id]')
    if (!hit || !opts.root.value?.contains(hit)) {
      over.value = null
      return
    }
    const id = hit.dataset.dropId ?? ''
    if (!id || dragIds.value.includes(id)) {
      over.value = null
      return
    }
    let after = false
    if (hit.dataset.dropKind !== 'group') {
      const r = hit.getBoundingClientRect()
      after = lastY > r.top + r.height / 2
    }
    if (over.value?.id !== id || over.value.after !== after) {
      over.value = { id, after }
    }
  }

  // tick 每帧检查是否需要自动滚动；滚动后列表在指针下移动了，重新计算放置目标
  function tick() {
    raf = 0
    if (!active || !scroller) {
      return
    }
    const page = scroller === document.scrollingElement || scroller === document.documentElement
    const r = page ? { top: 0, bottom: window.innerHeight } : scroller.getBoundingClientRect()
    const top = Math.max(r.top, 0)
    const bottom = Math.min(r.bottom, window.innerHeight)
    const v = edgeSpeed(lastY, top, bottom)
    if (v) {
      const before = scroller.scrollTop
      scroller.scrollTop = before + v
      if (scroller.scrollTop !== before) {
        hitTest()
      }
      raf = requestAnimationFrame(tick)
    }
  }

  function onMove(e: PointerEvent) {
    if (!pending || e.pointerId !== pending.pointerId) {
      return
    }
    lastX = e.clientX
    lastY = e.clientY
    if (!active) {
      if (Math.hypot(lastX - pending.x0, lastY - pending.y0) < THRESHOLD) {
        return
      }
      active = true
      dragIds.value = pending.ids
      ghost.label = pending.label
      ghost.show = true
      scroller = scrollParent(opts.root.value)
      document.documentElement.classList.add('g-pointer-sorting')
    }
    e.preventDefault()
    ghost.x = lastX
    ghost.y = lastY
    hitTest()
    if (!raf) {
      raf = requestAnimationFrame(tick)
    }
  }

  function finish(commit: boolean) {
    const ids = dragIds.value
    const o = over.value
    const wasActive = active
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    window.removeEventListener('pointercancel', onCancel)
    window.removeEventListener('keydown', onKey, true)
    if (raf) {
      cancelAnimationFrame(raf)
      raf = 0
    }
    document.documentElement.classList.remove('g-pointer-sorting')
    pending = null
    active = false
    scroller = null
    dragIds.value = []
    over.value = null
    ghost.show = false
    if (commit && wasActive && o && ids.length) {
      opts.onDrop(ids, o)
    }
  }

  function onUp(e: PointerEvent) {
    if (pending && e.pointerId === pending.pointerId) {
      finish(true)
    }
  }
  function onCancel(e: PointerEvent) {
    if (pending && e.pointerId === pending.pointerId) {
      finish(false)
    }
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape' && active) {
      // 只取消拖动，不让对话框也跟着关闭
      e.preventDefault()
      e.stopPropagation()
      finish(false)
    }
  }

  // start 在拖动把手的 pointerdown 中调用：ids 为要一起移动的条目（按显示顺序），label 为跟随指针的提示文字
  function start(e: PointerEvent, ids: string[], label: string) {
    if (!ids.length || (e.pointerType === 'mouse' && e.button !== 0)) {
      return
    }
    if (pending) {
      finish(false)
    }
    // 阻止鼠标按下时开始选中文字；触屏由把手的 touch-action: none 阻止页面滚动
    e.preventDefault()
    pending = { ids, label, pointerId: e.pointerId, x0: e.clientX, y0: e.clientY }
    lastX = e.clientX
    lastY = e.clientY
    window.addEventListener('pointermove', onMove, { passive: false })
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onCancel)
    window.addEventListener('keydown', onKey, true)
  }

  // dropClass 返回条目在拖动中的样式：被拖动、插入线在上方或下方、分组标题为放置目标
  const dragging = computed(() => new Set(dragIds.value))
  function dropClass(id: string) {
    const o = over.value
    return {
      dragging: dragging.value.has(id),
      'drop-before': o?.id === id && !o.after,
      'drop-after': o?.id === id && o.after,
    }
  }

  onBeforeUnmount(() => finish(false))

  return { dragIds, over, ghost, start, dropClass, cancel: () => finish(false) }
}
