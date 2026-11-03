<template>
  <div>
    <PageHeader title="NetBird" :desc="t('netbird.desc')" :icon="Bird">
      <ResetConfigButton base="/api/netbird" :desc="t('components.resetConfig.netbird')" @done="onReset" />
      <el-button :icon="RefreshCw" :loading="restarting" @click="restart">{{ t('common.restart') }}</el-button>
      <el-button type="primary" :icon="Save" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
    </PageHeader>

    <el-alert v-if="status?.error" type="error" :title="status.error" :closable="false" show-icon class="block-gap" />
    <el-alert v-if="status?.authUrl" type="warning" :closable="false" show-icon class="block-gap" :title="t('netbird.needLogin')">
      <!-- 复制按钮复制登录地址本身，便于在其他设备上打开完成登录 -->
      <UrlLink :href="status.authUrl" :text="t('netbird.authLink')" class="auth-link" />
      <div v-if="status.authCode" class="auth-code">{{ t('netbird.authCode') }} <span class="mono">{{ status.authCode }}</span></div>
      <div class="form-help">{{ t('netbird.authHelp') }}</div>
    </el-alert>

    <div class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Activity :size="16" />{{ t('common.status') }}</div>
        <StatusBadge v-bind="badge" />
      </div>
      <div class="g-card-body">
        <div v-if="status?.self" class="info-grid">
          <Field :label="t('netbird.self')" mono>{{ status.self.name }}</Field>
          <Field label="NetBird IP" mono>
            <div v-for="ip in status.self.ips" :key="ip">{{ ip }}</div>
          </Field>
          <Field v-if="status.self.fqdn" label="FQDN" mono>{{ status.self.fqdn }}</Field>
          <Field v-if="status.management" :label="t('netbird.management')">
            <StatusBadge :type="status.management.connected ? 'running' : 'error'" :text="status.management.connected ? t('netbird.connected') : t('netbird.disconnected')" />
          </Field>
          <Field v-if="status.signal" :label="t('netbird.signal')">
            <StatusBadge :type="status.signal.connected ? 'running' : 'error'" :text="status.signal.connected ? t('netbird.connected') : t('netbird.disconnected')" />
          </Field>
          <Field v-if="status.relaysTotal" :label="t('netbird.relays')" mono>{{ status.relaysUp }} / {{ status.relaysTotal }}</Field>
          <Field :label="t('netbird.wireguard')">{{ status.self.kernel ? t('netbird.kernel') : t('netbird.userspaceImpl') }}</Field>
          <Field v-if="status.version" :label="t('netbird.version')" mono>{{ status.version }}</Field>
        </div>
        <div v-else class="help block">
          {{ settings.enable ? t('netbird.notConnected') : t('netbird.notEnabledHint') }}
        </div>
        <div v-if="connErrors.length" class="health">
          <el-tag v-for="e in connErrors" :key="e" type="warning" size="small">{{ e }}</el-tag>
        </div>
      </div>
    </div>

    <div v-if="status?.peers?.length" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title">
          <Waypoints :size="16" />{{ t('netbird.peers') }}
          <span class="g-card-desc">{{ t('netbird.peerCount', { connected: status.peersConnected, count: status.peers.length }) }}</span>
        </div>
      </div>
      <div class="g-card-body peers">
        <div v-if="status.peers.some((p) => p.status === 'Idle')" class="form-help">{{ t('netbird.idleHint') }}</div>
        <div v-for="p in status.peers" :key="p.fqdn || p.name" class="peer">
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

    <div v-if="loaded" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><SlidersHorizontal :size="16" />{{ t('netbird.config') }}</div>
      </div>
      <div class="g-card-body">
        <el-form label-width="130px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('common.enable')"><el-switch v-model="settings.enable" /></el-form-item>
          <el-form-item :label="t('netbird.setupKey')">
            <SecretInput v-model="settings.setupKey" :placeholder="t('netbird.setupKeyPlaceholder')" :reveal="secretOf('/api/netbird/secrets', 'setupKey')" />
            <div class="form-help">{{ t('netbird.setupKeyHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('netbird.managementUrl')">
            <el-input v-model="settings.managementUrl" :placeholder="defaultManagementUrl" class="mono" />
            <div class="form-help">{{ t('netbird.managementUrlHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('netbird.hostname')"><el-input v-model="settings.hostname" :placeholder="t('netbird.hostnamePlaceholder')" /></el-form-item>
          <el-form-item :label="t('netbird.acceptRoutes')"><el-switch v-model="settings.acceptRoutes" /><span class="help">{{ t('netbird.acceptRoutesHelp') }}</span></el-form-item>
          <el-form-item :label="t('netbird.serverRoutes')"><el-switch v-model="settings.serverRoutes" /><span class="help">{{ t('netbird.serverRoutesHelp') }}</span></el-form-item>
          <el-form-item :label="t('netbird.acceptDns')"><el-switch v-model="settings.acceptDns" /><span class="help">{{ t('netbird.acceptDnsHelp') }}</span></el-form-item>
          <el-form-item :label="t('netbird.blockInbound')"><el-switch v-model="settings.blockInbound" /><span class="help">{{ t('netbird.blockInboundHelp') }}</span></el-form-item>
        </el-form>
      </div>
    </div>

    <div v-if="loaded" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Cpu :size="16" />{{ t('netbird.runMode') }}</div>
      </div>
      <div class="g-card-body">
        <el-form label-width="130px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('netbird.manageDaemon')">
            <el-switch v-model="settings.manageDaemon" />
            <span class="help">{{ t('netbird.manageDaemonHelp') }}</span>
          </el-form-item>
          <template v-if="settings.manageDaemon">
            <el-form-item :label="t('netbird.userspace')">
              <el-switch v-model="settings.userspace" />
              <span class="help">{{ t('netbird.userspaceHelp') }}</span>
            </el-form-item>
            <el-form-item v-if="settings.userspace" :label="t('netbird.localForwarding')">
              <el-switch v-model="settings.localForwarding" />
              <div class="form-help">{{ t('netbird.localForwardingHelp') }}</div>
            </el-form-item>
            <el-form-item v-if="settings.userspace" :label="t('netbird.socks5')">
              <el-input v-model="settings.socks5" :placeholder="t('netbird.socks5Placeholder')" class="mono" />
              <div class="form-help">{{ t('netbird.socks5Help') }}</div>
              <!-- SOCKS5 代理没有认证，监听到本机以外时提醒 -->
              <el-alert v-if="listensBeyondLocal(settings.socks5)" class="socks-warn" type="warning" :closable="false" show-icon :title="t('netbird.socks5Public')" />
            </el-form-item>
            <el-form-item :label="t('netbird.wgPort')">
              <el-input-number v-model="settings.port" :min="0" :max="65535" controls-position="right" />
              <span class="help">{{ t('netbird.wgPortHelp') }}</span>
            </el-form-item>
            <el-form-item :label="t('netbird.stateDir')"><el-input v-model="settings.stateDir" :placeholder="defaultStateDir" class="mono" /></el-form-item>
            <el-form-item :label="t('netbird.daemonArgs')">
              <el-input v-model="settings.daemonExtraArgs" :placeholder="t('netbird.daemonArgsPlaceholder')" class="mono" />
            </el-form-item>
            <el-form-item :label="t('netbird.env')">
              <el-input v-model="settings.daemonEnv" type="textarea" :rows="3" :placeholder="t('netbird.envPlaceholder')" class="mono" />
              <div class="form-help">{{ t('netbird.envHelp') }}</div>
            </el-form-item>
          </template>
          <el-form-item v-else :label="t('netbird.daemonAddr')">
            <el-input v-model="settings.daemonAddr" :placeholder="t('netbird.daemonAddrPlaceholder')" class="mono" />
          </el-form-item>
          <el-form-item :label="t('netbird.netbirdPath')"><el-input v-model="settings.netbirdPath" :placeholder="t('netbird.autoFind')" class="mono" /></el-form-item>
          <el-form-item :label="t('netbird.extraArgs')">
            <el-input v-model="settings.extraUpArgs" :placeholder="t('netbird.extraArgsPlaceholder')" class="mono" />
          </el-form-item>
        </el-form>
      </div>
    </div>

    <ProgramInstall :key="resetCount" base="/api/netbird" :active-paths="[status?.cli]" @changed="loadStatus" />

    <LogCard url="/api/netbird/logs" clearable />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Activity, Bird, Cpu, RefreshCw, Save, SlidersHorizontal, Waypoints } from 'lucide-vue-next'
import { api } from '../api'
import SecretInput from '../components/SecretInput.vue'
import { secretOf } from '../secrets'
import PageHeader from '../components/PageHeader.vue'
import UrlLink from '../components/UrlLink.vue'
import Field from '../components/Field.vue'
import { listensBeyondLocal } from '../netaddr'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import ProgramInstall from '../components/ProgramInstall.vue'
import ResetConfigButton from '../components/ResetConfigButton.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'

interface Settings {
  enable: boolean
  manageDaemon: boolean
  netbirdPath: string
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

const { t } = useI18n()
const { labelPosition } = useFormLayout()
const settings = reactive<Settings>({
  enable: false,
  manageDaemon: true,
  netbirdPath: '',
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
})
// loaded 表示配置已从后端读取，避免表单先以默认值闪现
const loaded = ref(false)
const defaultStateDir = ref('')
const defaultManagementUrl = ref('')
const status = ref<Status>()
const saving = ref(false)
const restarting = ref(false)
// 定时刷新，标签页在后台时暂停
const polling = usePolling(loadStatus, 5000)

// badge 把守护进程的连接状态映射为状态徽标
const badge = computed((): { type: StatusType; text: string } => {
  const s = status.value
  if (!settings.enable) {
    return { type: 'off', text: t('netbird.notEnabled') }
  }
  if (s?.error) {
    return { type: 'error', text: t('netbird.error') }
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
})

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

// connErrors 汇总管理服务与信令服务的连接错误
const connErrors = computed(() => [status.value?.management?.error, status.value?.signal?.error].filter((e): e is string => !!e))

async function loadStatus() {
  try {
    status.value = await api.get<Status>('/api/netbird/status')
  } catch {
    // 轮询失败时保留上一次的状态
  }
}

async function loadSettings() {
  const r = await api.get<{ settings: Settings; defaultStateDir: string; defaultManagementUrl: string }>('/api/netbird/settings')
  Object.assign(settings, r.settings)
  loaded.value = true
  defaultStateDir.value = r.defaultStateDir
  defaultManagementUrl.value = r.defaultManagementUrl
}

async function save() {
  saving.value = true
  try {
    await api.put('/api/netbird/settings', settings)
    ElMessage.success(t('common.saved'))
    await loadSettings()
    await loadStatus()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function restart() {
  restarting.value = true
  try {
    await api.post('/api/netbird/restart')
    await loadStatus()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    restarting.value = false
  }
}

// resetCount 在恢复默认配置后递增，让在线安装卡片重新读取下载代理
const resetCount = ref(0)

// onReset 在恢复默认配置后重新读取设置与状态
async function onReset() {
  resetCount.value++
  await Promise.all([loadSettings(), loadStatus()])
}

onMounted(() => {
  loadSettings()
  loadStatus()
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
