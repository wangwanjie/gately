<template>
  <el-dialog v-model="visible" :title="t('siteStats.title', { name: title })" width="720px" :fullscreen="isMobile" append-to-body @closed="stop">
    <div class="range">
      <el-radio-group v-model="minutes" size="small" @change="load">
        <el-radio-button :value="60">{{ t('siteStats.hour1') }}</el-radio-button>
        <el-radio-button :value="360">{{ t('siteStats.hour6') }}</el-radio-button>
        <el-radio-button :value="1440">{{ t('siteStats.hour24') }}</el-radio-button>
      </el-radio-group>
      <span class="meta">{{ t('siteStats.rangeHelp') }}</span>
    </div>
    <template v-if="data">
      <div class="kpis">
        <div class="kpi"><div class="v">{{ total.toLocaleString() }}</div><div class="l">{{ t('siteStats.requests') }}</div></div>
        <div class="kpi"><div class="v">{{ formatBytes(data.bytesOut) }}</div><div class="l">{{ t('siteStats.bytesOut') }}</div></div>
        <div class="kpi"><div class="v">{{ data.status['4xx'] }}</div><div class="l">4xx</div></div>
        <div class="kpi"><div class="v" :class="{ bad: data.status['5xx'] > 0 }">{{ data.status['5xx'] }}</div><div class="l">5xx</div></div>
      </div>
      <div class="chart-title">{{ t('siteStats.requests') }}</div>
      <Sparkline :series="[{ values: data.series.map((p) => p.requests), color: 'var(--el-color-primary)' }]" :capacity="data.series.length" :height="70" />
      <div class="chart-title">{{ t('siteStats.errors') }}</div>
      <Sparkline
        :series="[
          { values: data.series.map((p) => p.s4xx), color: 'var(--el-color-warning)' },
          { values: data.series.map((p) => p.s5xx), color: 'var(--el-color-danger)' },
        ]"
        :capacity="data.series.length"
        :height="50"
      />
      <!-- 首字节延迟：每分钟一个 P50 / P95 估算值，没有可统计响应的分钟为 0 -->
      <div class="chart-title">
        {{ t('siteStats.latency') }}
        <span class="meta">{{ t('siteStats.latencyPeak', { p95: formatMs(peakP95) }) }}</span>
      </div>
      <Sparkline
        :series="[
          { values: data.series.map((p) => p.p50 ?? 0), color: 'var(--el-color-primary)' },
          { values: data.series.map((p) => p.p95 ?? 0), color: 'var(--el-color-warning)' },
        ]"
        :capacity="data.series.length"
        :height="50"
      />
      <div class="meta">{{ t('siteStats.latencyHelp') }}</div>
      <div v-if="data.upstreams?.length" class="chart-title">{{ t('siteStats.upstreams') }}</div>
      <!-- 同一地址可能同时出现在站点后端与分路径、按用户分流的后端中，按位置区分 -->
      <div v-for="(u, i) in data.upstreams ?? []" :key="i" class="up">
        <StatusBadge :type="u.healthy ? 'running' : 'error'" :text="u.healthy ? t('siteStats.healthy') : t('siteStats.unavailable')" />
        <span class="mono">{{ u.url }}</span>
        <span class="meta">
          <template v-if="u.scope === 'location'">{{ t('siteStats.scopeLocation', { m: u.match }) }} · </template>
          <template v-else-if="u.scope === 'user'">{{ t('siteStats.scopeUser', { m: u.match }) }} · </template>
          {{ t('siteStats.upstreamMeta', { weight: u.weight, conns: u.conns }) }} · {{ t('siteStats.upstreamRequests', { n: u.requests ?? 0 }) }}<template v-if="u.backup"> · {{ t('siteStats.backup') }}</template>
        </span>
      </div>
      <template v-if="blockedTotal > 0">
        <div class="chart-title">{{ t('siteStats.blocked') }}</div>
        <div class="blocked">
          <span v-for="b in blockKinds" :key="b.key" class="blocked-item">{{ t(b.label) }} <b>{{ data.blocked?.[b.key] ?? 0 }}</b></span>
        </div>
      </template>
      <div class="tops">
        <div>
          <div class="chart-title">{{ t('siteStats.topIps') }}</div>
          <div v-for="ip in data.topIps" :key="ip.key" class="top"><span class="mono">{{ ip.key }}</span><span>{{ ip.count }}</span></div>
          <div v-if="!data.topIps.length" class="meta">{{ t('siteStats.noData') }}</div>
        </div>
        <div>
          <div class="chart-title">{{ t('siteStats.topCountries') }}</div>
          <div v-for="c in data.topCountries" :key="c.key" class="top"><span>{{ c.key }}</span><span>{{ c.count }}</span></div>
          <div v-if="!data.topCountries.length" class="meta">{{ t('siteStats.noDataGeo') }}</div>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { api } from '../api'
import Sparkline from './Sparkline.vue'
import StatusBadge from './StatusBadge.vue'
import { useFormLayout } from '../composables/useIsMobile'

interface View {
  // p50、p95 为每分钟首字节延迟的估算分位数（毫秒）
  series: { t: number; requests: number; bytes: number; s4xx: number; s5xx: number; p50?: number; p95?: number }[]
  status: Record<string, number>
  bytesOut: number
  topIps: { key: string; count: number }[]
  topCountries: { key: string; count: number }[]
  upstreams?: { url: string; weight: number; backup: boolean; healthy: boolean; conns: number; scope?: string; match?: string; requests?: number }[]
  // blocked 为按原因分的累计拦截次数
  blocked?: Record<string, number>
}

const { t } = useI18n()
const { isMobile } = useFormLayout()
const visible = ref(false)
const title = ref('')
const minutes = ref(60)
const data = ref<View>()
let target = { listener: '', site: '' }
let timer: number | undefined

const total = computed(() => (data.value?.series ?? []).reduce((n, p) => n + p.requests, 0))
// 拦截原因的展示顺序
const blockKinds = [
  { key: 'ban', label: 'siteStats.block_ban' },
  { key: 'ip', label: 'siteStats.block_ip' },
  { key: 'country', label: 'siteStats.block_country' },
  { key: 'rateLimit', label: 'siteStats.block_rateLimit' },
  { key: 'waf', label: 'siteStats.block_waf' },
]
const peakP95 = computed(() => Math.max(0, ...(data.value?.series ?? []).map((p) => p.p95 ?? 0)))

function formatMs(v: number) {
  if (!v) {
    return '-'
  }
  return v >= 1000 ? `${(v / 1000).toFixed(2)} s` : `${v.toFixed(0)} ms`
}

const blockedTotal = computed(() => Object.values(data.value?.blocked ?? {}).reduce((n, v) => n + v, 0))

function formatBytes(n: number) {
  if (n < 1024) {
    return `${n} B`
  }
  if (n < 1024 ** 2) {
    return `${(n / 1024).toFixed(1)} KB`
  }
  if (n < 1024 ** 3) {
    return `${(n / 1024 ** 2).toFixed(1)} MB`
  }
  return `${(n / 1024 ** 3).toFixed(2)} GB`
}

async function load() {
  try {
    data.value = await api.get<View>(`/api/webservice/stats?listener=${encodeURIComponent(target.listener)}&site=${encodeURIComponent(target.site)}&minutes=${minutes.value}`)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

function stop() {
  window.clearInterval(timer)
}

// 对话框开着时离开页面组件直接卸载，停止定时刷新
onBeforeUnmount(stop)

function open(listener: string, site: string, name: string) {
  target = { listener, site }
  title.value = name
  data.value = undefined
  visible.value = true
  load()
  stop()
  timer = window.setInterval(load, 10000)
}

defineExpose({ open })
</script>

<style scoped>
.range {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.kpis {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
  margin-bottom: 8px;
}
.kpi {
  background: var(--el-fill-color-light);
  border-radius: 8px;
  padding: 10px 12px;
}
.kpi .v {
  font-size: 18px;
  font-weight: 600;
}
.kpi .v.bad {
  color: var(--el-color-danger);
}
.kpi .l {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.chart-title {
  margin: 12px 0 4px;
  font-size: 13px;
  font-weight: 600;
}
.up {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 3px 0;
}
.tops {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
}
.top {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  padding: 2px 0;
}
.blocked {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.blocked-item b {
  color: var(--el-color-warning);
  font-variant-numeric: tabular-nums;
}
</style>
