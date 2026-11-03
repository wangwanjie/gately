<template>
  <el-dialog v-model="visible" :title="t('siteConns.title', { name: title })" width="980px" :fullscreen="isMobile" append-to-body @closed="stop">
    <div class="toolbar">
      <el-input v-model="query" :placeholder="t('siteConns.searchPlaceholder')" clearable size="small" class="search" :prefix-icon="Search" @input="onSearch" />
      <span class="meta">{{ t('siteConns.summary', { total: data?.total ?? 0, closed: data?.closed ?? 0 }) }}</span>
      <span class="meta refresh">{{ t('siteConns.autoRefresh') }}</span>
    </div>
    <el-tabs v-model="tab">
      <el-tab-pane :label="t('siteConns.activeTab', { count: data?.matched ?? 0 })" name="active">
        <el-table :data="data?.active ?? []" size="small" :empty-text="t('siteConns.noActive')" max-height="460" row-key="id">
          <el-table-column :label="t('siteConns.source')" min-width="160">
            <template #default="{ row }">
              <div class="mono">{{ hostPort(row) }}</div>
              <div v-if="row.peer" class="meta mono" :title="t('siteConns.peerHelp')">{{ t('siteConns.via', { peer: row.peer }) }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="t('siteConns.proto')" width="80">
            <template #default="{ row }">
              <span class="mono">{{ protoText(row.proto) }}</span>
              <el-tag v-if="row.websocket" size="small" type="success" effect="plain" class="ws">WS</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('siteConns.request')" min-width="240">
            <template #default="{ row }">
              <div class="mono ellipsis" :title="requestText(row)">{{ requestText(row) }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="t('siteConns.start')" width="140">
            <template #default="{ row }">{{ timeText(row.start) }}</template>
          </el-table-column>
          <el-table-column :label="t('siteConns.duration')" width="80">
            <template #default="{ row }"><span class="mono">{{ durText(row.durationMs) }}</span></template>
          </el-table-column>
          <el-table-column :label="t('siteConns.traffic')" width="110">
            <template #default="{ row }">
              <div class="mono">↓ {{ formatBytes(row.down) }}</div>
              <div v-if="row.up" class="mono meta">↑ {{ formatBytes(row.up) }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="140" fixed="right">
            <template #default="{ row }">
              <el-popconfirm :title="t('siteConns.closeConfirm')" width="260" @confirm="closeConn(row, 0)">
                <template #reference>
                  <el-button text size="small" type="danger" :icon="Unplug">{{ t('siteConns.close') }}</el-button>
                </template>
              </el-popconfirm>
              <el-button text size="small" type="danger" :icon="ShieldBan" :aria-label="t('siteConns.ban')" :title="t('siteConns.ban')" @click="openBan(row)" />
            </template>
          </el-table-column>
        </el-table>
        <div v-if="data && data.matched > data.active.length" class="meta note">{{ t('siteConns.truncated', { shown: data.active.length, matched: data.matched }) }}</div>
      </el-tab-pane>
      <el-tab-pane :label="t('siteConns.historyTab', { count: data?.history.length ?? 0 })" name="history">
        <el-table :data="data?.history ?? []" size="small" :empty-text="t('siteConns.noHistory')" max-height="460">
          <el-table-column :label="t('siteConns.source')" min-width="160">
            <template #default="{ row }">
              <div class="mono">{{ hostPort(row) }}</div>
              <div v-if="row.peer" class="meta mono">{{ t('siteConns.via', { peer: row.peer }) }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="t('siteConns.proto')" width="80">
            <template #default="{ row }">
              <span class="mono">{{ protoText(row.proto) }}</span>
              <el-tag v-if="row.websocket" size="small" type="success" effect="plain" class="ws">WS</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('siteConns.request')" min-width="220">
            <template #default="{ row }">
              <div class="mono ellipsis" :title="requestText(row)">{{ requestText(row) }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="t('siteConns.startEnd')" width="150">
            <template #default="{ row }">
              <div>{{ timeText(row.start) }}</div>
              <div class="meta">{{ row.end ? timeText(row.end) : '-' }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="t('siteConns.duration')" width="80">
            <template #default="{ row }"><span class="mono">{{ durText(row.durationMs) }}</span></template>
          </el-table-column>
          <el-table-column :label="t('siteConns.traffic')" width="110">
            <template #default="{ row }">
              <div class="mono">↓ {{ formatBytes(row.down) }}</div>
              <div v-if="row.up" class="mono meta">↑ {{ formatBytes(row.up) }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="t('siteConns.result')" width="90" fixed="right">
            <template #default="{ row }">
              <el-tag size="small" :type="row.result === 'closed' ? 'danger' : 'success'">{{ row.result === 'closed' ? t('siteConns.resultClosed') : t('siteConns.resultDone') }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
        <div class="meta note">{{ t('siteConns.historyHelp') }}</div>
      </el-tab-pane>
    </el-tabs>
    <div class="meta note">{{ t('siteConns.help') }}</div>

    <el-dialog v-model="banVisible" :title="t('siteConns.banTitle')" width="420px" append-to-body>
      <p v-if="banRow">{{ t('siteConns.banDesc', { ip: banRow.ip }) }}</p>
      <p v-if="banRow?.ip.includes(':')" class="meta">{{ t('siteConns.banIpv6') }}</p>
      <div class="ban-hours">
        <!-- 勾选永久时不限时长（接口 banSeconds 为 -1） -->
        <el-input-number v-model="banHours" :min="1" :max="720" :disabled="banPermanent" />
        <span>{{ t('siteConns.hours') }}</span>
        <el-checkbox v-model="banPermanent">{{ t('siteConns.permanent') }}</el-checkbox>
      </div>
      <template #footer>
        <el-button @click="banVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :loading="banning" @click="confirmBan">{{ t('siteConns.banConfirm') }}</el-button>
      </template>
    </el-dialog>
  </el-dialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Search, ShieldBan, Unplug } from 'lucide-vue-next'
import { api, formatBytes } from '../api'
import { useFormLayout } from '../composables/useIsMobile'

// Conn 对应 GET /api/webservice/connections 返回的一条进行中或已结束的请求
interface Conn {
  id: string
  ip: string
  port: number
  peer?: string
  proto: string
  method: string
  host: string
  path: string
  websocket?: boolean
  start: string
  end?: string
  durationMs: number
  down: number
  up: number
  result?: 'done' | 'closed'
}

interface ConnsView {
  total: number
  matched: number
  active: Conn[]
  history: Conn[]
  closed: number
}

const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const { isMobile } = useFormLayout()
const visible = ref(false)
const title = ref('')
const tab = ref('active')
const query = ref('')
const data = ref<ConnsView>()
let target = { listener: '', site: '' }
let timer: number | undefined
let searchTimer: number | undefined
// seq 丢弃过期的响应：搜索词变化或弹窗重开后，先发出的慢请求不能覆盖新结果
let seq = 0

const banVisible = ref(false)
const banRow = ref<Conn>()
const banHours = ref(24)
const banPermanent = ref(false)
const banning = ref(false)

function hostPort(c: Conn) {
  return c.ip.includes(':') ? `[${c.ip}]:${c.port}` : `${c.ip}:${c.port}`
}

function protoText(p: string) {
  const m = /^HTTP\/(\d)/.exec(p)
  return m ? `h${m[1]}` : p
}

function requestText(c: Conn) {
  return `${c.method} ${c.host}${c.path}`
}

// timeText 当天只显示时间，其他日期带上日期
function timeText(s: string) {
  const d = new Date(s)
  return d.toDateString() === new Date().toDateString() ? d.toLocaleTimeString() : d.toLocaleString()
}

// durText 把毫秒格式化为 m:ss 或 h:mm:ss，不足 1 秒显示毫秒
function durText(ms: number) {
  if (ms < 1000) {
    return `${ms}ms`
  }
  const total = Math.floor(ms / 1000)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const sec = String(total % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${sec}` : `${m}:${sec}`
}

async function load() {
  const my = ++seq
  try {
    const q = query.value.trim()
    const v = await api.get<ConnsView>(
      `/api/webservice/connections?listener=${encodeURIComponent(target.listener)}&site=${encodeURIComponent(target.site)}${q ? `&q=${encodeURIComponent(q)}` : ''}`,
    )
    if (my === seq && visible.value) {
      data.value = v
    }
  } catch (e) {
    if (my === seq && visible.value) {
      stop()
      ElMessage.error((e as Error).message)
    }
  }
}

// schedule 每 2 秒刷新一次；上一次请求结束后才安排下一次，避免慢网络下请求堆积
function schedule() {
  window.clearTimeout(timer)
  timer = window.setTimeout(async () => {
    if (!visible.value) {
      return
    }
    await load()
    if (visible.value && timer !== undefined) {
      schedule()
    }
  }, 2000)
}

function stop() {
  window.clearTimeout(timer)
  window.clearTimeout(searchTimer)
  timer = undefined
}

function onSearch() {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(load, 300)
}

// closeConn 断开（banSeconds 非 0 时同时封禁，-1 为永久）一条请求，返回是否成功，供封禁对话框决定是否关闭。
async function closeConn(row: Conn, banSeconds: number): Promise<boolean> {
  let ok = false
  try {
    const r = await api.post<{ closed: number; banKey?: string }>('/api/webservice/connections/close', {
      listener: target.listener,
      site: target.site,
      id: row.id,
      banSeconds,
    })
    if (banSeconds !== 0) {
      ElMessage.success(t('siteConns.bannedMsg', { key: r.banKey ?? row.ip, count: r.closed }))
    } else {
      ElMessage.success(t('siteConns.closedMsg'))
    }
    emit('changed')
    ok = true
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
  await load()
  return ok
}

function openBan(row: Conn) {
  banRow.value = row
  banHours.value = 24
  banPermanent.value = false
  banVisible.value = true
}

async function confirmBan() {
  if (!banRow.value) {
    return
  }
  banning.value = true
  try {
    // 封禁失败（如封禁时长超出范围、地址是管理员自己）时保留对话框，便于修改后重试
    if (await closeConn(banRow.value, banPermanent.value ? -1 : banHours.value * 3600)) {
      banVisible.value = false
    }
  } finally {
    banning.value = false
  }
}

// 对话框开着时离开页面（后退、侧滑返回）组件直接卸载，不会触发 @closed，这里停止轮询
onBeforeUnmount(() => {
  visible.value = false
  stop()
})

function open(listener: string, site: string, name: string) {
  target = { listener, site }
  title.value = name
  data.value = undefined
  query.value = ''
  tab.value = 'active'
  visible.value = true
  load()
  schedule()
}

defineExpose({ open })
</script>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}
.search {
  width: 220px;
}
.refresh {
  margin-left: auto;
}
.meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.note {
  margin-top: 8px;
  line-height: 1.5;
}
.ws {
  margin-left: 4px;
}
.ban-hours {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
</style>
