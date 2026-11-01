<template>
  <!-- Web 服务拓扑视图：互联网 → Gately → 监听 → 站点 / 动作 → 后端 / 去向，每 2 秒刷新 -->
  <div class="topo">
    <div class="topo-bar">
      <div class="ip-filter" :class="{ on: !!ipFilter }">
        <Filter :size="15" class="bar-icon" />
        <el-select
          v-model="ipModel"
          class="ip-select"
          filterable
          allow-create
          default-first-option
          clearable
          :placeholder="t('webTopology.filterPlaceholder')"
          :no-data-text="t('webTopology.ipNone')"
          :filter-method="(q: string) => (ipQuery = q)"
          @change="onIpChange"
          @visible-change="(v: boolean) => !v && (ipQuery = '')"
        >
          <template #label="{ value }">{{ showIP(String(value)) }}</template>
          <el-option v-for="ip in ipOptions" :key="ip.ip" :value="ip.ip" :label="showIP(ip.ip)">
            <span class="mono">{{ showIP(ip.ip) }}</span>
            <span class="opt-meta">{{ ip.active ? t('webTopology.ipActive', { n: ip.active }) : ago(ip.last) }}</span>
          </el-option>
        </el-select>
        <template v-if="ipFilter">
          <span class="filter-state">
            {{ ipInfo ? t('webTopology.filtering', { ip: showIP(ipFilter) }) : t('webTopology.ipInactive', { ip: showIP(ipFilter) }) }}
          </span>
          <el-button size="small" :icon="X" @click="clearIp">{{ t('webTopology.clearFilter') }}</el-button>
        </template>
      </div>
      <div class="bar-right">
        <div v-if="data" class="kpis">
          <span class="kpi" :title="t('webTopology.qpsHelp')"><b>{{ formatQps(data.rate.qps) }}</b> {{ t('webTopology.qpsUnit') }}</span>
          <span class="kpi" :title="t('webTopology.bpsHelp')"><b>{{ formatRate(data.rate.bps) }}</b></span>
          <span class="kpi" :title="t('webTopology.activeHelp')"><b>{{ data.active }}</b> {{ t('webTopology.active') }}</span>
          <span class="kpi" :title="t('webTopology.activeIpsHelp')"><b>{{ data.ipTotal }}</b> IP</span>
        </div>
        <el-tooltip :content="t('webTopology.maskTip')" placement="top">
          <el-button :type="masked ? 'primary' : 'default'" :icon="masked ? EyeOff : Eye" @click="masked = !masked">{{ t('webTopology.mask') }}</el-button>
        </el-tooltip>
        <el-button v-if="listenerKeys.length" :icon="allCollapsed ? ChevronsUpDown : ChevronsDownUp" @click="toggleAll">
          {{ allCollapsed ? t('common.expandAll') : t('common.collapseAll') }}
        </el-button>
      </div>
    </div>

    <div class="topo-main">
      <div ref="canvasEl" class="canvas" @click="select('gately')">
        <div v-if="!data && !loadError" v-loading="true" class="canvas-empty" />
        <div v-else-if="loadError && !data" class="canvas-empty msg">{{ loadError }}</div>
        <template v-else-if="layout">
          <svg
            class="graph"
            :class="{ 'has-focus': !!litSet, 'no-motion': !motion }"
            :viewBox="`0 ${-HEAD} ${layout.width} ${layout.height + HEAD}`"
            :style="{ maxWidth: layout.width + 'px' }"
            role="img"
            :aria-label="t('webTopology.title')"
            @mouseleave="hover = ''"
          >
            <!-- 列标题放在图内，随图一起缩放 -->
            <g class="cols">
              <text v-for="(c, i) in colTitles" :key="i" :x="i < 2 ? COLS[i].x + COLS[i].w / 2 : COLS[i].x" :y="-HEAD + 14" :text-anchor="i < 2 ? 'middle' : 'start'">{{ c }}</text>
            </g>
            <defs>
              <!-- 每种图标只定义一次，节点用 <use> 引用，节点很多时不必为每个节点创建图标组件 -->
              <symbol v-for="(ic, name) in icons" :id="'gtopo-' + name" :key="name" viewBox="0 0 24 24">
                <component :is="ic" :size="24" />
              </symbol>
              <linearGradient id="topo-edge-hot" x1="0" x2="1" y1="0" y2="0">
                <stop offset="0" stop-color="var(--topo-accent)" stop-opacity="0.35" />
                <stop offset="1" stop-color="var(--topo-accent)" stop-opacity="1" />
              </linearGradient>
              <radialGradient id="topo-core" cx="0.5" cy="0.4" r="0.7">
                <stop offset="0" stop-color="var(--topo-accent)" stop-opacity="0.55" />
                <stop offset="1" stop-color="var(--topo-accent)" stop-opacity="0.08" />
              </radialGradient>
            </defs>
            <!-- 连线：底线 + 有流量时叠加流动虚线；粗细按 QPS 对数增长 -->
            <g class="edges">
              <g v-for="e in layout.edges" :key="e.id" class="edge" :class="edgeClass(e)">
                <path class="edge-base" :d="e.d" :style="{ strokeWidth: edgeWidth(e.rate) }" />
                <path v-if="animated(e)" class="edge-flow" :class="'lv' + flowLevel(e.rate)" :d="e.d" :style="{ strokeWidth: edgeWidth(e.rate) }" />
              </g>
            </g>
            <g class="edge-labels">
              <template v-for="e in layout.edges" :key="e.id">
                <text v-if="showLabel(e)" class="edge-label" :class="edgeClass(e)" :x="e.labelX" :y="e.labelY" text-anchor="middle">
                  {{ formatQps(e.rate.qps) }}/s · {{ formatRate(e.rate.bps) }}
                </text>
              </template>
            </g>
            <!-- 节点 -->
            <g
              v-for="n in layout.nodes"
              :key="n.id"
              class="node"
              :class="nodeClass(n)"
              :transform="`translate(${n.x},${n.y})`"
              @mouseenter="hover = n.id"
              @mouseleave="hover = ''"
              @click.stop="select(n.id)"
            >
              <template v-if="n.kind === 'internet' || n.kind === 'gately'">
                <circle class="hub-halo" :cx="n.w / 2" :cy="n.h / 2" :r="n.w / 2 + 6" />
                <circle class="hub" :cx="n.w / 2" :cy="n.h / 2" :r="n.w / 2" />
                <use :href="n.kind === 'internet' ? '#gtopo-globe' : '#gtopo-router'" :x="n.w / 2 - 13" :y="n.h / 2 - 13" width="26" height="26" class="hub-icon" />
                <text class="hub-title" :x="n.w / 2" :y="n.h + 16" text-anchor="middle">{{ n.kind === 'internet' ? t('webTopology.internet') : 'Gately' }}</text>
                <text class="hub-sub" :x="n.w / 2" :y="n.h + 30" text-anchor="middle">{{ hubSub(n) }}</text>
              </template>
              <template v-else>
                <rect class="card" :width="n.w" :height="n.h" rx="10" />
                <rect class="card-accent" x="0" y="8" width="3" :height="n.h - 16" rx="1.5" />
                <circle class="icon-bg" cx="23" :cy="n.h / 2" r="14" />
                <use :href="'#gtopo-' + nodeIcon(n)" x="14" :y="n.h / 2 - 9" width="18" height="18" class="node-icon" />
                <text class="title" x="44" y="19">{{ fit(nodeTitle(n), n.w - 64) }}</text>
                <text class="sub" x="44" y="35">{{ fit(nodeSub(n), n.w - 54) }}</text>
                <circle class="dot" :cx="n.w - 10" cy="10" r="4" />
                <g v-if="n.kind === 'listener'" class="fold" @click.stop="toggleListener(n)">
                  <rect :x="n.w - 26" :y="n.h - 22" width="20" height="18" rx="5" />
                  <use :href="n.collapsed ? '#gtopo-right' : '#gtopo-down'" :x="n.w - 23" :y="n.h - 20" width="14" height="14" />
                </g>
                <g v-if="ipCount(n.id) > 0 && n.kind !== 'upstream'" class="ipbadge">
                  <rect :x="n.w - 30" :y="-8" :width="26" height="15" rx="7.5" />
                  <text :x="n.w - 17" :y="3" text-anchor="middle">{{ ipCount(n.id) > 99 ? '99+' : ipCount(n.id) }}</text>
                </g>
              </template>
            </g>
          </svg>
          <div v-if="!layout.nodes.some((n) => n.kind === 'listener')" class="canvas-hint">{{ query ? t('webTopology.noMatch') : t('webTopology.empty') }}</div>
        </template>
      </div>

      <aside class="panel">
        <template v-if="detail">
          <div class="panel-head">
            <div class="panel-title">{{ detail.title }}</div>
            <div class="panel-sub">{{ detail.sub }}</div>
            <div v-if="detail.tags.length" class="tags">
              <el-tag v-for="tg in detail.tags" :key="tg.text" size="small" :type="tg.type" effect="plain" round disable-transitions>{{ tg.text }}</el-tag>
            </div>
            <div v-if="detail.error" class="panel-error">{{ detail.error }}</div>
          </div>
          <div class="metrics">
            <div v-for="m in detail.metrics" :key="m.label" class="metric">
              <div class="m-label">
                {{ m.label }}
                <el-tooltip v-if="m.help" :content="m.help" placement="top"><Info :size="12" class="m-help" /></el-tooltip>
              </div>
              <div class="m-value" :class="m.cls">{{ m.value }}</div>
            </div>
          </div>
          <template v-if="seriesFor === detailFocus && data?.series?.length">
            <div class="chart-title">
              {{ t('webTopology.curve') }}
              <span class="chart-meta">{{ t('webTopology.curveMeta', { max: formatQps(seriesMax) }) }}</span>
            </div>
            <Sparkline :series="[{ values: data.series.map((p) => p.requests), color: 'var(--el-color-primary)' }]" :capacity="data.series.length" :height="56" />
            <div class="chart-title">{{ t('webTopology.curveBytes') }}</div>
            <Sparkline :series="[{ values: data.series.map((p) => p.bytes), color: 'var(--g-chart-2)' }]" :capacity="data.series.length" :height="40" />
          </template>
          <div v-if="detail.rows.length" class="rows">
            <div v-for="r in detail.rows" :key="r.label" class="row">
              <span class="r-label">{{ r.label }}</span>
              <span class="r-value" :class="r.cls">{{ r.value }}</span>
            </div>
          </div>
          <div v-if="detail.actions.length" class="panel-actions">
            <el-button v-for="a in detail.actions" :key="a.key" size="small" :icon="a.icon" @click="a.run">{{ a.label }}</el-button>
          </div>
          <div class="chart-title ips-title">
            {{ t('webTopology.activeIps') }}
            <span class="chart-meta">{{ panelIps.length }}</span>
            <el-tooltip :content="t('webTopology.activeIpsHelp')" placement="top"><Info :size="12" class="m-help" /></el-tooltip>
          </div>
          <div v-if="data?.ipTruncated" class="ip-warn">{{ t('webTopology.ipTruncated') }}</div>
          <div class="ip-list">
            <div v-if="!panelIps.length" class="ip-empty">{{ t('webTopology.ipNone') }}</div>
            <button v-for="ip in panelIps" :key="ip.ip" type="button" class="ip-row" :class="{ on: ip.ip === ipFilter }" @click="setIp(ip.ip)">
              <span class="mono ip">{{ showIP(ip.ip) }}</span>
              <span class="ip-meta">
                <span v-if="ip.n" class="live">{{ t('webTopology.ipActive', { n: ip.n }) }}</span>
                <span v-else>{{ ago(ip.last) }}</span>
              </span>
            </button>
          </div>
        </template>
      </aside>
    </div>

    <div class="topo-foot">
      <span class="pulse" :class="{ off: paused || !!loadError }" />
      <span>{{ paused ? t('webTopology.paused') : loadError ? loadError : t('webTopology.updated', { time: updatedAt }) }}</span>
      <span class="foot-note">{{ t('webTopology.note') }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import {
  AppWindow,
  ArrowRightLeft,
  BarChart3,
  Ban,
  ChevronDown,
  ChevronRight,
  ChevronsDownUp,
  ChevronsUpDown,
  CornerUpRight,
  EthernetPort,
  Eye,
  EyeOff,
  FileText,
  Filter,
  FolderOpen,
  Globe,
  Info,
  Lock,
  Pencil,
  Router,
  SearchX,
  Server,
  Unplug,
  X,
} from 'lucide-vue-next'
import { api, formatBytes, formatRate } from '../api'
import Sparkline from './Sparkline.vue'
import { useCollapsed } from '../composables/useCollapsed'
import {
  COLS,
  activeAt,
  buildLayout,
  edgeWidth,
  flowLevel,
  formatCount,
  formatQps,
  ipPath,
  isValidIP,
  maskHost,
  maskIP,
  maskText,
  maskURL,
  nodeCovers,
  pathOf,
  type LayoutEdge,
  type LayoutNode,
  type TopoIP,
  type TopologyView,
} from '../topology'

const props = defineProps<{ query: string }>()
const emit = defineEmits<{
  stats: [listener: string, site: string]
  conns: [listener: string, site: string]
  edit: [listener: string, site: string]
}>()

const { t } = useI18n()

const data = ref<TopologyView>()
const loadError = ref('')
const updatedAt = ref('')
const paused = ref(false)
// selected 为点击选中的节点（详情面板展示它），hover 为悬停中的节点（只影响高亮）
const selected = ref('gately')
const hover = ref('')
const ipFilter = ref('')
const ipModel = ref('')
// seriesFor 为当前曲线所属的节点，切换选中节点后等新数据到达再显示，避免把上一个节点的曲线当成新节点的
const seriesFor = ref('')

const maskKey = 'gately.topology.mask'
const masked = ref(readFlag(maskKey))
watch(masked, (v) => writeFlag(maskKey, v))

// 折叠状态与列表视图分开保存
const { isCollapsed, setCollapsed, setAll } = useCollapsed('web-topology')
const collapsedSet = computed(() => new Set((data.value?.listeners ?? []).filter((l) => isCollapsed(l.key)).map((l) => l.key)))
const listenerKeys = computed(() => (data.value?.listeners ?? []).map((l) => l.key))
const allCollapsed = computed(() => listenerKeys.value.length > 0 && listenerKeys.value.every((k) => isCollapsed(k)))
function toggleAll() {
  setAll(listenerKeys.value, !allCollapsed.value)
}
function toggleListener(n: LayoutNode) {
  if (n.listener) {
    setCollapsed(n.listener.key, !n.collapsed)
  }
}

const query = computed(() => props.query.trim())
const layout = computed(() => (data.value ? buildLayout(data.value, collapsedSet.value, query.value) : undefined))

// 系统开启“减少动态效果”时不播放连线动画
const reduceMotion = typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
// 连线很多时只给高亮链路播放动画，减少重绘
const motion = computed(() => !reduceMotion)
const manyEdges = computed(() => (layout.value?.edges.length ?? 0) > 120)

// HEAD 为图顶部留给列标题的高度
const HEAD = 26
const colTitles = computed(() => [t('webTopology.colInternet'), 'Gately', t('webTopology.colListener'), t('webTopology.colSite'), t('webTopology.colBackend')])

// 活动 IP
const ipInfo = computed(() => data.value?.ips.find((i) => i.ip === ipFilter.value))
// ipQuery 为下拉框中输入的搜索词：同时按原始地址与打码后的文字匹配，打码时输入完整地址也能搜到
const ipQuery = ref('')
const ipOptions = computed(() => {
  const list = data.value?.ips ?? []
  const q = ipQuery.value.trim().toLowerCase()
  if (!q) {
    return list
  }
  return list.filter((i) => i.ip.toLowerCase().includes(q) || showIP(i.ip).toLowerCase().includes(q))
})

// litSet 为需要高亮的节点集合，null 表示全部正常显示：悬停优先，其次 IP 筛选，最后选中的节点
const litSet = computed<Set<string> | null>(() => {
  const l = layout.value
  if (!l) {
    return null
  }
  // 悬停的节点可能已在刷新中被移除（此时浏览器通常不触发 mouseleave），跳过它
  if (hover.value && l.byId.has(hover.value)) {
    return pathOf(l, hover.value)
  }
  // 筛选的 IP 已不活跃时不再整图变暗，顶部文案会说明它已不活跃
  if (ipFilter.value && ipInfo.value) {
    return ipPath(l, ipInfo.value)
  }
  return pathOf(l, selected.value)
})

// 每个节点上的活动 IP 数（画布上只显示个数）
const ipCounts = computed(() => {
  const m = new Map<string, number>()
  for (const ip of data.value?.ips ?? []) {
    const seen = new Set<string>()
    for (const n of ip.nodes) {
      const ids = [n]
      if (n.startsWith('s:') || n.startsWith('a:')) {
        ids.push('l:' + n.slice(2).split('/')[0])
      }
      for (const id of ids) {
        if (!seen.has(id)) {
          seen.add(id)
          m.set(id, (m.get(id) ?? 0) + 1)
        }
      }
    }
  }
  return m
})
function ipCount(id: string) {
  return ipCounts.value.get(id) ?? 0
}

function nodeClass(n: LayoutNode) {
  const lit = litSet.value
  return [
    'k-' + n.kind,
    'st-' + nodeState(n),
    {
      dim: !!lit && !lit.has(n.id),
      hot: !!lit && lit.has(n.id),
      sel: selected.value === n.id,
    },
  ]
}

function edgeClass(e: LayoutEdge) {
  const lit = litSet.value
  const on = !!lit && lit.has(e.from) && lit.has(e.to)
  return { dim: !!lit && !on, hot: on }
}

function animated(e: LayoutEdge) {
  if (!motion.value || flowLevel(e.rate) === 0) {
    return false
  }
  if (manyEdges.value) {
    const lit = litSet.value
    return !!lit && lit.has(e.from) && lit.has(e.to)
  }
  return true
}

// 连线标注只显示在高亮链路上（悬停、选中或按 IP 筛选），全部显示时会与节点文字重叠；平时速率看节点副标题
function showLabel(e: LayoutEdge) {
  const lit = litSet.value
  if (!lit || !lit.has(e.from) || !lit.has(e.to)) {
    return false
  }
  return e.rate.qps > 0 || e.rate.bps > 0
}

// nodeState 返回节点状态，决定状态点颜色：ok 正常、down 故障、off 未运行、expected 预期的拒绝或断开（非故障）、idle 未知
function nodeState(n: LayoutNode): string {
  switch (n.kind) {
    case 'internet':
    case 'gately':
      return data.value?.enabled ? 'ok' : 'off'
    case 'listener':
      if (n.listener?.error) {
        return 'down'
      }
      return n.listener?.running ? 'ok' : 'off'
    case 'site':
      if (!n.site?.running) {
        return 'off'
      }
      return n.site.type === 'close' ? 'expected' : 'ok'
    case 'action':
      return 'expected'
    case 'terminal':
      if (!n.site?.running) {
        return 'off'
      }
      return n.site.type === 'close' ? 'expected' : 'ok'
    case 'upstream':
      if (n.upstream?.healthy === undefined) {
        return 'off'
      }
      return n.upstream.healthy ? 'ok' : 'down'
  }
  return 'idle'
}

// icons 为画布用到的图标，在 <defs> 中各定义一次
const icons: Record<string, Component> = {
  globe: Globe,
  router: Router,
  lock: Lock,
  port: EthernetPort,
  ban: Ban,
  redirect: CornerUpRight,
  folder: FolderOpen,
  file: FileText,
  app: AppWindow,
  https: ArrowRightLeft,
  notfound: SearchX,
  unplug: Unplug,
  server: Server,
  right: ChevronRight,
  down: ChevronDown,
}

// nodeIcon 返回节点图标在 icons 中的名称
function nodeIcon(n: LayoutNode): string {
  const byType = (type?: string, fallback = 'file') => (type === 'close' ? 'ban' : type === 'redirect' ? 'redirect' : type === 'static' ? 'folder' : type === 'text' ? 'file' : fallback)
  switch (n.kind) {
    case 'listener':
      return n.listener?.tls ? 'lock' : 'port'
    case 'site':
      return byType(n.site?.type, 'app')
    case 'action':
      return n.action?.kind === 'redirect' ? 'https' : 'notfound'
    case 'terminal':
      return n.site?.type === 'close' ? 'unplug' : byType(n.site?.type)
    default:
      return 'server'
  }
}

function showIP(ip: string) {
  return masked.value ? maskIP(ip) : ip
}

function listenerAddr(l: NonNullable<LayoutNode['listener']>) {
  const host = l.listenIp ? (masked.value ? maskHost(l.listenIp) : l.listenIp) : ''
  return (host.includes(':') ? `[${host}]` : host) + ':' + l.port
}

function siteName(s: NonNullable<LayoutNode['site']>) {
  if (s.default) {
    return t('web.defaultSite')
  }
  return s.name || (s.domains[0] ? showHost(s.domains[0]) : '') || s.key
}

// showHost 在打码时隐藏以 IP 作为域名的站点地址
function showHost(h: string) {
  return masked.value ? maskHost(h) : h
}

const typeKeys: Record<string, string> = {
  proxy: 'web.typeProxy',
  redirect: 'web.typeRedirect',
  static: 'web.typeStatic',
  text: 'web.typeText',
  close: 'web.typeClose',
}
// 拦截原因与防护标记的文案键（写全键名，便于文案校验脚本识别）
const blockKeys: Record<string, string> = {
  ban: 'webTopology.block_ban',
  ip: 'webTopology.block_ip',
  country: 'webTopology.block_country',
  rateLimit: 'webTopology.block_rateLimit',
  waf: 'webTopology.block_waf',
}
const guardKeys: Record<string, string> = {
  auth: 'webTopology.guard_auth',
  waf: 'webTopology.guard_waf',
  ip: 'webTopology.guard_ip',
  country: 'webTopology.guard_country',
  rateLimit: 'webTopology.guard_rateLimit',
  denyClose: 'webTopology.guard_denyClose',
}

function typeLabel(type: string) {
  return typeKeys[type] ? t(typeKeys[type]) : type
}

function matchLabel(s: NonNullable<LayoutNode['site']>) {
  if (s.default) {
    return t('webTopology.matchDefault')
  }
  if (s.domains.includes('*')) {
    return t('webTopology.matchAny')
  }
  return s.domains.some((d) => d.startsWith('*.')) ? t('webTopology.matchWildcard') : t('webTopology.matchExact')
}

function rateText(r: { qps: number; bps: number }) {
  return `${formatQps(r.qps)}/s · ${formatRate(r.bps)}`
}

function nodeTitle(n: LayoutNode): string {
  switch (n.kind) {
    case 'listener': {
      const l = n.listener!
      return (l.name ? l.name + ' · ' : '') + listenerAddr(l)
    }
    case 'site':
      return siteName(n.site!)
    case 'action':
      return n.action!.kind === 'redirect' ? t('webTopology.actionRedirect') : t('webTopology.actionNotFound')
    case 'terminal':
      return terminalTitle(n.site!)
    case 'upstream': {
      const u = n.upstream!
      return masked.value ? maskURL(u.url) : u.url
    }
  }
  return ''
}

function terminalTitle(s: NonNullable<LayoutNode['site']>) {
  switch (s.type) {
    case 'close':
      return t('webTopology.terminalClose')
    case 'redirect':
      // 跳转目标可能是内网地址，打码时一并隐藏
      return s.target ? (masked.value ? maskURL(s.target) : s.target) : t('web.typeRedirect')
    case 'static':
      return s.target || t('web.typeStatic')
    default:
      return t('webTopology.terminalText', { code: s.target || '200' })
  }
}

function nodeSub(n: LayoutNode): string {
  switch (n.kind) {
    case 'listener': {
      const l = n.listener!
      if (!l.running) {
        return l.error ? t('webTopology.listenerError') : t('webTopology.notRunning')
      }
      const extra = n.collapsed ? ' · ' + t('webTopology.folded', { n: n.hidden ?? 0 }) : ''
      return rateText(l.rate) + extra
    }
    case 'site': {
      const s = n.site!
      if (!s.running) {
        return s.enable ? t('webTopology.notRunning') : t('webTopology.disabled')
      }
      return `${matchLabel(s)} · ${formatQps(s.rate.qps)}/s · ${t('webTopology.activeShort', { n: s.active })}`
    }
    case 'action':
      return `${n.action!.kind === 'redirect' ? '301 / 308' : '404'} · ${formatQps(n.action!.rate.qps)}/s`
    case 'terminal': {
      const s = n.site!
      if (s.type === 'close') {
        return t('webTopology.expected') + ' · 444'
      }
      return typeLabel(s.type) + ' · ' + rateText(s.rate)
    }
    case 'upstream': {
      const u = n.upstream!
      const scope = u.scope === 'location' ? t('webTopology.scopeLocation', { m: u.match }) : u.scope === 'user' ? t('webTopology.scopeUser', { m: u.match }) : ''
      const health = u.healthy === undefined ? t('webTopology.upUnknown') : u.healthy ? t('webTopology.upHealthy') : t('webTopology.upDown')
      return [scope, u.backup ? t('webTopology.upBackup') : '', health, formatQps(u.rate.qps) + '/s'].filter(Boolean).join(' · ')
    }
  }
  return ''
}

function hubSub(n: LayoutNode) {
  const v = data.value
  if (!v) {
    return ''
  }
  if (n.kind === 'internet') {
    return t('webTopology.internetSub', { n: v.ipTotal })
  }
  const running = v.listeners.filter((l) => l.running).length
  return t('webTopology.gatelySub', { running, total: v.listeners.length })
}

// fit 按估算宽度截断文本（SVG 文本不会自动省略）：ASCII 约 6.6px，其余字符约 12px
function fit(s: string, px: number): string {
  let w = 0
  for (let i = 0; i < s.length; i++) {
    w += s.charCodeAt(i) < 128 ? 6.6 : 12
    if (w > px) {
      return s.slice(0, Math.max(1, i - 1)) + '…'
    }
  }
  return s
}

function ago(sec: number) {
  const now = data.value?.time ?? Math.floor(Date.now() / 1000)
  const d = Math.max(0, now - sec)
  return d <= 1 ? t('webTopology.ipNow') : t('webTopology.ipAgo', { s: d })
}

// ---- 详情面板 ----

interface Metric {
  label: string
  value: string
  help?: string
  cls?: string
}
interface Detail {
  title: string
  sub: string
  error?: string
  tags: { text: string; type?: 'success' | 'warning' | 'danger' | 'info' | 'primary' }[]
  metrics: Metric[]
  rows: Metric[]
  actions: { key: string; label: string; icon: Component; run: () => void }[]
}

const selectedNode = computed(() => layout.value?.byId.get(selected.value) ?? layout.value?.byId.get('gately'))
// detailFocus 为接口 focus 参数：去向节点的曲线用所属站点的，互联网与 Gately 都是整个实例
const detailFocus = computed(() => {
  const id = selectedNode.value?.id ?? 'gately'
  if (id.startsWith('t:')) {
    return 's:' + id.slice(2)
  }
  return id === 'internet' ? 'gately' : id
})

const seriesMax = computed(() => Math.max(0, ...(data.value?.series ?? []).map((p) => p.requests)))

function rateMetrics(r: { qps: number; bps: number; qps60: number }, bpsHelp?: string): Metric[] {
  return [
    { label: t('webTopology.qps'), value: formatQps(r.qps), help: t('webTopology.qpsHelp') },
    { label: t('webTopology.qps60'), value: formatQps(r.qps60), help: t('webTopology.qps60Help') },
    { label: t('webTopology.bps'), value: formatRate(r.bps), help: bpsHelp ?? t('webTopology.bpsHelp') },
  ]
}

const detail = computed<Detail | undefined>(() => {
  const n = selectedNode.value
  const v = data.value
  if (!n || !v) {
    return undefined
  }
  const d: Detail = { title: '', sub: '', tags: [], metrics: [], rows: [], actions: [] }
  switch (n.kind) {
    case 'internet':
    case 'gately': {
      d.title = n.kind === 'internet' ? t('webTopology.internet') : 'Gately'
      d.sub = n.kind === 'internet' ? t('webTopology.internetDesc') : hubSub(n)
      if (!v.enabled) {
        d.tags.push({ text: t('webTopology.moduleOff'), type: 'info' })
      }
      d.metrics = [
        ...rateMetrics(v.rate),
        { label: t('webTopology.active'), value: String(v.active), help: t('webTopology.activeHelp') },
        { label: t('webTopology.requests'), value: formatCount(v.requests), help: t('webTopology.requestsHelp') },
        { label: t('webTopology.activeIps'), value: String(v.ipTotal), help: t('webTopology.activeIpsHelp') },
      ]
      break
    }
    case 'listener': {
      const l = n.listener!
      d.title = l.name || listenerAddr(l)
      d.sub = [listenerAddr(l), l.tls ? 'HTTPS' : 'HTTP', l.http3 ? 'HTTP/3' : ''].filter(Boolean).join(' · ')
      // 错误信息里常带监听地址（如 listen tcp 192.168.1.5:443: bind: ...）
      d.error = masked.value && l.error ? maskText(l.error) : l.error
      d.tags.push(l.running ? { text: t('webTopology.running'), type: 'success' } : { text: l.enable ? t('webTopology.notRunning') : t('webTopology.disabled'), type: 'info' })
      if (l.tls && l.plainHttp === 'redirect') {
        d.tags.push({ text: t('webTopology.plainRedirect') })
      } else if (l.tls && l.plainHttp === 'serve') {
        d.tags.push({ text: t('webTopology.plainServe') })
      }
      if (l.proxyProtocol) {
        d.tags.push({ text: 'PROXY protocol' })
      }
      d.metrics = [
        ...rateMetrics(l.rate),
        { label: t('webTopology.active'), value: String(l.active), help: t('webTopology.activeHelp') },
        { label: t('webTopology.requests'), value: formatCount(l.requests), help: t('webTopology.requestsHelp') },
        { label: t('webTopology.sites'), value: String(l.sites.length) },
      ]
      break
    }
    case 'site': {
      const s = n.site!
      const lk = n.parent!.slice(2)
      d.title = siteName(s)
      d.sub = `${typeLabel(s.type)} · ${matchLabel(s)}${s.group ? ' · ' + s.group : ''}`
      d.tags.push(s.running ? { text: t('webTopology.running'), type: 'success' } : { text: s.enable ? t('webTopology.notRunning') : t('webTopology.disabled'), type: 'info' })
      if (s.type === 'close') {
        d.tags.push({ text: t('webTopology.expected'), type: 'info' })
      }
      for (const g of s.guards) {
        d.tags.push({ text: guardKeys[g] ? t(guardKeys[g]) : g, type: 'primary' })
      }
      d.metrics = [
        ...rateMetrics(s.rate),
        { label: t('webTopology.active'), value: String(s.active), help: t('webTopology.activeHelp') },
        { label: t('webTopology.requests'), value: formatCount(s.requests), help: t('webTopology.requestsHelp') },
        { label: t('webTopology.errors'), value: formatCount(s.errors), help: t('webTopology.errorsHelp'), cls: s.errors ? 'bad' : '' },
      ]
      const blockedTotal = Object.values(s.blocked).reduce((a, b) => a + b, 0)
      d.rows.push({ label: t('webTopology.blocked'), value: formatCount(blockedTotal), help: t('webTopology.blockedHelp'), cls: blockedTotal ? 'warn' : '' })
      for (const [k, label] of Object.entries(blockKeys)) {
        if (s.blocked[k]) {
          d.rows.push({ label: '　' + t(label), value: formatCount(s.blocked[k]), cls: 'warn' })
        }
      }
      d.rows.push({ label: t('webTopology.status'), value: ['2xx', '3xx', '4xx', '5xx'].map((c) => `${c} ${formatCount(s.status[c] ?? 0)}`).join(' · ') })
      if (s.closed) {
        d.rows.push({ label: t('webTopology.closed'), value: formatCount(s.closed) })
      }
      if (s.domains.length && !s.default) {
        d.rows.push({ label: t('webTopology.domains'), value: s.domains.map(showHost).join(', ') })
      }
      if (s.running) {
        d.actions.push({ key: 'stats', label: t('webTopology.openStats'), icon: BarChart3, run: () => emit('stats', lk, s.key) })
        d.actions.push({ key: 'conns', label: t('webTopology.openConns'), icon: ArrowRightLeft, run: () => emit('conns', lk, s.key) })
      }
      d.actions.push({ key: 'edit', label: t('common.edit'), icon: Pencil, run: () => emit('edit', lk, s.key) })
      break
    }
    case 'action': {
      const a = n.action!
      d.title = a.kind === 'redirect' ? t('webTopology.actionRedirect') : t('webTopology.actionNotFound')
      d.sub = a.kind === 'redirect' ? t('webTopology.actionRedirectDesc') : t('webTopology.actionNotFoundDesc')
      d.tags.push({ text: t('webTopology.expected'), type: 'info' })
      d.metrics = [...rateMetrics(a.rate), { label: t('webTopology.requests'), value: formatCount(a.requests), help: t('webTopology.requestsHelp') }]
      break
    }
    case 'terminal': {
      const s = n.site!
      d.title = terminalTitle(s)
      d.sub = s.type === 'close' ? t('webTopology.terminalCloseDesc') : `${typeLabel(s.type)} · ${siteName(s)}`
      if (s.type === 'close') {
        d.tags.push({ text: t('webTopology.expected'), type: 'info' })
      }
      d.metrics = [...rateMetrics(s.rate), { label: t('webTopology.requests'), value: formatCount(s.requests), help: t('webTopology.requestsHelp') }]
      break
    }
    case 'upstream': {
      const u = n.upstream!
      d.title = masked.value ? maskURL(u.url) : u.url
      d.sub = siteName(n.site!)
      if (u.healthy === undefined) {
        d.tags.push({ text: t('webTopology.upUnknown'), type: 'info' })
      } else {
        d.tags.push(u.healthy ? { text: t('webTopology.upHealthy'), type: 'success' } : { text: t('webTopology.upDown'), type: 'danger' })
      }
      if (u.scope === 'location') {
        d.tags.push({ text: t('webTopology.scopeLocation', { m: u.match }) })
      } else if (u.scope === 'user') {
        d.tags.push({ text: t('webTopology.scopeUser', { m: u.match }) })
      }
      if (u.backup) {
        d.tags.push({ text: t('webTopology.upBackup'), type: 'warning' })
      }
      d.metrics = [
        ...rateMetrics(u.rate, t('webTopology.bpsUpstreamHelp')),
        { label: t('webTopology.conns'), value: String(u.conns), help: t('webTopology.connsHelp') },
        { label: t('webTopology.requests'), value: formatCount(u.requests), help: t('webTopology.upRequestsHelp') },
        { label: t('webTopology.upBytes'), value: formatBytes(u.bytes), help: t('webTopology.bpsUpstreamHelp') },
      ]
      d.rows.push({ label: t('webTopology.weight'), value: String(u.weight) })
      break
    }
  }
  return d
})

// 详情面板的活动 IP：经过选中节点的地址，n 为它在该节点上进行中的请求数；进行中的排在前面
const panelIps = computed<(TopoIP & { n: number })[]>(() => {
  const node = selectedNode.value
  if (!node) {
    return []
  }
  const list = (data.value?.ips ?? []).filter((ip) => nodeCovers(ip, node.id)).map((ip) => ({ ...ip, n: activeAt(ip, node.id) }))
  // 接口已按总进行中数与最后出现时间排序，这里只把该节点上进行中的提到前面（稳定排序保留原顺序）
  return list.sort((a, b) => (b.n > 0 ? 1 : 0) - (a.n > 0 ? 1 : 0))
})

// ---- 交互 ----

function select(id: string) {
  // 曲线所属的节点变化后由下方对 detailFocus 的监听立即刷新
  selected.value = id
}

function setIp(ip: string) {
  ipFilter.value = ip
  ipModel.value = ip
  reload()
}

function onIpChange(v: string) {
  const s = (v ?? '').trim()
  // 手动输入的不完整地址（如 1.2.3）不交给后端，否则每次刷新都会报错、画布停在旧数据
  if (s && !isValidIP(s)) {
    ElMessage.warning(t('webTopology.ipInvalid'))
    ipModel.value = ipFilter.value
    return
  }
  ipFilter.value = s
  reload()
}

function clearIp() {
  ipFilter.value = ''
  ipModel.value = ''
  reload()
}

// ---- 轮询 ----

let timer: number | undefined
let seq = 0
let alive = true

async function load() {
  const my = ++seq
  const focus = detailFocus.value
  try {
    const q = new URLSearchParams({ focus })
    // 筛选值在选择时已校验为完整地址
    if (ipFilter.value && isValidIP(ipFilter.value)) {
      q.set('ip', ipFilter.value)
    }
    const v = await api.get<TopologyView>('/api/webservice/topology?' + q.toString())
    if (my !== seq || !alive) {
      return
    }
    data.value = v
    seriesFor.value = focus
    loadError.value = ''
    updatedAt.value = new Date().toLocaleTimeString()
  } catch (e) {
    if (my === seq && alive) {
      loadError.value = (e as Error).message
    }
  }
}

// schedule 上一次请求结束后再安排下一次，避免慢网络下请求堆积；页面在后台时暂停
function schedule() {
  window.clearTimeout(timer)
  timer = window.setTimeout(async () => {
    if (!alive) {
      return
    }
    if (document.hidden) {
      paused.value = true
      schedule()
      return
    }
    paused.value = false
    await load()
    if (alive) {
      schedule()
    }
  }, 2000)
}

async function reload() {
  window.clearTimeout(timer)
  await load()
  if (alive) {
    schedule()
  }
}

function onVisibility() {
  if (!document.hidden && paused.value) {
    paused.value = false
    reload()
  }
}

onMounted(() => {
  document.addEventListener('visibilitychange', onVisibility)
  reload()
})

onBeforeUnmount(() => {
  alive = false
  window.clearTimeout(timer)
  document.removeEventListener('visibilitychange', onVisibility)
})

// 选中的节点因折叠或搜索暂时消失时，详情面板回落到 Gately（见 selectedNode），节点重新出现后恢复选中；
// 曲线所属节点变化时立即刷新，避免最多 2 秒没有曲线
watch(detailFocus, (v, old) => {
  if (v !== old) {
    reload()
  }
})

function readFlag(k: string) {
  try {
    return localStorage.getItem(k) === '1'
  } catch {
    return false
  }
}

function writeFlag(k: string, v: boolean) {
  try {
    localStorage.setItem(k, v ? '1' : '0')
  } catch {
    // 存储不可用时仅本次会话生效
  }
}

defineExpose({ reload })
</script>

<style scoped>
.topo {
  --topo-accent: var(--el-color-primary);
  --topo-bg: #0b1220;
  --topo-bg2: #111b2e;
  --topo-card: #131d31;
  --topo-card-hi: #18253f;
  --topo-line: #2a3a58;
  --topo-text: #e6edf7;
  --topo-sub: #8b9ab3;
  --topo-ok: #34d399;
  --topo-down: #f87171;
  --topo-off: #64748b;
  --topo-expected: #94a3b8;
  display: flex;
  flex-direction: column;
  gap: 12px;
  /* 按自身宽度决定详情面板放在右侧还是画布下方 */
  container-type: inline-size;
}
.topo-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.ip-filter {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 6px 10px;
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  background: var(--el-bg-color);
  transition: border-color 0.2s, box-shadow 0.2s;
}
.ip-filter.on {
  border-color: var(--el-color-primary);
  box-shadow: 0 0 0 3px var(--el-color-primary-light-9);
}
.bar-icon {
  color: var(--el-text-color-secondary);
}
.ip-select {
  width: 230px;
}
.filter-state {
  font-size: 13px;
  color: var(--el-color-primary);
}
.opt-meta {
  float: right;
  margin-left: 16px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.kpis {
  display: flex;
  gap: 6px;
  margin-right: 4px;
}
.kpi {
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--el-fill-color-light);
  font-size: 12px;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}
.kpi b {
  color: var(--el-text-color-primary);
  font-variant-numeric: tabular-nums;
}
.topo-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: 12px;
  align-items: start;
}
@container (max-width: 1100px) {
  .topo-main {
    grid-template-columns: minmax(0, 1fr);
  }
  .panel {
    position: static;
    max-height: none;
  }
  .canvas {
    max-height: 70vh;
  }
}
.canvas {
  position: relative;
  overflow: auto;
  min-height: 320px;
  max-height: calc(100vh - 210px);
  padding: 8px 12px 16px;
  border-radius: calc(var(--g-radius) + 2px);
  background:
    radial-gradient(circle at 1px 1px, rgba(148, 163, 184, 0.12) 1px, transparent 0) 0 0 / 22px 22px,
    radial-gradient(1200px 500px at 10% 0%, rgba(56, 189, 248, 0.08), transparent 60%),
    linear-gradient(160deg, var(--topo-bg2), var(--topo-bg));
  box-shadow: inset 0 0 0 1px rgba(148, 163, 184, 0.12);
}
.canvas-empty {
  height: 300px;
}
.canvas-empty.msg,
.canvas-hint {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--topo-sub);
  font-size: 13px;
}
.canvas-hint {
  padding: 24px 0;
}
.cols text {
  font-size: 10.5px;
  letter-spacing: 0.08em;
  fill: var(--topo-sub);
  opacity: 0.8;
}
.graph {
  display: block;
  /* 按容器宽度等比缩放，最多原始大小；窄屏下保持最小宽度并横向滚动，文字不至于太小 */
  width: 100%;
  min-width: 760px;
  height: auto;
  margin: 0 auto;
  overflow: visible;
  font-family: var(--g-font);
}

/* 连线 */
.edge-base {
  fill: none;
  stroke: var(--topo-line);
  stroke-linecap: round;
  transition: stroke 0.25s, opacity 0.25s;
}
.edge-flow {
  fill: none;
  stroke: var(--topo-accent);
  stroke-linecap: round;
  stroke-dasharray: 2 14;
  opacity: 0.85;
  animation: topo-flow 1.6s linear infinite;
}
.edge-flow.lv1 {
  animation-duration: 2.6s;
  opacity: 0.55;
}
.edge-flow.lv3 {
  animation-duration: 0.8s;
}
.no-motion .edge-flow {
  animation: none;
}
.edge.hot .edge-base {
  stroke: url(#topo-edge-hot);
}
.edge.dim {
  opacity: 0.18;
}
@keyframes topo-flow {
  to {
    stroke-dashoffset: -32;
  }
}
.edge-label {
  font-size: 10.5px;
  fill: var(--topo-sub);
  paint-order: stroke;
  stroke: var(--topo-bg);
  stroke-width: 3px;
  font-variant-numeric: tabular-nums;
  pointer-events: none;
}
.edge-label.hot {
  fill: var(--topo-text);
}

/* 节点 */
.node {
  cursor: pointer;
  transition: opacity 0.25s;
}
.node.dim {
  opacity: 0.28;
}
.card {
  fill: var(--topo-card);
  stroke: rgba(148, 163, 184, 0.18);
  stroke-width: 1;
  transition: fill 0.2s, stroke 0.2s;
}
.node:hover .card,
.node.hot .card {
  fill: var(--topo-card-hi);
  stroke: rgba(148, 163, 184, 0.35);
}
.node.sel .card {
  stroke: var(--topo-accent);
  stroke-width: 1.5;
  filter: drop-shadow(0 0 8px color-mix(in srgb, var(--topo-accent) 55%, transparent));
}
.card-accent {
  fill: var(--topo-off);
}
.st-ok .card-accent {
  fill: var(--topo-ok);
}
.st-down .card-accent {
  fill: var(--topo-down);
}
.st-expected .card-accent {
  fill: var(--topo-expected);
}
.icon-bg {
  fill: rgba(148, 163, 184, 0.1);
}
.st-ok .icon-bg {
  fill: color-mix(in srgb, var(--topo-accent) 18%, transparent);
}
.st-down .icon-bg {
  fill: rgba(248, 113, 113, 0.16);
}
.node-icon {
  color: var(--topo-sub);
}
.st-ok .node-icon {
  color: var(--topo-accent);
}
.st-down .node-icon {
  color: var(--topo-down);
}
.title {
  font-size: 12.5px;
  font-weight: 600;
  fill: var(--topo-text);
}
.sub {
  font-size: 11px;
  fill: var(--topo-sub);
  font-variant-numeric: tabular-nums;
}
.st-off .title {
  fill: var(--topo-sub);
}
.dot {
  fill: var(--topo-off);
}
.st-ok .dot {
  fill: var(--topo-ok);
}
.st-down .dot {
  fill: var(--topo-down);
  animation: topo-blink 1.4s ease-in-out infinite;
}
.st-expected .dot {
  fill: var(--topo-expected);
}
.no-motion .st-down .dot {
  animation: none;
}
@keyframes topo-blink {
  50% {
    opacity: 0.35;
  }
}
.fold rect {
  fill: rgba(148, 163, 184, 0.12);
}
.fold:hover rect {
  fill: rgba(148, 163, 184, 0.25);
}
.fold {
  color: var(--topo-sub);
}
.ipbadge rect {
  fill: var(--topo-accent);
}
.ipbadge text {
  font-size: 9.5px;
  font-weight: 700;
  fill: #fff;
}

/* 互联网与 Gately */
.hub {
  fill: url(#topo-core);
  stroke: color-mix(in srgb, var(--topo-accent) 70%, transparent);
  stroke-width: 1.5;
}
.hub-halo {
  fill: none;
  stroke: color-mix(in srgb, var(--topo-accent) 30%, transparent);
  stroke-width: 1;
  stroke-dasharray: 3 5;
  transform-box: fill-box;
  transform-origin: center;
  animation: topo-spin 24s linear infinite;
}
.no-motion .hub-halo {
  animation: none;
}
@keyframes topo-spin {
  to {
    transform: rotate(360deg);
  }
}
.k-gately .hub {
  stroke-width: 2;
}
.node.sel .hub {
  filter: drop-shadow(0 0 10px color-mix(in srgb, var(--topo-accent) 70%, transparent));
}
.hub-icon {
  color: #fff;
}
.st-off .hub-icon {
  color: var(--topo-sub);
}
.hub-title {
  font-size: 12.5px;
  font-weight: 600;
  fill: var(--topo-text);
}
.hub-sub {
  font-size: 10.5px;
  fill: var(--topo-sub);
}

/* 详情面板 */
.panel {
  position: sticky;
  top: 72px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: calc(100vh - 210px);
  overflow: auto;
  padding: 14px;
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  background: var(--el-bg-color);
}
.panel-title {
  font-size: 15px;
  font-weight: 600;
  word-break: break-all;
}
.panel-sub {
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-all;
}
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 8px;
}
.panel-error {
  margin-top: 8px;
  font-size: 12px;
  color: var(--el-color-danger);
  word-break: break-all;
}
.metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}
.metric {
  padding: 8px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
}
.m-label {
  display: flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
.m-help {
  color: var(--el-text-color-placeholder);
  cursor: help;
}
.m-value {
  margin-top: 2px;
  font-size: 15px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  word-break: break-all;
}
.bad {
  color: var(--el-color-danger);
}
.warn {
  color: var(--el-color-warning);
}
.chart-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--el-text-color-regular);
}
.chart-meta {
  font-weight: 400;
  color: var(--el-text-color-secondary);
}
.rows {
  display: flex;
  flex-direction: column;
}
.row {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 5px 0;
  border-bottom: 1px dashed var(--g-border);
  font-size: 12px;
}
.r-label {
  flex-shrink: 0;
  color: var(--el-text-color-secondary);
  white-space: pre;
}
.r-value {
  text-align: right;
  word-break: break-all;
  font-variant-numeric: tabular-nums;
}
.panel-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.panel-actions .el-button + .el-button {
  margin-left: 0;
}
.ips-title {
  margin-top: 4px;
}
.ip-warn {
  font-size: 12px;
  color: var(--el-color-warning);
}
.ip-list {
  display: flex;
  flex-direction: column;
  max-height: 280px;
  overflow: auto;
  border: 1px solid var(--g-border);
  border-radius: 8px;
}
.ip-empty {
  padding: 14px;
  text-align: center;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.ip-row {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 10px;
  border: 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
  background: none;
  color: inherit;
  font: inherit;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}
.ip-row:last-child {
  border-bottom: 0;
}
.ip-row:hover {
  background: var(--el-fill-color-light);
}
.ip-row.on {
  background: var(--el-color-primary-light-9);
}
.ip {
  word-break: break-all;
}
.ip-meta {
  flex-shrink: 0;
  color: var(--el-text-color-secondary);
}
.live {
  color: var(--el-color-success);
}
.mono {
  font-family: var(--g-mono);
}
.topo-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.foot-note {
  flex-basis: 100%;
  color: var(--el-text-color-placeholder);
}
.pulse {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--el-color-success);
  box-shadow: 0 0 0 0 color-mix(in srgb, var(--el-color-success) 60%, transparent);
  animation: topo-pulse 2s ease-out infinite;
}
.pulse.off {
  background: var(--el-text-color-placeholder);
  animation: none;
}
@keyframes topo-pulse {
  70% {
    box-shadow: 0 0 0 6px transparent;
  }
}
@media (prefers-reduced-motion: reduce) {
  .pulse {
    animation: none;
  }
}
</style>
