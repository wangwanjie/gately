<template>
  <!-- 托管进程列表：概览页与性能监控页共用；sparks 为 true 时每个数值下方画最近约 60 秒的迷你曲线 -->
  <div class="mp-list">
    <component
      :is="row.path ? RouterLink : 'div'"
      v-for="row in rows"
      :key="row.key"
      :to="row.path"
      class="mp-row"
      :class="{ link: !!row.path }"
    >
      <div class="mp-main">
        <div class="mp-title">
          <el-tag size="small" effect="plain" class="mp-module">{{ row.moduleTitle }}</el-tag>
          <span class="mp-name" :title="row.p.name">{{ row.p.name }}</span>
          <span class="mp-prog mono">{{ row.p.program }}</span>
        </div>
        <div class="mp-meta">
          <span class="mono">PID {{ row.p.pid }}</span>
          <span>{{ t('managedProc.uptime', { d: row.uptime }) }}</span>
          <span v-if="row.p.threads">{{ t('managedProc.threads', { n: row.p.threads }) }}</span>
          <span v-if="row.p.restarts" class="warn">{{ t('managedProc.restarts', { n: row.p.restarts }) }}</span>
        </div>
      </div>
      <div class="mp-val">
        <span class="mp-label">CPU</span>
        <b>{{ row.cpu }}</b>
        <Sparkline v-if="sparks" class="mp-spark" :series="[{ values: row.cpuSeries, color: 'var(--el-color-primary)' }]" :capacity="capacity" :height="22" />
      </div>
      <div class="mp-val">
        <span class="mp-label">{{ t('managedProc.memory') }}</span>
        <b>{{ row.rss }}</b>
        <Sparkline v-if="sparks" class="mp-spark" :series="[{ values: row.rssSeries, color: 'var(--g-chart-2)' }]" :capacity="capacity" :height="22" />
      </div>
    </component>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { formatBytes, formatDuration, type ManagedProcess } from '../api'
import { navByModule } from '../nav'
import Sparkline from './Sparkline.vue'

// ProcSample 是性能监控接口返回的一次采样：cpu 为百分比，rss 为 MB，取不到时为 null
export interface ProcSample {
  t: number
  cpu: number | null
  rss: number | null
}

export type ManagedRow = ManagedProcess & { samples?: ProcSample[] }

const props = withDefaults(defineProps<{ items: ManagedRow[]; sparks?: boolean; now?: number; capacity?: number }>(), {
  sparks: false,
  now: undefined,
  capacity: 12,
})

const { t } = useI18n()

// formatCpu 与概览页 Gately 进程的 CPU 口径一致：按整机核数归一化，小于 10% 时保留一位小数
function formatCpu(v: number | null | undefined) {
  if (v == null) {
    return '-'
  }
  return v.toFixed(v >= 10 ? 0 : 1) + '%'
}

const rows = computed(() =>
  props.items.map((p) => {
    const nav = navByModule(p.module)
    const since = Date.parse(p.since)
    const now = props.now ?? Date.now()
    const samples = p.samples ?? []
    return {
      // 同一实例重启后 PID 会变，按模块、名称与程序区分，避免列表重排时整行重建
      key: `${p.module}\u0000${p.name}\u0000${p.program}`,
      p,
      // 未知模块（例如更新的后端）显示模块标识，不跳转
      moduleTitle: nav ? t(nav.titleKey) : p.module || '-',
      path: nav?.path,
      uptime: isNaN(since) ? '-' : formatDuration((now - since) / 1000),
      cpu: formatCpu(p.cpu),
      rss: p.rss ? formatBytes(p.rss) : '-',
      // 迷你曲线只画取得到的值
      cpuSeries: samples.map((s) => s.cpu).filter((v): v is number => v != null),
      rssSeries: samples.map((s) => s.rss).filter((v): v is number => v != null),
    }
  }),
)
</script>

<style scoped>
.mp-list {
  display: flex;
  flex-direction: column;
}
.mp-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 96px 96px;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  color: inherit;
  text-decoration: none;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.mp-row:last-child {
  border-bottom: none;
}
.mp-row.link:hover {
  background: var(--el-fill-color-light);
}
.mp-main {
  min-width: 0;
}
.mp-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.mp-module {
  flex-shrink: 0;
}
.mp-name {
  font-size: 14px;
  font-weight: 500;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mp-prog {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.mp-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 12px;
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.mp-meta .warn {
  color: var(--el-color-warning);
}
.mp-val {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  min-width: 0;
}
.mp-label {
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
.mp-val b {
  font-size: 14px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.mp-spark {
  width: 100%;
  margin-top: 2px;
}
/* 手机上名称占满一行，两个数值在下一行左右排列 */
@media (max-width: 767.98px) {
  .mp-row {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 8px 16px;
    padding: 10px 12px;
  }
  .mp-main {
    grid-column: 1 / -1;
  }
  .mp-val {
    align-items: flex-start;
  }
}
</style>
