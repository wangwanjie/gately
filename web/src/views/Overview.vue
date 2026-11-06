<template>
  <div class="overview">
    <!-- ============ 主机信息 ============ -->
    <section class="hero">
      <div class="hero-grid" aria-hidden="true" />
      <div class="hero-main">
        <div class="hero-icon"><Server :size="26" :stroke-width="1.8" /></div>
        <div class="hero-text">
          <div class="hero-name">
            {{ host?.hostname || '-' }}
            <span v-if="host?.inContainer" class="chip"><Container :size="12" />{{ t('overview.inContainer') }}</span>
          </div>
          <div class="hero-sub">{{ platformLine }}</div>
          <div class="hero-meta">
            <span v-if="host?.bootTime"><Power :size="13" />{{ t('overview.systemUptime', { duration: formatDuration(now / 1000 - host.bootTime) }) }}</span>
            <span v-if="status"><Clock :size="13" />{{ status.timeZone }}</span>
            <span v-if="host"><span class="live-dot" /><RefreshCw :size="13" />{{ t('overview.updatedAt', { time: new Date(host.sampledAt).toLocaleTimeString() }) }}</span>
          </div>
        </div>
      </div>
      <div class="hero-side">
        <div class="hero-kv">
          <span class="k">Gately</span>
          <span class="v mono" :title="status ? `${status.commit} · ${status.buildDate}` : ''">{{ status?.version ?? '-' }}</span>
        </div>
        <div class="hero-kv">
          <span class="k">{{ t('overview.uptime') }}</span>
          <span class="v">{{ status ? formatDuration((now - new Date(status.startTime).getTime()) / 1000) : '-' }}</span>
        </div>
        <div class="hero-kv">
          <span class="k">{{ t('overview.service') }}</span>
          <StatusBadge :type="health.type" :text="health.text" />
        </div>
        <el-button size="small" round :icon="Stethoscope" @click="selfCheck?.open()">{{ t('selfcheck.run') }}</el-button>
      </div>
    </section>

    <!-- ============ 资源仪表 ============ -->
    <section class="gauges">
      <div class="gauge-card">
        <div class="gauge-head"><Cpu :size="15" />CPU</div>
        <div class="gauge-body">
          <RingGauge :value="host?.cpu.usage" />
          <div class="gauge-info">
            <div class="gi-main" :title="host?.cpu.model">{{ host?.cpu.model || t('overview.processor') }}</div>
            <div class="gi-row"><span>{{ t('overview.cores') }}</span><b>{{ host?.cpu.cores ?? '-' }}</b></div>
            <div class="gi-row"><span>{{ t('overview.load') }}</span><b class="mono">{{ loadText }}</b></div>
            <div class="gi-row"><span>{{ t('overview.peakAvg') }}</span><b>{{ peakAvg(hist.cpu) }}</b></div>
          </div>
        </div>
        <Sparkline :series="[{ values: hist.cpu, color: 'var(--el-color-primary)' }]" :max="100" :capacity="HISTORY" :height="36" class="gauge-spark" />
      </div>

      <div class="gauge-card">
        <div class="gauge-head"><MemoryStick :size="15" />{{ t('overview.memory') }}</div>
        <div class="gauge-body">
          <RingGauge :value="memPercent" />
          <div class="gauge-info">
            <div class="gi-main">{{ mem ? `${formatBytes(mem.used)} / ${formatBytes(mem.total)}` : '-' }}</div>
            <div class="gi-row"><span>{{ t('overview.available') }}</span><b>{{ mem ? formatBytes(mem.available) : '-' }}</b></div>
            <div class="gi-row">
              <span>{{ t('overview.swap') }}</span><b>{{ mem && mem.swapTotal ? `${formatBytes(mem.swapUsed)} / ${formatBytes(mem.swapTotal)}` : t('overview.swapOff') }}</b>
            </div>
            <div class="gi-row"><span>{{ t('overview.peakAvg') }}</span><b>{{ peakAvg(hist.mem) }}</b></div>
          </div>
        </div>
        <Sparkline :series="[{ values: hist.mem, color: 'var(--g-chart-2)' }]" :max="100" :capacity="HISTORY" :height="36" class="gauge-spark" />
      </div>

      <div class="gauge-card">
        <div class="gauge-head"><HardDrive :size="15" />{{ t('overview.disk') }}</div>
        <div class="gauge-body">
          <RingGauge :value="diskPercent" />
          <div class="gauge-info">
            <div class="gi-main">{{ disk ? `${formatBytes(disk.used)} / ${formatBytes(disk.total)}` : '-' }}</div>
            <div class="gi-row"><span>{{ t('overview.free') }}</span><b>{{ disk ? formatBytes(disk.free) : '-' }}</b></div>
            <div class="gi-row"><span>{{ t('overview.location') }}</span><b class="mono ellipsis" :title="disk?.path">{{ disk?.path ?? '-' }}</b></div>
          </div>
        </div>
        <div class="disk-bar">
          <div class="disk-fill" :class="diskLevel" :style="{ width: (diskPercent ?? 0) + '%' }" />
        </div>
        <div class="disk-note">{{ t('overview.diskNote') }}</div>
      </div>

      <div class="gauge-card">
        <div class="gauge-head"><Activity :size="15" />{{ t('overview.network') }}</div>
        <div class="net-rates">
          <div class="net-rate">
            <span class="net-dot rx" /><ArrowDown :size="14" />
            <div>
              <div class="net-val">{{ formatRate(netTotal.rx) }}</div>
              <div class="net-sub">{{ t('overview.cumulative', { size: formatBytes(netTotal.rxBytes) }) }}</div>
            </div>
          </div>
          <div class="net-rate">
            <span class="net-dot tx" /><ArrowUp :size="14" />
            <div>
              <div class="net-val">{{ formatRate(netTotal.tx) }}</div>
              <div class="net-sub">{{ t('overview.cumulative', { size: formatBytes(netTotal.txBytes) }) }}</div>
            </div>
          </div>
        </div>
        <div class="net-peak">{{ t('overview.peakRate', { rx: formatRate(Math.max(0, ...hist.rx)), tx: formatRate(Math.max(0, ...hist.tx)) }) }}</div>
        <Sparkline
          :series="[
            { values: hist.rx, color: 'var(--el-color-primary)' },
            { values: hist.tx, color: 'var(--g-chart-2)' },
          ]"
          :capacity="HISTORY"
          :height="56"
          class="gauge-spark"
        />
      </div>
    </section>

    <!-- ============ 网卡与服务状态 ============ -->
    <section class="cols">
      <div class="col">
      <div class="g-card ifaces-card">
        <div class="g-card-head">
          <div class="g-card-title"><Network :size="16" />{{ t('overview.interfaces') }}</div>
          <span class="g-card-desc">{{ host ? t('overview.count', { count: host.interfaces.length }) : '' }}</span>
        </div>
        <div v-if="publicIPs.length" class="public-ips">
          <div v-for="p in publicIPs" :key="p.label" class="public-ip">
            <span class="pip-label"><Globe :size="13" />{{ t('overview.publicIp', { label: p.label }) }}</span>
            <span class="mono copyable" :title="t('overview.clickToCopy')" @click="copy(p.value, $event)">{{ p.value }}</span>
          </div>
        </div>
        <div class="ifaces">
          <div v-for="i in host?.interfaces ?? []" :key="i.name" class="iface">
            <div class="iface-head">
              <span class="iface-name mono">{{ i.name }}</span>
              <span v-if="i.mac" class="iface-mac mono">{{ i.mac }}</span>
              <span class="iface-rate">
                <span><ArrowDown :size="12" />{{ formatRate(i.rxRate) }}</span>
                <span><ArrowUp :size="12" />{{ formatRate(i.txRate) }}</span>
              </span>
            </div>
            <div class="iface-addrs">
              <span v-for="a in i.ipv4" :key="a" class="addr v4 mono copyable" :title="t('overview.clickToCopy')" @click="copy(a.split('/')[0], $event)">{{ a }}</span>
              <span v-for="a in visibleV6(i.ipv6)" :key="a" class="addr v6 mono copyable" :title="t('overview.clickToCopy')" @click="copy(a.split('/')[0], $event)">{{ a }}</span>
              <el-tooltip v-if="i.ipv6.length > visibleV6(i.ipv6).length" placement="top" :show-after="200">
                <template #content>
                  <div v-for="a in i.ipv6" :key="a" class="mono">{{ a }}</div>
                </template>
                <span class="addr more">{{ t('overview.moreIpv6', { count: i.ipv6.length - visibleV6(i.ipv6).length }) }}</span>
              </el-tooltip>
            </div>
            <div class="iface-foot">
              <span>MTU {{ i.mtu }}</span>
              <span>{{ t('overview.received', { size: formatBytes(i.rxBytes) }) }}</span>
              <span>{{ t('overview.sent', { size: formatBytes(i.txBytes) }) }}</span>
            </div>
          </div>
          <div v-if="host && !host.interfaces.length" class="empty">{{ t('overview.noInterfaces') }}</div>
        </div>
      </div>

      <!-- ============ Gately 进程 ============ -->
      <section class="g-card proc grow">
        <div class="g-card-head">
          <div class="g-card-title"><Boxes :size="16" />{{ t('overview.process') }}</div>
          <span class="g-card-desc">{{ status ? `${status.os}/${status.arch} · ${status.goVersion}` : '' }}</span>
        </div>
        <div class="proc-grid">
          <div v-for="it in procItems" :key="it.label" class="proc-item" :class="{ wide: it.wide }">
            <div class="proc-label">{{ it.label }}</div>
            <div class="proc-value" :class="{ mono: it.mono }" :title="it.value">{{ it.value }}</div>
          </div>
        </div>
      </section>

      <!-- ============ 托管进程（frpc、tailscaled 等），没有时不显示 ============ -->
      <section v-if="managed.length" class="g-card managed-card">
        <div class="g-card-head">
          <div class="g-card-title"><Layers :size="16" />{{ t('managedProc.title') }}</div>
          <span class="g-card-desc">{{ managedTotal }}</span>
        </div>
        <ManagedProcList :items="managed" :now="now" />
      </section>
      </div>
      <div class="g-card services-card">
        <div class="g-card-head">
          <div class="g-card-title"><HeartPulse :size="16" />{{ t('overview.serviceStatus') }}</div>
          <StatusBadge :type="health.type" :text="health.text" />
        </div>
        <div class="services">
          <router-link v-for="s in services" :key="s.id" :to="s.path" class="service">
            <span class="svc-dot" :class="s.level" />
            <span class="svc-name">{{ s.title }}</span>
            <span class="svc-items">
              <span v-for="it in s.items" :key="it.label" class="svc-item">
                <span class="svc-label">{{ it.label }}</span>
                <span :class="['svc-value', it.level]">{{ it.value }}</span>
              </span>
            </span>
          </router-link>
          <div v-if="status && !services.length" class="empty">{{ t('overview.noServices') }}</div>
        </div>
      </div>
    </section>

    <SelfCheck ref="selfCheck" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { copyText } from '../composables/useClipboard'
import {
  Activity,
  ArrowDown,
  ArrowUp,
  Boxes,
  Container,
  Cpu,
  Globe,
  Clock,
  HardDrive,
  HeartPulse,
  Layers,
  MemoryStick,
  Network,
  Power,
  RefreshCw,
  Server,
  Stethoscope,
} from 'lucide-vue-next'
import { api, formatBytes, formatDuration, formatRate, type Status } from '../api'
import RingGauge from '../components/RingGauge.vue'
import SelfCheck from '../components/SelfCheck.vue'
import Sparkline from '../components/Sparkline.vue'
import ManagedProcList from '../components/ManagedProcList.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import { navByModule } from '../nav'

const { t } = useI18n()

// 轮询间隔与趋势图保留的采样点数（约 3 分钟）
const INTERVAL = 3000
const HISTORY = 60

const selfCheck = ref<InstanceType<typeof SelfCheck>>()
const status = ref<Status>()
const now = ref(Date.now())
const hist = reactive({ cpu: [] as number[], mem: [] as number[], rx: [] as number[], tx: [] as number[] })
let timer: number | undefined

const host = computed(() => status.value?.host)
const mem = computed(() => host.value?.memory)
const disk = computed(() => host.value?.disk)
const memPercent = computed(() => (mem.value ? (mem.value.used / mem.value.total) * 100 : undefined))
const diskPercent = computed(() => (disk.value ? (disk.value.used / disk.value.total) * 100 : undefined))
const diskLevel = computed(() => {
  const v = diskPercent.value ?? 0
  if (v >= 90) {
    return 'danger'
  }
  return v >= 75 ? 'warning' : ''
})

const platformLine = computed(() => {
  const h = host.value
  if (!h) {
    return ''
  }
  const parts = [h.platform, h.kernel ? `${h.os === 'linux' ? 'Linux' : h.os} ${h.kernel}` : h.os, h.arch]
  return parts.filter(Boolean).join(' · ')
})

const loadText = computed(() => {
  const l = host.value?.cpu.load
  return l && l.length ? l.map((v) => v.toFixed(2)).join(' / ') : '-'
})

// 所有展示网卡的速率与累计流量之和
const netTotal = computed(() => {
  const total = { rx: undefined as number | undefined, tx: undefined as number | undefined, rxBytes: 0, txBytes: 0 }
  for (const i of host.value?.interfaces ?? []) {
    total.rxBytes += i.rxBytes
    total.txBytes += i.txBytes
    if (i.rxRate !== undefined) {
      total.rx = (total.rx ?? 0) + i.rxRate
    }
    if (i.txRate !== undefined) {
      total.tx = (total.tx ?? 0) + i.txRate
    }
  }
  return total
})

// 公网地址取自动态域名模块最近一次获取到的 IP，不额外访问外部服务
const publicIPs = computed(() => {
  const ddns = status.value?.modules.find((m) => m.id === 'ddns')
  return (ddns?.summary ?? []).filter((it) => it.label === 'IPv4' || it.label === 'IPv6')
})

// 每个有摘要的模块一行，状态取摘要中最严重的等级；公网 IP 已在网卡区域展示，这里略去
const rank: Record<string, number> = { ok: 1, warn: 2, error: 3 }
const services = computed(() => {
  const out: { id: string; title: string; path: string; level: string; items: { label: string; value: string; level?: string }[] }[] = []
  for (const m of status.value?.modules ?? []) {
    const nav = navByModule(m.id)
    const items = (m.summary ?? []).filter((it) => it.label !== 'IPv4' && it.label !== 'IPv6')
    // 已关闭的模块不展示
    if (!nav || !m.enabled || !items.length) {
      continue
    }
    let level = 'ok'
    for (const it of items) {
      if (it.level && (rank[it.level] ?? 0) > (rank[level] ?? 0)) {
        level = it.level
      }
    }
    out.push({ id: m.id, title: t(nav.titleKey), path: nav.path, level, items })
  }
  return out
})

const health = computed<{ type: StatusType; text: string }>(() => {
  if (!status.value) {
    return { type: 'pending', text: t('overview.loading') }
  }
  const bad = services.value.filter((s) => s.level === 'error').length
  const warn = services.value.filter((s) => s.level === 'warn').length
  if (bad) {
    return { type: 'error', text: t('overview.abnormal', { count: bad }) }
  }
  if (warn) {
    return { type: 'warn', text: t('overview.attention', { count: warn }) }
  }
  return { type: 'ok', text: t('overview.allOk') }
})

const procItems = computed(() => {
  const s = status.value
  const p = s?.host.process
  if (!s || !p) {
    return []
  }
  const items = [
    { label: t('overview.proc.version'), value: s.version, mono: true },
    { label: t('overview.proc.commit'), value: s.commit ? s.commit.slice(0, 12) : '-', mono: true },
    { label: t('overview.proc.buildTime'), value: formatTime(s.buildDate) },
    { label: t('overview.proc.startTime'), value: new Date(s.startTime).toLocaleString() },
    { label: t('overview.proc.pid'), value: String(p.pid), mono: true },
    { label: t('overview.proc.procCpu'), value: p.cpu !== undefined ? p.cpu.toFixed(1) + '%' : '-' },
    { label: t('overview.proc.rss'), value: p.rss ? formatBytes(p.rss) : '-' },
    { label: t('overview.proc.goHeap'), value: `${formatBytes(p.heapAlloc)} / ${formatBytes(p.heapSys)}` },
    { label: t('overview.proc.goroutines'), value: String(p.goroutines) },
    { label: t('overview.proc.threads'), value: p.threads ? String(p.threads) : '-' },
    { label: t('overview.proc.fds'), value: p.fds ? String(p.fds) : '-' },
    { label: t('overview.proc.gcCount'), value: String(p.numGC) },
    { label: t('overview.proc.configDir'), value: s.configDir, mono: true, wide: true },
  ]
  return items as { label: string; value: string; mono?: boolean; wide?: boolean }[]
})

// 托管进程及合计：CPU 只累加取得到的值，都取不到时显示 -
const managed = computed(() => host.value?.managed ?? [])
const managedTotal = computed(() => {
  let cpu: number | undefined
  let rss = 0
  for (const p of managed.value) {
    if (p.cpu != null) {
      cpu = (cpu ?? 0) + p.cpu
    }
    rss += p.rss ?? 0
  }
  return t('managedProc.total', {
    count: managed.value.length,
    cpu: cpu === undefined ? '-' : cpu.toFixed(cpu >= 10 ? 0 : 1) + '%',
    rss: rss ? formatBytes(rss) : '-',
  })
})

// 构建时间可能是 unknown 等非日期字符串，无法解析时原样显示
function formatTime(v: string) {
  if (!v) {
    return '-'
  }
  const d = new Date(v)
  return isNaN(d.getTime()) ? v : d.toLocaleString()
}

// 趋势序列的峰值与均值，没有采样时显示 -
function peakAvg(arr: number[]) {
  if (!arr.length) {
    return '-'
  }
  const avg = arr.reduce((a, b) => a + b, 0) / arr.length
  return `${Math.max(...arr).toFixed(0)}% / ${avg.toFixed(0)}%`
}

function visibleV6(list: string[]) {
  // 优先展示全局地址，链路本地地址收进“更多”
  return list.filter((a) => !a.startsWith('fe80:')).slice(0, 2)
}

function push(arr: number[], v: number | undefined) {
  if (v === undefined) {
    return
  }
  arr.push(v)
  if (arr.length > HISTORY) {
    arr.shift()
  }
}

// 点击地址复制；自动复制失败时选中被点击的地址供手动复制
function copy(text: string, e: MouseEvent) {
  copyText(text, { el: e.currentTarget as HTMLElement })
}

async function load() {
  try {
    status.value = await api.get<Status>('/api/status')
    now.value = Date.now()
    push(hist.cpu, host.value?.cpu.usage)
    push(hist.mem, memPercent.value)
    push(hist.rx, netTotal.value.rx)
    push(hist.tx, netTotal.value.tx)
  } catch {
    // 下一轮重试
  }
}

// 页面不可见时暂停轮询，切回时立即刷新
function onVisibility() {
  if (document.hidden) {
    window.clearInterval(timer)
    timer = undefined
  } else if (timer === undefined) {
    load()
    timer = window.setInterval(load, INTERVAL)
  }
}

onMounted(() => {
  load()
  timer = window.setInterval(load, INTERVAL)
  document.addEventListener('visibilitychange', onVisibility)
})
onBeforeUnmount(() => {
  window.clearInterval(timer)
  document.removeEventListener('visibilitychange', onVisibility)
})
</script>

<style scoped>
.overview {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.overview > .g-card + .g-card {
  margin-top: 0;
}

/* ---------- 主机信息 ---------- */
.hero {
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  flex-wrap: wrap;
  padding: 22px 24px;
  border-radius: 14px;
  border: 1px solid var(--g-border);
  background:
    radial-gradient(120% 140% at 0% 0%, var(--el-color-primary-light-9) 0%, transparent 55%),
    radial-gradient(80% 120% at 100% 100%, color-mix(in srgb, var(--g-chart-2) 10%, transparent) 0%, transparent 60%),
    var(--el-bg-color);
}
/* 科技感网格底纹，向右下渐隐 */
.hero-grid {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background-image:
    linear-gradient(var(--g-border) 1px, transparent 1px),
    linear-gradient(90deg, var(--g-border) 1px, transparent 1px);
  background-size: 28px 28px;
  opacity: 0.55;
  -webkit-mask-image: radial-gradient(70% 120% at 100% 0%, #000 0%, transparent 70%);
  mask-image: radial-gradient(70% 120% at 100% 0%, #000 0%, transparent 70%);
}
.hero-main,
.hero-side {
  position: relative;
}
.live-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--el-color-success);
  animation: g-pulse 2s ease-out infinite;
}
@keyframes g-pulse {
  0% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--el-color-success) 55%, transparent);
  }
  100% {
    box-shadow: 0 0 0 7px transparent;
  }
}
.hero-main {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
}
.hero-icon {
  width: 56px;
  height: 56px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  background: linear-gradient(135deg, var(--el-color-primary), var(--el-color-primary-dark-2));
  box-shadow: 0 6px 16px -6px color-mix(in srgb, var(--el-color-primary) 60%, transparent);
  flex-shrink: 0;
}
.hero-text {
  min-width: 0;
}
.hero-name {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  font-size: 22px;
  font-weight: 700;
  line-height: 1.25;
  word-break: break-all;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 22px;
  padding: 0 8px;
  font-size: 12px;
  font-weight: 500;
  border-radius: 999px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.hero-sub {
  margin-top: 4px;
  font-size: 13px;
  color: var(--el-text-color-regular);
}
.hero-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
  margin-top: 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.hero-meta span {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.hero-side {
  display: flex;
  gap: 28px;
  flex-wrap: wrap;
}
.hero-kv {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.hero-kv .k {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.hero-kv .v {
  font-size: 16px;
  font-weight: 600;
  line-height: 22px;
}

/* ---------- 资源仪表 ---------- */
.gauges {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}
.gauge-card {
  position: relative;
  display: flex;
  flex-direction: column;
  padding: 16px 18px 0;
  background: var(--el-bg-color);
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  overflow: hidden;
  min-width: 0;
}
/* 卡片顶部渐变高光线 */
.gauge-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 2px;
  background: linear-gradient(90deg, var(--el-color-primary), var(--g-chart-2), transparent);
  opacity: 0.75;
}
.net-peak {
  margin-top: 10px;
  font-size: 12px;
  color: var(--el-text-color-placeholder);
  font-variant-numeric: tabular-nums;
}
.gauge-head {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
}
.gauge-body {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-top: 14px;
}
.gauge-info {
  flex: 1;
  min-width: 0;
  font-size: 12px;
}
.gi-main {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 6px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.gi-row {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  line-height: 1.9;
  color: var(--el-text-color-secondary);
}
.gi-row span {
  flex-shrink: 0;
  white-space: nowrap;
}
.gi-row b {
  font-weight: 500;
  color: var(--el-text-color-primary);
  min-width: 0;
  text-align: right;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.gauge-spark {
  margin: auto -18px 0;
  padding-top: 12px;
  width: calc(100% + 36px);
}
.disk-bar {
  height: 6px;
  margin-top: 18px;
  border-radius: 999px;
  background: var(--el-fill-color);
  overflow: hidden;
}
.disk-fill {
  height: 100%;
  border-radius: inherit;
  background: var(--el-color-primary);
  transition: width 0.6s ease;
}
.disk-fill.warning {
  background: var(--el-color-warning);
}
.disk-fill.danger {
  background: var(--el-color-danger);
}
.disk-note {
  margin: 8px 0 16px;
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}
.net-rates {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 14px;
}
.net-rate {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--el-text-color-secondary);
}
.net-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 2px;
}
.net-dot.rx {
  background: var(--el-color-primary);
}
.net-dot.tx {
  background: var(--g-chart-2);
}
.net-val {
  font-size: 18px;
  font-weight: 700;
  color: var(--el-text-color-primary);
  font-variant-numeric: tabular-nums;
  line-height: 1.2;
}
.net-sub {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}

/* ---------- 网卡与服务 ---------- */
.cols {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  gap: 16px;
  align-items: stretch;
}
/* 左列：网卡 + 进程信息，右列：服务状态；两列等高，空白由卡片内部均分填充 */
.col {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}
.col > .g-card + .g-card {
  margin-top: 0;
}
.grow {
  flex: 1;
}
.services-card {
  display: flex;
  flex-direction: column;
}
.public-ips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 24px;
  padding: 12px 16px;
  border-bottom: 1px dashed var(--g-border);
  font-size: 13px;
}
.public-ip {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  word-break: break-all;
}
.public-ip .copyable {
  min-width: 0;
  word-break: break-all;
}
.pip-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--el-text-color-secondary);
  flex-shrink: 0;
}
.ifaces {
  padding: 4px 0;
}
.iface {
  padding: 12px 16px;
}
.iface + .iface {
  border-top: 1px solid var(--el-border-color-lighter);
}
.iface-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.iface-name {
  font-weight: 600;
  font-size: 14px;
}
.iface-mac {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}
.iface-rate {
  margin-left: auto;
  display: inline-flex;
  gap: 12px;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--el-text-color-secondary);
}
.iface-rate span {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}
.iface-addrs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.addr {
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 6px;
  line-height: 18px;
  word-break: break-all;
}
.addr.v4 {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.addr.v6 {
  color: var(--g-chart-2);
  background: color-mix(in srgb, var(--g-chart-2) 10%, transparent);
}
.addr.more {
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
  cursor: default;
}
.addr.copyable:hover {
  text-decoration: none;
  filter: brightness(0.95);
}
.iface-foot {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
  margin-top: 8px;
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}
.services {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 6px;
}
.service {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px;
  border-radius: 8px;
  color: inherit;
  text-decoration: none;
  transition: background 0.15s;
}
.service:hover {
  background: var(--el-fill-color-light);
}
.svc-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  background: var(--el-color-success);
  box-shadow: 0 0 0 3px var(--el-color-success-light-9);
}
.svc-dot.error {
  animation: g-blink 1.2s ease-in-out infinite;
}
@keyframes g-blink {
  50% {
    opacity: 0.35;
  }
}
@media (prefers-reduced-motion: reduce) {
  .live-dot,
  .svc-dot.error {
    animation: none;
  }
}
.svc-dot.warn {
  background: var(--el-color-warning);
  box-shadow: 0 0 0 3px var(--el-color-warning-light-9);
}
.svc-dot.error {
  background: var(--el-color-danger);
  box-shadow: 0 0 0 3px var(--el-color-danger-light-9);
}
.svc-name {
  width: 76px;
  flex-shrink: 0;
  font-weight: 600;
  font-size: 13px;
  line-height: 20px;
}
.svc-items {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 2px 14px;
  font-size: 12px;
  line-height: 20px;
}
.svc-item {
  display: inline-flex;
  gap: 4px;
  min-width: 0;
}
.svc-label {
  color: var(--el-text-color-secondary);
}
.svc-value {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.svc-value.ok {
  color: var(--el-color-success);
}
.svc-value.warn {
  color: var(--el-color-warning);
}
.svc-value.error {
  color: var(--el-color-danger);
}
.empty {
  padding: 24px 16px;
  text-align: center;
  font-size: 13px;
  color: var(--el-text-color-placeholder);
}

/* ---------- 进程信息 ---------- */
.proc.grow {
  display: flex;
  flex-direction: column;
}
.proc-grid {
  flex: 1;
  grid-auto-rows: 1fr;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
  /* 抵消最后一列与最后一行多出的分隔线 */
  margin: 0 -1px -1px 0;
}
.proc-item {
  padding: 12px 16px;
  min-width: 0;
  box-shadow:
    inset -1px 0 0 var(--el-border-color-lighter),
    inset 0 -1px 0 var(--el-border-color-lighter);
}
.proc-item.wide {
  grid-column: span 2;
}
.proc-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.proc-value {
  margin-top: 3px;
  font-size: 14px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.proc {
  overflow: hidden;
}
.managed-card {
  overflow: hidden;
}

@media (max-width: 1280px) {
  .gauges {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 1000px) {
  .cols {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 767.98px) {
  .overview {
    gap: 12px;
  }
  .hero {
    padding: 16px;
    gap: 16px;
  }
  .hero-icon {
    width: 44px;
    height: 44px;
    border-radius: 12px;
  }
  .hero-name {
    font-size: 18px;
  }
  .hero-side {
    width: 100%;
    justify-content: space-between;
    gap: 12px;
    padding-top: 14px;
    border-top: 1px dashed var(--g-border);
  }
  .gauges {
    grid-template-columns: 1fr;
    gap: 12px;
  }
  .proc-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .svc-name {
    width: 64px;
  }
}
</style>
