<template>
  <div>
    <PageHeader title="Tailscale" :desc="t('tailscale.desc')" :icon="Network">
      <ResetConfigButton base="/api/tailscale" :desc="t('components.resetConfig.tailscale')" @done="onReset" />
    </PageHeader>

    <!-- 每个等待登录的实例单独提示；登录链接只返回给有写权限的身份 -->
    <template v-for="st in statuses" :key="st.key">
      <el-alert v-if="st.authUrl" type="warning" :closable="false" show-icon class="block-gap" :title="t('tailscale.needLoginOf', { name: st.name })">
        <!-- 复制按钮复制登录地址本身，便于在其他设备上打开完成登录 -->
        <UrlLink :href="st.authUrl" :text="t('tailscale.authLink')" class="auth-link" />
      </el-alert>
    </template>

    <div class="g-card">
      <div class="g-card-head g-sticky-bar">
        <div class="g-card-title"><Network :size="16" />{{ t('tailscale.instances') }} <span v-if="anyStatus?.version" class="g-card-desc">{{ anyStatus.version }}</span></div>
        <div class="head-actions">
          <ListTools :gl="gl" size="small" />
          <CompactToggle v-model="compact" size="small" />
          <el-button type="primary" size="small" :icon="Plus" @click="openEditor()">{{ t('tailscale.addInstance') }}</el-button>
        </div>
      </div>
      <div class="g-card-body">
        <GroupedList :gl="gl" :compact="compact" grid :empty-text="t('tailscale.noInstances')" :empty-image-size="60">
          <template #item="{ it, item: v, sorting, bind }">
            <EntityCard v-bind="bind" :off="!v.enable" :compact="compact || sorting">
              <template v-if="sorting" #lead>
                <SortCheck :gl="gl" :id="it.id" />
              </template>
              <template #title>{{ v.name }}</template>
              <template #sub><span class="mono">{{ v.loginServer || 'Tailscale' }}</span><template v-if="v.remark"> · {{ v.remark }}</template></template>
              <template #status><StatusBadge v-bind="badgeOf(v)" /></template>
              <Field :label="t('tailscale.runMode')">{{ modeText(v) }}</Field>
              <Field v-if="statusOf(v.key)?.self" label="Tailscale IP" mono>
                <div v-for="ip in statusOf(v.key)?.self?.ips" :key="ip">{{ ip }}</div>
              </Field>
              <Field v-if="statusOf(v.key)?.tailnet" label="Tailnet" mono>{{ statusOf(v.key)?.tailnet }}</Field>
              <Field v-if="v.advertiseRoutes?.length" :label="t('tailscale.subnetRoutes')" mono>{{ v.advertiseRoutes.join(', ') }}</Field>
              <Field v-if="v.manageDaemon && v.port" :label="t('tailscale.wgPort')" mono>{{ v.port }}</Field>
              <Field v-if="v.manageDaemon" :label="t('tailscale.stateDir')" mono>{{ v.stateDir || v.defaultStateDir }}</Field>
              <template v-if="sorting" #actions>
                <SortMoves :gl="gl" :it="it" />
              </template>
              <template v-else #actions>
                <el-switch :model-value="v.enable" size="small" @change="(x: boolean | string | number) => toggle(v.key, Boolean(x))" />
                <el-button text size="small" :icon="RefreshCw" :disabled="!v.enable" @click="restart(v.key)">{{ t('common.restart') }}</el-button>
                <el-button text size="small" :icon="Pencil" @click="openEditor(v)">{{ t('common.edit') }}</el-button>
                <el-button text size="small" :icon="CopyPlus" @click="openEditor(v, true)">{{ t('common.duplicate') }}</el-button>
                <el-popconfirm :title="t('tailscale.confirmDelete')" width="300" @confirm="remove(v.key)">
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
          <span v-if="st.peers.length" class="g-card-desc">{{ t('tailscale.peerCount', { count: st.peers.length }) }}</span>
        </div>
        <el-popconfirm v-if="st.backendState === 'Running'" :title="t('tailscale.confirmLogout')" width="280" @confirm="logout(st.key)">
          <template #reference>
            <el-button size="small" :icon="LogOut">{{ t('tailscale.logout') }}</el-button>
          </template>
        </el-popconfirm>
      </div>
      <div class="g-card-body">
        <el-alert v-if="st.error" type="error" :title="st.error" :closable="false" show-icon class="block-gap" />
        <div v-if="st.self" class="info-grid">
          <Field :label="t('tailscale.self')" mono>{{ st.self.name }}</Field>
          <Field label="Tailscale IP" mono>
            <div v-for="ip in st.self.ips" :key="ip">{{ ip }}</div>
          </Field>
          <Field v-if="st.tailnet" label="Tailnet" mono>{{ st.tailnet }}</Field>
          <Field v-if="st.magicDnsSuffix" label="MagicDNS" mono>{{ st.magicDnsSuffix }}</Field>
        </div>
        <div v-else class="help block">{{ t('tailscale.notConnected') }}</div>
        <div v-if="st.health?.length" class="health">
          <el-tag v-for="h in st.health" :key="h" type="warning" size="small">{{ h }}</el-tag>
        </div>
        <div v-if="st.peers.length" class="peers">
          <div v-for="p in st.peers" :key="p.dnsName || p.name" class="peer">
            <StatusBadge :type="p.online ? 'running' : 'off'" :text="p.online ? t('tailscale.online') : t('tailscale.offline')" />
            <div class="peer-main">
              <div class="peer-name">
                {{ p.name }}
                <el-tag v-if="p.exitNodeOption" size="small" type="info">{{ t('tailscale.exitNode') }}</el-tag>
                <el-tag v-if="p.routes?.length" size="small" type="info">{{ t('tailscale.subnetRoutes') }}</el-tag>
              </div>
              <div class="peer-sub mono">{{ p.ips.join('  ') }}<template v-if="p.routes?.length"> · {{ p.routes.join(', ') }}</template></div>
            </div>
            <span class="peer-os">{{ p.os }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Terminal :size="16" />{{ t('tailscale.program') }}</div>
        <el-button size="small" :icon="Save" :loading="savingSettings" @click="saveSettings">{{ t('common.save') }}</el-button>
      </div>
      <div class="g-card-body">
        <el-form label-width="130px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('tailscale.tailscaledPath')"><el-input v-model="program.tailscaledPath" :placeholder="t('tailscale.autoFind')" class="mono" /></el-form-item>
          <el-form-item :label="t('tailscale.tailscalePath')">
            <el-input v-model="program.tailscalePath" :placeholder="cliPath || t('tailscale.autoFind')" class="mono" />
            <div class="form-help">{{ t('tailscale.programHelp') }}</div>
          </el-form-item>
        </el-form>
      </div>
    </div>

    <ProgramInstall :key="resetCount" base="/api/tailscale" :active-paths="[cliPath]" @changed="load" />

    <LogCard url="/api/tailscale/logs" clearable />

    <!-- 实例编辑 -->
    <el-dialog v-model="editorVisible" :title="editing.key ? t('tailscale.editInstance') : t('tailscale.addInstance')" width="640px" :fullscreen="isMobile">
      <CopyHint :from="copyFrom" secrets />
      <el-form :model="editing" label-width="130px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" :placeholder="t('tailscale.namePlaceholder')" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>
        <el-form-item :label="t('tailscale.authKey')">
          <SecretInput v-model="editing.authKey" :placeholder="t('tailscale.authKeyPlaceholder')" :reveal="secretOf(secretKey && `/api/tailscale/instances/${secretKey}/secrets`, 'authKey')" />
        </el-form-item>
        <el-form-item :label="t('tailscale.hostname')"><el-input v-model="editing.hostname" :placeholder="t('tailscale.hostnamePlaceholder')" /></el-form-item>
        <el-form-item :label="t('tailscale.loginServer')">
          <el-input v-model="editing.loginServer" :placeholder="t('tailscale.loginServerPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('tailscale.subnetRoutes')">
          <el-input v-model="routesText" :placeholder="t('tailscale.routesPlaceholder')" class="mono" />
          <div class="form-help">{{ t('tailscale.routesHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('tailscale.exitNode')"><el-switch v-model="editing.advertiseExitNode" /><span class="help">{{ t('tailscale.exitNodeHelp') }}</span></el-form-item>
        <el-form-item :label="t('tailscale.acceptRoutes')"><el-switch v-model="editing.acceptRoutes" /><span class="help">{{ t('tailscale.acceptRoutesHelp') }}</span></el-form-item>
        <el-form-item :label="t('tailscale.acceptDns')"><el-switch v-model="editing.acceptDns" /><span class="help">{{ t('tailscale.acceptDnsHelp') }}</span></el-form-item>
        <el-form-item :label="t('tailscale.extraArgs')">
          <el-input v-model="editing.extraUpArgs" :placeholder="t('tailscale.extraArgsPlaceholder')" class="mono" />
        </el-form-item>

        <el-divider content-position="left">{{ t('tailscale.runMode') }}</el-divider>
        <el-form-item :label="t('tailscale.manageDaemon')">
          <el-switch v-model="editing.manageDaemon" />
          <span class="help">{{ t('tailscale.manageDaemonHelp') }}</span>
        </el-form-item>
        <template v-if="editing.manageDaemon">
          <el-form-item :label="t('tailscale.userspace')">
            <el-switch v-model="editing.userspace" />
            <span class="help">{{ t('tailscale.userspaceHelp') }}</span>
            <div v-if="!editing.userspace" class="form-help">{{ t('tailscale.kernelHelp') }}</div>
          </el-form-item>
          <el-form-item v-if="editing.userspace" :label="t('tailscale.socks5')">
            <el-input v-model="editing.socks5" :placeholder="t('tailscale.socks5Placeholder')" class="mono" />
            <div class="form-help">{{ t('tailscale.socks5Help') }}</div>
            <!-- SOCKS5 代理没有认证，监听到本机以外时提醒 -->
            <el-alert v-if="listensBeyondLocal(editing.socks5)" class="socks-warn" type="warning" :closable="false" show-icon :title="t('tailscale.socks5Public')" />
          </el-form-item>
          <el-form-item :label="t('tailscale.wgPort')">
            <el-input-number v-model="editing.port" :min="0" :max="65535" controls-position="right" />
            <span class="help">{{ t('tailscale.wgPortHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('tailscale.stateDir')">
            <el-input v-model="editing.stateDir" :placeholder="editingDefaultStateDir || t('tailscale.stateDirPlaceholder')" class="mono" />
            <div class="form-help">{{ t('tailscale.stateDirHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('tailscale.socket')">
            <el-input v-model="editing.socket" :placeholder="t('tailscale.socketManagedPlaceholder')" class="mono" />
          </el-form-item>
          <el-form-item :label="t('tailscale.daemonArgs')">
            <el-input v-model="editing.daemonExtraArgs" :placeholder="t('tailscale.daemonArgsPlaceholder')" class="mono" />
          </el-form-item>
          <el-form-item :label="t('tailscale.env')">
            <el-input v-model="editing.daemonEnv" type="textarea" :rows="3" :placeholder="t('tailscale.envPlaceholder')" class="mono" />
            <div class="form-help">{{ t('tailscale.envHelp') }}</div>
          </el-form-item>
        </template>
        <el-form-item v-else :label="t('tailscale.socket')">
          <el-input v-model="editing.socket" :placeholder="t('tailscale.socketPlaceholder')" class="mono" />
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
import { CopyPlus, LogOut, Network, Pencil, Plus, RefreshCw, Save, Terminal, Trash2, Waypoints } from 'lucide-vue-next'
import { api } from '../api'
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
import { useListText, splitCommasOrSpaces } from '../composables/useListText'
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
  socket: string
  userspace: boolean
  socks5: string
  authKey: string
  hostname: string
  loginServer: string
  advertiseRoutes: string[] | null
  advertiseExitNode: boolean
  acceptRoutes: boolean
  acceptDns: boolean
  extraUpArgs: string
  port: number
  daemonExtraArgs: string
  daemonEnv: string
  remark: string
  // legacyStateDir 表示实例迁移自旧版单实例配置，由服务端维护
  legacyStateDir?: boolean
}
// InstanceView 附带进程状态、启动错误与默认状态目录
type InstanceView = Instance & {
  key: string
  state: { running: boolean; error?: string }
  error: string
  defaultStateDir: string
}
interface Peer {
  name: string
  dnsName: string
  ips: string[]
  os: string
  online: boolean
  exitNodeOption: boolean
  routes?: string[]
}
interface Status {
  key: string
  name: string
  enable: boolean
  daemon: { running: boolean }
  // cli 为实际使用的 tailscale 命令路径
  cli?: string
  version: string
  error: string
  backendState: string
  authUrl?: string
  tailnet?: string
  magicDnsSuffix?: string
  health?: string[]
  self?: Peer
  peers: Peer[]
}
interface SettingsView {
  settings: { tailscaledPath: string; tailscalePath: string }
  cli: string
}

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()

const instances = ref<InstanceView[]>([])
const statuses = ref<Status[]>([])
const program = reactive({ tailscaledPath: '', tailscalePath: '' })
const cliPath = ref('')
const savingSettings = ref(false)
const saving = ref(false)
// 定时刷新实例与状态，标签页在后台时暂停
const polling = usePolling(refresh, 5000)

const compact = useCompactView('tailscale.instances')
const gl = useGroupedList<InstanceView>({
  storageKey: 'tailscale.instances.groups',
  items: () => instances.value,
  base: '/api/tailscale/instances',
  reload: load,
  matches: (v, q) => [v.name, v.group ?? '', v.remark, v.loginServer, v.hostname, ...(v.advertiseRoutes ?? [])].join('\n').toLowerCase().includes(q),
})

const enabledStatuses = computed(() => statuses.value.filter((s) => s.enable))
// anyStatus 用于在标题旁显示 tailscale 版本，所有实例共用同一个命令
const anyStatus = computed(() => statuses.value.find((s) => s.version))

function statusOf(key: string): Status | undefined {
  return statuses.value.find((s) => s.key === key)
}

// modeText 返回实例的运行方式：外部守护进程、内核 TUN 或用户态网络
function modeText(v: Instance): string {
  if (!v.manageDaemon) {
    return t('tailscale.modeExternal')
  }
  return v.userspace ? t('tailscale.modeUserspace') : t('tailscale.modeKernel')
}

// badgeOf 把实例的进程与 tailscale 后端状态映射为状态徽标
function badgeOf(v: InstanceView): { type: StatusType; text: string; title?: string } {
  if (!v.enable) {
    return { type: 'off', text: t('tailscale.notEnabled') }
  }
  const s = statusOf(v.key)
  const err = v.error || s?.error
  if (err) {
    return { type: 'error', text: t('tailscale.error'), title: err }
  }
  switch (s?.backendState) {
    case 'Running':
      return { type: 'running', text: t('tailscale.connected') }
    case 'NeedsLogin':
    case 'NeedsMachineAuth':
      return { type: 'warn', text: t('tailscale.waitLogin') }
    case 'Stopped':
      return { type: 'off', text: t('common.stopped') }
    default:
      return { type: 'pending', text: t('tailscale.connecting') }
  }
}

async function load() {
  const [list, st, settings] = await Promise.all([
    api.get<InstanceView[]>('/api/tailscale/instances'),
    api.get<Status[]>('/api/tailscale/status'),
    api.get<SettingsView>('/api/tailscale/settings'),
  ])
  instances.value = list
  statuses.value = st
  program.tailscaledPath = settings.settings.tailscaledPath
  program.tailscalePath = settings.settings.tailscalePath
  cliPath.value = settings.cli
}

// refresh 只刷新实例运行状态与连接状态，不影响正在编辑的表单
async function refresh() {
  try {
    const [list, st] = await Promise.all([api.get<InstanceView[]>('/api/tailscale/instances'), api.get<Status[]>('/api/tailscale/status')])
    instances.value = list
    statuses.value = st
  } catch {
    // 轮询失败时保留上一次的状态
  }
}

async function saveSettings() {
  savingSettings.value = true
  try {
    await api.put('/api/tailscale/settings', { ...program })
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

// 输入框保留原始文本，逗号、空格可正常输入（见 useListText）
const routesText = useListText(() => editing.value.advertiseRoutes, (v) => (editing.value.advertiseRoutes = v), {
  split: splitCommasOrSpaces,
  separator: ', ',
})

function emptyInstance(): Instance {
  return {
    name: '',
    group: '',
    enable: true,
    manageDaemon: true,
    stateDir: '',
    socket: '',
    userspace: true,
    socks5: '',
    authKey: '',
    hostname: '',
    loginServer: '',
    advertiseRoutes: [],
    advertiseExitNode: false,
    acceptRoutes: false,
    acceptDns: false,
    extraUpArgs: '',
    port: 0,
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
  out.advertiseRoutes = [...(v.advertiseRoutes ?? [])]
  return out
}

// openEditor 打开实例编辑框：copy 为 true 时以 v 为模板新建副本。状态目录、控制套接字与端口不能被两个实例共用，
// 副本清空这几项，使用自己的默认值；副本若为内核模式则先停用，避免与模板同时启用
function openEditor(v?: InstanceView, copy = false) {
  editing.value = v ? pickInstance(v) : emptyInstance()
  editingDefaultStateDir.value = v?.defaultStateDir ?? ''
  copyFrom.value = null
  if (v && copy) {
    copyFrom.value = { key: v.key, name: v.name }
    editing.value.key = ''
    editing.value.name = uniqueName(v.name, instances.value.map((x) => x.name))
    editing.value.stateDir = ''
    editing.value.socket = ''
    editing.value.port = 0
    editing.value.legacyStateDir = false
    if (editing.value.manageDaemon && !editing.value.userspace) {
      editing.value.enable = false
    }
    editingDefaultStateDir.value = ''
  }
  editorVisible.value = true
}

async function save() {
  const { key, ...body } = editing.value
  // 名称留空时用控制服务器的主机名
  if (!body.name.trim()) {
    body.name = hostOf(body.loginServer) || 'tailscale'
  }
  saving.value = true
  try {
    if (key) {
      await api.put(`/api/tailscale/instances/${key}`, body)
    } else {
      await createItem('/api/tailscale/instances', body, copyFrom.value)
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
    await api.put(`/api/tailscale/instances/${key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function restart(key: string) {
  try {
    await api.post(`/api/tailscale/instances/${key}/restart`)
    await refresh()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function logout(key: string) {
  try {
    await api.post(`/api/tailscale/instances/${key}/logout`)
    ElMessage.success(t('tailscale.loggedOut'))
    await refresh()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(key: string) {
  try {
    await api.del(`/api/tailscale/instances/${key}`)
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
