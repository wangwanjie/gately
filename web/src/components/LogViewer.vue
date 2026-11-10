<template>
  <!-- full 为全屏查看：盖住整个页面（含导航与弹窗），z-index 取 Element Plus 的下一层，之后打开的下拉框仍在其上 -->
  <div class="log-viewer" :class="{ fill, full }" :style="full ? { zIndex: fullZ } : undefined">
    <div class="toolbar">
      <el-select v-model="filters.level" size="small" class="f-level">
        <el-option :label="t('components.logViewer.levelAll')" value="all" />
        <el-option :label="t('components.logViewer.levelInfo')" value="info" />
        <el-option :label="t('components.logViewer.levelWarn')" value="warn" />
        <el-option :label="t('components.logViewer.levelError')" value="error" />
      </el-select>
      <!-- 来源列表由后端给出（配置中的全部来源与日志中出现过的来源，包括已落盘的），展开时刷新，并合并已加载日志中的来源；
           也可以直接输入。清除时置为空串（默认是 undefined，拼查询参数时会出错） -->
      <el-select
        v-model="filters.source"
        size="small"
        class="f-source"
        clearable
        value-on-clear=""
        filterable
        allow-create
        default-first-option
        :placeholder="t('components.logViewer.source')"
        @visible-change="(open: boolean) => open && loadSources()"
      >
        <el-option v-for="s in sourceOptions" :key="s" :label="s" :value="s" />
      </el-select>
      <el-input v-model="filters.q" size="small" class="f-q" clearable :prefix-icon="Search" :placeholder="t('components.logViewer.keyword')">
        <template #suffix>
          <el-tooltip :content="t('components.logViewer.keywordTip')" placement="top">
            <CircleHelp :size="13" class="tip" />
          </el-tooltip>
        </template>
      </el-input>
      <!-- 不包含的关键词：包含其中任意一个即排除，与上面的关键词同时生效 -->
      <el-input v-model="filters.exclude" size="small" class="f-q" clearable :prefix-icon="SearchX" :placeholder="t('components.logViewer.exclude')">
        <template #suffix>
          <el-tooltip :content="t('components.logViewer.excludeTip')" placement="top">
            <CircleHelp :size="13" class="tip" />
          </el-tooltip>
        </template>
      </el-input>
      <el-button size="small" :type="advancedCount ? 'primary' : 'default'" :plain="!!advancedCount" :icon="Filter" @click="showAdvanced = !showAdvanced">
        {{ t('components.logViewer.moreFilters') }}<span v-if="advancedCount" class="count">{{ advancedCount }}</span>
      </el-button>
      <!-- 站点条件按名称完整匹配，由拓扑页等入口带入或点击日志中的站点标签设置 -->
      <el-tag v-if="kinds.access && filters.site" closable size="small" class="f-site" @close="filters.site = ''">
        {{ t('components.logViewer.siteChip', { site: filters.site }) }}
      </el-tag>
      <!-- 监听器条件按 key 匹配，由站点日志入口带入：同名站点分在 IPv4、IPv6 两个监听器时只看本监听器的日志 -->
      <el-tag v-if="kinds.access && filters.listener" closable size="small" class="f-site" :title="filters.listenerLabel" @close="clearListener">
        {{ t('components.logViewer.listenerChip', { listener: filters.listenerLabel || filters.listener }) }}
      </el-tag>
      <el-button v-if="active" size="small" text :icon="RotateCcw" @click="resetFilters">{{ t('components.logViewer.reset') }}</el-button>
      <!-- 访问日志的显示开关：只影响页面显示，日志照常保存；选择记在浏览器里 -->
      <template v-if="kinds.access">
        <el-tooltip :content="t('components.logViewer.hideBannedTip')" placement="top">
          <el-checkbox v-model="hide.banned" size="small">{{ t('components.logViewer.hideBanned') }}</el-checkbox>
        </el-tooltip>
        <el-tooltip :content="t('components.logViewer.hideAdminTip')" placement="top">
          <el-checkbox v-model="hide.admin" size="small">{{ t('components.logViewer.hideAdmin') }}</el-checkbox>
        </el-tooltip>
      </template>
      <el-checkbox v-model="follow" size="small">{{ t('components.logViewer.autoScroll') }}</el-checkbox>
      <!-- 导出与清空放在一组，窄屏换行时一起靠右 -->
      <span class="actions">
        <!-- 导出当前筛选条件下的全部日志（含落盘的历史与归档），以下载链接流式保存，不经过页面内存 -->
        <el-dropdown trigger="click" :disabled="!!error" @command="exportLogs">
          <el-button size="small" text :icon="Download" :disabled="!!error" :title="t('components.logViewer.exportTip')">{{ t('components.logViewer.export') }}</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="csv">{{ t('components.logViewer.exportCsv') }}</el-dropdown-item>
              <el-dropdown-item command="jsonl">{{ t('components.logViewer.exportJsonl') }}</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button v-if="clearable" size="small" text :icon="Trash2" @click="clear">{{ t('common.clear') }}</el-button>
        <!-- 全屏查看：日志区占满整个窗口，Esc 或再次点击退出 -->
        <el-button
          size="small"
          text
          class="full-btn"
          :icon="full ? Minimize2 : Maximize2"
          :title="full ? t('components.logViewer.exitFullscreen') : t('components.logViewer.fullscreen')"
          :aria-label="full ? t('components.logViewer.exitFullscreen') : t('components.logViewer.fullscreen')"
          :aria-pressed="full"
          @click="toggleFull"
        />
      </span>
    </div>
    <div v-show="showAdvanced" class="toolbar advanced">
      <el-input v-model="filters.ip" size="small" class="f-ip" clearable :placeholder="t('components.logViewer.ip')" />
      <el-input v-if="kinds.host" v-model="filters.host" size="small" class="f-host" clearable :placeholder="t('components.logViewer.host')" />
      <el-input v-if="kinds.status" v-model="filters.status" size="small" class="f-status" clearable :placeholder="t('components.logViewer.status')" />
      <el-select v-model="filters.range" size="small" class="f-range">
        <el-option v-for="r in ranges" :key="r" :label="t(`components.logViewer.range.${r}`)" :value="r" />
      </el-select>
      <el-date-picker
        v-if="filters.range === 'custom'"
        v-model="filters.custom"
        type="datetimerange"
        size="small"
        class="f-custom"
        :start-placeholder="t('components.logViewer.from')"
        :end-placeholder="t('components.logViewer.to')"
      />
      <span class="hint">{{ t('components.logViewer.fieldTip') }}</span>
    </div>
    <div v-if="error" class="error-tip">{{ error }}</div>
    <div class="box-wrap">
      <div
        ref="box"
        class="box mono"
        :style="boxStyle"
        @scroll.passive="onScroll"
        @wheel.passive="onUserScroll"
        @touchstart.passive="onUserScroll"
        @keydown="onUserScroll"
        @pointerdown="onUserScroll"
      >
        <!-- 内容容器：观察其高度变化（新日志、懒渲染的行显示出真实高度），贴底时保持贴底 -->
        <div ref="inner">
          <div v-if="hasMore" class="more">
            <el-button size="small" text :loading="loadingMore" @click="loadEarlier">{{ t('components.logViewer.loadEarlier') }}</el-button>
          </div>
          <div v-for="e in entries" :key="e.seq" :class="['line', e.level]">
            <span class="time">{{ e.timeText }}</span>
            <span class="lvl">{{ e.level.toUpperCase() }}</span>
            <span v-if="e.source" class="src" :title="t('components.logViewer.filterBy')" @click="applyField('source', e.source)">[{{ e.source }}]</span>
            <span>
              <template v-for="(p, i) in e.parts" :key="i">
                <mark v-if="p.hit">{{ p.text }}</mark>
                <template v-else>{{ p.text }}</template>
              </template>
            </span>
            <!-- 消息里没有出现的字段（例如 WAF 命中日志的域名、访问日志的站点名称）以标签显示，点击即按该字段过滤 -->
            <span
              v-for="f in e.extra"
              :key="f.key"
              :class="['field', { static: !clickableField(f.key) }]"
              :title="clickableField(f.key) ? t('components.logViewer.filterBy') : undefined"
              @click="applyField(f.key, f.value)"
            >
              {{ fieldLabel(f.key) }}={{ fieldValue(f.key, f.value) }}
            </span>
          </div>
          <div v-if="!entries.length" class="empty">{{ active ? t('components.logViewer.noMatch') : t('components.logViewer.empty') }}</div>
        </div>
      </div>
      <!-- 翻看历史（离开底部）或关闭了自动滚动时有新日志到达，显示回到最新的按钮 -->
      <button v-if="showJump" type="button" class="jump" @click="jumpLatest">
        <ArrowDownToLine :size="13" />
        <span>{{ newCount > 0 ? t('components.logViewer.newLogs', { n: newCount > 999 ? '999+' : newCount }) : t('components.logViewer.jumpLatest') }}</span>
      </button>
    </div>
    <!-- 拖动条：调整日志区高度（鼠标与触摸），双击恢复默认，获得焦点后可用方向键调整；撑满父容器与全屏时不需要 -->
    <div
      v-if="resizable"
      class="resizer"
      :class="{ dragging }"
      role="separator"
      aria-orientation="horizontal"
      tabindex="0"
      :aria-valuemin="MIN_HEIGHT"
      :aria-valuemax="maxHeight(viewportH)"
      :aria-valuenow="boxHeightNow"
      :aria-label="t('components.logViewer.resizeTip')"
      :title="t('components.logViewer.resizeTip')"
      @pointerdown="startDrag"
      @dblclick="resetHeight"
      @keydown="onResizeKey"
    >
      <span class="grip" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, useZIndex } from 'element-plus'
import { ArrowDownToLine, CircleHelp, Download, Filter, Maximize2, Minimize2, RotateCcw, Search, SearchX, Trash2 } from 'lucide-vue-next'
import { api, ApiError, type LogEntry } from '../api'
import { usePolling } from '../composables/usePolling'
import { useScrollLock } from '../composables/useScrollLock'
import { MIN_HEIGHT, atBottom, clampHeight, keyHeight, loadHeight, maxHeight, saveHeight } from '../logView'

const { t, locale } = useI18n()

// height 可覆盖默认高度；fill 时组件撑满父容器，日志区占据工具栏以外的全部高度（独立日志页用）；
// initial 为打开时的过滤条件，例如拓扑页按站点与访客 IP 查看访问日志（site 为站点名称，完整匹配）；
// listener 为监听器 key（完整匹配），listenerLabel 只用于在标签上显示
const props = defineProps<{
  url: string
  clearable?: boolean
  height?: string
  fill?: boolean
  initial?: { ip?: string; site?: string; host?: string; listener?: string; listenerLabel?: string }
}>()

// 首次只拉最近 PAGE 条，更早的日志点“加载更早的日志”按页补充；轮询追加后最多保留 MAX 条
const PAGE = 200
const MAX = 2000

// 消息按关键词切成片段，命中的片段高亮；extra 为消息中没有出现的字段
type Part = { text: string; hit: boolean }
type Row = LogEntry & { timeText: string; parts: Part[]; extra: { key: string; value: string }[] }

// 带结构化字段的模块显示对应的过滤项；IP 对所有日志都可用（没有字段时从消息中识别地址）。
// access 表示有访问日志，显示“隐藏被封禁来源”“隐藏后台自身请求”开关
const moduleKinds: Record<string, { host?: boolean; status?: boolean; access?: boolean }> = {
  webservice: { host: true, status: true, access: true },
  waf: { host: true },
}
const moduleId = computed(() => props.url.match(/^\/api\/([^/]+)\/logs$/)?.[1] ?? '')
const kinds = computed(() => moduleKinds[moduleId.value] ?? {})

// 时间范围：相对范围在每次查询时按当前时间计算，新日志仍会持续追加
const ranges = ['all', '15m', '1h', '6h', '24h', '7d', 'custom'] as const
type Range = (typeof ranges)[number]
const rangeMs: Record<string, number> = { '15m': 15 * 60e3, '1h': 3600e3, '6h': 6 * 3600e3, '24h': 24 * 3600e3, '7d': 7 * 24 * 3600e3 }

function emptyFilters() {
  return {
    level: 'all',
    source: '',
    q: '',
    exclude: '',
    ip: '',
    host: '',
    site: '',
    listener: '',
    listenerLabel: '',
    status: '',
    range: 'all' as Range,
    custom: null as [Date, Date] | null,
  }
}
const filters = reactive({ ...emptyFilters(), ...(props.initial ?? {}) })
// 带入了 IP 或域名条件时展开高级过滤，让用户看到当前生效的条件
const showAdvanced = ref(!!(props.initial?.ip || props.initial?.host))

// 访问日志的显示开关，保存在 localStorage，不随“重置”清除。
// 管理员自己的请求（尤其是日志页每 2 秒的轮询）默认隐藏，避免“看日志产生日志”；被封禁来源的请求默认显示，便于审计
const hideKey = 'gately.logs.hide'
function loadHide() {
  const out = { banned: false, admin: true }
  try {
    const v = JSON.parse(localStorage.getItem(hideKey) ?? '{}')
    if (typeof v?.banned === 'boolean') {
      out.banned = v.banned
    }
    if (typeof v?.admin === 'boolean') {
      out.admin = v.admin
    }
  } catch {
    // 存储不可用或内容损坏时使用默认值
  }
  return out
}
const hide = reactive(loadHide())
watch(hide, (v) => {
  try {
    localStorage.setItem(hideKey, JSON.stringify(v))
  } catch {
    // 存储不可用时只在本页生效
  }
})

const entries = ref<Row[]>([])
const follow = ref(true)
const box = ref<HTMLElement>()
const inner = ref<HTMLElement>()
// 缓冲中可能还有更早的日志
const hasMore = ref(false)
const loadingMore = ref(false)
// 过滤参数无效时后端返回的错误
const error = ref('')
// 已加载日志中出现过的来源；切换过滤条件后仍保留
const seenSources = ref<string[]>([])
// 后端给出的来源：配置中的全部来源（例如还没有访问记录的监听器）与日志中出现过的来源（包括已挤出内存、只在文件里的）
const apiSources = ref<string[]>([])
// away：用户离开了底部在翻看历史，此时新日志照常追加，但不再自动滚到底部打断浏览；
// newCount：离开底部或关闭自动滚动后到达的新日志条数，显示在“回到最新”按钮上
const away = ref(false)
const newCount = ref(0)
const showJump = computed(() => entries.value.length > 0 && (away.value || newCount.value > 0))
let lastSeq = 0
let loaded = false
// 轮询裁剪上限：用户每加载一页历史就放宽相应条数，避免刚加载的旧日志被下一次轮询裁掉
let keep = MAX
// 清空或修改过滤条件时递增，丢弃之前发出、之后才返回的请求结果
let generation = 0
// 正在进行首次拉取的 generation，-1 表示没有。reload 直接调用 poll，绕过了 usePolling 的防重入，
// 首次拉取未返回时轮询触发的 poll 据此跳过，避免同一组条件重复发出首次查询
let initialGen = -1

const sourceOptions = computed(() => {
  const list = [...new Set([...apiSources.value, ...seenSources.value])].sort((a, b) => a.localeCompare(b))
  if (filters.source && !list.includes(filters.source)) {
    list.unshift(filters.source)
  }
  return list
})

// loadSources 拉取来源列表；失败时保留上次的结果，下拉里仍有已加载日志中的来源
let sourcesLoading = false
async function loadSources() {
  if (sourcesLoading) {
    return
  }
  sourcesLoading = true
  try {
    const list = await api.get<string[]>(`${props.url}/sources`)
    apiSources.value = Array.isArray(list) ? list : []
  } catch {
    // 来源列表只是辅助，失败不提示
  } finally {
    sourcesLoading = false
  }
}

// exportLogs 按当前筛选条件导出日志：用下载链接让浏览器直接把响应流式写入文件，导出大量日志时不占页面内存。
// 链接带不上界面语言的请求头，用 lang 参数让导出内容与关键词匹配都按界面语言
async function exportLogs(format: string) {
  // 先用轻量的来源接口确认会话仍有效：会话过期时 api 会跳转登录页，避免浏览器把 401 的错误 JSON 当作导出文件保存
  try {
    await api.get(`${props.url}/sources`)
  } catch (e) {
    if (e instanceof ApiError && e.status !== 401) {
      ElMessage.error(e.message)
    }
    return
  }
  const q = new URLSearchParams({ format, lang: String(locale.value) })
  const a = document.createElement('a')
  a.href = `${props.url}/export?${q.toString()}${filterQuery()}`
  a.rel = 'noopener'
  a.download = ''
  document.body.appendChild(a)
  a.click()
  a.remove()
}

// 高级过滤项中已填写的数量，显示在按钮上；自定义时间范围清空了日期时不会发出时间条件，不计入
const rangeActive = computed(() => (filters.range === 'custom' ? !!filters.custom?.[0] : filters.range !== 'all'))
const advancedCount = computed(() => [filters.ip, kinds.value.host && filters.host, kinds.value.status && filters.status, rangeActive.value].filter(Boolean).length)
const active = computed(
  () =>
    filters.level !== 'all' ||
    !!filters.source ||
    !!filters.q.trim() ||
    !!filters.exclude.trim() ||
    (kinds.value.access && (!!filters.site || !!filters.listener)) ||
    advancedCount.value > 0,
)

// filterQuery 把过滤条件拼成查询参数（不含 since/before/limit）
function filterQuery(): string {
  const q = new URLSearchParams()
  if (filters.level !== 'all') {
    q.set('level', filters.level)
  }
  const add = (k: string, v: string) => {
    if (v.trim()) {
      q.set(k, v.trim())
    }
  }
  add('source', filters.source)
  add('q', filters.q)
  add('exclude', filters.exclude)
  add('ip', filters.ip)
  if (kinds.value.host) {
    add('host', filters.host)
  }
  if (kinds.value.status) {
    add('status', filters.status)
  }
  if (kinds.value.access) {
    add('site', filters.site)
    add('listener', filters.listener)
    if (hide.banned) {
      q.set('hideBanned', '1')
    }
    if (hide.admin) {
      q.set('hideAdmin', '1')
    }
  }
  if (filters.range === 'custom') {
    if (filters.custom) {
      q.set('from', filters.custom[0].toISOString())
      q.set('to', filters.custom[1].toISOString())
    }
  } else if (filters.range !== 'all') {
    q.set('from', new Date(Date.now() - rangeMs[filters.range]).toISOString())
  }
  const s = q.toString()
  return s ? `&${s}` : ''
}

// 关键词拆分，逐字符照搬后端 logbuf.ParseKeywords，保证高亮的词与实际过滤的词一致：
// 空格、制表符与全角空格分隔；双引号开始短语（可以出现在词中间），再遇到双引号时结束当前词，未闭合时到末尾为止；
// 词首的 - 表示排除（排除词不高亮），单独一个 - 不构成词
function highlightTerms(): string[] {
  const out: string[] = []
  let cur = ''
  let quoted = false
  let neg = false
  let has = false
  const emit = () => {
    if (has && cur && !neg) {
      out.push(cur.toLowerCase())
    }
    cur = ''
    quoted = false
    neg = false
    has = false
  }
  // for...of 按码点遍历，与 Go 的 range 字符串一致
  for (const r of filters.q) {
    if (r === '"') {
      if (quoted) {
        emit()
      } else {
        quoted = true
        has = true
      }
    } else if (!quoted && (r === ' ' || r === '\t' || r === '\u3000')) {
      emit()
    } else if (r === '-' && !quoted && !has) {
      neg = true
      has = true
    } else {
      cur += r
      has = true
    }
  }
  emit()
  return out
}

function escapeRe(s: string) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function splitParts(msg: string, re: RegExp | null): Part[] {
  if (!re) {
    return [{ text: msg, hit: false }]
  }
  const parts: Part[] = []
  let last = 0
  for (const m of msg.matchAll(re)) {
    const i = m.index ?? 0
    if (i > last) {
      parts.push({ text: msg.slice(last, i), hit: false })
    }
    parts.push({ text: m[0], hit: true })
    last = i + m[0].length
  }
  if (last < msg.length) {
    parts.push({ text: msg.slice(last), hit: false })
  }
  return parts
}

function toRows(list: LogEntry[]): Row[] {
  const terms = highlightTerms()
  const re = terms.length ? new RegExp(terms.map(escapeRe).join('|'), 'gi') : null
  const sources = new Set(seenSources.value)
  const rows = list.map((e) => {
    if (e.source) {
      sources.add(e.source)
    }
    // 方法、路径等字段通常已在消息里或过于琐碎，只为可用于筛选的字段显示标签
    const extra = Object.entries(e.fields ?? {})
      .filter(([k, v]) => tagKeys.includes(k) && v && !e.message.includes(v))
      .map(([key, value]) => ({ key, value }))
    return { ...e, timeText: new Date(e.time).toLocaleString(), parts: splitParts(e.message, re), extra }
  })
  if (sources.size !== seenSources.value.length) {
    seenSources.value = [...sources].sort()
  }
  return rows
}

const fieldKeys = ['ip', 'host', 'site', 'status', 'method', 'path', 'rule', 'user', 'deny', 'admin']
// deny 为访问被拦截的原因，admin 为请求所带管理后台登录态的用户名
const tagKeys = ['ip', 'host', 'site', 'status', 'rule', 'user', 'deny', 'admin']
const denyReasons = ['ban', 'ip', 'country', 'rateLimit', 'waf']
function fieldLabel(key: string) {
  return fieldKeys.includes(key) ? t(`components.logViewer.field.${key}`) : key
}
// 拦截原因与后台用户只作提示：没有对应的过滤项，作为关键词也会误命中其他日志，由上方的显示开关控制
function clickableField(key: string) {
  return key !== 'deny' && key !== 'admin'
}
// fieldValue 把拦截原因翻译成界面文字，其他字段原样显示
function fieldValue(key: string, value: string) {
  return key === 'deny' && denyReasons.includes(value) ? t(`components.logViewer.deny.${value}`) : value
}

// applyField 点击来源或字段标签时按该值过滤；没有专门过滤项的字段作为关键词追加
function applyField(key: string, value: string) {
  if (key === 'source') {
    filters.source = value
    return
  }
  if (!clickableField(key)) {
    return
  }
  if (key === 'ip') {
    filters.ip = value
  } else if (key === 'site' && kinds.value.access) {
    // 站点名称按完整匹配过滤，不会混入名称相近的其他站点
    filters.site = value
    return
  } else if ((key === 'host' || key === 'site') && kinds.value.host) {
    filters.host = value
  } else if (key === 'status' && kinds.value.status) {
    filters.status = value
  } else {
    const word = /\s/.test(value) ? `"${value}"` : value
    filters.q = filters.q.trim() ? `${filters.q.trim()} ${word}` : word
    return
  }
  showAdvanced.value = true
}

// clearListener 去掉监听器条件，显示用的名称一并清除
function clearListener() {
  filters.listener = ''
  filters.listenerLabel = ''
}

function resetFilters() {
  Object.assign(filters, emptyFilters())
}

// scrollToEnd 开启自动滚动时滚到最新一条。行使用 content-visibility: auto，未渲染的行按估算高度（19px）计算；
// 长行换行（移动端尤其明显）后实际更高，滚到底后底部的行按真实高度布局，总高度随之变大，只滚一次会停在中间。
// 每次滚到底后等两帧（第一帧的回调早于这一帧的布局），布局更新后若仍未贴底就再滚，最多几次
// 用户离开底部时不自动滚动；force 用于“回到最新”按钮，不受自动滚动开关限制。
// 滚动期间 autoScrolling 为真，onScroll 忽略这些由程序触发的滚动；用户在此期间滚轮、触摸或按下滚动条时
// （onUserScroll）递增 autoToken 中止循环，避免与用户的操作抢滚动位置
let autoToken = 0
let autoScrolling = false
async function scrollToEnd(force = false) {
  if (!force && (!follow.value || away.value)) {
    return
  }
  await nextTick()
  const el = box.value
  if (!el) {
    return
  }
  const token = ++autoToken
  autoScrolling = true
  try {
    for (let i = 0; i < 8; i++) {
      if (token !== autoToken) {
        return
      }
      el.scrollTop = el.scrollHeight
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
      if (token !== autoToken || (!force && !follow.value) || el.scrollHeight - el.scrollTop - el.clientHeight < 2) {
        break
      }
    }
  } finally {
    if (token === autoToken) {
      autoScrolling = false
      away.value = !atBottom(el)
      if (!away.value) {
        newCount.value = 0
      }
    }
  }
}

// onUserScroll 用户开始滚动（滚轮、触摸、按下滚动条）时中止正在进行的自动滚动
function onUserScroll() {
  if (autoScrolling) {
    autoToken++
    autoScrolling = false
  }
}

// onScroll 记录用户是否离开了底部；回到底部时清零新日志计数，恢复自动滚动
function onScroll() {
  const el = box.value
  if (autoScrolling || !el) {
    return
  }
  away.value = !atBottom(el)
  if (!away.value) {
    newCount.value = 0
  }
}

// jumpLatest 回到最新一条并恢复自动滚动
function jumpLatest() {
  away.value = false
  newCount.value = 0
  void scrollToEnd(true)
}

// 首次拉取最近一页，之后按序号增量拉取，只追加新日志
async function poll() {
  const gen = generation
  const fq = filterQuery()
  if (!loaded && initialGen === gen) {
    return
  }
  try {
    if (!loaded) {
      initialGen = gen
      const list = await api.get<LogEntry[]>(`${props.url}?since=0&limit=${PAGE}${fq}`)
      if (gen !== generation) {
        return
      }
      loaded = true
      error.value = ''
      hasMore.value = list.length >= PAGE
      entries.value = toRows(list)
      if (list.length) {
        lastSeq = list[list.length - 1].seq
      }
      await scrollToEnd()
      return
    }
    const list = await api.get<LogEntry[]>(`${props.url}?since=${lastSeq}${fq}`)
    if (gen !== generation) {
      return
    }
    if (list.length) {
      lastSeq = list[list.length - 1].seq
      // 用户在翻看历史或关闭了自动滚动：不打断当前位置，累计新日志条数
      if (away.value || !follow.value) {
        newCount.value += list.length
      }
      const next = entries.value.concat(toRows(list))
      if (next.length > keep) {
        // 超出上限时丢掉最旧的部分，仍可通过“加载更早的日志”找回
        hasMore.value = true
      }
      entries.value = next.slice(-keep)
      await scrollToEnd()
    }
  } catch (e) {
    // 过滤参数无效时显示原因；网络抖动时下一轮重试
    if (gen === generation && !loaded) {
      error.value = (e as Error).message
      entries.value = []
      hasMore.value = false
    }
  } finally {
    // 只清除自己设置的标记；条件已变化时新的首次拉取会设置新的 generation
    if (initialGen === gen) {
      initialGen = -1
    }
  }
}

// loadEarlier 拉取当前最早一条之前的一页日志，并保持当前可见位置不跳动
async function loadEarlier() {
  const first = entries.value[0]
  if (!first || loadingMore.value) {
    return
  }
  loadingMore.value = true
  const gen = generation
  try {
    const list = await api.get<LogEntry[]>(`${props.url}?before=${first.seq}&limit=${PAGE}${filterQuery()}`)
    if (gen !== generation) {
      return
    }
    hasMore.value = list.length >= PAGE
    if (!list.length) {
      return
    }
    const el = box.value
    const oldHeight = el?.scrollHeight ?? 0
    // 用户在翻看历史：放宽裁剪上限保住刚加载的这一页；视为离开底部，新日志不再把位置拉到底部
    away.value = true
    keep = Math.max(keep, entries.value.length + list.length)
    entries.value = toRows(list).concat(entries.value)
    await nextTick()
    if (el) {
      el.scrollTop += el.scrollHeight - oldHeight
      away.value = !atBottom(el)
    }
  } catch {
    // 失败时保留按钮，用户可再次点击
  } finally {
    loadingMore.value = false
  }
}

// reload 丢弃已加载的日志，按当前条件重新拉取最近一页
function reload() {
  generation++
  loaded = false
  lastSeq = 0
  keep = MAX
  hasMore.value = false
  follow.value = true
  away.value = false
  newCount.value = 0
  void poll()
}

async function clear() {
  await api.del(props.url)
  generation++
  entries.value = []
  // 清空时后端一并清除了记录的来源，重新拉取（只剩配置中的来源）
  seenSources.value = []
  apiSources.value = []
  void loadSources()
  hasMore.value = false
  keep = MAX
  away.value = false
  newCount.value = 0
}

// ---- 日志区高度：拖动条调整，按位置（页面卡片 / 弹窗）分别记在浏览器里 ----
const heightScope = props.height ? 'dialog' : 'card'
function storage() {
  try {
    return window.localStorage
  } catch {
    return undefined
  }
}
const viewportH = ref(window.innerHeight)
// 用户调整过的高度（px），null 表示默认高度（CSS 或 height 属性）
const customH = ref<number | null>(loadHeight(storage(), heightScope))
// 日志区当前的实际高度，供拖动条的 aria-valuenow 使用
const boxHeightNow = ref(0)
const full = ref(false)
const resizable = computed(() => !props.fill && !full.value)
const boxStyle = computed(() => {
  if (props.fill || full.value) {
    return undefined
  }
  if (customH.value != null) {
    // 视口变矮（例如旋转屏幕）时按当前视口收紧，保存的值不变
    return { height: `${clampHeight(customH.value, viewportH.value)}px` }
  }
  return props.height ? { height: props.height } : undefined
})

const dragging = ref(false)
let dragStartY = 0
let dragStartH = 0
let dragId = -1
let dragEl: HTMLElement | null = null

// startDrag 按下拖动条开始调整高度：捕获指针，鼠标移出拖动条也能继续拖；触摸时由 touch-action: none 阻止页面滚动
function startDrag(e: PointerEvent) {
  if (e.button !== 0 || !box.value || dragging.value) {
    return
  }
  e.preventDefault()
  dragEl = e.currentTarget as HTMLElement
  dragStartY = e.clientY
  dragStartH = box.value.getBoundingClientRect().height
  dragId = e.pointerId
  try {
    dragEl.setPointerCapture(e.pointerId)
  } catch {
    // 指针已失效时退化为普通事件
  }
  dragEl.addEventListener('pointermove', onDrag)
  dragEl.addEventListener('pointerup', endDrag)
  dragEl.addEventListener('pointercancel', endDrag)
  dragEl.addEventListener('lostpointercapture', endDrag)
  dragging.value = true
}

// onDrag 指针移动时更新高度；pointermove 每帧最多触发一次，只改一个响应式数值，开销很小
function onDrag(e: PointerEvent) {
  if (e.pointerId !== dragId) {
    return
  }
  customH.value = clampHeight(dragStartH + e.clientY - dragStartY, viewportH.value)
}

// endDrag 松开或指针被系统取消时结束拖动并保存高度
function endDrag(e?: PointerEvent) {
  if (e && e.pointerId !== dragId) {
    return
  }
  if (dragEl) {
    dragEl.removeEventListener('pointermove', onDrag)
    dragEl.removeEventListener('pointerup', endDrag)
    dragEl.removeEventListener('pointercancel', endDrag)
    dragEl.removeEventListener('lostpointercapture', endDrag)
    dragEl = null
  }
  if (!dragging.value) {
    return
  }
  dragging.value = false
  dragId = -1
  saveHeight(storage(), heightScope, customH.value)
}

// resetHeight 双击拖动条恢复默认高度
function resetHeight() {
  customH.value = null
  saveHeight(storage(), heightScope, null)
}

// onResizeKey 拖动条获得焦点时用方向键、PageUp/PageDown、Home/End 调整高度
function onResizeKey(e: KeyboardEvent) {
  const cur = box.value?.getBoundingClientRect().height ?? 0
  const h = keyHeight(e.key, cur, viewportH.value)
  if (h === undefined) {
    return
  }
  e.preventDefault()
  customH.value = h
  saveHeight(storage(), heightScope, h)
}

// 视口尺寸变化时按帧合并更新，供高度上限使用
let resizeRaf = 0
function onWindowResize() {
  if (resizeRaf) {
    return
  }
  resizeRaf = requestAnimationFrame(() => {
    resizeRaf = 0
    viewportH.value = window.innerHeight
  })
}

// 日志区尺寸变化（拖动、全屏、窗口缩放、筛选栏换行）或内容高度变化（content-visibility 跳过渲染的行
// 显示后按真实高度布局，长行换行时比估算的高）时：贴底并开启自动滚动的保持贴底，避免最新日志被挤出视野。
// 滚动事件先于 ResizeObserver 回调处理，用户上翻时 away 已经更新，不会被拉回底部
let boxObserver: ResizeObserver | undefined
function observeBox() {
  const el = box.value
  if (!el || typeof ResizeObserver === 'undefined') {
    return
  }
  boxObserver = new ResizeObserver(() => {
    boxHeightNow.value = Math.round(el.getBoundingClientRect().height)
    if (follow.value && !away.value && !autoScrolling) {
      el.scrollTop = el.scrollHeight
    }
  })
  boxObserver.observe(el)
  if (inner.value) {
    boxObserver.observe(inner.value)
  }
}

// ---- 全屏查看：用固定定位盖住整个窗口（iOS Safari 不支持元素全屏 API），并锁住下方页面的滚动 ----
const { nextZIndex } = useZIndex()
const fullZ = ref(0)
const lockScroll = useScrollLock('.log-viewer.full')
function toggleFull() {
  setFull(!full.value)
}
function setFull(on: boolean) {
  if (on === full.value) {
    return
  }
  if (on) {
    fullZ.value = nextZIndex()
    window.addEventListener('keydown', onFullKey, true)
  } else {
    window.removeEventListener('keydown', onFullKey, true)
  }
  full.value = on
  lockScroll(on)
}
// onFullKey 全屏时按 Esc 退出。在捕获阶段处理并阻止传播，弹窗里的日志退出全屏时不会连带关闭弹窗；
// 下拉框、日期选择等弹出层打开时先让 Esc 关闭弹出层
function onFullKey(e: KeyboardEvent) {
  if (e.key !== 'Escape' || e.isComposing) {
    return
  }
  const popper = Array.from(document.querySelectorAll<HTMLElement>('.el-select__popper, .el-picker__popper, .el-dropdown__popper')).some(
    (el) => el.style.display !== 'none' && el.getAttribute('aria-hidden') !== 'true',
  )
  if (popper) {
    return
  }
  e.preventDefault()
  e.stopPropagation()
  setFull(false)
}

onMounted(() => {
  window.addEventListener('resize', onWindowResize)
  observeBox()
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', onWindowResize)
  window.removeEventListener('keydown', onFullKey, true)
  if (resizeRaf) {
    cancelAnimationFrame(resizeRaf)
  }
  boxObserver?.disconnect()
  endDrag()
})

// 修改过滤条件后稍等片刻再查询，输入关键词时不必每个字都请求一次。
// 相对时间范围用到 Date.now()，它不是响应式依赖，只有过滤条件本身变化时才会重新计算并触发
let debounce: number | undefined
watch(
  () => filterQuery(),
  () => {
    window.clearTimeout(debounce)
    debounce = window.setTimeout(reload, 350)
  },
)
watch(
  () => filters.range,
  (v) => {
    if (v === 'custom' && !filters.custom) {
      // 自定义范围默认最近一小时，便于在此基础上调整
      filters.custom = [new Date(Date.now() - 3600e3), new Date()]
    }
  },
)
onBeforeUnmount(() => window.clearTimeout(debounce))

// 每 2 秒增量拉取，标签页在后台时暂停
const polling = usePolling(poll, 2000)

onMounted(async () => {
  void loadSources()
  // 首次拉取完成后再开始轮询，避免首屏请求较慢时重复拉取
  await poll()
  polling.start()
})
</script>

<style scoped>
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  align-items: center;
  margin-bottom: 10px;
}
.advanced {
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
}
.f-level {
  width: 110px;
}
.f-source {
  width: 150px;
}
.f-q {
  width: 240px;
}
.f-site {
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.f-ip {
  width: 300px;
}
.f-host {
  width: 180px;
}
.f-status {
  width: 130px;
}
.f-range {
  width: 130px;
}
.f-custom {
  max-width: 360px;
}
.tip {
  cursor: help;
}
.count {
  margin-left: 4px;
  font-weight: 600;
}
.hint {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
/* 导出与清空靠右排列 */
.actions {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
}
.actions .el-button + .el-button {
  margin-left: 0;
}
.error-tip {
  color: var(--el-color-danger);
  font-size: 12px;
  margin-bottom: 8px;
}
.box {
  height: 300px;
  overflow: auto;
  background: var(--el-fill-color-lighter);
  border-radius: 8px;
  border: 1px solid var(--g-border);
  padding: 8px 10px;
  font-size: 12px;
  line-height: 1.6;
}
.line {
  white-space: pre-wrap;
  word-break: break-all;
  /* 滚动区域外的行跳过布局与绘制，日志多时首屏与滚动更流畅；auto 让浏览器记住已渲染行的真实高度 */
  content-visibility: auto;
  contain-intrinsic-size: auto 19px;
}
.more {
  text-align: center;
  margin-bottom: 4px;
}
.time {
  color: var(--el-text-color-secondary);
  margin-right: 8px;
}
.lvl {
  display: inline-block;
  width: 48px;
}
.src {
  color: var(--gately-accent);
  margin-right: 6px;
  cursor: pointer;
}
.src:hover,
.field:hover {
  text-decoration: underline;
}
.field {
  margin-left: 8px;
  padding: 0 5px;
  border-radius: 4px;
  font-size: 11px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color);
  cursor: pointer;
}
.field.static {
  cursor: default;
  text-decoration: none;
}
mark {
  color: inherit;
  background: color-mix(in srgb, var(--el-color-warning) 35%, transparent);
  border-radius: 2px;
}
.warn .lvl {
  color: var(--el-color-warning);
}
.error .lvl,
.error {
  color: var(--el-color-danger);
}
.debug {
  opacity: 0.7;
}
.empty {
  color: var(--el-text-color-secondary);
}
/* 日志区与“回到最新”按钮的定位容器 */
.box-wrap {
  position: relative;
}
.jump {
  position: absolute;
  right: 14px;
  bottom: 12px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border: 1px solid var(--el-color-primary-light-5);
  border-radius: 999px;
  background: var(--el-bg-color-overlay);
  color: var(--el-color-primary);
  font-size: 12px;
  line-height: 1.4;
  box-shadow: var(--el-box-shadow-light);
  cursor: pointer;
}
.jump:hover {
  background: var(--el-color-primary-light-9);
}
/* 拖动条：上下留出触摸区域，中间一条短横线作提示；拖动时页面不选中文字 */
.resizer {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 12px;
  margin-top: 2px;
  cursor: ns-resize;
  touch-action: none;
  user-select: none;
  -webkit-user-select: none;
  border-radius: 6px;
  outline: none;
}
.grip {
  width: 44px;
  height: 4px;
  border-radius: 2px;
  background: var(--el-border-color);
  transition: background-color 0.15s;
}
.resizer:hover .grip,
.resizer:focus-visible .grip,
.resizer.dragging .grip {
  background: var(--el-color-primary);
}
.resizer:focus-visible {
  box-shadow: 0 0 0 2px var(--el-color-primary-light-7);
}
.actions .full-btn {
  padding-left: 6px;
  padding-right: 6px;
}
/* 撑满父容器：工具栏高度随筛选条件换行变化，日志区自动占据剩余空间，页面本身不再滚动 */
.log-viewer.fill,
.log-viewer.full {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.log-viewer.fill {
  height: 100%;
}
.log-viewer.fill .box-wrap,
.log-viewer.full .box-wrap {
  flex: 1 1 auto;
  display: flex;
  flex-direction: column;
  min-height: 160px;
}
.log-viewer.fill .box,
.log-viewer.full .box {
  flex: 1 1 auto;
  height: auto;
  min-height: 0;
}
/* 全屏：固定定位盖住整个视口，z-index 由脚本按 Element Plus 的层级给出；
   工具栏过高（窄屏筛选项折成多行）时整体可滚动，日志区至少保留半屏 */
.log-viewer.full {
  position: fixed;
  inset: 0;
  height: 100dvh;
  padding: calc(12px + env(safe-area-inset-top)) 16px calc(12px + env(safe-area-inset-bottom));
  overflow: auto;
  overscroll-behavior: contain;
  background: var(--el-bg-color);
}
.log-viewer.full .box-wrap {
  min-height: 50dvh;
}
@media (max-width: 767.98px) {
  .box {
    height: 240px;
    font-size: 11px;
  }
  /* 触摸设备上拖动条加高，便于手指按住 */
  .resizer {
    height: 20px;
  }
  .grip {
    width: 56px;
    height: 5px;
  }
  .log-viewer.full {
    padding: calc(8px + env(safe-area-inset-top)) 10px calc(8px + env(safe-area-inset-bottom));
  }
  .time {
    display: block;
    margin-right: 0;
  }
  .f-source,
  .f-q,
  .f-ip,
  .f-host,
  .f-custom {
    width: 100%;
    max-width: none;
  }
}
</style>
