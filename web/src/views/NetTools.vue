<template>
  <div>
    <PageHeader :title="t('netTools.title')" :desc="t('netTools.desc')" :icon="Activity">
      <el-button :icon="ShieldBan" @click="openSettings">{{ t('netTools.denyList') }}</el-button>
    </PageHeader>

    <el-card shadow="never" class="card">
      <el-tabs v-model="tool" @tab-change="onToolChange">
        <el-tab-pane label="Ping" name="ping" />
        <el-tab-pane :label="t('netTools.traceroute')" name="traceroute" />
        <el-tab-pane :label="t('netTools.tcp')" name="tcp" />
        <el-tab-pane :label="t('netTools.dns')" name="dns" />
        <el-tab-pane :label="t('netTools.speed')" name="speed" />
        <el-tab-pane :label="t('netTools.scan')" name="scan" />
      </el-tabs>

      <el-form :model="form" inline class="form" @submit.prevent="run">
        <el-form-item v-if="['ping', 'traceroute', 'tcp', 'dns'].includes(tool)" :label="tool === 'dns' ? t('netTools.domainOrIp') : t('netTools.target')">
          <el-input v-model="form.target" :placeholder="tool === 'dns' ? 'example.com' : t('netTools.hostPlaceholder')" class="w-260" clearable />
        </el-form-item>
        <el-form-item v-if="tool === 'ping'" :label="t('netTools.count')"><el-input-number v-model="form.count" :min="1" :max="20" /></el-form-item>
        <el-form-item v-if="tool === 'tcp'" :label="t('netTools.port')">
          <el-input v-model="form.ports" placeholder="22,80,443,8000-8100" class="w-260 mono" />
        </el-form-item>
        <template v-if="tool === 'dns'">
          <el-form-item :label="t('netTools.type')">
            <el-select v-model="form.type" class="w-110">
              <el-option v-for="rt in ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'PTR']" :key="rt" :label="rt" :value="rt" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('netTools.dnsServer')">
            <el-input v-model="form.server" :placeholder="t('netTools.dnsServerPlaceholder')" class="w-360" clearable />
          </el-form-item>
        </template>
        <template v-if="tool === 'speed'">
          <el-form-item :label="t('netTools.downloadUrl')">
            <!-- 可选常用公开测速节点，也可直接输入任意下载地址；留空由后端使用 Cloudflare -->
            <el-select v-model="form.url" filterable allow-create default-first-option clearable :placeholder="t('netTools.downloadUrlPlaceholder')" class="w-360">
              <el-option v-for="n in speedNodes" :key="n.url" :label="`${n.provider} · ${t(`netTools.speedNodes.${n.place}`)}`" :value="n.url">
                <span>{{ n.provider }} · {{ t(`netTools.speedNodes.${n.place}`) }}</span>
                <span class="opt-host mono">{{ n.host }}</span>
              </el-option>
            </el-select>
          </el-form-item>
          <el-form-item :label="t('netTools.duration')"><el-input-number v-model="form.seconds" :min="2" :max="30" /></el-form-item>
        </template>
        <el-form-item v-if="tool === 'scan'" :label="t('netTools.cidr')">
          <el-select v-model="form.cidr" filterable allow-create default-first-option clearable :placeholder="t('netTools.cidrPlaceholder')" class="w-260">
            <el-option v-for="n in networks" :key="n" :label="n" :value="n" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" native-type="submit" :icon="Play" :loading="starting" :disabled="running">{{ t('netTools.start') }}</el-button>
          <el-button v-if="running" :icon="Square" @click="cancel">{{ t('common.stop') }}</el-button>
        </el-form-item>
      </el-form>
      <div v-if="tool === 'scan'" class="form-help">{{ t('netTools.scanHelp') }}</div>
    </el-card>

    <el-card v-if="current" shadow="never" class="card">
      <template #header>
        <div class="head">
          <span>{{ current.title }}</span>
          <StatusBadge :type="badge.type" :text="badge.text" />
        </div>
      </template>
      <pre ref="outEl" class="out mono">{{ current.lines.join('\n') }}</pre>
      <div v-if="current.error" class="err">{{ current.error }}</div>

      <el-table v-if="current.tool === 'scan' && hosts.length" :data="hosts" size="small" stripe class="tbl">
        <el-table-column prop="ip" label="IP" width="140" />
        <el-table-column prop="mac" label="MAC" width="160"><template #default="{ row }"><span class="mono">{{ row.mac || '-' }}</span></template></el-table-column>
        <el-table-column prop="hostname" :label="t('netTools.hostname')" min-width="160" show-overflow-tooltip />
        <el-table-column :label="t('netTools.openPorts')" min-width="140"><template #default="{ row }">{{ (row.ports ?? []).join(', ') || '-' }}</template></el-table-column>
        <el-table-column label="" width="120">
          <template #default="{ row }">
            <el-button v-if="row.mac" text size="small" :icon="Power" @click="addWol(row)">{{ t('netTools.addWol') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card v-if="history.length" shadow="never" class="card">
      <template #header>{{ t('netTools.recentTasks') }}</template>
      <div v-for="task in history" :key="task.id" class="hist" @click="select(task.id)">
        <StatusBadge :type="statusType(task.status)" :text="statusText(task.status)" />
        <span class="t-title">{{ task.title }}</span>
        <span class="meta">{{ new Date(task.started).toLocaleTimeString() }}</span>
      </div>
    </el-card>

    <el-dialog v-model="settingsVisible" :title="t('netTools.denyList')" width="480px" :fullscreen="isMobile">
      <p class="form-help">{{ t('netTools.denyHelp') }}</p>
      <el-input v-model="denyText" type="textarea" :rows="5" class="mono" placeholder="10.0.0.0/8" />
      <template #footer>
        <el-button @click="settingsVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Activity, Play, Power, ShieldBan, Square } from 'lucide-vue-next'
import { api } from '../api'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import { useFormLayout } from '../composables/useIsMobile'

interface Host {
  ip: string
  mac?: string
  hostname?: string
  ports?: number[]
}
interface Task {
  id: string
  tool: string
  title: string
  status: string
  error?: string
  lines: string[]
  result?: unknown
  started: string
}

// 常用公开测速文件：都能直接返回 200（后端不跟随重定向），单个文件下完后会重复下载直到时长结束。
// Cloudflare 的 bytes 需小于 1 亿，否则返回 403。
const speedNodes = [
  { provider: 'Cloudflare', place: 'anycast', url: 'https://speed.cloudflare.com/__down?bytes=50000000' },
  { provider: 'Vultr', place: 'tokyo', url: 'https://hnd-jp-ping.vultr.com/vultr.com.1000MB.bin' },
  { provider: 'Vultr', place: 'singapore', url: 'https://sgp-ping.vultr.com/vultr.com.1000MB.bin' },
  { provider: 'Vultr', place: 'losAngeles', url: 'https://lax-ca-us-ping.vultr.com/vultr.com.1000MB.bin' },
  { provider: 'Hetzner', place: 'singapore', url: 'https://sin-speed.hetzner.com/1GB.bin' },
  { provider: 'Hetzner', place: 'germany', url: 'https://fsn1-speed.hetzner.com/1GB.bin' },
  { provider: 'OVH', place: 'france', url: 'https://proof.ovh.net/files/1Gb.dat' },
].map((n) => ({ ...n, host: new URL(n.url).host }))

const { t } = useI18n()
const { isMobile } = useFormLayout()
const tool = ref('ping')
const form = reactive({ target: '', count: 4, ports: '22,80,443', type: 'A', server: '', url: '', seconds: 8, cidr: '' })
const networks = ref<string[]>([])
const starting = ref(false)
const current = ref<Task>()
const history = ref<Task[]>([])
const outEl = ref<HTMLElement>()
const settingsVisible = ref(false)
const denyText = ref('')
let timer: number | undefined

const running = computed(() => current.value?.status === 'running')
const hosts = computed(() => (current.value?.tool === 'scan' ? ((current.value.result as Host[] | undefined) ?? []) : []))
const badge = computed(() => ({ type: statusType(current.value?.status ?? ''), text: statusText(current.value?.status ?? '') }))

function statusType(s: string): StatusType {
  return s === 'running' ? 'pending' : s === 'done' ? 'running' : s === 'error' ? 'error' : 'off'
}
function statusText(s: string) {
  return ({ running: t('common.running'), done: t('netTools.done'), error: t('netTools.failed'), canceled: t('netTools.canceled') } as Record<string, string>)[s] ?? s
}

function onToolChange() {
  current.value = undefined
}

async function run() {
  starting.value = true
  try {
    const task = await api.post<Task>('/api/nettools/run', { tool: tool.value, ...form })
    current.value = task
    poll(task.id)
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    starting.value = false
  }
}

function poll(id: string) {
  window.clearInterval(timer)
  const tick = async () => {
    try {
      const task = await api.get<Task>(`/api/nettools/tasks/${id}`)
      current.value = task
      if (task.status !== 'running') {
        window.clearInterval(timer)
        loadHistory()
      }
    } catch {
      window.clearInterval(timer)
    }
  }
  timer = window.setInterval(tick, 700)
}

async function cancel() {
  if (current.value) {
    await api.del(`/api/nettools/tasks/${current.value.id}`)
  }
}

async function loadHistory() {
  history.value = await api.get<Task[]>('/api/nettools/tasks')
}

async function select(id: string) {
  const task = await api.get<Task>(`/api/nettools/tasks/${id}`)
  tool.value = task.tool
  current.value = task
  if (task.status === 'running') {
    poll(id)
  }
}

async function addWol(h: Host) {
  try {
    await api.post('/api/wol/devices', { name: h.hostname || h.ip, macs: [h.mac], broadcastIps: [], port: 9, repeat: 3, checkAddr: '', remark: t('netTools.wolRemark', { ip: h.ip }), enable: true })
    ElMessage.success(t('netTools.wolAdded'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function openSettings() {
  const s = await api.get<{ deny: string[] }>('/api/nettools/settings')
  denyText.value = (s.deny ?? []).join('\n')
  settingsVisible.value = true
}

async function saveSettings() {
  try {
    await api.put('/api/nettools/settings', { deny: denyText.value.split('\n').map((s) => s.trim()).filter(Boolean) })
    settingsVisible.value = false
    ElMessage.success(t('common.saved'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// 输出更新时滚动到底部
watch(
  () => current.value?.lines.length,
  async () => {
    await nextTick()
    if (outEl.value) {
      outEl.value.scrollTop = outEl.value.scrollHeight
    }
  },
)

onMounted(async () => {
  networks.value = await api.get<string[]>('/api/nettools/networks')
  await loadHistory()
})
onBeforeUnmount(() => window.clearInterval(timer))
</script>

<style scoped>
.card {
  margin-bottom: 14px;
}
.form {
  margin-top: 4px;
}
.w-260 {
  width: 260px;
}
.w-360 {
  width: 360px;
  max-width: 100%;
}
.opt-host {
  float: right;
  margin-left: 16px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.w-110 {
  width: 110px;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.out {
  margin: 0;
  max-height: 340px;
  overflow: auto;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  font-size: 12.5px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-all;
}
.err {
  margin-top: 8px;
  color: var(--el-color-danger);
  font-size: 13px;
}
.tbl {
  margin-top: 12px;
}
.hist {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 5px 0;
  cursor: pointer;
}
.t-title {
  flex: 1;
}
.meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
