<template>
  <div>
    <PageHeader title="NetBird" :desc="t('netbird.desc')" :icon="Bird">
      <ResetConfigButton v-if="!isReadonly" base="/api/netbird" :desc="t('components.resetConfig.netbird')" @done="onReset" />
    </PageHeader>

    <!-- 每个等待登录的实例单独提示；登录链接与设备码只返回给有写权限的身份 -->
    <template v-for="st in statuses" :key="st.key">
      <el-alert v-if="st.authUrl" type="warning" :closable="false" show-icon class="block-gap" :title="t('netbird.needLoginOf', { name: st.name })">
        <!-- 复制按钮复制登录地址本身，便于在其他设备上打开完成登录 -->
        <UrlLink :href="st.authUrl" :text="t('netbird.authLink')" class="auth-link" />
        <div v-if="st.authCode" class="auth-code">{{ t('netbird.authCode') }} <span class="mono">{{ st.authCode }}</span></div>
        <div class="form-help">{{ t('netbird.authHelp') }}</div>
      </el-alert>
    </template>

    <div class="g-card">
      <div class="g-card-head g-sticky-bar">
        <div class="g-card-title"><Bird :size="16" />{{ t('netbird.instances') }} <span v-if="anyStatus?.version" class="g-card-desc">{{ anyStatus.version }}</span></div>
        <div class="head-actions">
          <!-- 排序与批量操作会修改配置，只读账号不显示 -->
          <ListTools v-if="!isReadonly" :gl="gl" size="small" />
          <CompactToggle v-model="compact" size="small" />
          <el-button v-if="!isReadonly" type="primary" size="small" :icon="Plus" @click="openEditor()">{{ t('netbird.addInstance') }}</el-button>
        </div>
      </div>
      <div class="g-card-body">
        <GroupedList :gl="gl" :compact="compact" grid :empty-text="t('netbird.noInstances')" :empty-image-size="60">
          <template #item="{ it, item: v, sorting, bind }">
            <EntityCard v-bind="bind" :off="!v.enable" :compact="compact || sorting">
              <template v-if="sorting" #lead>
                <SortCheck :gl="gl" :id="it.id" />
              </template>
              <template #title>{{ v.name }}</template>
              <template #sub><span class="mono">{{ hostOf(v.managementUrl) || t('netbird.cloud') }}</span><template v-if="v.remark"> · {{ v.remark }}</template></template>
              <template #status><StatusBadge v-bind="badgeOf(v)" /></template>
              <Field :label="t('netbird.runMode')">{{ modeText(v) }}</Field>
              <Field v-if="statusOf(v.key)?.self" label="NetBird IP" mono>
                <div v-for="ip in statusOf(v.key)?.self?.ips" :key="ip">{{ ip }}</div>
              </Field>
              <Field v-if="statusOf(v.key)?.self?.fqdn" label="FQDN" mono>{{ statusOf(v.key)?.self?.fqdn }}</Field>
              <Field v-if="v.manageDaemon" :label="t('netbird.wgPort')" mono>{{ v.port || defaultWgPort }}</Field>
              <Field v-if="v.manageDaemon && v.userspace" :label="t('netbird.socks5')" mono>{{ v.socks5 || defaultSocks5 }}</Field>
              <Field v-if="v.manageDaemon" :label="t('netbird.stateDir')" mono>{{ v.stateDir || v.defaultStateDir }}</Field>
              <Field v-else :label="t('netbird.daemonAddr')" mono>{{ v.daemonAddr || t('netbird.systemDefault') }}</Field>
              <template v-if="sorting" #actions>
                <SortMoves :gl="gl" :it="it" />
              </template>
              <template v-else-if="!isReadonly" #actions>
                <el-switch :model-value="v.enable" size="small" @change="(x: boolean | string | number) => toggle(v.key, Boolean(x))" />
                <el-button text size="small" :icon="RefreshCw" :disabled="!v.enable" @click="restart(v.key)">{{ t('common.restart') }}</el-button>
                <el-button text size="small" :icon="Pencil" @click="openEditor(v)">{{ t('common.edit') }}</el-button>
                <el-button text size="small" :icon="CopyPlus" @click="openEditor(v, true)">{{ t('common.duplicate') }}</el-button>
                <el-popconfirm :title="t('netbird.confirmDelete')" width="300" @confirm="remove(v.key)">
                  <template #reference>
                    <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
                  </template>
                </el-popconfirm>
              </template>
            </EntityCard>
          </template>
        </GroupedList>
      </div>
    </div>

    <!-- 每个已启用实例的连接详情与节点列表 -->
    <div v-for="st in enabledStatuses" :key="st.key" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title">
          <Waypoints :size="16" />{{ st.name }}
          <span v-if="st.peers.length" class="g-card-desc">{{ t('netbird.peerCount', { connected: st.peersConnected, count: st.peers.length }) }}</span>
        </div>
        <el-popconfirm v-if="!isReadonly && st.daemonStatus === 'Connected'" :title="t('netbird.confirmLogout')" width="280" @confirm="logout(st.key)">
          <template #reference>
            <el-button size="small" :icon="LogOut">{{ t('netbird.logout') }}</el-button>
          </template>
        </el-popconfirm>
      </div>
      <div class="g-card-body">
        <el-alert v-if="st.error" type="error" :title="st.error" :closable="false" show-icon class="block-gap" />
        <div v-if="st.self" class="info-grid">
          <Field :label="t('netbird.self')" mono>{{ st.self.name }}</Field>
          <Field label="NetBird IP" mono>
            <div v-for="ip in st.self.ips" :key="ip">{{ ip }}</div>
          </Field>
          <Field v-if="st.self.fqdn" label="FQDN" mono>{{ st.self.fqdn }}</Field>
          <Field v-if="st.management" :label="t('netbird.management')">
            <StatusBadge :type="st.management.connected ? 'running' : 'error'" :text="st.management.connected ? t('netbird.connected') : t('netbird.disconnected')" />
          </Field>
          <Field v-if="st.signal" :label="t('netbird.signal')">
            <StatusBadge :type="st.signal.connected ? 'running' : 'error'" :text="st.signal.connected ? t('netbird.connected') : t('netbird.disconnected')" />
          </Field>
          <Field v-if="st.relaysTotal" :label="t('netbird.relays')" mono>{{ st.relaysUp }} / {{ st.relaysTotal }}</Field>
          <Field :label="t('netbird.wireguard')">{{ st.self.kernel ? t('netbird.kernel') : t('netbird.userspaceImpl') }}</Field>
        </div>
        <div v-else class="help block">{{ t('netbird.notConnected') }}</div>
        <div v-if="connErrors(st).length" class="health">
          <el-tag v-for="e in connErrors(st)" :key="e" type="warning" size="small">{{ e }}</el-tag>
        </div>
        <div v-if="st.peers.length" class="peers">
          <div v-if="st.peers.some((p) => p.status === 'Idle')" class="form-help">{{ t('netbird.idleHint') }}</div>
          <div v-for="p in st.peers" :key="p.fqdn || p.name" class="peer">
            <StatusBadge v-bind="peerBadge(p)" />
            <div class="peer-main">
              <div class="peer-name">
                {{ p.name }}
                <el-tag v-if="p.connType" size="small" :type="p.connType === 'P2P' ? 'success' : 'info'">{{ p.connType === 'P2P' ? t('netbird.p2p') : t('netbird.relayed') }}</el-tag>
                <el-tag v-if="p.routes?.length" size="small" type="info">{{ t('netbird.routes') }}</el-tag>
              </div>
              <div class="peer-sub mono">{{ p.ips.join('  ') }}<template v-if="p.routes?.length"> · {{ p.routes.join(', ') }}</template></div>
            </div>
            <span v-if="p.latencyMs" class="peer-os mono">{{ p.latencyMs.toFixed(1) }} ms</span>
          </div>
        </div>
      </div>
    </div>

    <div class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Terminal :size="16" />{{ t('netbird.program') }}</div>
        <el-button v-if="!isReadonly" size="small" :icon="Save" :loading="savingSettings" @click="saveSettings">{{ t('common.save') }}</el-button>
      </div>
      <div class="g-card-body">
        <el-form label-width="130px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('netbird.netbirdPath')">
            <el-input v-model="program.netbirdPath" :placeholder="cliPath || t('netbird.autoFind')" :disabled="isReadonly" class="mono" />
            <div class="form-help">{{ t('netbird.programHelp') }}</div>
          </el-form-item>
        </el-form>
      </div>
    </div>

    <!-- 在线安装、更新与卸载都会修改程序，只读账号不显示 -->
    <ProgramInstall v-if="!isReadonly" :key="resetCount" base="/api/netbird" :active-paths="[cliPath]" @changed="load" />

    <LogCard url="/api/netbird/logs" :clearable="!isReadonly" />

    <!-- 实例编辑 -->
    <el-dialog v-model="editorVisible" :title="editing.key ? t('netbird.editInstance') : t('netbird.addInstance')" width="640px" :fullscreen="isMobile">
      <CopyHint :from="copyFrom" secrets />
      <el-form :model="editing" label-width="130px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" :placeholder="t('netbird.namePlaceholder')" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>
        <el-form-item :label="t('netbird.setupKey')">
          <SecretInput v-model="editing.setupKey" :placeholder="t('netbird.setupKeyPlaceholder')" :reveal="secretOf(secretKey && `/api/netbird/instances/${secretKey}/secrets`, 'setupKey')" />
          <div class="form-help">{{ t('netbird.setupKeyHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('netbird.managementUrl')">
          <el-input v-model="editing.managementUrl" :placeholder="defaultManagementUrl" class="mono" />
          <div class="form-help">{{ t('netbird.managementUrlHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('netbird.hostname')"><el-input v-model="editing.hostname" :placeholder="t('netbird.hostnamePlaceholder')" /></el-form-item>
        <el-form-item :label="t('netbird.acceptRoutes')"><el-switch v-model="editing.acceptRoutes" /><span class="help">{{ t('netbird.acceptRoutesHelp') }}</span></el-form-item>
        <el-form-item :label="t('netbird.serverRoutes')"><el-switch v-model="editing.serverRoutes" /><span class="help">{{ t('netbird.serverRoutesHelp') }}</span></el-form-item>
        <el-form-item :label="t('netbird.acceptDns')"><el-switch v-model="editing.acceptDns" /><span class="help">{{ t('netbird.acceptDnsHelp') }}</span></el-form-item>
        <el-form-item :label="t('netbird.blockInbound')"><el-switch v-model="editing.blockInbound" /><span class="help">{{ t('netbird.blockInboundHelp') }}</span></el-form-item>
        <el-form-item :label="t('netbird.extraArgs')">
          <el-input v-model="editing.extraUpArgs" :placeholder="t('netbird.extraArgsPlaceholder')" class="mono" />
        </el-form-item>

        <el-divider content-position="left">{{ t('netbird.runMode') }}</el-divider>
        <el-form-item :label="t('netbird.manageDaemon')">
          <el-switch v-model="editing.manageDaemon" />
          <span class="help">{{ t('netbird.manageDaemonHelp') }}</span>
        </el-form-item>
        <template v-if="editing.manageDaemon">
          <el-form-item :label="t('netbird.userspace')">
            <el-switch v-model="editing.userspace" />
            <span class="help">{{ t('netbird.userspaceHelp') }}</span>
            <div v-if="!editing.userspace" class="form-help">{{ t('netbird.kernelHelp') }}</div>
          </el-form-item>
          <el-form-item v-if="editing.userspace" :label="t('netbird.localForwarding')">
            <el-switch v-model="editing.localForwarding" />
            <div class="form-help">{{ t('netbird.localForwardingHelp') }}</div>
          </el-form-item>
          <el-form-item v-if="editing.userspace" :label="t('netbird.socks5')">
            <el-input v-model="editing.socks5" :placeholder="t('netbird.socks5Placeholder')" class="mono" />
            <div class="form-help">{{ t('netbird.socks5Help') }}</div>
            <!-- SOCKS5 代理没有认证，监听到本机以外时提醒 -->
            <el-alert v-if="listensBeyondLocal(editing.socks5)" class="socks-warn" type="warning" :closable="false" show-icon :title="t('netbird.socks5Public')" />
          </el-form-item>
          <el-form-item :label="t('netbird.wgPort')">
            <el-input-number v-model="editing.port" :min="0" :max="65535" controls-position="right" />
            <span class="help">{{ t('netbird.wgPortHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('netbird.stateDir')">
            <el-input v-model="editing.stateDir" :placeholder="editingDefaultStateDir || t('netbird.stateDirPlaceholder')" class="mono" />
            <div class="form-help">{{ t('netbird.stateDirHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('netbird.daemonAddr')">
            <el-input v-model="editing.daemonAddr" :placeholder="t('netbird.daemonAddrManagedPlaceholder')" class="mono" />
          </el-form-item>
          <el-form-item :label="t('netbird.daemonArgs')">
            <el-input v-model="editing.daemonExtraArgs" :placeholder="t('netbird.daemonArgsPlaceholder')" class="mono" />
          </el-form-item>
          <el-form-item :label="t('netbird.env')">
            <el-input v-model="editing.daemonEnv" type="textarea" :rows="3" :placeholder="t('netbird.envPlaceholder')" class="mono" />
            <div class="form-help">{{ t('netbird.envHelp') }}</div>
          </el-form-item>
        </template>
        <el-form-item v-else :label="t('netbird.daemonAddr')">
          <el-input v-model="editing.daemonAddr" :placeholder="t('netbird.daemonAddrPlaceholder')" class="mono" />
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Bird, CopyPlus, LogOut, Pencil, Plus, RefreshCw, Save, Terminal, Trash2, Waypoints } from 'lucide-vue-next'
import { api } from '../api'
import { isReadonly } from '../nav'
import SecretInput from '../components/SecretInput.vue'
import { secretOf } from '../secrets'
import CopyHint from '../components/CopyHint.vue'
import { createItem, uniqueName, type CopySource } from '../copyDraft'
import PageHeader from '../components/PageHeader.vue'
import EntityCard from '../components/EntityCard.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import { useGroupedList } from '../composables/useGroupSort'
import CompactToggle from '../components/CompactToggle.vue'
import { useCompactView } from '../composables/useCompactView'
import UrlLink from '../components/UrlLink.vue'
import Field from '../components/Field.vue'
import { listensBeyondLocal } from '../netaddr'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import ProgramInstall from '../components/ProgramInstall.vue'
import ResetConfigButton from '../components/ResetConfigButton.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'

// Instance 与后端实例字段一致；保存时只提交这些字段（后端拒绝未知字段）
interface Instance {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  enable: boolean
  manageDaemon: boolean
  stateDir: string
  daemonAddr: string
  userspace: boolean
  localForwarding: boolean
  socks5: string
  setupKey: string
  managementUrl: string
  hostname: string
  port: number
  acceptRoutes: boolean
  serverRoutes: boolean
  acceptDns: boolean
  blockInbound: boolean
  extraUpArgs: string
  daemonExtraArgs: string
  daemonEnv: string
  remark: string
  // legacyStateDir 表示实例迁移自旧版单实例配置，由服务端维护
  legacyStateDir?: boolean
}
// InstanceView 附带进程状态、启动错误（含冲突原因）与默认状态目录
type InstanceView = Instance & {
  key: string
  state: { running: boolean; error?: string }
  error: string
  defaultStateDir: string
}
interface Peer {
  name: string
  fqdn: string
  ips: string[]
  online: boolean
  // status 为 Idle、Connecting 或 Connected
  status: string
  connType?: string
  latencyMs?: number
  routes?: string[]
}
interface Conn {
  url: string
  connected: boolean
  error?: string
}
interface Status {
  key: string
  name: string
  enable: boolean
  daemon: { running: boolean }
  // cli 为实际使用的 netbird 程序路径
  cli?: string
  version: string
  error: string
  daemonStatus: string
  authUrl?: string
  authCode?: string
  management?: Conn
  signal?: Conn
  relaysTotal: number
  relaysUp: number
  self?: { name: string; fqdn: string; ips: string[]; kernel: boolean }
  peersConnected: number
  peers: Peer[]
}
interface SettingsView {
  settings: { netbirdPath: string }
  cli: string
  defaultManagementUrl: string
}

// 与后端冲突检查一致的 NetBird 默认值：WireGuard 端口 0 即 51820，用户态 SOCKS5 留空即 127.0.0.1:1080
const defaultWgPort = 51820
const defaultSocks5 = '127.0.0.1:1080'

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()

const instances = ref<InstanceView[]>([])
const statuses = ref<Status[]>([])
const program = reactive({ netbirdPath: '' })
const cliPath = ref('')
const defaultManagementUrl = ref('')
const savingSettings = ref(false)
const saving = ref(false)
// 定时刷新实例与状态，标签页在后台时暂停
const polling = usePolling(refresh, 5000)

const compact = useCompactView('netbird.instances')
const gl = useGroupedList<InstanceView>({
  storageKey: 'netbird.instances.groups',
  items: () => instances.value,
  base: '/api/netbird/instances',
  reload: load,
  matches: (v, q) => [v.name, v.group ?? '', v.remark, v.managementUrl, v.hostname].join('\n').toLowerCase().includes(q),
})

const enabledStatuses = computed(() => statuses.value.filter((s) => s.enable))
// anyStatus 用于在标题旁显示 netbird 版本，所有实例共用同一个程序
const anyStatus = computed(() => statuses.value.find((s) => s.version))

function statusOf(key: string): Status | undefined {
  return statuses.value.find((s) => s.key === key)
}

// modeText 返回实例的运行方式：系统已有的 NetBird 服务、内核 WireGuard 或用户态网络
function modeText(v: Instance): string {
  if (!v.manageDaemon) {
    return t('netbird.modeExternal')
  }
  return v.userspace ? t('netbird.modeUserspace') : t('netbird.modeKernel')
}

// badgeOf 把实例的进程与 NetBird 连接状态映射为状态徽标
function badgeOf(v: InstanceView): { type: StatusType; text: string; title?: string } {
  if (!v.enable) {
    return { type: 'off', text: t('netbird.notEnabled') }
  }
  const s = statusOf(v.key)
  const err = v.error || s?.error
  if (err) {
    return { type: 'error', text: t('netbird.error'), title: err }
  }
  switch (s?.daemonStatus) {
    case 'Connected':
      return { type: 'running', text: t('netbird.connected') }
    case 'NeedsLogin':
    case 'LoginFailed':
    case 'SessionExpired':
      return { type: 'warn', text: t('netbird.waitLogin') }
    case 'Idle':
      return { type: 'off', text: t('netbird.disconnected') }
    default:
      return { type: 'pending', text: t('netbird.connecting') }
  }
}

// peerBadge 显示节点连接状态：Idle 表示当前没有连接，开启按需连接时有流量会自动建立
function peerBadge(p: Peer): { type: StatusType; text: string } {
  if (p.online) {
    return { type: 'running', text: t('netbird.online') }
  }
  if (p.status === 'Connecting') {
    return { type: 'pending', text: t('netbird.connecting') }
  }
  return { type: 'off', text: t('netbird.idle') }
}

// connErrors 汇总实例的管理服务与信令服务连接错误
function connErrors(st: Status): string[] {
  return [st.management?.error, st.signal?.error].filter((e): e is string => !!e)
}

async function load() {
  const [list, st, settings] = await Promise.all([
    api.get<InstanceView[]>('/api/netbird/instances'),
    api.get<Status[]>('/api/netbird/status'),
    api.get<SettingsView>('/api/netbird/settings'),
  ])
  instances.value = list
  statuses.value = st
  program.netbirdPath = settings.settings.netbirdPath
  cliPath.value = settings.cli
  defaultManagementUrl.value = settings.defaultManagementUrl
}

// refresh 只刷新实例运行状态与连接状态，不影响正在编辑的表单
async function refresh() {
  try {
    const [list, st] = await Promise.all([api.get<InstanceView[]>('/api/netbird/instances'), api.get<Status[]>('/api/netbird/status')])
    instances.value = list
    statuses.value = st
  } catch {
    // 轮询失败时保留上一次的状态
  }
}

async function saveSettings() {
  savingSettings.value = true
  try {
    await api.put('/api/netbird/settings', { ...program })
    ElMessage.success(t('common.saved'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    savingSettings.value = false
  }
}

// ---- 实例编辑 ----

const editorVisible = ref(false)
const editing = ref<Instance>(emptyInstance())
// editingDefaultStateDir 为正在编辑的实例留空状态目录时使用的目录，新增时为空（key 由服务端分配）
const editingDefaultStateDir = ref('')
// 复制实例时的模板，普通新增与编辑时为 null；查看密钥明文时复制取自模板
const copyFrom = ref<CopySource | null>(null)
const secretKey = computed(() => editing.value.key || copyFrom.value?.key)

function emptyInstance(): Instance {
  return {
    name: '',
    group: '',
    enable: true,
    manageDaemon: true,
    stateDir: '',
    daemonAddr: '',
    userspace: true,
    localForwarding: true,
    socks5: '',
    setupKey: '',
    managementUrl: '',
    hostname: '',
    port: 0,
    acceptRoutes: false,
    serverRoutes: true,
    acceptDns: false,
    blockInbound: false,
    extraUpArgs: '',
    daemonExtraArgs: '',
    daemonEnv: '',
    remark: '',
  }
}

// pickInstance 只取实例字段，去掉运行状态
function pickInstance(v: InstanceView): Instance {
  const base = emptyInstance()
  const out = { key: v.key, legacyStateDir: v.legacyStateDir } as Instance
  for (const k of Object.keys(base) as (keyof Instance)[]) {
    ;(out as unknown as Record<string, unknown>)[k] = v[k] ?? base[k]
  }
  return out
}

// freePort 返回托管实例尚未使用的 WireGuard 端口：默认 51820 未被占用时返回 0（即默认），否则从 51821 起找第一个空闲端口
function freePort(): number {
  const used = new Set(instances.value.filter((x) => x.manageDaemon).map((x) => x.port || defaultWgPort))
  if (!used.has(defaultWgPort)) {
    return 0
  }
  let p = defaultWgPort + 1
  while (used.has(p)) {
    p++
  }
  return p
}

// freeSocks5 返回托管的用户态实例尚未使用的本机 SOCKS5 地址：127.0.0.1:1080 未被占用时返回空（即默认），否则从 1081 起顺延
function freeSocks5(): string {
  const used = new Set(instances.value.filter((x) => x.manageDaemon && x.userspace).map((x) => Number((x.socks5 || defaultSocks5).split(':').pop())))
  if (!used.has(1080)) {
    return ''
  }
  let p = 1081
  while (used.has(p)) {
    p++
  }
  return `127.0.0.1:${p}`
}

// openEditor 打开实例编辑框：copy 为 true 时以 v 为模板新建副本。状态目录、控制地址、WireGuard 端口与 SOCKS5 地址
// 不能被两个实例共用，新增与副本清空前两项使用自己的默认值，后两项预填一个空闲值；副本若为内核模式则先停用，避免与模板同时启用
function openEditor(v?: InstanceView, copy = false) {
  editing.value = v ? pickInstance(v) : emptyInstance()
  editingDefaultStateDir.value = v?.defaultStateDir ?? ''
  copyFrom.value = null
  if (v && copy) {
    copyFrom.value = { key: v.key, name: v.name }
    editing.value.key = ''
    editing.value.name = uniqueName(v.name, instances.value.map((x) => x.name))
    editing.value.stateDir = ''
    editing.value.daemonAddr = v.manageDaemon ? '' : v.daemonAddr
    editing.value.legacyStateDir = false
    if (editing.value.manageDaemon && !editing.value.userspace) {
      editing.value.enable = false
    }
    editingDefaultStateDir.value = ''
  }
  if (!v || copy) {
    editing.value.port = freePort()
    editing.value.socks5 = freeSocks5()
  }
  editorVisible.value = true
}

async function save() {
  const { key, ...body } = editing.value
  // 名称留空时用管理服务的主机名
  if (!body.name.trim()) {
    body.name = hostOf(body.managementUrl) || 'netbird'
  }
  saving.value = true
  try {
    if (key) {
      await api.put(`/api/netbird/instances/${key}`, body)
    } else {
      await createItem('/api/netbird/instances', body, copyFrom.value)
    }
    editorVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

// hostOf 返回地址中的主机名，无法解析时为空
function hostOf(url: string): string {
  try {
    return new URL(url).hostname
  } catch {
    return ''
  }
}

async function toggle(key: string, enable: boolean) {
  try {
    await api.put(`/api/netbird/instances/${key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function restart(key: string) {
  try {
    await api.post(`/api/netbird/instances/${key}/restart`)
    await refresh()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function logout(key: string) {
  try {
    await api.post(`/api/netbird/instances/${key}/logout`)
    ElMessage.success(t('netbird.loggedOut'))
    await refresh()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(key: string) {
  try {
    await api.del(`/api/netbird/instances/${key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// resetCount 在恢复默认配置后递增，让在线安装卡片重新读取下载代理
const resetCount = ref(0)

async function onReset() {
  resetCount.value++
  await load()
}

onMounted(() => {
  load().catch((e) => ElMessage.error((e as Error).message))
  polling.start()
})
</script>

<style scoped>
.block-gap {
  margin-bottom: 16px;
}
.auth-link {
  font-weight: 500;
}
.auth-code {
  margin-top: 6px;
}
.auth-code .mono {
  font-weight: 600;
  letter-spacing: 1px;
}
.svc-form {
  max-width: 640px;
}
.svc-form .el-form-item:last-child {
  margin-bottom: 0;
}
.head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  gap: 14px;
}
.health {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.peers {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 12px;
}
.peer {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid var(--g-border);
}
.peer:last-child {
  border-bottom: none;
}
.peer-main {
  flex: 1;
  min-width: 0;
}
.peer-name {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 500;
}
.peer-sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  overflow-wrap: anywhere;
}
.peer-os {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.socks-warn {
  margin-top: 6px;
}
</style>
