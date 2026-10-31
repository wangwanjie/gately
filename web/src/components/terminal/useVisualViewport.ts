// 跟踪可视视口（visualViewport），软键盘弹出时可据此调整终端区域高度，并判断软键盘是否弹出。
import { onBeforeUnmount, onMounted, ref } from 'vue'

// 可视视口比基准高度矮出这么多时，认为软键盘已弹出（地址栏伸缩只有几十像素）
const KEYBOARD_THRESHOLD = 120

export function useVisualViewport() {
  const height = ref(0)
  const top = ref(0)
  const keyboardOpen = ref(false)
  const supported = typeof window !== 'undefined' && !!window.visualViewport

  // 基准高度取当前宽度下见过的最大视口高度：Android 部分浏览器弹出键盘时布局视口也会缩小，
  // 单看 innerHeight 与可视视口的差值会漏判。宽度变化（旋转屏幕）时重新记录。
  let baseWidth = 0
  let baseHeight = 0

  function update() {
    const vv = window.visualViewport
    if (!vv) {
      return
    }
    height.value = Math.round(vv.height)
    top.value = Math.round(vv.offsetTop)
    if (window.innerWidth !== baseWidth) {
      baseWidth = window.innerWidth
      baseHeight = 0
    }
    baseHeight = Math.max(baseHeight, vv.height, document.documentElement.clientHeight)
    // 双指放大时可视视口也会变矮，这种情况不算键盘弹出
    keyboardOpen.value = vv.scale <= 1.01 && baseHeight - vv.height > KEYBOARD_THRESHOLD
  }

  onMounted(() => {
    update()
    window.visualViewport?.addEventListener('resize', update)
    window.visualViewport?.addEventListener('scroll', update)
  })
  onBeforeUnmount(() => {
    window.visualViewport?.removeEventListener('resize', update)
    window.visualViewport?.removeEventListener('scroll', update)
  })
  return { height, top, keyboardOpen, supported }
}
