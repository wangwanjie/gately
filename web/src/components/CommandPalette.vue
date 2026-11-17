<template>
  <!-- 全局搜索（命令面板）：Ctrl/⌘+K、侧边栏搜索框或手机顶栏图标打开；上下键选择，回车跳转 -->
  <el-dialog
    v-model="visible"
    class="cmd-palette"
    :fullscreen="isMobile"
    :show-close="false"
    :width="isMobile ? undefined : 'min(640px, calc(100vw - 32px))'"
    top="12vh"
    append-to-body
    :aria-label="t('palette.ariaLabel')"
    @open-auto-focus="focusInput"
    @opened="onOpened"
    @closed="onClosed"
  >
    <div class="cp-input-row">
      <Search :size="18" class="cp-icon" aria-hidden="true" />
      <input
        ref="inputEl"
        v-model="query"
        class="cp-input"
        type="text"
        role="combobox"
        aria-autocomplete="list"
        :aria-expanded="results.length > 0"
        aria-controls="cp-list"
        :aria-activedescendant="active >= 0 ? `cp-opt-${active}` : undefined"
        :aria-label="t('palette.ariaLabel')"
        :placeholder="t('palette.placeholder')"
        autocomplete="off"
        spellcheck="false"
        @keydown="onKeydown"
      />
      <button v-if="isMobile" class="cp-close" type="button" @click="visible = false">{{ t('common.cancel') }}</button>
      <kbd v-else class="cp-kbd">Esc</kbd>
    </div>
    <div ref="listEl" class="cp-list">
      <ul id="cp-list" role="listbox" :aria-label="t('palette.ariaLabel')">
        <template v-for="(e, i) in results" :key="e.id">
          <li v-if="i === 0 || results[i - 1].kind !== e.kind" class="cp-kind" role="presentation">{{ kindLabel(e.kind) }}</li>
          <li
            :id="`cp-opt-${i}`"
            role="option"
            class="cp-item"
            :class="{ active: i === active }"
            :aria-selected="i === active"
            :data-kind="e.kind"
            @mousemove="active = i"
            @click="go(e)"
          >
            <component :is="iconOf(e)" :size="16" class="cp-item-icon" aria-hidden="true" />
            <span class="cp-item-title">{{ e.title }}</span>
            <span v-if="e.sub" class="cp-item-sub">{{ e.sub }}</span>
          </li>
        </template>
      </ul>
      <div v-if="!results.length" class="cp-empty">{{ loading ? t('palette.loading') : t('palette.empty') }}</div>
      <div v-else-if="loading" class="cp-loading">{{ t('palette.loading') }}</div>
    </div>
    <div v-if="!isMobile" class="cp-foot">
      <span><kbd>↑</kbd><kbd>↓</kbd>{{ t('palette.hintSelect') }}</span>
      <span><kbd>Enter</kbd>{{ t('palette.hintOpen') }}</span>
      <span><kbd>Esc</kbd>{{ t('palette.hintClose') }}</span>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ArrowLeftRight, Globe, Search, Server, ShieldCheck, Settings } from 'lucide-vue-next'
import { api } from '../api'
import { useIsMobile } from '../composables/useIsMobile'
import { isReadonly, itemEnabled, moduleEnabled, navByPath, navGroups, pathEnabled, type NavItem } from '../nav'
import { certEntries, ddnsEntries, forwardEntries, moveIndex, search, siteEntries, type EntryKind, type PaletteEntry } from '../palette'
import { bumpPageKey } from '../composables/usePalette'

const visible = defineModel<boolean>({ default: false })

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const isMobile = useIsMobile()
const query = ref('')
const active = ref(0)
const inputEl = ref<HTMLInputElement>()
const listEl = ref<HTMLElement>()

// ---------- 页面条目：侧边栏入口、汇总页标签、隐藏页面与设置页标签 ----------

// 设置页的标签与名称键名（只有管理员能进入设置页）
const settingsTabs: [string, string][] = [
  ['account', 'settings.tabs.account'],
  ['access', 'settings.tabs.access'],
  ['runtime', 'settings.tabs.runtime'],
  ['modules', 'settings.tabs.modules'],
  ['backup', 'settings.tabs.backup'],
  ['diagnostics', 'settings.tabs.diagnostics'],
]

// pageItemEntry 生成一个页面条目：副文字为所在分组，汇总页标签再加上汇总页名称
function pageItemEntry(groupKey: string, item: NavItem): PaletteEntry {
  const hub = item.hub && item.hub !== item.path ? navByPath(item.hub) : undefined
  const sub = [groupKey ? t(groupKey) : '', hub ? t(hub.titleKey) : ''].filter(Boolean).join(' · ')
  // 关键词带上路由路径，输入英文路径（如 audit、webdav）也能找到
  return { id: 'page:' + item.path, kind: 'page', title: t(item.titleKey), sub, path: item.path, keywords: item.path.replace(/[/-]/g, ' ') }
}

const pageEntries = computed<PaletteEntry[]>(() => {
  const out: PaletteEntry[] = []
  for (const g of navGroups) {
    for (const item of g.items) {
      // 汇总页入口本身没有内容，搜索直接落到各标签
      if (item.hubRoot || !itemEnabled(item)) {
        continue
      }
      out.push(pageItemEntry(g.titleKey, item))
      if (item.path === '/settings') {
        for (const [tab, key] of settingsTabs) {
          out.push({ id: 'page:/settings?tab=' + tab, kind: 'page', title: t(key), sub: t('nav.items.settings.title'), path: '/settings', query: { tab }, keywords: 'settings ' + tab })
        }
      }
    }
  }
  return out
})

// ---------- 配置数据条目：打开面板时按需读取各模块的列表接口，缓存一分钟 ----------

interface Source {
  kind: EntryKind
  module: string
  url: string
  build: (data: any) => PaletteEntry[]
}

const sources: Source[] = [
  { kind: 'site', module: 'webservice', url: '/api/webservice/listeners', build: siteEntries },
  { kind: 'forward', module: 'portforward', url: '/api/portforward/rules', build: forwardEntries },
  { kind: 'ddns', module: 'ddns', url: '/api/ddns/tasks', build: ddnsEntries },
  { kind: 'cert', module: 'ssl', url: '/api/ssl/certs', build: certEntries },
]

const CACHE_MS = 60_000
const dataEntries = ref<PaletteEntry[]>([])
const loading = ref(false)
let loadedAt = 0
// 记录读取时的身份，换成只读账号或开关模块后重新读取
let loadedFor = ''

async function loadData() {
  const who = `${isReadonly.value}|${sources.map((s) => moduleEnabled(s.module)).join(',')}`
  if (loading.value || (Date.now() - loadedAt < CACHE_MS && loadedFor === who)) {
    return
  }
  loading.value = true
  // 只读取已开启模块的列表；读取失败（如没有权限）的类别直接略过，不打扰用户
  const results = await Promise.all(
    sources
      .filter((s) => moduleEnabled(s.module) && pathEnabled(pathOf(s.kind)))
      .map(async (s) => {
        try {
          return s.build(await api.get(s.url))
        } catch {
          return []
        }
      }),
  )
  dataEntries.value = results.flat()
  loadedAt = Date.now()
  loadedFor = who
  loading.value = false
}

// pathOf 返回各类数据所在的页面，用于判断当前身份能否进入
function pathOf(kind: EntryKind): string {
  return { page: '/overview', site: '/web', forward: '/portforward', ddns: '/ddns', cert: '/ssl' }[kind]
}

// 空查询时只列页面，避免一打开就铺满配置数据
const results = computed(() => (query.value.trim() ? search([...pageEntries.value, ...dataEntries.value], query.value) : search(pageEntries.value, '', 50)))

// 改了查询词时回到第一条；配置数据读取完成后列表变短时把选中项收回范围内
watch(query, () => {
  active.value = 0
})
watch(
  () => results.value.length,
  (len) => {
    if (active.value >= len || active.value < 0) {
      active.value = len ? 0 : -1
    }
  },
)

const kindKeys: Record<EntryKind, string> = {
  page: 'palette.kinds.page',
  site: 'palette.kinds.site',
  forward: 'palette.kinds.forward',
  ddns: 'palette.kinds.ddns',
  cert: 'palette.kinds.cert',
}

function kindLabel(kind: EntryKind): string {
  return t(kindKeys[kind])
}

const kindIcons: Record<EntryKind, Component> = { page: Settings, site: Server, forward: ArrowLeftRight, ddns: Globe, cert: ShieldCheck }

// iconOf 页面条目用页面自己的图标，数据条目用所在模块的图标
function iconOf(e: PaletteEntry): Component {
  if (e.kind === 'page') {
    return navByPath(e.path)?.icon ?? kindIcons.page
  }
  return kindIcons[e.kind]
}

// ---------- 交互 ----------

function onKeydown(ev: KeyboardEvent) {
  // 输入法组字过程中的方向键与回车交给输入法
  if (ev.isComposing) {
    return
  }
  if (ev.key === 'ArrowDown' || ev.key === 'ArrowUp') {
    ev.preventDefault()
    active.value = moveIndex(active.value, ev.key === 'ArrowDown' ? 1 : -1, results.value.length)
    nextTick(scrollActive)
  } else if (ev.key === 'Enter') {
    ev.preventDefault()
    const e = results.value[active.value]
    if (e) {
      go(e)
    }
  }
}

// scrollActive 让键盘选中的条目保持在可视范围内
function scrollActive() {
  listEl.value?.querySelector('.cp-item.active')?.scrollIntoView({ block: 'nearest' })
}

async function go(e: PaletteEntry) {
  visible.value = false
  const sameQuery = JSON.stringify(e.query ?? {}) === JSON.stringify(route.query)
  if (route.path === e.path && e.query && !sameQuery) {
    // 已在目标页时页面不会重新挂载，部分页面只在挂载时读取参数（如 Web 服务页打开站点编辑框），这里让它重新挂载一次
    bumpPageKey()
  }
  await router.push({ path: e.path, query: e.query })
}

// focusInput 打开时把焦点放到输入框：对话框的焦点陷阱会在下一个微任务里把焦点移到对话框容器，这里排在它之后执行
function focusInput() {
  setTimeout(() => inputEl.value?.focus(), 0)
  void loadData()
}

// 关闭动画尚未结束就再次打开时对话框不会重新触发自动聚焦，这里按打开状态再补一次
watch(visible, (v) => {
  if (v) {
    focusInput()
  }
})

// 打开动画结束后再确认一次焦点（用户可能在动画期间点了别处）
function onOpened() {
  if (document.activeElement !== inputEl.value) {
    inputEl.value?.focus()
  }
}

function onClosed() {
  query.value = ''
}
</script>

<style scoped>
.cp-input-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 14px;
  height: 52px;
  border-bottom: 1px solid var(--g-border);
}
.cp-icon {
  color: var(--el-text-color-secondary);
  flex-shrink: 0;
}
.cp-input {
  flex: 1;
  min-width: 0;
  height: 100%;
  border: none;
  outline: none;
  background: transparent;
  color: var(--el-text-color-primary);
  /* 16px 以上 iOS 聚焦时不会自动放大页面 */
  font-size: 16px;
}
.cp-input::placeholder {
  color: var(--el-text-color-placeholder);
}
.cp-close {
  border: none;
  background: transparent;
  color: var(--el-color-primary);
  font-size: 15px;
  padding: 8px 0 8px 8px;
  cursor: pointer;
}
kbd {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 5px;
  border: 1px solid var(--g-border);
  border-radius: 4px;
  font-family: inherit;
  font-size: 11px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
}
.cp-list {
  max-height: min(420px, 60vh);
  overflow-y: auto;
  padding: 6px;
}
.cp-list ul {
  list-style: none;
  margin: 0;
  padding: 0;
}
.cp-kind {
  padding: 10px 10px 4px;
  font-size: 12px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
}
.cp-item {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 40px;
  padding: 6px 10px;
  border-radius: 8px;
  cursor: pointer;
  color: var(--el-text-color-regular);
}
.cp-item.active {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}
.cp-item-icon {
  flex-shrink: 0;
}
.cp-item-title {
  font-size: 14px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.cp-item-sub {
  margin-left: auto;
  padding-left: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
  flex-shrink: 1;
  max-width: 55%;
  text-align: right;
}
.cp-empty,
.cp-loading {
  padding: 18px 10px;
  text-align: center;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.cp-loading {
  padding: 8px 10px;
}
.cp-foot {
  display: flex;
  gap: 16px;
  padding: 8px 14px;
  border-top: 1px solid var(--g-border);
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.cp-foot span {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
@media (max-width: 767.98px) {
  .cp-list {
    max-height: none;
  }
  .cp-item-sub {
    max-width: 45%;
  }
}
</style>

<style>
/* 命令面板去掉对话框自带的标题区与内边距，输入框贴边 */
.el-dialog.cmd-palette {
  padding: 0;
  overflow: hidden;
}
.el-dialog.cmd-palette > .el-dialog__header {
  display: none;
}
.el-dialog.cmd-palette > .el-dialog__body {
  padding: 0;
}
.el-dialog.cmd-palette.is-fullscreen > .el-dialog__body {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.el-dialog.cmd-palette.is-fullscreen .cp-list {
  flex: 1;
  min-height: 0;
}
</style>
