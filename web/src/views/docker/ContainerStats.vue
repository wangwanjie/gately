<template>
  <div class="stats">
    <div class="tiles">
      <div class="tile">
        <div class="t-label">CPU</div>
        <div class="t-value">{{ cur ? cur.cpuPercent.toFixed(1) : '-' }}<small>%</small></div>
        <Sparkline :series="[{ values: cpu, color: 'var(--el-color-primary)' }]" :height="40" :capacity="CAP" :max="cpuMax" />
      </div>
      <div class="tile">
        <div class="t-label">{{ t('docker.container.stats.memory') }}</div>
        <div class="t-value">{{ cur ? formatBytes(cur.memUsage) : '-' }}<small v-if="cur && cur.memLimit"> / {{ formatBytes(cur.memLimit) }}</small></div>
        <Sparkline :series="[{ values: mem, color: 'var(--g-chart-2)' }]" :height="40" :capacity="CAP" :max="100" />
        <div class="t-sub">{{ cur ? cur.memPercent.toFixed(1) : '-' }}%</div>
      </div>
      <div class="tile">
        <div class="t-label">{{ t('docker.container.stats.netTitle') }}</div>
        <div class="t-value sm">↓ {{ rate(rxRate) }}　↑ {{ rate(txRate) }}</div>
        <Sparkline
          :series="[
            { values: rx, color: 'var(--el-color-primary)' },
            { values: tx, color: 'var(--g-chart-2)' },
          ]"
          :height="40"
          :capacity="CAP"
        />
        <div class="t-sub">{{ t('docker.container.stats.netTotal', { rx: cur ? formatBytes(cur.netRx) : '-', tx: cur ? formatBytes(cur.netTx) : '-' }) }}</div>
      </div>
      <div class="tile">
        <div class="t-label">{{ t('docker.container.stats.ioTitle') }}</div>
        <div class="t-value sm">{{ t('docker.container.stats.ioRate', { read: rate(brRate), write: rate(bwRate) }) }}</div>
        <Sparkline
          :series="[
            { values: br, color: 'var(--el-color-primary)' },
            { values: bw, color: 'var(--g-chart-2)' },
          ]"
          :height="40"
          :capacity="CAP"
        />
        <div class="t-sub">{{ t('docker.container.stats.ioTotal', { read: cur ? formatBytes(cur.blockRead) : '-', write: cur ? formatBytes(cur.blockWrite) : '-', pids: cur?.pids ?? '-' }) }}</div>
      </div>
    </div>

    <div class="g-card proc">
      <div class="g-card-head">
        <div class="g-card-title"><Cpu :size="16" />{{ t('docker.container.stats.processes') }}</div>
        <el-button size="small" :icon="RefreshCw" @click="loadProcs">{{ t('common.refresh') }}</el-button>
      </div>
      <div class="g-card-body">
        <div v-if="!isMobile" class="tbl-wrap">
          <table class="tbl mono">
            <thead>
              <tr><th v-for="title in procs.titles" :key="title">{{ title }}</th></tr>
            </thead>
            <tbody>
              <tr v-for="(p, i) in procs.processes" :key="i"><td v-for="(c, j) in p" :key="j">{{ c }}</td></tr>
            </tbody>
          </table>
        </div>
        <div v-else class="pcards">
          <div v-for="(p, i) in procs.processes" :key="i" class="pcard">
            <div v-for="(c, j) in p" :key="j" class="pline">
              <span class="pk">{{ procs.titles[j] }}</span><span class="pv mono">{{ c }}</span>
            </div>
          </div>
        </div>
        <el-empty v-if="!procs.processes.length" :description="t('docker.container.stats.noProcesses')" :image-size="50" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Cpu, RefreshCw } from 'lucide-vue-next'
import { api } from '../../api'
import Sparkline from '../../components/Sparkline.vue'
import { useIsMobile } from '../../composables/useIsMobile'
import { usePolling } from '../../composables/usePolling'
import { formatBytes } from './format'
import type { ContainerStats } from './types'

const { t } = useI18n()

const props = defineProps<{ containerId: string }>()
const isMobile = useIsMobile()
const CAP = 60

const cur = ref<ContainerStats>()
const cpu = ref<number[]>([])
const mem = ref<number[]>([])
const rx = ref<number[]>([])
const tx = ref<number[]>([])
const br = ref<number[]>([])
const bw = ref<number[]>([])
const rxRate = ref<number>()
const txRate = ref<number>()
const brRate = ref<number>()
const bwRate = ref<number>()
const procs = ref<{ titles: string[]; processes: string[][] }>({ titles: [], processes: [] })
let prev: { at: number; s: ContainerStats } | undefined
// docker stats 单次采样本身要 1~2 秒：用统一轮询，上一次未返回时跳过本轮，标签页在后台时暂停
const { start: startPolling } = usePolling(() => sample(), 2000)
let warned = false

// 多核 CPU 占用可能超过 100%，纵轴上限随数据放大
const cpuMax = computed(() => Math.max(100, ...cpu.value) * 1.1)

function rate(v: number | undefined): string {
  return v === undefined ? '-' : `${formatBytes(Math.round(v))}/s`
}

function push(arr: number[], v: number) {
  arr.push(v)
  if (arr.length > CAP) {
    arr.shift()
  }
}

async function sample() {
  try {
    const s = await api.get<ContainerStats>(`/api/docker/containers/${props.containerId}/stats`)
    const now = Date.now()
    cur.value = s
    push(cpu.value, s.cpuPercent)
    push(mem.value, s.memPercent)
    if (prev) {
      const dt = Math.max(0.5, (now - prev.at) / 1000)
      const d = (a: number, b: number) => Math.max(0, a - b) / dt
      rxRate.value = d(s.netRx, prev.s.netRx)
      txRate.value = d(s.netTx, prev.s.netTx)
      brRate.value = d(s.blockRead, prev.s.blockRead)
      bwRate.value = d(s.blockWrite, prev.s.blockWrite)
      push(rx.value, rxRate.value)
      push(tx.value, txRate.value)
      push(br.value, brRate.value)
      push(bw.value, bwRate.value)
    }
    prev = { at: now, s }
  } catch (e) {
    if (!warned) {
      warned = true
      ElMessage.error((e as Error).message)
    }
  }
}

async function loadProcs() {
  try {
    const r = await api.get<{ titles: string[]; processes: string[][] }>(`/api/docker/containers/${props.containerId}/processes`)
    procs.value = { titles: r.titles ?? [], processes: r.processes ?? [] }
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(() => {
  sample()
  loadProcs()
  startPolling()
})
</script>

<style scoped>
.tiles {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.tile {
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  padding: 12px 14px;
  min-width: 0;
}
.t-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.t-value {
  font-size: 22px;
  font-weight: 700;
  margin: 2px 0 6px;
  font-variant-numeric: tabular-nums;
  word-break: break-all;
}
.t-value.sm {
  font-size: 15px;
  line-height: 1.8;
}
.t-value small {
  font-size: 12px;
  font-weight: 400;
  color: var(--el-text-color-secondary);
}
.t-sub {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-all;
}
.tbl-wrap {
  overflow-x: auto;
}
.tbl {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}
.tbl th,
.tbl td {
  text-align: left;
  padding: 6px 10px;
  border-bottom: 1px solid var(--g-border);
  white-space: nowrap;
}
.tbl th {
  color: var(--el-text-color-secondary);
  font-weight: 500;
}
.pcards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.pcard {
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius-sm);
  padding: 8px 10px;
}
.pline {
  display: flex;
  gap: 8px;
  font-size: 12px;
  line-height: 1.7;
}
.pk {
  width: 64px;
  flex-shrink: 0;
  color: var(--el-text-color-secondary);
}
.pv {
  min-width: 0;
  word-break: break-all;
}
@media (max-width: 767.98px) {
  .tiles {
    grid-template-columns: 1fr;
  }
}
</style>
