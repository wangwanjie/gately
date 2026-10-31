<template>
  <!-- 移动端虚拟按键栏：软键盘上没有的功能键与常用符号，单行横向滑动。
       按下时阻止默认行为，避免终端输入框失焦导致软键盘收起。 -->
  <div class="keybar" @contextmenu.prevent>
    <div class="scroller">
      <button
        v-for="k in keys"
        :key="k.label"
        class="kb"
        :class="{ on: k.sticky && (k.sticky === 'ctrl' ? ctrl : alt) }"
        v-bind="bind(k)"
      >
        <component :is="k.icon" v-if="k.icon" :size="16" />
        <template v-else>{{ k.label }}</template>
      </button>
      <button class="kb wide" v-bind="bind({ label: t('terminal.keybar.shortcuts'), action: 'shortcuts' })">
        <Zap :size="14" />{{ t('terminal.keybar.shortcuts') }}
      </button>
    </div>
    <!-- 固定在右侧：键盘弹出时收起键盘，常驻且键盘收起时呼出键盘 -->
    <button
      class="kb fixed"
      v-bind="bind({ label: keyboardOpen ? t('terminal.keybar.hideKeyboard') : t('terminal.keybar.showKeyboard'), action: 'keyboard' })"
    >
      <KeyboardOff v-if="keyboardOpen" :size="16" />
      <Keyboard v-else :size="16" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Component } from 'vue'
import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp, Keyboard, KeyboardOff, Zap } from 'lucide-vue-next'
import type { KeyName } from './terminalKeys'

const { t } = useI18n()

defineProps<{ ctrl: boolean; alt: boolean; keyboardOpen: boolean }>()
const emit = defineEmits<{
  key: [name: KeyName]
  text: [s: string]
  'toggle-ctrl': []
  'toggle-alt': []
  shortcuts: []
  keyboard: []
}>()

interface Key {
  label: string
  icon?: Component
  key?: KeyName
  text?: string
  sticky?: 'ctrl' | 'alt'
  action?: 'shortcuts' | 'keyboard'
  repeat?: boolean
}

// 最常用的放前面，打开就能看到；其余左右滑动查看
const keys: Key[] = [
  { label: 'Esc', key: 'esc' },
  { label: 'Tab', key: 'tab' },
  { label: 'Ctrl', sticky: 'ctrl' },
  { label: 'Alt', sticky: 'alt' },
  { label: '←', icon: ArrowLeft, key: 'left', repeat: true },
  { label: '↓', icon: ArrowDown, key: 'down', repeat: true },
  { label: '↑', icon: ArrowUp, key: 'up', repeat: true },
  { label: '→', icon: ArrowRight, key: 'right', repeat: true },
  { label: '|', text: '|' },
  { label: '~', text: '~' },
  { label: '/', text: '/' },
  { label: '-', text: '-' },
  { label: '_', text: '_' },
  { label: 'Home', key: 'home' },
  { label: 'End', key: 'end' },
  { label: 'PgUp', key: 'pgup', repeat: true },
  { label: 'PgDn', key: 'pgdn', repeat: true },
]

// 手指移动超过该距离视为滑动按键栏，不触发按键
const MOVE_TOLERANCE = 10

let timer: ReturnType<typeof setTimeout> | undefined
let interval: ReturnType<typeof setInterval> | undefined
let pressed: Key | undefined
let startX = 0
let startY = 0
let repeating = false

function fire(k: Key) {
  if (k.sticky === 'ctrl') {
    emit('toggle-ctrl')
  } else if (k.sticky === 'alt') {
    emit('toggle-alt')
  } else if (k.action === 'shortcuts') {
    emit('shortcuts')
  } else if (k.action === 'keyboard') {
    emit('keyboard')
  } else if (k.key) {
    emit('key', k.key)
  } else if (k.text) {
    emit('text', k.text)
  }
}

function stop() {
  if (timer) {
    clearTimeout(timer)
    timer = undefined
  }
  if (interval) {
    clearInterval(interval)
    interval = undefined
  }
  pressed = undefined
  repeating = false
}

// 按键栏可以横向滑动，所以按键在抬起时才触发，滑动（pointercancel 或移动过远）则取消。
// 方向键等长按 350ms 后先触发一次，再以 70ms 间隔连发。
function down(k: Key, e: PointerEvent) {
  stop()
  pressed = k
  startX = e.clientX
  startY = e.clientY
  if (k.repeat) {
    timer = setTimeout(() => {
      repeating = true
      fire(k)
      interval = setInterval(() => fire(k), 70)
    }, 350)
  }
}

function move(e: PointerEvent) {
  if (pressed && !repeating && Math.hypot(e.clientX - startX, e.clientY - startY) > MOVE_TOLERANCE) {
    stop()
  }
}

function up(k: Key) {
  if (pressed === k && !repeating) {
    fire(k)
  }
  stop()
}

function bind(k: Key) {
  return {
    onPointerdown: (e: PointerEvent) => {
      // 阻止默认行为后不会派发兼容鼠标事件，按键不会抢走终端输入框焦点；滚动由 touch-action 处理，不受影响
      e.preventDefault()
      down(k, e)
    },
    onPointermove: move,
    onPointerup: () => up(k),
    onPointercancel: stop,
    onPointerleave: (e: PointerEvent) => {
      // 触屏抬起后也会派发 pointerleave，只在鼠标移出时取消
      if (e.pointerType === 'mouse') {
        stop()
      }
    },
    onMousedown: (e: MouseEvent) => e.preventDefault(),
    // iOS Safari 在 touchend 后合成点击时会让终端输入框失焦、收起键盘（点在图标上尤其明显），
    // 阻止 touchend 默认行为即可取消合成点击；已开始滑动时事件不可取消，不影响滚动
    onTouchend: (e: TouchEvent) => {
      if (e.cancelable) {
        e.preventDefault()
      }
    },
    'aria-label': k.label,
  }
}

onBeforeUnmount(stop)
</script>

<style scoped>
.keybar {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 5px 6px calc(5px + env(safe-area-inset-bottom));
  border-top: 1px solid var(--g-border);
  background: var(--el-bg-color);
  user-select: none;
  -webkit-user-select: none;
  -webkit-touch-callout: none;
}
.scroller {
  flex: 1;
  min-width: 0;
  display: flex;
  gap: 4px;
  overflow-x: auto;
  overscroll-behavior-x: contain;
  scrollbar-width: none;
  touch-action: pan-x;
}
.scroller::-webkit-scrollbar {
  display: none;
}
.kb {
  flex: 0 0 auto;
  min-width: 40px;
  height: 36px;
  padding: 0 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  border: 1px solid var(--g-border);
  border-radius: 8px;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
  font-family: var(--g-mono);
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
  touch-action: pan-x;
  -webkit-tap-highlight-color: transparent;
}
/* 图标不接收触摸，点击目标始终是按钮本身 */
.kb :deep(svg) {
  pointer-events: none;
}
.kb:active {
  background: var(--el-color-primary-light-8);
}
.kb.on {
  background: var(--el-color-primary);
  border-color: var(--el-color-primary);
  color: #fff;
}
.kb.wide {
  font-family: var(--g-font);
}
.kb.fixed {
  touch-action: manipulation;
}
</style>
