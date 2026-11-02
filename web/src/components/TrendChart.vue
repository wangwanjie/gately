<template>
  <!-- 带坐标轴的趋势图：按时间定位各点，缺失的数据（null）断开折线；悬停显示该时刻各序列的读数 -->
  <div ref="box" class="trend" :style="{ height: height + 'px' }" @mousemove="onMove" @mouseleave="hover = -1">
    <div class="axis-y">
      <span>{{ format(top) }}</span>
      <span>{{ format(top / 2) }}</span>
      <span>0</span>
    </div>
    <div class="plot">
      <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none">
        <line v-for="g in [0, 0.5, 1]" :key="g" x1="0" :x2="W" :y1="2 + g * (H - 4)" :y2="2 + g * (H - 4)" class="grid" vector-effect="non-scaling-stroke" />
        <g v-for="(s, i) in paths" :key="i">
          <path v-if="s.area" :d="s.area" :style="{ fill: s.color }" fill-opacity="0.1" />
          <path :d="s.line" :style="{ stroke: s.color }" stroke-width="1.6" fill="none" vector-effect="non-scaling-stroke" stroke-linejoin="round" :stroke-dasharray="s.dashed ? '4 3' : undefined" />
        </g>
      </svg>
      <div v-if="hover >= 0" class="cursor" :style="{ left: xPct(times[hover]) + '%' }" />
      <div v-if="hover >= 0" class="tip" :class="{ right: xPct(times[hover]) > 60 }" :style="{ left: xPct(times[hover]) + '%' }">
        <div class="tip-time">{{ timeLabel(times[hover], true) }}</div>
        <div v-for="s in series" :key="s.name" class="tip-row">
          <span class="dot" :style="{ background: s.color }" />{{ s.name }}
          <b>{{ s.values[hover] == null ? '-' : format(s.values[hover] as number) }}</b>
        </div>
      </div>
      <div v-if="!hasData" class="empty">{{ emptyText }}</div>
    </div>
    <div class="axis-x">
      <span>{{ timeLabel(start) }}</span>
      <span>{{ timeLabel((start + end) / 2) }}</span>
      <span>{{ timeLabel(end) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

export interface TrendSeries {
  name: string
  color: string
  values: (number | null)[]
  // dashed 用于峰值等辅助序列
  dashed?: boolean
}

const props = withDefaults(
  defineProps<{
    // times 为各点的时间（Unix 秒），与每条序列的 values 一一对应
    times: number[]
    series: TrendSeries[]
    // from、to 为横轴范围（Unix 秒），缺省时取第一个与最后一个点
    from?: number
    to?: number
    // max 为纵轴上限，缺省时按数据自动取
    max?: number
    height?: number
    format?: (v: number) => string
    emptyText?: string
  }>(),
  { from: undefined, to: undefined, max: undefined, height: 120, format: (v: number) => (Math.abs(v) >= 100 ? v.toFixed(0) : v.toFixed(1)), emptyText: '-' },
)

const W = 600
const H = 100
const box = ref<HTMLElement>()
const hover = ref(-1)

const start = computed(() => props.from ?? props.times[0] ?? 0)
const end = computed(() => props.to ?? props.times[props.times.length - 1] ?? 1)
const span = computed(() => Math.max(1, end.value - start.value))
const hasData = computed(() => props.series.some((s) => s.values.some((v) => v != null)))

const top = computed(() => {
  if (props.max !== undefined) {
    return props.max
  }
  let m = 0
  for (const s of props.series) {
    for (const v of s.values) {
      if (v != null && v > m) {
        m = v
      }
    }
  }
  // 留出顶部余量，并取一个便于阅读的整数刻度
  return niceCeil(Math.max(m * 1.1, 1))
})

function niceCeil(v: number) {
  const p = Math.pow(10, Math.floor(Math.log10(v)))
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (v <= m * p) {
      return m * p
    }
  }
  return 10 * p
}

function xOf(t: number) {
  return ((t - start.value) / span.value) * W
}

function xPct(t: number) {
  return Math.min(100, Math.max(0, ((t - start.value) / span.value) * 100))
}

function yOf(v: number) {
  return H - (Math.min(Math.max(v, 0), top.value) / top.value) * (H - 4) - 2
}

const paths = computed(() =>
  props.series.map((s) => {
    // 遇到 null 断开为多段，避免把缺失的时段画成直线
    const segs: [number, number][][] = []
    let cur: [number, number][] = []
    s.values.forEach((v, i) => {
      if (v == null || props.times[i] === undefined) {
        if (cur.length) {
          segs.push(cur)
        }
        cur = []
        return
      }
      cur.push([xOf(props.times[i]), yOf(v)])
    })
    if (cur.length) {
      segs.push(cur)
    }
    let line = ''
    let area = ''
    for (const seg of segs) {
      // 孤立的单点画成一小段，否则看不见
      const pts = seg.length === 1 ? [seg[0], [seg[0][0] + 2, seg[0][1]] as [number, number]] : seg
      const d = pts.map((p, i) => `${i ? 'L' : 'M'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' ')
      line += d + ' '
      if (!s.dashed) {
        area += `${d} L${pts[pts.length - 1][0].toFixed(1)},${H} L${pts[0][0].toFixed(1)},${H} Z `
      }
    }
    return { line, area, color: s.color, dashed: s.dashed }
  }),
)

// 悬停时取横向最近的点
function onMove(e: MouseEvent) {
  const el = box.value?.querySelector('.plot') as HTMLElement | null
  if (!el || !props.times.length) {
    return
  }
  const r = el.getBoundingClientRect()
  const frac = (e.clientX - r.left) / Math.max(1, r.width)
  if (frac < 0 || frac > 1) {
    hover.value = -1
    return
  }
  const t = start.value + frac * span.value
  let best = 0
  for (let i = 1; i < props.times.length; i++) {
    if (Math.abs(props.times[i] - t) < Math.abs(props.times[best] - t)) {
      best = i
    }
  }
  hover.value = best
}

function timeLabel(t: number, withSeconds = false) {
  if (!t) {
    return ''
  }
  const d = new Date(t * 1000)
  // 跨度超过 1 小时时不显示秒
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: withSeconds || span.value <= 3600 ? '2-digit' : undefined })
}
</script>

<style scoped>
.trend {
  display: grid;
  grid-template-columns: auto 1fr;
  grid-template-rows: 1fr auto;
  column-gap: 6px;
  font-size: 11px;
  color: var(--el-text-color-secondary);
  font-variant-numeric: tabular-nums;
  user-select: none;
}
.axis-y {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  text-align: right;
  min-width: 28px;
  line-height: 1;
}
.plot {
  position: relative;
  min-width: 0;
}
.plot svg {
  display: block;
  width: 100%;
  height: 100%;
}
.grid {
  stroke: var(--el-border-color-lighter);
  stroke-width: 1;
}
.axis-x {
  grid-column: 2;
  display: flex;
  justify-content: space-between;
  padding-top: 3px;
}
.cursor {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 1px;
  background: var(--el-border-color-darker);
  pointer-events: none;
}
.tip {
  position: absolute;
  top: 4px;
  transform: translateX(8px);
  background: var(--el-bg-color-overlay);
  border: 1px solid var(--el-border-color-light);
  border-radius: 6px;
  padding: 6px 8px;
  box-shadow: var(--el-box-shadow-light);
  white-space: nowrap;
  pointer-events: none;
  z-index: 2;
  color: var(--el-text-color-regular);
}
.tip.right {
  transform: translateX(calc(-100% - 8px));
}
.tip-time {
  margin-bottom: 3px;
  color: var(--el-text-color-secondary);
}
.tip-row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.tip-row b {
  margin-left: auto;
  padding-left: 12px;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
