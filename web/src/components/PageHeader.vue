<template>
  <div
    ref="head"
    class="page-head"
    :class="{ 'is-sticky': sticky, 'is-stuck': sticky && stuck }"
    :style="layout ? { '--ph-offset': layout.offset + 'px' } : undefined"
  >
    <div ref="main" class="page-head-main">
      <div v-if="icon" class="page-head-icon">
        <component :is="icon" :size="20" :stroke-width="2" />
      </div>
      <div class="page-head-text">
        <h2>{{ title }}</h2>
        <p v-if="desc">{{ desc }}</p>
      </div>
    </div>
    <div v-if="$slots.default" ref="actions" class="page-head-actions">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useSlots, watch } from 'vue'
import { useIsMobile } from '../composables/useIsMobile'
import { headLayout, headOwner, isStuck, type HeadLayout } from '../stickyHead'

defineProps<{ title: string; desc?: string; icon?: Component }>()

// 页面头部吸顶（issue #64）：有操作栏的头部吸在顶栏下方，长列表滚动时操作栏始终可用。
// 只改 sticky 的 top，不改变头部在文档流里的高度，滚动时内容不会跳动。
const head = ref<HTMLElement>()
const main = ref<HTMLElement>()
const actions = ref<HTMLElement>()
const layout = ref<HeadLayout | null>(null)
// 是否已吸住，只用于显示底部分隔阴影
const stuck = ref(false)
const isMobile = useIsMobile()
const slots = useSlots()
// 只有带操作栏的头部才吸顶；标题本身已显示在顶栏面包屑里
const sticky = computed(() => !!slots.default)

let headerH = 56
let frame = 0
// 窗口尺寸变化后需要在下一帧重新量取
let dirty = false
let ro: ResizeObserver | undefined
// me 标识写入者，卸载时只清理自己写的 --g-page-head-h
const me = Symbol('page-head')

const root = document.documentElement

// measure 重新量取头部尺寸；字号、语言、窗口宽度或按钮数量变化都会触发。
function measure() {
  const el = head.value
  if (!el || !main.value) {
    return
  }
  headerH = parseFloat(getComputedStyle(root).getPropertyValue('--g-header-h')) || 56
  // 用相对头部边框顶端的距离，不依赖 offsetParent，吸住与否都不影响差值
  const box = el.getBoundingClientRect()
  const m = main.value.getBoundingClientRect()
  layout.value = headLayout({
    headHeight: box.height,
    mainTop: m.top - box.top,
    mainHeight: m.height,
    actionsTop: actions.value ? actions.value.getBoundingClientRect().top - box.top : null,
  })
  // 吸住后露出的高度发布给页内工具栏（g-sticky-bar）、锚点留白等使用
  headOwner.current = me
  root.style.setProperty('--g-page-head-h', (layout.value?.visible ?? 0) + 'px')
  update()
}

// update 在每帧最多执行一次，只读取位置并切换吸住状态
function update() {
  frame = 0
  if (dirty) {
    dirty = false
    // measure 末尾会再次调用 update
    measure()
    return
  }
  const el = head.value
  const l = layout.value
  stuck.value = !!el && !!l && isStuck(el.getBoundingClientRect().top, headerH, l.offset)
}

function schedule() {
  if (!frame) {
    frame = requestAnimationFrame(update)
  }
}

// 窗口尺寸变化（旋转屏幕、浏览器工具栏伸缩）时重新量取；ResizeObserver 之外的兜底
function onResize() {
  dirty = true
  schedule()
}

onMounted(() => {
  measure()
  ro = new ResizeObserver(() => measure())
  if (head.value) {
    ro.observe(head.value)
  }
  if (actions.value) {
    ro.observe(actions.value)
  }
  window.addEventListener('scroll', schedule, { passive: true })
  window.addEventListener('resize', onResize, { passive: true })
})

// 断点切换时 --g-header-h 随之变化，需要重新量取
watch(isMobile, () => nextTick(measure))

onBeforeUnmount(() => {
  ro?.disconnect()
  window.removeEventListener('scroll', schedule)
  window.removeEventListener('resize', onResize)
  if (frame) {
    cancelAnimationFrame(frame)
  }
  // 新页面的头部可能已经写入了自己的值，只有最后写入者仍是本组件时才清除
  if (headOwner.current === me) {
    headOwner.current = null
    root.style.removeProperty('--g-page-head-h')
  }
})
</script>

<style scoped>
.page-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px 16px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
/* 吸顶：向外扩到内容区左右边缘并铺上页面底色，列表滚到下面时被完整遮住。
   上下内边距与负外边距相互抵消，头部在文档流里的位置和高度与不吸顶时一致。 */
.page-head.is-sticky {
  position: sticky;
  top: calc(var(--g-header-h) - var(--ph-offset, 0px));
  z-index: 8;
  margin: -10px calc(-1 * var(--g-content-px)) 4px;
  padding: 10px var(--g-content-px) 12px;
  background: var(--el-bg-color-page);
  transition: box-shadow 0.2s ease;
}
.page-head.is-stuck {
  box-shadow:
    0 1px 0 var(--g-border),
    0 8px 12px -12px rgba(0, 0, 0, 0.25);
}
.page-head-main {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}
.page-head-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  flex-shrink: 0;
}
.page-head-text {
  min-width: 0;
}
h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  line-height: 1.3;
}
p {
  margin: 2px 0 0;
  font-size: 13px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
}
.page-head-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.page-head-actions :deep(.el-button + .el-button) {
  margin-left: 0;
}
@media (max-width: 767.98px) {
  .page-head {
    align-items: flex-start;
    flex-direction: column;
  }
  .page-head-icon {
    width: 36px;
    height: 36px;
  }
  .page-head-actions {
    width: 100%;
  }
}
@media (prefers-reduced-motion: reduce) {
  .page-head.is-sticky {
    transition: none;
  }
}
</style>
