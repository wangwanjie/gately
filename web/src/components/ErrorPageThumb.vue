<template>
  <!-- 错误页缩略图：按 width × height 的虚拟视口渲染页面，再整体缩放到容器宽度。
       iframe 使用空 sandbox：页面运行在不透明源中，脚本、表单、弹窗与顶层跳转全部禁止 -->
  <div ref="box" class="thumb" :style="{ aspectRatio: `${width} / ${height}` }">
    <iframe
      v-if="html"
      class="frame"
      sandbox=""
      referrerpolicy="no-referrer"
      :srcdoc="html"
      :title="title"
      :tabindex="interactive ? 0 : -1"
      :aria-hidden="interactive ? undefined : 'true'"
      :style="{ width: `${width}px`, height: `${height}px`, transform: `scale(${scale})`, pointerEvents: interactive ? 'auto' : 'none' }"
    />
    <div v-else-if="error" class="state err">{{ error }}</div>
    <div v-else class="state"><span class="spinner" /></div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    html?: string
    // width 与 height 为渲染页面的虚拟视口，桌面 1280×800，手机 390×780
    width?: number
    height?: number
    title?: string
    error?: string
    // interactive 为 true 时可在预览中滚动页面
    interactive?: boolean
  }>(),
  { html: '', width: 1280, height: 800, title: '', error: '', interactive: false },
)

const box = ref<HTMLElement>()
const scale = ref(0.2)
let ro: ResizeObserver | null = null

// update 按容器宽度计算缩放比例
function update() {
  if (box.value) {
    scale.value = box.value.clientWidth / props.width
  }
}

watch(() => props.width, update)

onMounted(() => {
  ro = new ResizeObserver(update)
  if (box.value) {
    ro.observe(box.value)
  }
  update()
})

onBeforeUnmount(() => ro?.disconnect())
</script>

<style scoped>
.thumb {
  position: relative;
  width: 100%;
  overflow: hidden;
  background: var(--el-fill-color-light);
}
.frame {
  position: absolute;
  top: 0;
  left: 0;
  border: 0;
  transform-origin: 0 0;
  background: #fff;
}
.state {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  padding: 8px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  text-align: center;
}
.state.err {
  color: var(--el-color-danger);
}
.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid var(--el-border-color);
  border-top-color: var(--el-color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
