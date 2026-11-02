<template>
  <div>
    <PageHeader :title="t('perf.title')" :desc="t('perf.desc')" :icon="Gauge">
      <el-radio-group v-model="range" size="small" @change="loadOverview">
        <el-radio-button value="60s">{{ t('perf.range60s') }}</el-radio-button>
        <el-radio-button value="1h">{{ t('perf.range1h') }}</el-radio-button>
        <el-radio-button value="24h">{{ t('perf.range24h') }}</el-radio-button>
      </el-radio-group>
      <el-button type="primary" :icon="Plus" @click="openRule()">{{ t('perf.addRule') }}</el-button>
    </PageHeader>

    <!-- 进行中的告警：已触发的在前，计时中的在后 -->
    <div v-if="alerts.length" class="alerts">
      <div v-for="a in alerts" :key="a.rule + a.subject" class="alert" :class="a.state === 'firing' ? a.level : 'pending'">
        <StatusBadge :type="a.state === 'firing' ? (a.level === 'error' ? 'error' : 'warn') : 'pending'" :text="a.state === 'firing' ? t('perf.firing') : t('perf.pending')" />
        <b>{{ a.name }}</b>
        <span>{{ targetName(a) }}</span>
        <span class="meta">{{ t('perf.alertValue', { value: formatMetric(ruleMetric(a.rule), a.value) }) }}</span>
        <span class="meta">{{ a.state === 'firing' ? t('perf.firedAgo', { d: ago(a.firedAt) }) : t('perf.pendingFor', { d: ago(a.since) }) }}</span>
      </div>
    </div>

    <h3 class="section">{{ t('perf.hostSection') }}</h3>
    <div class="charts">
      <div v-for="c in charts" :key="c.key" class="chart-card">
        <div class="chart-head">
          <span>{{ c.title }}</span>
          <b>{{ c.latest }}</b>
        </div>
        <TrendChart :times="times" :series="c.series" :from="from" :to="to" :max="c.max" :format="c.format" :height="110" :empty-text="t('perf.noData')" />
      </div>
    </div>
    <div class="form-help">{{ t('perf.historyHelp') }}</div>

    <template v-if="webEnabled">
      <h3 class="section">{{ t('perf.sitesSection') }}</h3>
      <el-table :data="sites" size="small" stripe :empty-text="t('perf.noSites')" class="sites" @row-click="openSite">
        <el-table-column :label="t('perf.site')" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">{{ row.name }}</template>
        </el-table-column>
        <el-table-column :label="t('perf.active')" width="100" align="right">
          <template #default="{ row }">{{ row.active }}</template>
        </el-table-column>
        <el-table-column :label="t('perf.qps')" width="100" align="right">
          <template #default="{ row }">{{ row.qps == null ? '-' : row.qps.toFixed(row.qps >= 10 ? 0 : 1) }}</template>
        </el-table-column>
        <el-table-column :label="t('perf.err5xx')" width="100" align="right">
          <template #default="{ row }">
            <span :class="{ bad: (row.err5xx ?? 0) > 0 }">{{ row.err5xx == null ? '-' : row.err5xx.toFixed(1) + '%' }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('perf.p95')" width="110" align="right">
          <template #default="{ row }">{{ row.p95 == null ? '-' : formatMs(row.p95) }}</template>
        </el-table-column>
        <el-table-column :label="t('perf.upstreams')" min-width="150">
          <template #default="{ row }">
            <template v-if="row.upstreams?.length">
              <span :class="{ bad: unhealthy(row) > 0 }">{{ t('perf.upstreamSummary', { up: row.upstreams.length - unhealthy(row), total: row.upstreams.length }) }}</span>
              <span class="meta"> · {{ t('perf.upstreamConns', { n: row.upstreams.reduce((n: number, u: UpstreamView) => n + u.conns, 0) }) }}</span>
            </template>
            <span v-else class="meta">-</span>
          </template>
        </el-table-column>
      </el-table>
      <div class="form-help">{{ t('perf.sitesHelp') }}</div>
    </template>

    <div class="section-head">
      <h3 class="section">{{ t('perf.rules') }}</h3>
      <div class="section-tools">
        <ListTools :gl="glRules" size="small" />
        <CompactToggle v-model="compactRules" size="small" />
      </div>
    </div>
    <el-alert v-if="notifyHint" :type="notifyHint.type" :closable="false" show-icon class="notify-hint">
      <template #title>
        {{ notifyHint.text }}
        <router-link v-if="notifyHint.link" to="/notify">{{ t('perf.goNotify') }}</router-link>
      </template>
    </el-alert>
    <GroupedList :gl="glRules" :compact="compactRules" grid :empty-text="t('perf.noRules')">
      <template #item="{ it, item: r, sorting, bind }">
        <EntityCard v-bind="bind" :off="!r.enable" :compact="compactRules || sorting">
          <template #lead>
            <SortCheck v-if="sorting" :gl="glRules" :id="it.id" />
            <el-switch v-else :model-value="r.enable" @change="(v: string | number | boolean) => toggleRule(r, Boolean(v))" />
          </template>
          <template #title>{{ r.name }}</template>
          <template #sub>
            <el-tag size="small" :type="r.level === 'error' ? 'danger' : 'warning'" effect="plain">{{ r.level === 'error' ? t('perf.levelError') : t('perf.levelWarn') }}</el-tag>
          </template>
          <Field :label="t('perf.condition')">{{ conditionText(r) }}</Field>
          <Field v-if="isSiteMetric(r.metric)" :label="t('perf.scope')">{{ r.sites?.length ? r.sites.map(siteName).join(t('perf.separator')) : t('perf.allSites') }}</Field>
          <Field v-if="r.remark" :label="t('common.remark')">{{ r.remark }}</Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="glRules" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="Pencil" @click="openRule(r)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('perf.confirmDeleteRule')" @confirm="removeRule(r)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <LogCard url="/api/perf/logs" clearable />

    <el-dialog v-model="ruleVisible" :title="editing.key ? t('perf.editRule') : t('perf.addRule')" width="580px" :fullscreen="isMobile">
      <el-form :model="editing" label-width="110px" :label-position="labelPosition">
        <el-form-item v-if="!editing.key" :label="t('perf.template')">
          <div class="tpl-list">
            <el-button v-for="tpl in templates" :key="tpl.id" size="small" round @click="applyTemplate(tpl.rule)">{{ tpl.name }}</el-button>
          </div>
        </el-form-item>
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" maxlength="64" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="glRules.groupNames.value" />
        <el-form-item :label="t('perf.metric')">
          <el-select v-model="editing.metric" style="width: 100%" @change="onMetricChange">
            <el-option-group v-for="g in metricGroups" :key="g.scope" :label="g.label">
              <el-option v-for="m in g.items" :key="m.id" :label="metricLabel(m.id)" :value="m.id" />
            </el-option-group>
          </el-select>
          <div class="form-help">{{ metricHelp(editing.metric) }}</div>
        </el-form-item>
        <el-form-item :label="t('perf.threshold')">
          <div class="cond">
            <el-select v-model="editing.op" style="width: 80px">
              <el-option v-for="op in ['>', '>=', '<', '<=']" :key="op" :label="op" :value="op" />
            </el-select>
            <el-input-number v-model="editing.threshold" :min="0" :max="currentDef?.max" :step="thresholdStep" :precision="currentDef?.unit === '%' ? 1 : undefined" controls-position="right" />
            <span class="meta">{{ unitLabel(currentDef?.unit) }}</span>
          </div>
        </el-form-item>
        <el-form-item :label="t('perf.duration')">
          <div class="cond">
            <el-input-number v-model="editing.duration" :min="1" :max="1440" controls-position="right" />
            <span class="meta">{{ t('perf.minutes') }}</span>
          </div>
          <div class="form-help">{{ t('perf.durationHelp') }}</div>
        </el-form-item>
        <el-form-item v-if="isSiteMetric(editing.metric)" :label="t('perf.scope')">
          <el-select v-model="editing.sites" multiple filterable clearable style="width: 100%" :placeholder="t('perf.allSites')">
            <el-option v-for="s in siteOptions" :key="s.key" :label="s.name" :value="s.key" />
          </el-select>
          <div class="form-help">{{ t('perf.scopeHelp') }}</div>
        </el-form-item>
        <el-form-item v-if="needsMinRequests(editing.metric)" :label="t('perf.minRequests')">
          <el-input-number v-model="editing.minRequests" :min="1" :max="1000000" controls-position="right" />
          <div class="form-help">{{ t('perf.minRequestsHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('perf.level')">
          <el-radio-group v-model="editing.level">
            <el-radio value="warn">{{ t('perf.levelWarn') }}</el-radio>
            <el-radio value="error">{{ t('perf.levelError') }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" maxlength="200" /></el-form-item>
        <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>
        <div class="form-help">{{ t('perf.notifyHelp') }}</div>
      </el-form>
      <template #footer>
        <el-button @click="ruleVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveRule">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <SiteStats ref="siteStats" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Gauge, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api } from '../api'
import { moduleEnabled } from '../nav'
import PageHeader from '../components/PageHeader.vue'
import EntityCard from '../components/EntityCard.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import CompactToggle from '../components/CompactToggle.vue'
import Field from '../components/Field.vue'
import StatusBadge from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import SiteStats from '../components/SiteStats.vue'
import TrendChart, { type TrendSeries } from '../components/TrendChart.vue'
import { useGroupedList } from '../composables/useGroupSort'
import { useCompactView } from '../composables/useCompactView'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'

interface MetricDef {
  id: string
  scope: string
  unit: string
  window: boolean
  max: number
}

interface Rule {
  key?: string
  name: string
  group?: string
  enable: boolean
  metric: string
  op: string
  threshold: number
  duration: number
  sites: string[]
  minRequests: number
  level: string
  remark: string
}

interface Point {
  t: number
  cpu: number | null
  cpuMax: number | null
  mem: number | null
  disk: number | null
  load1: number | null
  procCpu: number | null
  procCpuMax: number | null
  procRss: number | null
  procFds: number | null
  goroutines: number | null
}

interface Overview {
  range: string
  step: number
  series: Point[]
  latest: Point | null
}

interface UpstreamView {
  key: string
  url: string
  route?: string
  healthy: boolean
  conns: number
}

interface SiteView {
  key: string
  name: string
  active: number
  qps: number | null
  err5xx: number | null
  p95: number | null
  requests: number
  latencies: number
  upstreams?: UpstreamView[]
}

interface AlertState {
  rule: string
  name: string
  subject: string
  target: string
  state: string
  since: number
  firedAt: number
  value: number
  level: string
}

interface NotifyRule {
  enable: boolean
  events: string[]
}

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
// 模块开关来自会话信息，用 computed 保证会话加载后同步更新
const webEnabled = computed(() => moduleEnabled('webservice'))
const notifyEnabled = computed(() => moduleEnabled('notify'))

const range = ref<'60s' | '1h' | '24h'>('1h')
const overview = ref<Overview>()
const sites = ref<SiteView[]>([])
const alerts = ref<AlertState[]>([])
const rules = ref<Rule[]>([])
const metrics = ref<MetricDef[]>([])
const notifyRules = ref<NotifyRule[] | null>(null)
const ruleVisible = ref(false)
const editing = ref<Rule>(emptyRule())
const siteStats = ref<InstanceType<typeof SiteStats>>()
const compactRules = useCompactView('perf.rules')
const glRules = useGroupedList<Rule>({
  storageKey: 'perf.rules.groups',
  items: () => rules.value,
  base: '/api/perf/rules',
  reload: loadRules,
  matches: (r, q) => [r.name, r.group ?? '', r.remark, metricLabel(r.metric)].join('\n').toLowerCase().includes(q),
})

// 各时间档的跨度（秒）
const spans: Record<string, number> = { '60s': 60, '1h': 3600, '24h': 86400 }
const to = computed(() => overview.value?.latest?.t ?? Math.floor(Date.now() / 1000))
const from = computed(() => to.value - spans[range.value])
const times = computed(() => (overview.value?.series ?? []).map((p) => p.t))

const colors = { a: 'var(--el-color-primary)', b: 'var(--g-chart-2)', warn: 'var(--el-color-warning)' }
const pct = (v: number) => v.toFixed(v >= 10 ? 0 : 1) + '%'
const num = (v: number) => (v >= 100 ? v.toFixed(0) : v.toFixed(v >= 10 ? 1 : 2))

function col(get: (p: Point) => number | null) {
  return (overview.value?.series ?? []).map(get)
}

function latestOf(get: (p: Point) => number | null, fmt: (v: number) => string) {
  const p = overview.value?.latest
  const v = p ? get(p) : null
  return v == null ? '-' : fmt(v)
}

// 60 秒档每个点就是一次采样，均值与峰值相同，不画峰值线
function withPeak(name: string, color: string, avg: (p: Point) => number | null, peak: (p: Point) => number | null): TrendSeries[] {
  const out: TrendSeries[] = [{ name, color, values: col(avg) }]
  if (range.value !== '60s') {
    out.push({ name: t('perf.peak'), color: colors.warn, values: col(peak), dashed: true })
  }
  return out
}

const charts = computed(() => [
  { key: 'cpu', title: t('perf.chartHostCpu'), max: 100, format: pct, latest: latestOf((p) => p.cpu, pct), series: withPeak(t('perf.average'), colors.a, (p) => p.cpu, (p) => p.cpuMax) },
  {
    key: 'mem',
    title: t('perf.chartHostMem'),
    max: 100,
    format: pct,
    latest: latestOf((p) => p.mem, pct),
    series: [
      { name: t('perf.memory'), color: colors.a, values: col((p) => p.mem) },
      { name: t('perf.disk'), color: colors.b, values: col((p) => p.disk) },
    ],
  },
  { key: 'load', title: t('perf.chartLoad'), max: undefined, format: num, latest: latestOf((p) => p.load1, num), series: [{ name: t('perf.load1'), color: colors.a, values: col((p) => p.load1) }] },
  { key: 'pcpu', title: t('perf.chartProcCpu'), max: undefined, format: pct, latest: latestOf((p) => p.procCpu, pct), series: withPeak(t('perf.average'), colors.b, (p) => p.procCpu, (p) => p.procCpuMax) },
  { key: 'rss', title: t('perf.chartProcRss'), max: undefined, format: (v: number) => v.toFixed(0) + ' MB', latest: latestOf((p) => p.procRss, (v) => v.toFixed(0) + ' MB'), series: [{ name: 'RSS', color: colors.b, values: col((p) => p.procRss) }] },
  {
    key: 'fds',
    title: t('perf.chartProcFds'),
    max: undefined,
    format: (v: number) => v.toFixed(0),
    latest: latestOf((p) => p.procFds, (v) => v.toFixed(0)),
    series: [
      { name: t('perf.fds'), color: colors.b, values: col((p) => p.procFds) },
      { name: t('perf.goroutines'), color: colors.a, values: col((p) => p.goroutines) },
    ],
  },
])

const currentDef = computed(() => metrics.value.find((m) => m.id === editing.value.metric))
const thresholdStep = computed(() => {
  switch (currentDef.value?.unit) {
    case '%':
      return 5
    case 'MB':
      return 64
    case 'ms':
      return 100
  }
  return 10
})

const metricGroups = computed(() => {
  const order = ['host', 'process', 'site', 'upstream']
  return order
    .map((scope) => ({ scope, label: t(`perf.scopes.${scope}`), items: metrics.value.filter((m) => m.scope === scope) }))
    .filter((g) => g.items.length && (webEnabled.value || (g.scope !== 'site' && g.scope !== 'upstream')))
})

// 站点范围的选项：运行中的站点，加上规则里引用但当前未运行的站点（按标识显示）
const siteOptions = computed(() => {
  const out = sites.value.map((s) => ({ key: s.key, name: s.name }))
  for (const k of editing.value.sites ?? []) {
    if (!out.some((s) => s.key === k)) {
      out.push({ key: k, name: siteName(k) })
    }
  }
  return out
})

const templates = computed(() => [
  { id: 'cpu', name: t('perf.tpl.hostCpu'), rule: { name: t('perf.tpl.hostCpu'), metric: 'host.cpu', op: '>', threshold: 90, duration: 5, level: 'warn' } },
  { id: 'mem', name: t('perf.tpl.hostMem'), rule: { name: t('perf.tpl.hostMem'), metric: 'host.mem', op: '>', threshold: 90, duration: 5, level: 'warn' } },
  { id: 'disk', name: t('perf.tpl.hostDisk'), rule: { name: t('perf.tpl.hostDisk'), metric: 'host.disk', op: '>', threshold: 90, duration: 10, level: 'error' } },
  { id: 'pcpu', name: t('perf.tpl.procCpu'), rule: { name: t('perf.tpl.procCpu'), metric: 'proc.cpu', op: '>', threshold: 80, duration: 5, level: 'warn' } },
  { id: 'rss', name: t('perf.tpl.procRss'), rule: { name: t('perf.tpl.procRss'), metric: 'proc.rss', op: '>', threshold: 1024, duration: 10, level: 'warn' } },
  ...(webEnabled.value
    ? [
        { id: '5xx', name: t('perf.tpl.site5xx'), rule: { name: t('perf.tpl.site5xx'), metric: 'site.err5xx', op: '>', threshold: 5, duration: 3, level: 'error', minRequests: 20 } },
        { id: 'p95', name: t('perf.tpl.siteP95'), rule: { name: t('perf.tpl.siteP95'), metric: 'site.p95', op: '>', threshold: 2000, duration: 5, level: 'warn', minRequests: 20 } },
        { id: 'active', name: t('perf.tpl.siteActive'), rule: { name: t('perf.tpl.siteActive'), metric: 'site.active', op: '>', threshold: 500, duration: 3, level: 'warn' } },
      ]
    : []),
])

// 通知中心里有没有订阅性能告警的规则：没有时提示，告警只会写进日志
const notifyHint = computed<{ type: 'warning' | 'info'; text: string; link: boolean } | null>(() => {
  if (!rules.value.some((r) => r.enable)) {
    return null
  }
  if (!notifyEnabled.value) {
    return { type: 'info', text: t('perf.notifyDisabled'), link: false }
  }
  if (notifyRules.value === null) {
    return null
  }
  const subscribed = notifyRules.value.some((r) => r.enable && r.events.some((e) => globMatch(e, 'perf.alert')))
  return subscribed ? null : { type: 'warning', text: t('perf.notifyMissing'), link: true }
})

// globMatch 与后端 path.Match 一致地处理 * 与 ?（事件类型不含 /）
function globMatch(pattern: string, s: string) {
  if (pattern === '*') {
    return true
  }
  const re = new RegExp('^' + pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*').replace(/\?/g, '.') + '$')
  return re.test(s)
}

function emptyRule(): Rule {
  return { name: '', enable: true, metric: 'host.cpu', op: '>', threshold: 90, duration: 5, sites: [], minRequests: 0, level: 'warn', remark: '' }
}

function defOf(id: string) {
  return metrics.value.find((m) => m.id === id)
}

function isSiteMetric(id: string) {
  const s = defOf(id)?.scope
  return s === 'site' || s === 'upstream'
}

// 5xx 比例与延迟需要最小请求数，每秒请求数不需要
function needsMinRequests(id: string) {
  return id === 'site.err5xx' || id === 'site.p95'
}

function metricLabel(id: string) {
  return t(`perf.metrics.${id.replace('.', '_')}`)
}

function metricHelp(id: string) {
  return t(`perf.metricHelp.${id.replace('.', '_')}`)
}

function unitLabel(unit?: string) {
  switch (unit) {
    case '%':
      return '%'
    case 'MB':
      return 'MB'
    case 'req/s':
      return t('perf.unitQps')
    case 'ms':
      return t('perf.unitMs')
  }
  return ''
}

function formatMs(v: number) {
  return v >= 1000 ? (v / 1000).toFixed(2) + ' s' : v.toFixed(0) + ' ms'
}

function formatMetric(id: string, v: number) {
  switch (defOf(id)?.unit) {
    case '%':
      return v.toFixed(1) + '%'
    case 'MB':
      return v.toFixed(0) + ' MB'
    case 'req/s':
      return v.toFixed(1) + ' ' + t('perf.unitQps')
    case 'ms':
      return formatMs(v)
  }
  return v.toFixed(0)
}

function conditionText(r: Rule) {
  return t('perf.conditionText', { metric: metricLabel(r.metric), op: r.op, value: `${r.threshold}${unitLabel(defOf(r.metric)?.unit) ? ' ' + unitLabel(defOf(r.metric)?.unit) : ''}`, minutes: r.duration })
}

function siteName(key: string) {
  return sites.value.find((s) => s.key === key)?.name ?? key
}

function ruleMetric(key: string) {
  return rules.value.find((r) => r.key === key)?.metric ?? ''
}

function targetName(a: AlertState) {
  if (a.subject === 'host') {
    return t('perf.targetHost')
  }
  if (a.subject === 'process') {
    return t('perf.targetProcess')
  }
  return a.target
}

function ago(sec: number) {
  const d = Math.max(0, Math.floor(Date.now() / 1000) - sec)
  if (d < 60) {
    return t('perf.seconds', { n: d })
  }
  if (d < 3600) {
    return t('perf.minutesN', { n: Math.floor(d / 60) })
  }
  return t('perf.hoursMinutes', { h: Math.floor(d / 3600), m: Math.floor((d % 3600) / 60) })
}

function unhealthy(s: SiteView) {
  return (s.upstreams ?? []).filter((u) => !u.healthy).length
}

function openSite(row: SiteView) {
  // 站点标识为 “监听 key/站点 key”
  const i = row.key.lastIndexOf('/')
  siteStats.value?.open(row.key.slice(0, i), row.key.slice(i + 1), row.name)
}

function onMetricChange() {
  const r = editing.value
  if (!isSiteMetric(r.metric)) {
    r.sites = []
  }
  r.minRequests = needsMinRequests(r.metric) ? r.minRequests || 20 : 0
  const max = defOf(r.metric)?.max
  if (max !== undefined && r.threshold > max) {
    r.threshold = max
  }
}

function applyTemplate(tpl: Partial<Rule>) {
  editing.value = { ...emptyRule(), ...tpl }
}

function openRule(r?: Rule) {
  editing.value = r ? { ...emptyRule(), ...JSON.parse(JSON.stringify(r)) } : emptyRule()
  editing.value.sites ??= []
  ruleVisible.value = true
}

async function saveRule() {
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/perf/rules/${key}`, body)
    } else {
      await api.post('/api/perf/rules', body)
    }
    ruleVisible.value = false
    await loadRules()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function toggleRule(r: Rule, enable: boolean) {
  try {
    await api.put(`/api/perf/rules/${r.key}/enable`, { enable })
    await loadRules()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function removeRule(r: Rule) {
  try {
    await api.del(`/api/perf/rules/${r.key}`)
    await loadRules()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function loadRules() {
  rules.value = await api.get<Rule[]>('/api/perf/rules')
}

async function loadOverview() {
  try {
    overview.value = await api.get<Overview>(`/api/perf/overview?range=${range.value}`)
    lastOverview = Date.now()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function loadLive() {
  const [a, s] = await Promise.all([api.get<AlertState[]>('/api/perf/alerts'), webEnabled.value ? api.get<SiteView[]>('/api/perf/sites') : Promise.resolve([])])
  alerts.value = a
  sites.value = s
}

async function loadNotify() {
  if (!notifyEnabled.value) {
    return
  }
  try {
    notifyRules.value = await api.get<NotifyRule[]>('/api/notify/rules')
  } catch {
    // 只读令牌或权限不足时不提示
    notifyRules.value = null
  }
}

// 60 秒档每 5 秒刷新历史，其他档每 30 秒；告警与站点指标每 5 秒刷新
let lastOverview = 0
const polling = usePolling(async () => {
  const every = range.value === '60s' ? 5000 : 30000
  const jobs: Promise<unknown>[] = [loadLive()]
  if (Date.now() - lastOverview >= every - 500) {
    jobs.push(loadOverview())
  }
  await Promise.all(jobs)
}, 5000)

onMounted(async () => {
  try {
    metrics.value = await api.get<MetricDef[]>('/api/perf/metrics')
    await Promise.all([loadOverview(), loadLive(), loadRules(), loadNotify()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
  polling.start()
})
</script>

<style scoped>
.section-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px 12px;
}
.section-tools {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.section {
  margin: 22px 0 10px;
  font-size: 15px;
  font-weight: 600;
}
.charts {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;
}
.chart-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  padding: 10px 12px 8px;
}
.chart-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  font-size: 13px;
  margin-bottom: 6px;
}
.chart-head b {
  font-size: 16px;
  font-variant-numeric: tabular-nums;
}
.alerts {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 8px;
}
.alert {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 12px;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid var(--el-border-color-lighter);
  font-size: 13px;
}
.alert.error {
  border-color: var(--el-color-danger-light-5);
  background: var(--el-color-danger-light-9);
}
.alert.warn {
  border-color: var(--el-color-warning-light-5);
  background: var(--el-color-warning-light-9);
}
.meta {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.bad {
  color: var(--el-color-danger);
}
.sites :deep(.el-table__row) {
  cursor: pointer;
}
.cond {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tpl-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.notify-hint {
  margin-bottom: 10px;
}
.notify-hint a {
  margin-left: 6px;
}
.form-help {
  margin-top: 6px;
}
</style>
