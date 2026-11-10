<template>
  <!-- 访客审计：按访客地址汇总全部站点的访问，数据来自 GET /api/webservice/visitors -->
  <div class="visitors-page">
    <PageHeader :title="t('visitors.title')" :desc="t('visitors.desc')" :icon="Footprints">
      <el-radio-group v-model="days" size="small" class="range" @change="reload">
        <el-radio-button v-for="r in ranges" :key="r.value" :value="r.value">{{ r.label }}</el-radio-button>
      </el-radio-group>
      <el-button :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      <el-button v-if="!isReadonly" :icon="Settings2" @click="openSettings">{{ t('visitors.settings') }}</el-button>
    </PageHeader>

    <el-alert v-if="info?.disable" type="warning" :closable="false" show-icon class="paused" :title="t('visitors.paused')">
      <template v-if="!isReadonly" #default>
        <el-button size="small" type="primary" plain :loading="saving" @click="resume">{{ t('visitors.resume') }}</el-button>
      </template>
    </el-alert>
    <el-alert v-if="info?.error" type="error" :closable="false" show-icon class="paused" :title="t('visitors.saveError', { error: info.error })" />

    <!-- 汇总：统计范围内、按当前筛选条件 -->
    <div class="tiles">
      <button type="button" class="tile" :class="{ active: !filter }" @click="setFilter('')">
        <span class="tile-label"><Users :size="14" />{{ t('visitors.tileVisitors') }}</span>
        <b>{{ num(summary.visitors) }}</b>
        <span class="tile-sub">{{ t('visitors.tileNew', { n: num(summary.new) }) }}</span>
      </button>
      <div class="tile">
        <span class="tile-label"><Activity :size="14" />{{ t('visitors.tileRequests') }}</span>
        <b>{{ num(summary.requests) }}</b>
        <span class="tile-sub">{{ t('visitors.tileRange', { range: rangeLabel }) }}</span>
      </div>
      <button type="button" class="tile" :class="{ active: filter === 'denied', danger: summary.denied > 0 }" @click="setFilter('denied')">
        <span class="tile-label"><ShieldX :size="14" />{{ t('visitors.tileDenied') }}</span>
        <b>{{ num(summary.denied) }}</b>
        <span class="tile-sub">{{ t('visitors.tileDeniedRate', { rate: deniedRate }) }}</span>
      </button>
      <button type="button" class="tile" :class="{ active: filter === 'authed' }" @click="setFilter('authed')">
        <span class="tile-label"><UserCheck :size="14" />{{ t('visitors.tileAuthed') }}</span>
        <b>{{ num(summary.authed) }}</b>
        <span class="tile-sub">{{ t('visitors.tileAuthedSub') }}</span>
      </button>
      <button type="button" class="tile" :class="{ active: filter === 'banned', warn: summary.banned > 0 }" @click="setFilter('banned')">
        <span class="tile-label"><ShieldBan :size="14" />{{ t('visitors.tileBanned') }}</span>
        <b>{{ num(summary.banned) }}</b>
        <span class="tile-sub">{{ t('visitors.tileNoise', { n: num(summary.noise) }) }}</span>
      </button>
    </div>

    <!-- 每日趋势：柱高为请求数，红色部分为被拦截 -->
    <div v-if="daily.length > 1" class="g-card chart-card">
      <div class="g-card-head">
        <span class="g-card-title"><ChartColumn :size="16" />{{ t('visitors.dailyTitle') }}</span>
        <span v-if="focusDay" class="day-readout">
          <b>{{ focusDay.day }}</b>
          <span>{{ t('visitors.dayVisitors', { n: num(focusDay.visitors) }) }}</span>
          <span>{{ t('visitors.dayRequests', { n: num(focusDay.requests) }) }}</span>
          <span class="red">{{ t('visitors.dayDenied', { n: num(focusDay.denied) }) }}</span>
        </span>
      </div>
      <div class="g-card-body">
        <DayBars :days="daily" :selected="dayIndex" @select="(i: number) => (dayIndex = i)" />
      </div>
    </div>

    <div ref="listCard" class="g-card list-card">
      <div class="g-card-head filters g-sticky-bar g-sticky-desktop">
        <el-input v-model="keyword" clearable :prefix-icon="Search" :placeholder="t('visitors.search')" class="f-kw" @input="onKeyword" @clear="reload" />
        <el-select v-model="filter" class="f-filter" :placeholder="t('visitors.filterAll')" clearable value-on-clear="" @change="reload">
          <el-option v-for="f in filters" :key="f.value" :label="f.label" :value="f.value" />
        </el-select>
        <el-select v-model="site" class="f-site" filterable clearable value-on-clear="" :placeholder="t('visitors.siteAll')" @change="reload">
          <el-option v-for="s in siteOptions" :key="s.value" :label="s.label" :value="s.value" />
        </el-select>
        <el-select v-model="sort" class="f-sort" @change="reload">
          <el-option v-for="s in sorts" :key="s.value" :label="s.label" :value="s.value" />
        </el-select>
        <span class="g-card-desc total">{{ t('visitors.total', { n: num(total) }) }}</span>
      </div>

      <div v-loading="loading" class="g-card-body list-body">
        <el-empty v-if="!items.length && !loading" :description="emptyText" :image-size="72" />

        <!-- 桌面端：表格 -->
        <el-table v-else-if="!isMobile" :data="items" row-key="ip" class="v-table" @row-click="(row: Item) => openDetail(row.ip)">
          <el-table-column :label="t('visitors.colVisitor')" min-width="250">
            <template #default="{ row }">
              <div class="ip-line">
                <span class="mono ip">{{ row.ip }}</span>
                <span v-if="row.country" class="cc" :title="countryTitle(row.country)">{{ countryText(row.country) }}</span>
              </div>
              <div class="tags">
                <el-tag v-if="row.banned" size="small" type="danger" effect="dark">{{ t('visitors.banned') }}</el-tag>
                <el-tag v-if="row.noise" size="small" type="info" effect="plain">{{ t('visitors.noise') }}</el-tag>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="t('visitors.colRequests')" width="120" align="right">
            <template #default="{ row }">
              <div class="mono strong">{{ num(row.requests) }}</div>
              <div v-if="row.denied" class="mono red small">{{ t('visitors.deniedN', { n: num(row.denied) }) }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="t('visitors.colSites')" min-width="180">
            <template #default="{ row }">
              <div class="chips">
                <span v-for="s in row.sites" :key="s.listener + '/' + s.key + (s.unmatched ? '-' : '')" class="chip" :class="{ muted: s.unmatched }" :title="siteTitle(s)">{{ siteChip(s) }}</span>
                <span v-if="row.siteCount > row.sites.length" class="chip more">+{{ row.siteCount - row.sites.length }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="t('visitors.colIdentity')" min-width="130">
            <template #default="{ row }">
              <div v-if="row.identities.length" class="chips">
                <span v-for="id in row.identities.slice(0, 3)" :key="id.kind + id.name" class="chip ident" :class="id.kind" :title="identTitle(id)">
                  <component :is="id.kind === 'admin' ? ShieldCheck : User" :size="12" />{{ id.name }}
                </span>
              </div>
              <span v-else class="meta">-</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('visitors.colLast')" width="150">
            <template #default="{ row }">
              <div :title="fullTime(row.last)">{{ ago(row.last) }}</div>
              <div class="meta small" :title="fullTime(row.first)">{{ t('visitors.firstAt', { t: shortTime(row.first) }) }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="t('visitors.colLastReq')" min-width="260">
            <template #default="{ row }">
              <div class="req-line">
                <el-tag size="small" effect="plain" :type="statusType(row.lastReq.status, row.lastReq.deny)" class="st">{{ statusText(row.lastReq.status) }}</el-tag>
                <span class="mono ellipsis" :title="reqText(row.lastReq)">{{ row.lastReq.method }} {{ row.lastReq.path }}</span>
              </div>
              <div class="meta small ellipsis" :title="row.agent">{{ row.lastReq.host || '-' }}<template v-if="row.agent"> · {{ row.agent }}</template></div>
            </template>
          </el-table-column>
          <el-table-column width="48" align="center">
            <template #default>
              <ChevronRight :size="16" class="meta" />
            </template>
          </el-table-column>
        </el-table>

        <!-- 移动端：卡片 -->
        <div v-else class="v-cards">
          <button v-for="row in items" :key="row.ip" type="button" class="v-card" @click="openDetail(row.ip)">
            <div class="vc-top">
              <span class="mono ip">{{ row.ip }}</span>
              <span v-if="row.country" class="cc">{{ countryText(row.country) }}</span>
              <el-tag v-if="row.banned" size="small" type="danger" effect="dark">{{ t('visitors.banned') }}</el-tag>
              <el-tag v-if="row.noise" size="small" type="info" effect="plain">{{ t('visitors.noise') }}</el-tag>
              <span class="vc-time">{{ ago(row.last) }}</span>
            </div>
            <div class="vc-stats">
              <span>{{ t('visitors.reqN', { n: num(row.requests) }) }}</span>
              <span v-if="row.denied" class="red">{{ t('visitors.deniedN', { n: num(row.denied) }) }}</span>
              <span>{{ t('visitors.sitesN', { n: row.siteCount }) }}</span>
            </div>
            <div class="req-line">
              <el-tag size="small" effect="plain" :type="statusType(row.lastReq.status, row.lastReq.deny)" class="st">{{ statusText(row.lastReq.status) }}</el-tag>
              <span class="mono ellipsis">{{ row.lastReq.method }} {{ row.lastReq.host }}{{ row.lastReq.path }}</span>
            </div>
            <div v-if="row.identities.length || row.sites.length" class="chips">
              <span v-for="id in row.identities.slice(0, 2)" :key="id.kind + id.name" class="chip ident" :class="id.kind">
                <component :is="id.kind === 'admin' ? ShieldCheck : User" :size="12" />{{ id.name }}
              </span>
              <span v-for="s in row.sites.slice(0, 2)" :key="s.listener + '/' + s.key + (s.unmatched ? '-' : '')" class="chip" :class="{ muted: s.unmatched }">{{ siteChip(s) }}</span>
            </div>
          </button>
        </div>

        <div v-if="total > pageSize" class="pager" :class="{ mobile: isMobile }">
          <el-pagination
            v-model:current-page="page"
            :page-size="pageSize"
            :total="total"
            :layout="isMobile ? 'prev, pager, next' : 'total, prev, pager, next, jumper'"
            :pager-count="isMobile ? 5 : 7"
            :size="isMobile ? 'small' : 'default'"
            background
            @current-change="onPage"
          />
        </div>
      </div>
    </div>

    <div v-if="info" class="storage meta">
      <Database :size="13" />
      <span>{{ t('visitors.storage', { days: info.days, count: num(info.count), max: num(info.max), size: formatBytes(info.fileBytes) }) }}</span>
      <span v-if="info.savedAt">{{ t('visitors.savedAt', { t: shortTime(info.savedAt) }) }}</span>
      <span>{{ t('visitors.storageHelp') }}</span>
    </div>

    <!-- 访客详情 -->
    <el-drawer v-model="detailVisible" :size="isMobile ? '100%' : '620px'" :with-header="false" append-to-body class="visitor-drawer" @closed="detail = null">
      <div v-loading="detailLoading" class="drawer">
        <div class="d-head">
          <div class="d-title">
            <span class="mono d-ip">{{ detailIP }}</span>
            <el-button text size="small" :icon="Copy" :title="t('common.copy')" :aria-label="t('common.copy')" @click="copyText(detailIP)" />
            <span v-if="detail?.country" class="cc">{{ countryText(detail.country) }}</span>
            <el-tag v-if="detail?.banned" size="small" type="danger" effect="dark">{{ t('visitors.banned') }}</el-tag>
            <el-tag v-if="detail?.noiseOnly" size="small" type="info" effect="plain">{{ t('visitors.noise') }}</el-tag>
          </div>
          <el-button text :icon="X" :aria-label="t('common.close')" @click="detailVisible = false" />
        </div>
        <div class="d-actions">
          <el-button size="small" :icon="ScrollText" @click="openLogs">{{ t('visitors.viewLogs') }}</el-button>
          <template v-if="!isReadonly && detail">
            <el-button v-if="detail.banned" size="small" :icon="ShieldOff" :loading="banning" @click="unban">{{ t('visitors.unban') }}</el-button>
            <el-button v-else size="small" type="danger" plain :icon="ShieldBan" @click="banVisible = true">{{ t('visitors.ban') }}</el-button>
            <el-popconfirm :title="t('visitors.deleteConfirm')" width="260" @confirm="removeOne">
              <template #reference>
                <el-button size="small" :icon="Trash2">{{ t('visitors.deleteRecord') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </div>

        <template v-if="detail">
          <div class="d-grid">
            <div class="d-cell">
              <span>{{ t('visitors.firstSeen') }}</span>
              <b :title="fullTime(detail.first)">{{ shortTime(detail.first) }}</b>
            </div>
            <div class="d-cell">
              <span>{{ t('visitors.lastSeen') }}</span>
              <b :title="fullTime(detail.last)">{{ ago(detail.last) }}</b>
            </div>
            <div class="d-cell">
              <span>{{ t('visitors.totalRequests') }}</span>
              <b>{{ num(detail.requests) }}</b>
            </div>
            <div class="d-cell">
              <span>{{ t('visitors.totalDenied') }}</span>
              <b :class="{ red: detail.denied }">{{ num(detail.denied) }}</b>
            </div>
            <div class="d-cell">
              <span>{{ t('visitors.traffic') }}</span>
              <b>{{ formatBytes(detail.bytes) }}</b>
            </div>
            <div class="d-cell">
              <span>{{ t('visitors.closed') }}</span>
              <b>{{ num(detail.closed) }}</b>
            </div>
          </div>

          <section class="d-sec">
            <h4>{{ t('visitors.dailyTitle') }}</h4>
            <DayBars :days="detail.daily" compact />
          </section>

          <section class="d-sec">
            <h4>{{ t('visitors.statusTitle') }}</h4>
            <div class="status-bar">
              <span v-for="s in statusSegs" :key="s.label" :class="'seg ' + s.cls" :style="{ flexGrow: s.n }" :title="s.label + ': ' + s.n" />
            </div>
            <div class="status-legend">
              <span v-for="s in statusSegs" :key="s.label"><i :class="'dot ' + s.cls" />{{ s.label }} {{ num(s.n) }}</span>
            </div>
            <div v-if="detail.deny.length" class="chips deny">
              <span class="meta">{{ t('visitors.denyReasons') }}</span>
              <span v-for="d in detail.deny" :key="d.name" class="chip danger">{{ denyName(d.name) }} × {{ num(d.n) }}</span>
            </div>
          </section>

          <section class="d-sec">
            <h4>{{ t('visitors.sitesTitle') }}</h4>
            <div class="rows">
              <div v-for="s in detail.sites" :key="s.listener + '/' + s.key + (s.unmatched ? '-' : '')" class="row">
                <div class="row-main">
                  <span class="ellipsis" :class="{ meta: s.unmatched }">{{ siteName(s) }}</span>
                  <span class="meta small">{{ listenerName(s.listener) }}</span>
                </div>
                <div class="row-side">
                  <span class="mono">{{ num(s.req) }}</span>
                  <span v-if="s.denied" class="mono red small">{{ t('visitors.deniedN', { n: num(s.denied) }) }}</span>
                  <span class="meta small">{{ ago(s.last) }}</span>
                </div>
              </div>
            </div>
          </section>

          <section class="d-sec">
            <h4>{{ t('visitors.pathsTitle') }}</h4>
            <div class="rows">
              <div v-for="p in detail.paths" :key="p.path" class="row">
                <div class="row-main">
                  <el-tag size="small" effect="plain" :type="statusType(p.status)" class="st">{{ statusText(p.status) }}</el-tag>
                  <span class="mono ellipsis" :title="p.path">{{ p.path }}</span>
                </div>
                <div class="row-side">
                  <span class="mono">{{ num(p.req) }}</span>
                  <span class="meta small">{{ ago(p.last) }}</span>
                </div>
              </div>
            </div>
            <div class="meta small note">{{ t('visitors.pathsHelp') }}</div>
          </section>

          <section v-if="detail.identities.length" class="d-sec">
            <h4>{{ t('visitors.identitiesTitle') }}</h4>
            <div class="rows">
              <div v-for="id in detail.identities" :key="id.kind + id.name" class="row">
                <div class="row-main">
                  <span class="chip ident" :class="id.kind"><component :is="id.kind === 'admin' ? ShieldCheck : User" :size="12" />{{ id.name }}</span>
                  <span class="meta small">{{ id.kind === 'admin' ? t('visitors.kindAdmin') : t('visitors.kindUser') }}</span>
                </div>
                <div class="row-side">
                  <span class="mono">{{ num(id.req) }}</span>
                  <span class="meta small">{{ ago(id.last) }}</span>
                </div>
              </div>
            </div>
          </section>

          <section v-if="detail.agents.length" class="d-sec">
            <h4>{{ t('visitors.agentsTitle') }}</h4>
            <div class="rows">
              <div v-for="a in detail.agents" :key="a.ua" class="row agent">
                <div class="row-main"><span class="ua">{{ a.ua }}</span></div>
                <div class="row-side">
                  <span class="mono">{{ num(a.req) }}</span>
                </div>
              </div>
            </div>
          </section>
        </template>
      </div>
    </el-drawer>

    <el-dialog v-model="banVisible" :title="t('visitors.banTitle')" width="420px" append-to-body>
      <p>{{ t('visitors.banDesc', { ip: detailIP }) }}</p>
      <p v-if="detailIP.includes(':')" class="meta">{{ t('visitors.banIpv6') }}</p>
      <div class="ban-hours">
        <el-input-number v-model="banHours" :min="1" :max="720" :disabled="banPermanent" />
        <span>{{ t('visitors.hours') }}</span>
        <el-checkbox v-model="banPermanent">{{ t('visitors.permanent') }}</el-checkbox>
      </div>
      <template #footer>
        <el-button @click="banVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :loading="banning" @click="confirmBan">{{ t('visitors.ban') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="settingsVisible" :title="t('visitors.settingsTitle')" width="520px" :fullscreen="isMobile" append-to-body>
      <el-form :label-position="labelPosition" label-width="140px">
        <el-form-item :label="t('visitors.enable')">
          <el-switch v-model="form.enable" />
          <div class="form-help">{{ t('visitors.enableHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('visitors.days')">
          <el-input-number v-model="form.days" :min="1" :max="180" />
          <div class="form-help">{{ t('visitors.daysHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('visitors.max')">
          <el-input-number v-model="form.max" :min="100" :max="100000" :step="1000" />
          <div class="form-help">{{ t('visitors.maxHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('visitors.clearAll')">
          <el-button type="danger" plain :icon="Trash2" :disabled="!info?.count" @click="clearAll">{{ t('visitors.clearAllBtn', { n: num(info?.count ?? 0) }) }}</el-button>
        </el-form-item>
      </el-form>
      <div class="form-help">{{ t('visitors.privacyHelp') }}</div>
      <template #footer>
        <el-button @click="settingsVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="saveSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <SiteLogDialog ref="logsDialog" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Activity,
  ChartColumn,
  ChevronRight,
  Copy,
  Database,
  Footprints,
  RefreshCw,
  ScrollText,
  Search,
  Settings2,
  ShieldBan,
  ShieldCheck,
  ShieldOff,
  ShieldX,
  Trash2,
  User,
  UserCheck,
  Users,
  X,
} from 'lucide-vue-next'
import PageHeader from '../components/PageHeader.vue'
import SiteLogDialog from '../components/SiteLogDialog.vue'
import DayBars from '../components/VisitorDayBars.vue'
import { api, formatBytes } from '../api'
import { isReadonly } from '../nav'
import { useFormLayout } from '../composables/useIsMobile'
import { copyText } from '../composables/useClipboard'
import type { DayPoint, Ident, Item, LastReq, SiteRef, VisitorDetail, VisitorInfo, VisitorList, VisitorSummary } from '../visitors'
import { latestOnly, shrinkImpact } from '../visitors'

const { t, locale } = useI18n()
const { isMobile, labelPosition } = useFormLayout()

const ranges = computed(() => [
  { value: 1, label: t('visitors.rangeToday') },
  { value: 7, label: t('visitors.range7') },
  { value: 30, label: t('visitors.range30') },
  { value: 0, label: t('visitors.rangeAll') },
])
const filters = computed(() => [
  { value: 'denied', label: t('visitors.filterDenied') },
  { value: 'authed', label: t('visitors.filterAuthed') },
  { value: 'admin', label: t('visitors.filterAdmin') },
  { value: 'banned', label: t('visitors.filterBanned') },
  { value: 'noise', label: t('visitors.filterNoise') },
])
const sorts = computed(() => [
  { value: 'last', label: t('visitors.sortLast') },
  { value: 'requests', label: t('visitors.sortRequests') },
  { value: 'denied', label: t('visitors.sortDenied') },
  { value: 'first', label: t('visitors.sortFirst') },
])

const days = ref(7)
const keyword = ref('')
const filter = ref('')
const site = ref('')
const sort = ref('last')
const page = ref(1)
const pageSize = 50
const loading = ref(false)
const listCard = ref<HTMLElement>()

const items = ref<Item[]>([])
const total = ref(0)
const summary = ref<VisitorSummary>({ visitors: 0, requests: 0, denied: 0, authed: 0, banned: 0, noise: 0, new: 0 })
const daily = ref<DayPoint[]>([])
const sites = ref<SiteRef[]>([])
const listeners = ref<Record<string, string>>({})
const info = ref<VisitorInfo | null>(null)
// dayIndex 为趋势图中选中的一天，-1 表示最近一天
const dayIndex = ref(-1)

const nf = computed(() => new Intl.NumberFormat(locale.value))
function num(n: number): string {
  return nf.value.format(n)
}

const rangeLabel = computed(() => ranges.value.find((r) => r.value === days.value)?.label ?? '')
const deniedRate = computed(() => {
  const s = summary.value
  return s.requests ? ((s.denied / s.requests) * 100).toFixed(s.denied * 1000 < s.requests ? 2 : 1) + '%' : '0%'
})
const focusDay = computed(() => {
  const d = daily.value
  if (!d.length) {
    return null
  }
  return d[dayIndex.value >= 0 && dayIndex.value < d.length ? dayIndex.value : d.length - 1]
})
const emptyText = computed(() => (keyword.value || filter.value || site.value ? t('visitors.emptyFiltered') : t('visitors.empty')))

function listenerName(key: string): string {
  return listeners.value[key] ?? key
}
function siteName(s: { name: string; key: string; unmatched?: boolean }): string {
  if (s.unmatched) {
    return t('visitors.unmatched')
  }
  return s.name || s.key
}
function siteTitle(s: SiteRef): string {
  const base = listenerName(s.listener) + ' / ' + siteName(s)
  return s.req ? base + ' · ' + t('visitors.reqN', { n: num(s.req) }) : base
}
// 同名站点分在不同监听（例如分开监听 IPv4、IPv6）时，标签与筛选项附上监听名称区分
const dupNames = computed(() => {
  const counts: Record<string, number> = {}
  for (const s of sites.value) {
    if (!s.unmatched) {
      const n = siteName(s)
      counts[n] = (counts[n] ?? 0) + 1
    }
  }
  return new Set(Object.keys(counts).filter((n) => counts[n] > 1))
})
function siteChip(s: SiteRef): string {
  const name = siteName(s)
  return !s.unmatched && dupNames.value.has(name) ? `${name} (${listenerName(s.listener)})` : name
}
const siteOptions = computed(() => {
  return sites.value.map((s) => {
    const label = siteChip(s)
    return { value: s.unmatched ? '-' : `${s.listener}/${s.key}`, label }
  })
})
function identTitle(id: Ident): string {
  return (id.kind === 'admin' ? t('visitors.kindAdmin') : t('visitors.kindUser')) + ' · ' + t('visitors.reqN', { n: num(id.req) })
}
function countryText(cc: string): string {
  return cc === 'LAN' ? t('visitors.lan') : cc
}
function countryTitle(cc: string): string {
  return cc === 'LAN' ? t('visitors.lanHelp') : cc
}
function denyName(d: string): string {
  switch (d) {
    case 'ban':
      return t('components.logViewer.deny.ban')
    case 'ip':
      return t('components.logViewer.deny.ip')
    case 'country':
      return t('components.logViewer.deny.country')
    case 'rateLimit':
      return t('components.logViewer.deny.rateLimit')
    case 'waf':
      return t('components.logViewer.deny.waf')
  }
  return d
}
function statusText(s: number): string {
  return s ? String(s) : '-'
}
function statusType(s: number, deny?: string): 'success' | 'info' | 'warning' | 'danger' {
  if (deny || s === 444 || s >= 500) {
    return 'danger'
  }
  if (s >= 400) {
    return 'warning'
  }
  if (s >= 300) {
    return 'info'
  }
  return 'success'
}
function reqText(r: LastReq): string {
  return `${r.method} ${r.host}${r.path} → ${r.status}${r.siteName ? ' · ' + r.siteName : ''}`
}

// 时间显示：最近访问用相对时间，悬停显示完整时间
const nowSec = ref(Math.floor(Date.now() / 1000))
function ago(sec: number): string {
  const d = Math.max(0, nowSec.value - sec)
  if (d < 60) {
    return t('visitors.justNow')
  }
  if (d < 3600) {
    return t('visitors.minutesAgo', { n: Math.floor(d / 60) })
  }
  if (d < 86400) {
    return t('visitors.hoursAgo', { n: Math.floor(d / 3600) })
  }
  return t('visitors.daysAgo', { n: Math.floor(d / 86400) })
}
function fullTime(sec: number): string {
  return new Date(sec * 1000).toLocaleString(locale.value)
}
function shortTime(sec: number): string {
  return new Date(sec * 1000).toLocaleString(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

// 列表请求序号：条件快速切换时只采用最后一次请求的结果
const listSeq = latestOnly()

async function load() {
  const seq = listSeq.next()
  loading.value = true
  try {
    const qs = new URLSearchParams({ days: String(days.value), sort: sort.value, limit: String(pageSize), offset: String((page.value - 1) * pageSize) })
    if (keyword.value.trim()) {
      qs.set('q', keyword.value.trim())
    }
    if (filter.value) {
      qs.set('filter', filter.value)
    }
    if (site.value) {
      qs.set('site', site.value)
    }
    const res = await api.get<VisitorList>('/api/webservice/visitors?' + qs.toString())
    if (!listSeq.isLatest(seq)) {
      return
    }
    items.value = res.items
    total.value = res.total
    summary.value = res.summary
    daily.value = res.daily
    sites.value = res.sites
    listeners.value = res.listeners ?? {}
    info.value = res.info
    nowSec.value = Math.floor(Date.now() / 1000)
    // 筛选后总数变少时回到有数据的最后一页
    const last = Math.max(1, Math.ceil(res.total / pageSize))
    if (page.value > last) {
      page.value = last
      await load()
    }
  } catch (e) {
    if (listSeq.isLatest(seq)) {
      ElMessage.error((e as Error).message)
    }
  } finally {
    if (listSeq.isLatest(seq)) {
      loading.value = false
    }
  }
}

// 条件变化后回到第一页
function reload() {
  page.value = 1
  dayIndex.value = -1
  load()
}

function setFilter(v: string) {
  filter.value = filter.value === v ? '' : v
  reload()
}

let kwTimer: ReturnType<typeof setTimeout> | undefined
function onKeyword() {
  clearTimeout(kwTimer)
  kwTimer = setTimeout(reload, 300)
}

function onPage() {
  load()
  listCard.value?.scrollIntoView({ block: 'start', behavior: 'smooth' })
}

// ---------- 详情 ----------
const detailVisible = ref(false)
const detailLoading = ref(false)
const detailIP = ref('')
const detail = ref<VisitorDetail | null>(null)

// 详情请求序号：连续打开不同访客时，先发出的慢请求不会覆盖后打开的访客
const detailSeq = latestOnly()

async function loadDetail() {
  const seq = detailSeq.next()
  detailLoading.value = true
  try {
    const d = await api.get<VisitorDetail>('/api/webservice/visitors/detail?ip=' + encodeURIComponent(detailIP.value))
    if (!detailSeq.isLatest(seq)) {
      return
    }
    detail.value = d
    if (d.listeners) {
      listeners.value = { ...listeners.value, ...d.listeners }
    }
  } catch (e) {
    if (detailSeq.isLatest(seq)) {
      ElMessage.error((e as Error).message)
      detailVisible.value = false
    }
  } finally {
    if (detailSeq.isLatest(seq)) {
      detailLoading.value = false
    }
  }
}

function openDetail(ip: string) {
  detailIP.value = ip
  detail.value = null
  detailVisible.value = true
  loadDetail()
}

const statusSegs = computed(() => {
  const st = detail.value?.status ?? [0, 0, 0, 0, 0]
  const closed = detail.value?.closed ?? 0
  return [
    { label: '2xx', n: st[1], cls: 's2' },
    { label: '3xx', n: st[2], cls: 's3' },
    { label: '4xx', n: st[3] - closed, cls: 's4' },
    { label: '444', n: closed, cls: 's444' },
    { label: '5xx', n: st[4], cls: 's5' },
    { label: '1xx', n: st[0], cls: 's1' },
  ].filter((s) => s.n > 0)
})

const logsDialog = ref<InstanceType<typeof SiteLogDialog>>()
function openLogs() {
  // 访问日志按站点开启，没有站点名称时按地址查看全部日志
  logsDialog.value?.open({ name: '', label: '', ip: detailIP.value, accessLog: true })
}

const banVisible = ref(false)
const banHours = ref(24)
const banPermanent = ref(false)
const banning = ref(false)

async function confirmBan() {
  banning.value = true
  try {
    await api.post('/api/webservice/bans', { key: detailIP.value, seconds: banPermanent.value ? -1 : banHours.value * 3600 })
    ElMessage.success(t('visitors.banned'))
    banVisible.value = false
    await Promise.all([loadDetail(), load()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    banning.value = false
  }
}

async function unban() {
  banning.value = true
  try {
    await api.del('/api/webservice/bans', { key: detailIP.value })
    ElMessage.success(t('visitors.unbanned'))
    await Promise.all([loadDetail(), load()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    banning.value = false
  }
}

async function removeOne() {
  try {
    await api.post('/api/webservice/visitors/delete', { ips: [detailIP.value] })
    ElMessage.success(t('visitors.deleted'))
    detailVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// ---------- 设置 ----------
const settingsVisible = ref(false)
const saving = ref(false)
const form = reactive({ enable: true, days: 30, max: 10000 })

function openSettings() {
  const i = info.value
  form.enable = !i?.disable
  form.days = i?.days ?? 30
  form.max = i?.max ?? 10000
  settingsVisible.value = true
}

async function putSettings(body: { disable: boolean; days: number; max: number }) {
  saving.value = true
  try {
    info.value = await api.put<VisitorInfo>('/api/webservice/visitors/settings', body)
    return true
  } catch (e) {
    ElMessage.error((e as Error).message)
    return false
  } finally {
    saving.value = false
  }
}

async function saveSettings() {
  // 调小保留天数或最多访客数会立即删除超出范围的数据，逐项确认
  for (const x of shrinkImpact(info.value, form)) {
    const msg = x.kind === 'days' ? t('visitors.shrinkConfirm', { days: x.days }) : t('visitors.maxShrinkConfirm', { max: num(x.max), n: num(x.n) })
    try {
      await ElMessageBox.confirm(msg, t('visitors.settingsTitle'), {
        type: 'warning',
        confirmButtonText: t('common.save'),
        cancelButtonText: t('common.cancel'),
      })
    } catch {
      return
    }
  }
  if (await putSettings({ disable: !form.enable, days: form.days, max: form.max })) {
    ElMessage.success(t('common.saved'))
    settingsVisible.value = false
    load()
  }
}

async function resume() {
  const i = info.value
  if (i && (await putSettings({ disable: false, days: i.days, max: i.max }))) {
    load()
  }
}

async function clearAll() {
  try {
    await ElMessageBox.confirm(t('visitors.clearConfirm', { n: num(info.value?.count ?? 0) }), t('visitors.clearAll'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await api.post('/api/webservice/visitors/delete', { all: true })
    ElMessage.success(t('visitors.deleted'))
    settingsVisible.value = false
    reload()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(load)
</script>

<style scoped>
.paused {
  margin-bottom: 12px;
}
.paused :deep(.el-alert__content) {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
}
.tiles {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.tile {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 14px;
  text-align: left;
  font: inherit;
  color: inherit;
  background: var(--el-bg-color);
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  min-width: 0;
}
button.tile {
  cursor: pointer;
  transition: border-color 0.15s, box-shadow 0.15s;
}
button.tile:hover {
  border-color: var(--el-color-primary-light-5);
}
button.tile:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 1px;
}
.tile.active {
  border-color: var(--el-color-primary);
  box-shadow: 0 0 0 1px var(--el-color-primary) inset;
}
.tile-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.tile b {
  font-size: 24px;
  line-height: 1.2;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.tile.danger b {
  color: var(--el-color-danger);
}
.tile.warn b {
  color: var(--el-color-warning);
}
.tile-sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.chart-card {
  margin-bottom: 16px;
}
.day-readout {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  font-variant-numeric: tabular-nums;
}
.day-readout b {
  color: var(--el-text-color-primary);
  font-weight: 600;
}
.red {
  color: var(--el-color-danger);
}
.filters {
  justify-content: flex-start;
}
.f-kw {
  width: 260px;
}
.f-filter {
  width: 150px;
}
.f-site {
  width: 180px;
}
.f-sort {
  width: 130px;
}
.total {
  margin-left: auto;
}
.list-body {
  padding: 0;
  min-height: 120px;
}
.list-body > .el-empty {
  padding: 32px 0;
}
.v-table :deep(.el-table__row) {
  cursor: pointer;
}
.ip-line {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.ip {
  font-weight: 600;
  overflow-wrap: anywhere;
}
.cc {
  flex: none;
  font-size: 11px;
  line-height: 16px;
  padding: 0 5px;
  border-radius: 4px;
  color: var(--el-text-color-regular);
  background: var(--el-fill-color);
}
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}
.tags:empty {
  display: none;
}
.strong {
  font-weight: 600;
}
.small {
  font-size: 12px;
}
.meta {
  color: var(--el-text-color-secondary);
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  min-width: 0;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  max-width: 100%;
  padding: 0 7px;
  font-size: 12px;
  line-height: 20px;
  border-radius: 10px;
  background: var(--el-fill-color-light);
  border: 1px solid var(--g-border);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.chip.muted {
  color: var(--el-text-color-secondary);
  border-style: dashed;
}
.chip.more {
  color: var(--el-text-color-secondary);
}
.chip.ident.user {
  color: var(--el-color-success-dark-2);
  background: var(--el-color-success-light-9);
  border-color: var(--el-color-success-light-7);
}
.chip.ident.admin {
  color: var(--el-color-primary-dark-2);
  background: var(--el-color-primary-light-9);
  border-color: var(--el-color-primary-light-7);
}
.chip.danger {
  color: var(--el-color-danger);
  background: var(--el-color-danger-light-9);
  border-color: var(--el-color-danger-light-7);
}
.req-line {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.st {
  flex: none;
  font-family: var(--g-mono);
}
.ellipsis {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pager {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px;
  border-top: 1px solid var(--g-border);
}
.pager.mobile {
  justify-content: center;
  padding: 10px 4px;
}
.storage {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 12px;
  margin-top: 10px;
  font-size: 12px;
}
.v-cards {
  display: flex;
  flex-direction: column;
}
.v-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px 14px;
  font: inherit;
  color: inherit;
  text-align: left;
  background: none;
  border: none;
  border-bottom: 1px solid var(--g-border);
  cursor: pointer;
  min-width: 0;
}
.v-card:last-child {
  border-bottom: none;
}
.v-card:active {
  background: var(--el-fill-color-light);
}
.vc-top {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.vc-time {
  margin-left: auto;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.vc-stats {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  font-size: 12px;
  color: var(--el-text-color-regular);
  font-variant-numeric: tabular-nums;
}
.vc-stats .red {
  color: var(--el-color-danger);
}

/* ---------- 详情抽屉 ---------- */
.drawer {
  min-height: 100%;
  padding: 0 2px 16px;
}
.d-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  position: sticky;
  top: 0;
  z-index: 2;
  padding: 2px 0 8px;
  background: var(--el-bg-color);
}
.d-title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.d-ip {
  font-size: 18px;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.d-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 14px;
}
.d-actions .el-button + .el-button {
  margin-left: 0;
}
.d-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 16px;
}
.d-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  min-width: 0;
}
.d-cell span {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.d-cell b {
  font-size: 15px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.d-sec {
  margin-top: 18px;
}
.d-sec h4 {
  margin: 0 0 8px;
  font-size: 13px;
  font-weight: 600;
}
.status-bar {
  display: flex;
  height: 10px;
  border-radius: 5px;
  overflow: hidden;
  gap: 2px;
  background: var(--el-fill-color);
}
.seg {
  flex-basis: 0;
  min-width: 3px;
}
.s1 {
  background: var(--el-color-info-light-5);
}
.s2 {
  background: var(--el-color-success);
}
.s3 {
  background: var(--el-color-info);
}
.s4 {
  background: var(--el-color-warning);
}
.s444,
.s5 {
  background: var(--el-color-danger);
}
.s444 {
  opacity: 0.6;
}
.status-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 14px;
  margin-top: 8px;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--el-text-color-regular);
}
.dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 5px;
  vertical-align: 0;
}
.deny {
  margin-top: 10px;
  align-items: center;
}
.rows {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--g-border);
  border-radius: 8px;
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 7px 10px;
  min-width: 0;
}
.row + .row {
  border-top: 1px solid var(--g-border);
}
.row-main {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1 1 auto;
}
.row-side {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex: none;
  font-variant-numeric: tabular-nums;
}
.row.agent .row-main {
  align-items: flex-start;
}
.ua {
  font-size: 12px;
  overflow-wrap: anywhere;
  color: var(--el-text-color-regular);
}
.note {
  margin-top: 6px;
}
.ban-hours {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

@media (max-width: 1100px) {
  .tiles {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
@media (max-width: 767.98px) {
  .tiles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }
  .tiles > :first-child {
    grid-column: 1 / -1;
  }
  .tile {
    padding: 10px 12px;
  }
  .tile b {
    font-size: 20px;
  }
  .f-kw {
    width: 100%;
  }
  .f-filter,
  .f-site,
  .f-sort {
    width: calc(33.33% - 6px);
    flex: 1 1 100px;
  }
  .total {
    margin-left: 0;
  }
  .d-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .range {
    flex: none;
  }
}
</style>

<style>
/* 抽屉挂在 body 下，内边距需写在非 scoped 样式里 */
.visitor-drawer .el-drawer__body {
  padding: 14px 18px calc(14px + env(safe-area-inset-bottom));
}
</style>
