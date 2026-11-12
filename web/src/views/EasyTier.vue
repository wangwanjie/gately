<template>
  <div>
    <PageHeader title="EasyTier" :desc="t('easytier.desc')" :icon="Cable">
      <ResetConfigButton v-if="!isReadonly" base="/api/easytier" :desc="t('components.resetConfig.easytier')" @done="onReset" />
    </PageHeader>

    <div class="g-card">
      <div class="g-card-head g-sticky-bar">
        <div class="g-card-title"><Cable :size="16" />{{ t('easytier.instances') }} <span v-if="anyStatus?.version" class="g-card-desc">{{ anyStatus.version }}</span></div>
        <div class="head-actions">
          <!-- 排序与批量操作会修改配置，只读账号不显示 -->
          <ListTools v-if="!isReadonly" :gl="gl" size="small" />
          <CompactToggle v-model="compact" size="small" />
          <el-button v-if="!isReadonly" type="primary" size="small" :icon="Plus" @click="openEditor()">{{ t('easytier.addInstance') }}</el-button>
        </div>
      </div>
      <div class="g-card-body">
        <GroupedList :gl="gl" :compact="compact" grid :empty-text="t('easytier.noInstances')" :empty-image-size="60">
          <template #item="{ it, item: v, sorting, bind }">
            <EntityCard v-bind="bind" :off="!v.enable" :compact="compact || sorting">
              <template v-if="sorting" #lead>
                <SortCheck :gl="gl" :id="it.id" />
              </template>
              <template #title>{{ v.name }}</template>
              <template #sub><span class="mono">{{ v.networkName }}</span><template v-if="v.remark"> · {{ v.remark }}</template></template>
              <template #status><StatusBadge v-bind="badgeOf(v)" /></template>
              <Field :label="t('easytier.runMode')">{{ modeText(v) }}</Field>
              <Field :label="t('easytier.virtualIp')" mono>{{ ipText(v) }}</Field>
              <Field v-if="statusOf(v.key)?.self?.nat" :label="t('easytier.natType')">{{ statusOf(v.key)?.self?.nat }}</Field>
              <Field :label="t('easytier.listeners')" mono>{{ listenText(v) }}</Field>
              <template v-if="sorting" #actions>
                <SortMoves :gl="gl" :it="it" />
              </template>
              <template v-else-if="!isReadonly" #actions>
                <el-switch :model-value="v.enable" size="small" @change="(x: boolean | string | number) => toggle(v.key, Boolean(x))" />
                <el-button text size="small" :icon="RefreshCw" :disabled="!v.enable" @click="restart(v.key)">{{ t('common.restart') }}</el-button>
                <el-button text size="small" :icon="Pencil" @click="openEditor(v)">{{ t('common.edit') }}</el-button>
                <el-button text size="small" :icon="CopyPlus" @click="openEditor(v, true)">{{ t('common.duplicate') }}</el-button>
                <el-popconfirm :title="t('easytier.confirmDelete')" width="300" @confirm="remove(v.key)">
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

    <!-- 每个已启用实例的运行详情与节点列表 -->
    <div v-for="st in enabledStatuses" :key="st.key" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title">
          <Waypoints :size="16" />{{ st.name }}
          <span v-if="st.peers.length" class="g-card-desc">{{ t('easytier.peerCount', { direct: st.peersDirect, count: st.peers.length }) }}</span>
        </div>
      </div>
      <div class="g-card-body">
        <el-alert v-if="st.error" type="error" :title="st.error" :closable="false" show-icon class="block-gap" />
        <div v-if="st.self" class="info-grid">
          <Field :label="t('easytier.self')" mono>{{ st.self.name }}</Field>
          <Field :label="t('easytier.virtualIp')" mono>{{ st.self.ip || t('easytier.noIp') }}</Field>
          <Field v-if="st.self.nat" :label="t('easytier.natType')">{{ st.self.nat }}</Field>
          <Field v-if="st.self.publicIps.length" :label="t('easytier.publicIps')" mono>
            <div v-for="ip in st.self.publicIps" :key="ip">{{ ip }}</div>
          </Field>
          <Field v-if="st.self.listeners.length" :label="t('easytier.listeners')" mono>
            <div v-for="l in st.self.listeners" :key="l">{{ l }}</div>
          </Field>
        </div>
        <div v-else class="help block">{{ st.status === 'starting' ? t('easytier.starting') : t('easytier.notRunning') }}</div>
        <div v-if="st.status === 'running' && !st.peers.length" class="help block">{{ t('easytier.noPeers') }}</div>
        <div v-if="st.peers.length" class="peers">
          <div v-for="p in st.peers" :key="p.ip || p.name" class="peer">
            <StatusBadge v-bind="peerBadge(p)" />
            <div class="peer-main">
              <div class="peer-name">
                {{ p.name }}
                <el-tag v-if="p.tunnel" size="small" type="info">{{ p.tunnel }}</el-tag>
              </div>
              <div class="peer-sub mono">{{ p.ip }}<template v-if="p.nat"> · NAT {{ p.nat }}</template><template v-if="p.loss"> · {{ t('easytier.loss') }} {{ p.loss }}</template></div>
            </div>
            <div class="peer-os mono">
              <div v-if="p.latencyMs">{{ p.latencyMs.toFixed(1) }} ms</div>
              <div v-if="p.rx || p.tx">↓ {{ p.rx || '0' }} ↑ {{ p.tx || '0' }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Terminal :size="16" />{{ t('easytier.program') }}</div>
        <el-button v-if="!isReadonly" size="small" :icon="Save" :loading="savingSettings" @click="saveSettings">{{ t('common.save') }}</el-button>
      </div>
      <div class="g-card-body">
        <el-form label-width="130px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('easytier.corePath')">
            <el-input v-model="program.corePath" :placeholder="corePath || t('easytier.autoFind')" :disabled="isReadonly" class="mono" />
            <div class="form-help">{{ t('easytier.programHelp') }}</div>
          </el-form-item>
        </el-form>
      </div>
    </div>

    <!-- 在线安装、更新与卸载都会修改程序，只读账号不显示 -->
    <ProgramInstall v-if="!isReadonly" :key="resetCount" base="/api/easytier" :active-paths="[corePath]" @changed="load" />

    <LogCard url="/api/easytier/logs" :clearable="!isReadonly" />

    <!-- 实例编辑 -->
    <el-dialog v-model="editorVisible" :title="editing.key ? t('easytier.editInstance') : t('easytier.addInstance')" width="640px" :fullscreen="isMobile">
      <CopyHint :from="copyFrom" secrets />
      <el-form :model="editing" label-width="130px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" :placeholder="t('easytier.namePlaceholder')" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>
        <el-form-item :label="t('easytier.networkName')">
          <el-input v-model="editing.networkName" :placeholder="t('easytier.networkNamePlaceholder')" class="mono" />
        </el-form-item>
        <el-form-item :label="t('easytier.networkSecret')">
          <SecretInput v-model="editing.networkSecret" :placeholder="t('easytier.networkSecretPlaceholder')" :reveal="secretOf(secretKey && `/api/easytier/instances/${secretKey}/secrets`, 'networkSecret')" />
          <div class="form-help">{{ t('easytier.networkSecretHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('easytier.peers')">
          <el-input v-model="editing.peers" type="textarea" :rows="3" :placeholder="t('easytier.peersPlaceholder')" class="mono" />
          <div class="form-help">{{ t('easytier.peersHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('easytier.dhcp')">
          <el-switch v-model="editing.dhcp" />
          <span class="help">{{ t('easytier.dhcpHelp') }}</span>
        </el-form-item>
        <el-form-item v-if="!editing.dhcp" :label="t('easytier.ipv4')">
          <el-input v-model="editing.ipv4" :placeholder="t('easytier.ipv4Placeholder')" class="mono" />
          <div class="form-help">{{ t('easytier.ipv4Help') }}</div>
        </el-form-item>
        <el-form-item :label="t('easytier.hostname')"><el-input v-model="editing.hostname" :placeholder="t('easytier.hostnamePlaceholder')" /></el-form-item>
        <el-form-item :label="t('easytier.listeners')">
          <el-input v-model="editing.listeners" type="textarea" :rows="3" :placeholder="t('easytier.listenersPlaceholder')" class="mono" />
          <div class="form-help">{{ t('easytier.listenersHelp') }}</div>
        </el-form-item>

        <el-divider content-position="left">{{ t('easytier.runMode') }}</el-divider>
        <el-form-item :label="t('easytier.noTun')">
          <el-switch v-model="editing.noTun" />
          <span class="help">{{ t('easytier.noTunHelp') }}</span>
        </el-form-item>
        <el-form-item v-if="!editing.noTun" :label="t('easytier.devName')">
          <el-input v-model="editing.devName" :placeholder="t('easytier.devNamePlaceholder')" class="mono" />
        </el-form-item>
        <el-form-item :label="t('easytier.portForwards')">
          <el-input v-model="editing.portForwards" type="textarea" :rows="3" :placeholder="t('easytier.portForwardsPlaceholder')" class="mono" />
          <div class="form-help">{{ t('easytier.portForwardsHelp') }}</div>
          <!-- 端口转发没有认证，绑定到本机以外时提醒 -->
          <el-alert v-if="forwardsBeyondLocal(editing.portForwards)" class="socks-warn" type="warning" :closable="false" show-icon :title="t('easytier.portForwardsPublic')" />
        </el-form-item>
        <el-form-item :label="t('easytier.proxyNetworks')">
          <el-input v-model="editing.proxyNetworks" type="textarea" :rows="2" :placeholder="t('easytier.proxyNetworksPlaceholder')" class="mono" />
          <div class="form-help">{{ t('easytier.proxyNetworksHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('easytier.enableExitNode')">
          <el-switch v-model="editing.enableExitNode" />
          <span class="help">{{ t('easytier.enableExitNodeHelp') }}</span>
        </el-form-item>
        <el-form-item :label="t('easytier.exitNodes')">
          <el-input v-model="editing.exitNodes" type="textarea" :rows="2" :placeholder="t('easytier.exitNodesPlaceholder')" class="mono" />
          <div class="form-help">{{ t('easytier.exitNodesHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('easytier.latencyFirst')">
          <el-switch v-model="editing.latencyFirst" />
          <span class="help">{{ t('easytier.latencyFirstHelp') }}</span>
        </el-form-item>
        <el-form-item :label="t('easytier.kcpProxy')">
          <el-switch v-model="editing.kcpProxy" />
          <span class="help">{{ t('easytier.kcpProxyHelp') }}</span>
        </el-form-item>
        <el-form-item :label="t('easytier.quicProxy')">
          <el-switch v-model="editing.quicProxy" />
          <span class="help">{{ t('easytier.quicProxyHelp') }}</span>
        </el-form-item>
        <el-form-item :label="t('easytier.extraArgs')">
          <el-input v-model="editing.extraArgs" :placeholder="t('easytier.extraArgsPlaceholder')" class="mono" />
        </el-form-item>
        <el-form-item :label="t('easytier.env')">
          <el-input v-model="editing.env" type="textarea" :rows="3" :placeholder="t('easytier.envPlaceholder')" class="mono" />
          <div class="form-help">{{ t('easytier.envHelp') }}</div>
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
import { Cable, CopyPlus, Pencil, Plus, RefreshCw, Save, Terminal, Trash2, Waypoints } from 'lucide-vue-next'
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
import Field from '../components/Field.vue'
import { forwardsBeyondLocal } from '../netaddr'
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
  networkName: string
  networkSecret: string
  peers: string
  listeners: string
  dhcp: boolean
  ipv4: string
  hostname: string
  noTun: boolean
  devName: string
  proxyNetworks: string
  portForwards: string
  enableExitNode: boolean
  exitNodes: string
  latencyFirst: boolean
  kcpProxy: boolean
  quicProxy: boolean
  extraArgs: string
  env: string
  remark: string
}
// InstanceView 附带进程状态与启动错误（含冲突原因）
type InstanceView = Instance & {
  key: string
  state: { running: boolean; error?: string }
  error: string
}
interface Peer {
  name: string
  ip: string
  // cost 为 p2p 或 relay(N)
  cost: string
  direct: boolean
  latencyMs?: number
  loss?: string
  rx?: string
  tx?: string
  tunnel?: string
  nat?: string
}
interface Status {
  key: string
  name: string
  enable: boolean
  daemon: { running: boolean }
  version: string
  error: string
  // status 为 stopped、starting 或 running
  status: string
  self?: { name: string; ip: string; listeners: string[]; publicIps: string[]; nat?: string }
  peersDirect: number
  peers: Peer[]
}
interface SettingsView {
  settings: { corePath: string }
  core: string
}

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()

const instances = ref<InstanceView[]>([])
const statuses = ref<Status[]>([])
const program = reactive({ corePath: '' })
const corePath = ref('')
const savingSettings = ref(false)
const saving = ref(false)
// 定时刷新实例与状态，标签页在后台时暂停
const polling = usePolling(refresh, 5000)

const compact = useCompactView('easytier.instances')
const gl = useGroupedList<InstanceView>({
  storageKey: 'easytier.instances.groups',
  items: () => instances.value,
  base: '/api/easytier/instances',
  reload: load,
  matches: (v, q) => [v.name, v.group ?? '', v.remark, v.networkName, v.hostname].join('\n').toLowerCase().includes(q),
})

const enabledStatuses = computed(() => statuses.value.filter((s) => s.enable))
// anyStatus 用于在标题旁显示 easytier-core 版本，所有实例共用同一个程序
const anyStatus = computed(() => statuses.value.find((s) => s.version))

function statusOf(key: string): Status | undefined {
  return statuses.value.find((s) => s.key === key)
}

// modeText 返回实例的运行方式：TUN 网卡或用户态网络
function modeText(v: Instance): string {
  return v.noTun ? t('easytier.modeUserspace') : t('easytier.modeTun')
}

// ipText 返回实例的虚拟 IP：已运行时取实际值，否则按配置显示 DHCP、固定地址或仅转发
function ipText(v: InstanceView): string {
  const ip = statusOf(v.key)?.self?.ip
  if (ip) {
    return ip
  }
  if (v.dhcp) {
    return 'DHCP'
  }
  return v.ipv4 || t('easytier.noIp')
}

// listenText 汇总监听地址：未配置时只主动连接其他节点
function listenText(v: Instance): string {
  const list = v.listeners.split('\n').filter((l) => l.trim())
  if (!list.length) {
    return t('easytier.noListen')
  }
  return list.length > 1 ? `${list[0]} (+${list.length - 1})` : list[0]
}

// badgeOf 把实例的进程与运行状态映射为状态徽标
function badgeOf(v: InstanceView): { type: StatusType; text: string; title?: string } {
  if (!v.enable) {
    return { type: 'off', text: t('easytier.notEnabled') }
  }
  const s = statusOf(v.key)
  const err = v.error || s?.error
  if (err) {
    return { type: 'error', text: t('easytier.error'), title: err }
  }
  switch (s?.status) {
    case 'running':
      return { type: 'running', text: t('easytier.running') }
    case 'stopped':
      return { type: 'off', text: t('easytier.stopped') }
    default:
      return { type: 'pending', text: t('easytier.starting') }
  }
}

// peerBadge 显示节点的连接方式：直连为绿色，中继为蓝色
function peerBadge(p: Peer): { type: StatusType; text: string } {
  return p.direct ? { type: 'running', text: t('easytier.p2p') } : { type: 'pending', text: p.cost }
}

async function load() {
  const [list, st, settings] = await Promise.all([
    api.get<InstanceView[]>('/api/easytier/instances'),
    api.get<Status[]>('/api/easytier/status'),
    api.get<SettingsView>('/api/easytier/settings'),
  ])
  instances.value = list
  statuses.value = st
  program.corePath = settings.settings.corePath
  corePath.value = settings.core
}

// refresh 只刷新实例运行状态与节点状态，不影响正在编辑的表单
async function refresh() {
  try {
    const [list, st] = await Promise.all([api.get<InstanceView[]>('/api/easytier/instances'), api.get<Status[]>('/api/easytier/status')])
    instances.value = list
    statuses.value = st
  } catch {
    // 轮询失败时保留上一次的状态
  }
}

async function saveSettings() {
  savingSettings.value = true
  try {
    await api.put('/api/easytier/settings', { ...program })
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
// 复制实例时的模板，普通新增与编辑时为 null；查看密钥明文时复制取自模板
const copyFrom = ref<CopySource | null>(null)
const secretKey = computed(() => editing.value.key || copyFrom.value?.key)

function emptyInstance(): Instance {
  return {
    name: '',
    group: '',
    enable: true,
    networkName: '',
    networkSecret: '',
    peers: '',
    listeners: '',
    dhcp: true,
    ipv4: '',
    hostname: '',
    noTun: false,
    devName: '',
    proxyNetworks: '',
    portForwards: '',
    enableExitNode: false,
    exitNodes: '',
    latencyFirst: false,
    kcpProxy: false,
    quicProxy: false,
    extraArgs: '',
    env: '',
    remark: '',
  }
}

// pickInstance 只取实例字段，去掉运行状态
function pickInstance(v: InstanceView): Instance {
  const base = emptyInstance()
  const out = { key: v.key } as Instance
  for (const k of Object.keys(base) as (keyof Instance)[]) {
    ;(out as unknown as Record<string, unknown>)[k] = v[k] ?? base[k]
  }
  return out
}

// openEditor 打开实例编辑框：copy 为 true 时以 v 为模板新建副本。
// 监听端口、端口转发的本机端口与网卡名不能被两个同时启用的实例共用，副本先停用，由用户改完再启用
function openEditor(v?: InstanceView, copy = false) {
  editing.value = v ? pickInstance(v) : emptyInstance()
  copyFrom.value = null
  if (v && copy) {
    copyFrom.value = { key: v.key, name: v.name }
    editing.value.key = ''
    editing.value.name = uniqueName(v.name, instances.value.map((x) => x.name))
    editing.value.enable = false
  }
  editorVisible.value = true
}

async function save() {
  const { key, ...body } = editing.value
  // 名称留空时用组网名称
  if (!body.name.trim()) {
    body.name = body.networkName.trim() || 'easytier'
  }
  saving.value = true
  try {
    if (key) {
      await api.put(`/api/easytier/instances/${key}`, body)
    } else {
      await createItem('/api/easytier/instances', body, copyFrom.value)
    }
    editorVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function toggle(key: string, enable: boolean) {
  try {
    await api.put(`/api/easytier/instances/${key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function restart(key: string) {
  try {
    await api.post(`/api/easytier/instances/${key}/restart`)
    await refresh()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(key: string) {
  try {
    await api.del(`/api/easytier/instances/${key}`)
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
  text-align: right;
  white-space: nowrap;
}
.socks-warn {
  margin-top: 6px;
}
/* 窄屏时延迟与流量单独占一行，避免挤压节点地址 */
@media (max-width: 640px) {
  .peer {
    flex-wrap: wrap;
    row-gap: 4px;
  }
  .peer-main {
    flex: 1 1 0;
  }
  .peer-os {
    flex-basis: 100%;
    display: flex;
    gap: 14px;
    text-align: left;
    padding-left: 2px;
  }
}
</style>
