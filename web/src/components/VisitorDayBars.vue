<template>
  <!-- 访客审计的每日柱状图：柱高为请求数，红色部分为被拦截；点击（或悬停）一天在上层显示当天数字 -->
  <div class="day-bars" :class="{ compact }">
    <div class="bars" role="list">
      <button
        v-for="(d, i) in days"
        :key="d.day"
        type="button"
        role="listitem"
        class="bar"
        :class="{ on: i === current, empty: !d.requests }"
        :title="tip(d)"
        :aria-label="tip(d)"
        @mouseenter="emit('select', i)"
        @focus="emit('select', i)"
        @click="emit('select', i)"
      >
        <span class="fill" :style="{ height: pct(d.requests) }">
          <span class="denied" :style="{ height: d.requests ? (d.denied / d.requests) * 100 + '%' : '0' }" />
        </span>
      </button>
    </div>
    <div class="axis">
      <span>{{ short(days[0]?.day) }}</span>
      <span v-if="days.length > 6">{{ short(days[Math.floor((days.length - 1) / 2)]?.day) }}</span>
      <span>{{ short(days[days.length - 1]?.day) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { DayPoint } from '../visitors'

const props = defineProps<{ days: DayPoint[]; selected?: number; compact?: boolean }>()
const emit = defineEmits<{ select: [index: number] }>()
const { t } = useI18n()

const peak = computed(() => Math.max(1, ...props.days.map((d) => d.requests)))
const current = computed(() => (props.selected !== undefined && props.selected >= 0 ? props.selected : props.days.length - 1))

// 有请求的一天至少显示一小段，避免和没有请求的一天分不清
function pct(n: number): string {
  if (!n) {
    return '0'
  }
  return Math.max(4, (n / peak.value) * 100) + '%'
}
function short(day?: string): string {
  return day ? day.slice(5) : ''
}
function tip(d: DayPoint): string {
  return t('visitors.dayTip', { day: d.day, visitors: d.visitors, requests: d.requests, denied: d.denied })
}
</script>

<style scoped>
.bars {
  display: flex;
  align-items: flex-end;
  gap: 2px;
  height: 120px;
}
.compact .bars {
  height: 64px;
}
.bar {
  flex: 1 1 0;
  min-width: 0;
  height: 100%;
  padding: 0;
  display: flex;
  align-items: flex-end;
  background: none;
  border: none;
  border-radius: 3px;
  cursor: pointer;
}
.bar:hover,
.bar.on {
  background: var(--el-fill-color-light);
}
.bar:focus-visible {
  outline: 2px solid var(--el-color-primary);
}
.fill {
  position: relative;
  display: flex;
  flex-direction: column;
  justify-content: flex-start;
  width: 100%;
  border-radius: 3px 3px 0 0;
  background: var(--el-color-primary-light-5);
  overflow: hidden;
}
.bar.on .fill {
  background: var(--el-color-primary);
}
.denied {
  display: block;
  width: 100%;
  background: var(--el-color-danger);
  opacity: 0.85;
}
.bar.empty::after {
  content: '';
  display: block;
  width: 100%;
  height: 2px;
  border-radius: 1px;
  background: var(--el-fill-color-darker);
}
.bar.empty .fill {
  display: none;
}
.axis {
  display: flex;
  justify-content: space-between;
  margin-top: 6px;
  font-size: 11px;
  color: var(--el-text-color-secondary);
  font-variant-numeric: tabular-nums;
}
</style>
