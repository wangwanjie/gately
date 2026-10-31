// 终端显示偏好（字号、配色、移动端按键栏常驻），保存在 localStorage。
import { ref, watch } from 'vue'

const FONT_KEY = 'gately-term-font'
const THEME_KEY = 'gately-term-theme'
const KEYBAR_PIN_KEY = 'gately-term-keybar-pinned'
export const FONT_MIN = 9
export const FONT_MAX = 28

function clampFont(n: number): number {
  return Math.min(FONT_MAX, Math.max(FONT_MIN, Math.round(n)))
}

function initialFont(): number {
  const saved = Number(localStorage.getItem(FONT_KEY))
  if (saved >= FONT_MIN && saved <= FONT_MAX) {
    return saved
  }
  return window.matchMedia('(max-width: 767.98px)').matches ? 13 : 14
}

const fontSize = ref(initialFont())
// 未设置过时默认跟随界面主题
const themeName = ref(localStorage.getItem(THEME_KEY) || 'follow')
// 移动端按键栏默认跟随软键盘显隐，开启后始终显示
const keybarPinned = ref(localStorage.getItem(KEYBAR_PIN_KEY) === '1')

watch(fontSize, (v) => {
  localStorage.setItem(FONT_KEY, String(v))
})
watch(themeName, (v) => {
  localStorage.setItem(THEME_KEY, v)
})
watch(keybarPinned, (v) => {
  localStorage.setItem(KEYBAR_PIN_KEY, v ? '1' : '0')
})

export function useTermPrefs() {
  return {
    fontSize,
    themeName,
    keybarPinned,
    setFont(n: number) {
      fontSize.value = clampFont(n)
    },
    incFont() {
      fontSize.value = clampFont(fontSize.value + 1)
    },
    decFont() {
      fontSize.value = clampFont(fontSize.value - 1)
    },
  }
}
