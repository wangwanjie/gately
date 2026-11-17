<script setup lang="ts">
// 性能对比横向条形图：一个指标一张图，Gately 用品牌色突出，其他程序用中性灰作对照。
// 每根条都直接标出程序名与数值（颜色不是唯一的识别方式），完整数据另见页面里的表格。
import { computed, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    title: string
    // 单位，显示在数值后面，例如 "QPS"、"MiB"
    unit?: string
    // [程序名, 数值]；数值为 null 表示该程序不支持或未测
    items: [string, number | null][]
    // higher：越大越好；lower：越小越好。决定排序方向和标题旁的提示
    better?: 'higher' | 'lower'
    highlight?: string
    // 图下方的补充说明
    note?: string
    // 提示文字（随页面语言传入）
    betterLabel?: string
    naLabel?: string
    digits?: number
  }>(),
  { unit: '', better: 'higher', highlight: 'Gately', note: '', betterLabel: '', naLabel: 'N/A', digits: 0 }
)

// 按“更好”的方向排序，不支持的项放在最后
const rows = computed(() => {
  const list = props.items.map(([name, value]) => ({ name, value }))
  list.sort((a, b) => {
    if (a.value === null) {
      return 1
    }
    if (b.value === null) {
      return -1
    }
    return props.better === 'higher' ? b.value - a.value : a.value - b.value
  })
  return list
})

const max = computed(() => Math.max(...props.items.map(([, v]) => v ?? 0), 1))

function fmt(v: number | null) {
  if (v === null) {
    return props.naLabel
  }
  return v.toLocaleString(undefined, { maximumFractionDigits: props.digits, minimumFractionDigits: props.digits })
}

// 悬停的行，用于高亮与浮层
const hover = ref<string | null>(null)
</script>

<template>
  <figure class="bench-bars" :aria-label="title">
    <figcaption>
      <span class="bb-title">{{ title }}</span>
      <span v-if="betterLabel" class="bb-better">{{ betterLabel }}</span>
    </figcaption>
    <div class="bb-rows" role="list">
      <div
        v-for="r in rows"
        :key="r.name"
        class="bb-row"
        :class="{ hl: r.name === highlight, dim: hover && hover !== r.name, na: r.value === null }"
        role="listitem"
        :title="`${r.name}: ${fmt(r.value)} ${r.value === null ? '' : unit}`"
        @mouseenter="hover = r.name"
        @mouseleave="hover = null"
      >
        <span class="bb-name">{{ r.name }}</span>
        <span class="bb-track">
          <span
            v-if="r.value !== null"
            class="bb-bar"
            :style="{ width: `max(calc((100% - 92px) * ${r.value / max}), 3px)` }"
          />
          <span class="bb-val">{{ fmt(r.value) }}<small v-if="r.value !== null && unit">{{ unit }}</small></span>
        </span>
      </div>
    </div>
    <p v-if="note" class="bb-note">{{ note }}</p>
  </figure>
</template>

<style scoped>
.bench-bars {
  --bb-hl: #0e7c66;
  --bb-ctx: #b4b3ad;
  --bb-text: var(--vp-c-text-1);
  --bb-muted: var(--vp-c-text-2);
  margin: 20px 0 28px;
  padding: 16px 18px 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 12px;
  background: var(--vp-c-bg-soft);
}
:root.dark .bench-bars {
  --bb-hl: #1aa886;
  --bb-ctx: #55544f;
}
figcaption {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 10px;
  margin-bottom: 12px;
}
.bb-title {
  font-weight: 600;
  color: var(--bb-text);
}
.bb-better {
  font-size: 12px;
  color: var(--bb-muted);
}
.bb-rows {
  display: grid;
  gap: 2px;
}
.bb-row {
  display: grid;
  grid-template-columns: minmax(96px, 168px) 1fr;
  align-items: center;
  gap: 10px;
  min-height: 26px;
  font-size: 13px;
  transition: opacity 0.15s;
}
.bb-row.dim {
  opacity: 0.55;
}
.bb-name {
  color: var(--bb-muted);
  text-align: right;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bb-row.hl .bb-name,
.bb-row.hl .bb-val {
  color: var(--bb-text);
  font-weight: 600;
}
.bb-track {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.bb-bar {
  display: block;
  height: 14px;
  flex: none;
  border-radius: 0 4px 4px 0;
  background: var(--bb-ctx);
}
.bb-row.hl .bb-bar {
  background: var(--bb-hl);
}
.bb-val {
  white-space: nowrap;
  color: var(--bb-muted);
  font-variant-numeric: tabular-nums;
}
.bb-val small {
  margin-left: 3px;
  font-size: 11px;
}
.bb-row.na .bb-val {
  font-style: italic;
}
.bb-note {
  margin: 10px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--bb-muted);
}
@media (max-width: 560px) {
  .bb-row {
    grid-template-columns: 92px 1fr;
    font-size: 12px;
  }
}
</style>
