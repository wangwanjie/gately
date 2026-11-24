<template>
  <!-- 环形仪表：按百分比绘制圆弧，超过阈值时变为警告/危险色 -->
  <div class="ring" :style="{ width: size + 'px', height: size + 'px' }">
    <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`">
      <circle :cx="c" :cy="c" :r="r" class="track" :stroke-width="stroke" fill="none" />
      <circle
        v-if="value !== undefined"
        :cx="c"
        :cy="c"
        :r="r"
        class="bar"
        :class="level"
        :stroke-width="stroke"
        fill="none"
        stroke-linecap="round"
        :stroke-dasharray="`${len} ${circ}`"
        :transform="`rotate(-90 ${c} ${c})`"
      />
    </svg>
    <div class="center">
      <span class="num">{{ value === undefined ? '-' : Math.round(value) }}</span>
      <span v-if="value !== undefined" class="unit">%</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{ value?: number; size?: number; stroke?: number }>(), { value: undefined, size: 84, stroke: 8 })

const c = computed(() => props.size / 2)
const r = computed(() => (props.size - props.stroke) / 2)
const circ = computed(() => 2 * Math.PI * r.value)
// 百分比为 0 时仍保留一个点，避免圆弧完全消失看起来像“未知”
const len = computed(() => Math.max(0.5, (Math.min(100, Math.max(0, props.value ?? 0)) / 100) * circ.value))
const level = computed(() => {
  const v = props.value ?? 0
  if (v >= 90) {
    return 'danger'
  }
  if (v >= 75) {
    return 'warning'
  }
  return 'normal'
})
</script>

<style scoped>
.ring {
  position: relative;
  flex-shrink: 0;
}
.track {
  stroke: var(--el-fill-color);
}
.bar {
  transition: stroke-dasharray 0.6s ease;
}
.bar.normal {
  stroke: var(--el-color-primary);
}
.bar.warning {
  stroke: var(--el-color-warning);
}
.bar.danger {
  stroke: var(--el-color-danger);
}
.center {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: baseline;
  justify-content: center;
  padding-top: calc(50% - 14px);
  font-variant-numeric: tabular-nums;
}
.num {
  font-size: 22px;
  font-weight: 700;
  line-height: 1;
}
.unit {
  font-size: 12px;
  margin-left: 1px;
  color: var(--el-text-color-secondary);
}
</style>
