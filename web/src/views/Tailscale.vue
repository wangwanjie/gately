<template>
  <div>
    <PageHeader title="Tailscale" :desc="t('tailscale.desc')" :icon="Network">
      <ResetConfigButton base="/api/tailscale" :desc="t('components.resetConfig.tailscale')" @done="onReset" />
      <el-button :icon="RefreshCw" :loading="restarting" @click="restart">{{ t('common.restart') }}</el-button>
      <el-button type="primary" :icon="Save" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
    </PageHeader>

    <el-alert v-if="status?.error" type="error" :title="status.error" :closable="false" show-icon class="block-gap" />
    <el-alert v-if="status?.authUrl" type="warning" :closable="false" show-icon class="block-gap" :title="t('tailscale.needLogin')">
      <!-- 复制按钮复制登录地址本身，便于在其他设备上打开完成登录 -->
      <UrlLink :href="status.authUrl" :text="t('tailscale.authLink')" class="auth-link" />
    </el-alert>

    <div class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Activity :size="16" />{{ t('common.status') }}</div>
        <StatusBadge v-bind="badge" />
      </div>
      <div class="g-card-body">
        <div v-if="status?.self" class="info-grid">
          <Field :label="t('tailscale.self')" mono>{{ status.self.name }}</Field>
          <Field label="Tailscale IP" mono>
            <div v-for="ip in status.self.ips" :key="ip">{{ ip }}</div>
          </Field>
          <Field v-if="status.tailnet" label="Tailnet" mono>{{ status.tailnet }}</Field>
          <Field v-if="status.magicDnsSuffix" label="MagicDNS" mono>{{ status.magicDnsSuffix }}</Field>
          <Field v-if="status.version" :label="t('tailscale.version')" mono>{{ status.version }}</Field>
        </div>
        <div v-else class="help block">
          {{ settings.enable ? t('tailscale.notConnected') : t('tailscale.notEnabledHint') }}
        </div>
        <div v-if="status?.health?.length" class="health">
          <el-tag v-for="h in status.health" :key="h" type="warning" size="small">{{ h }}</el-tag>
        </div>
      </div>
    </div>

    <div v-if="status?.peers?.length" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Waypoints :size="16" />{{ t('tailscale.peers') }} <span class="g-card-desc">{{ t('tailscale.peerCount', { count: status.peers.length }) }}</span></div>
      </div>
      <div class="g-card-body peers">
        <div v-for="p in status.peers" :key="p.dnsName || p.name" class="peer">
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

    <div v-if="loaded" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><SlidersHorizontal :size="16" />{{ t('tailscale.config') }}</div>
      </div>
      <div class="g-card-body">
        <el-form label-width="130px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('common.enable')"><el-switch v-model="settings.enable" /></el-form-item>
          <el-form-item :label="t('tailscale.authKey')">
            <SecretInput v-model="settings.authKey" :placeholder="t('tailscale.authKeyPlaceholder')" :reveal="secretOf('/api/tailscale/secrets', 'authKey')" />
          </el-form-item>
          <el-form-item :label="t('tailscale.hostname')"><el-input v-model="settings.hostname" :placeholder="t('tailscale.hostnamePlaceholder')" /></el-form-item>
          <el-form-item :label="t('tailscale.loginServer')">
            <el-input v-model="settings.loginServer" :placeholder="t('tailscale.loginServerPlaceholder')" />
          </el-form-item>
          <el-form-item :label="t('tailscale.subnetRoutes')">
            <el-input v-model="routesText" :placeholder="t('tailscale.routesPlaceholder')" class="mono" />
            <div class="form-help">{{ t('tailscale.routesHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('tailscale.exitNode')"><el-switch v-model="settings.advertiseExitNode" /><span class="help">{{ t('tailscale.exitNodeHelp') }}</span></el-form-item>
          <el-form-item :label="t('tailscale.acceptRoutes')"><el-switch v-model="settings.acceptRoutes" /><span class="help">{{ t('tailscale.acceptRoutesHelp') }}</span></el-form-item>
          <el-form-item :label="t('tailscale.acceptDns')"><el-switch v-model="settings.acceptDns" /><span class="help">{{ t('tailscale.acceptDnsHelp') }}</span></el-form-item>
        </el-form>
      </div>
    </div>

    <div v-if="loaded" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Cpu :size="16" />{{ t('tailscale.runMode') }}</div>
      </div>
      <div class="g-card-body">
        <el-form label-width="130px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('tailscale.manageDaemon')">
            <el-switch v-model="settings.manageDaemon" />
            <span class="help">{{ t('tailscale.manageDaemonHelp') }}</span>
          </el-form-item>
          <template v-if="settings.manageDaemon">
            <el-form-item :label="t('tailscale.userspace')">
              <el-switch v-model="settings.userspace" />
              <span class="help">{{ t('tailscale.userspaceHelp') }}</span>
            </el-form-item>
            <el-form-item v-if="settings.userspace" :label="t('tailscale.socks5')">
              <el-input v-model="settings.socks5" :placeholder="t('tailscale.socks5Placeholder')" class="mono" />
              <div class="form-help">{{ t('tailscale.socks5Help') }}</div>
            </el-form-item>
            <el-form-item :label="t('tailscale.wgPort')">
              <el-input-number v-model="settings.port" :min="0" :max="65535" controls-position="right" />
              <span class="help">{{ t('tailscale.wgPortHelp') }}</span>
            </el-form-item>
            <el-form-item :label="t('tailscale.stateDir')"><el-input v-model="settings.stateDir" :placeholder="defaultStateDir" class="mono" /></el-form-item>
            <el-form-item :label="t('tailscale.tailscaledPath')"><el-input v-model="settings.tailscaledPath" :placeholder="t('tailscale.autoFind')" class="mono" /></el-form-item>
            <el-form-item :label="t('tailscale.daemonArgs')">
              <el-input v-model="settings.daemonExtraArgs" :placeholder="t('tailscale.daemonArgsPlaceholder')" class="mono" />
            </el-form-item>
            <el-form-item :label="t('tailscale.env')">
              <el-input v-model="settings.daemonEnv" type="textarea" :rows="3" :placeholder="t('tailscale.envPlaceholder')" class="mono" />
              <div class="form-help">{{ t('tailscale.envHelp') }}</div>
            </el-form-item>
          </template>
          <el-form-item v-else :label="t('tailscale.socket')">
            <el-input v-model="settings.socket" :placeholder="t('tailscale.socketPlaceholder')" class="mono" />
          </el-form-item>
          <el-form-item :label="t('tailscale.tailscalePath')"><el-input v-model="settings.tailscalePath" :placeholder="t('tailscale.autoFind')" class="mono" /></el-form-item>
          <el-form-item :label="t('tailscale.extraArgs')">
            <el-input v-model="settings.extraUpArgs" :placeholder="t('tailscale.extraArgsPlaceholder')" class="mono" />
          </el-form-item>
        </el-form>
      </div>
    </div>

    <ProgramInstall :key="resetCount" base="/api/tailscale" :active-paths="[status?.cli]" @changed="loadStatus" />

    <LogCard url="/api/tailscale/logs" clearable />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Activity, Cpu, Network, RefreshCw, Save, SlidersHorizontal, Waypoints } from 'lucide-vue-next'
import { api } from '../api'
import SecretInput from '../components/SecretInput.vue'
import { secretOf } from '../secrets'
import PageHeader from '../components/PageHeader.vue'
import UrlLink from '../components/UrlLink.vue'
import Field from '../components/Field.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import ProgramInstall from '../components/ProgramInstall.vue'
import ResetConfigButton from '../components/ResetConfigButton.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { useListText, splitCommasOrSpaces } from '../composables/useListText'
import { usePolling } from '../composables/usePolling'

interface Settings {
  enable: boolean
  manageDaemon: boolean
  tailscaledPath: string
  tailscalePath: string
  stateDir: string
  socket: string
  userspace: boolean
  socks5: string
  authKey: string
  hostname: string
  loginServer: string
  advertiseRoutes: string[]
  advertiseExitNode: boolean
  acceptRoutes: boolean
  acceptDns: boolean
  extraUpArgs: string
  port: number
  daemonExtraArgs: string
  daemonEnv: string
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

const { t } = useI18n()
const { labelPosition } = useFormLayout()
const settings = reactive<Settings>({
  enable: false,
  manageDaemon: true,
  tailscaledPath: '',
  tailscalePath: '',
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
})
// loaded 表示配置已从后端读取，避免表单先以默认值闪现
const loaded = ref(false)
const defaultStateDir = ref('')
const status = ref<Status>()
const saving = ref(false)
const restarting = ref(false)
// 定时刷新，标签页在后台时暂停
const polling = usePolling(loadStatus, 5000)

// 输入框保留原始文本，逗号、空格可正常输入（见 useListText）
const routesText = useListText(() => settings.advertiseRoutes, (v) => (settings.advertiseRoutes = v), {
  split: splitCommasOrSpaces,
  separator: ', ',
})

// badge 把 tailscale 的后端状态映射为状态徽标
const badge = computed((): { type: StatusType; text: string } => {
  const s = status.value
  if (!settings.enable) {
    return { type: 'off', text: t('tailscale.notEnabled') }
  }
  if (s?.error) {
    return { type: 'error', text: t('tailscale.error') }
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
})

async function loadStatus() {
  try {
    status.value = await api.get<Status>('/api/tailscale/status')
  } catch {
    // 轮询失败时保留上一次的状态
  }
}

async function loadSettings() {
  const r = await api.get<{ settings: Settings; defaultStateDir: string }>('/api/tailscale/settings')
  Object.assign(settings, r.settings)
  loaded.value = true
  defaultStateDir.value = r.defaultStateDir
}

async function save() {
  saving.value = true
  try {
    await api.put('/api/tailscale/settings', settings)
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
    await api.post('/api/tailscale/restart')
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
</style>
