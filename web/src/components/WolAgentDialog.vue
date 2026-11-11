<template>
  <!-- 网络唤醒客户端：连接状态、各系统的安装与卸载命令、凭据管理 -->
  <el-dialog :model-value="modelValue" :title="t('wol.agentTitle', { name: device?.name ?? '' })" width="640px" :fullscreen="isMobile" @update:model-value="emit('update:modelValue', $event)" @open="onOpen">
    <div class="agent-status">
      <StatusBadge v-bind="statusBadge" />
      <span v-if="info?.host" class="mono agent-host">{{ info.host }}</span>
    </div>
    <div v-if="info" class="agent-meta">
      <span v-if="info.os">{{ t('wol.agentSystem') }}<b class="mono">{{ info.os }}</b></span>
      <span v-if="info.version">{{ t('wol.agentVersion') }}<b class="mono">{{ info.version }}</b></span>
      <span v-if="info.addr">{{ t('wol.agentAddr') }}<b class="mono">{{ info.addr }}</b></span>
      <span v-if="info.online && info.since">{{ t('wol.agentSince') }}<b>{{ fmtTime(info.since) }}</b></span>
      <span v-else-if="info.lastSeen">{{ t('wol.agentLastSeen') }}<b>{{ fmtTime(info.lastSeen) }}</b></span>
    </div>

    <el-form label-width="110px" :label-position="labelPosition" class="agent-form" @submit.prevent>
      <el-form-item :label="t('wol.agentServer')">
        <el-input v-model="server" class="mono" :placeholder="apiOrigin" />
        <div v-if="!serverOk" class="form-help warn">{{ t('wol.agentServerInvalid') }}</div>
        <div v-else-if="basePath" class="form-help warn">{{ t('wol.agentGateway') }}</div>
        <div v-else class="form-help">{{ t('wol.agentServerHelp') }}</div>
        <div v-if="serverOk && server.trim().startsWith('http://')" class="form-help warn">{{ t('wol.agentHttpWarn') }}</div>
      </el-form-item>
      <el-form-item v-if="server.trim().startsWith('https://')" :label="t('wol.agentInsecure')">
        <el-switch v-model="insecure" />
        <div class="form-help">{{ t('wol.agentInsecureHelp') }}</div>
      </el-form-item>
      <el-form-item :label="t('wol.agentToken')">
        <div class="token-row">
          <el-input :model-value="tokenShown ? token : token ? SECRET_MASK : ''" readonly class="mono token-input" />
          <el-button :icon="tokenShown ? EyeOff : Eye" :disabled="!token" @click="tokenShown = !tokenShown" />
          <el-button :icon="RefreshCw" :loading="regenerating" @click="regenerate">{{ t('wol.agentRegenerate') }}</el-button>
        </div>
      </el-form-item>
    </el-form>

    <el-tabs v-model="platform" class="agent-tabs">
      <el-tab-pane v-for="p in AGENT_PLATFORMS" :key="p" :label="platformLabel[p]" :name="p" />
    </el-tabs>
    <div class="cmd-title">{{ t('wol.agentInstall') }}</div>
    <div class="cmd-box" :class="{ empty: !install }">
      <pre ref="installEl" class="mono">{{ install || t('wol.agentNoCommand') }}</pre>
      <el-button v-if="install" size="small" :icon="Copy" class="cmd-copy" @click="copyText(install, { el: installEl })">{{ t('common.copy') }}</el-button>
    </div>
    <div class="form-help">{{ installHelp }}</div>
    <div class="cmd-title">{{ t('wol.agentUninstall') }}</div>
    <div class="cmd-box">
      <pre ref="uninstallEl" class="mono">{{ uninstall }}</pre>
      <el-button size="small" :icon="Copy" class="cmd-copy" @click="copyText(uninstall, { el: uninstallEl })">{{ t('common.copy') }}</el-button>
    </div>
    <el-alert type="info" :closable="false" class="agent-note">
      {{ t('wol.agentNote') }}
      <a :href="scriptHref" download class="agent-download">{{ t('wol.agentDownload') }}</a>
    </el-alert>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">{{ t('common.close') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Copy, Eye, EyeOff, RefreshCw } from 'lucide-vue-next'
import { api } from '../api'
import { apiOrigin, basePath } from '../basePath'
import { fetchSecret, SECRET_MASK } from '../secrets'
import { copyText } from '../composables/useClipboard'
import { useFormLayout } from '../composables/useIsMobile'
import StatusBadge, { type StatusType } from './StatusBadge.vue'
import { AGENT_PLATFORMS, guessPlatform, installCommand, normalizeServer, uninstallCommand, type AgentPlatform } from '../wolAgent'

export interface AgentInfo {
  online: boolean
  host: string
  os: string
  version: string
  addr: string
  since: number
  lastSeen: number
}

const props = defineProps<{
  modelValue: boolean
  device: { key?: string; name: string } | null
  info?: AgentInfo
}>()
const emit = defineEmits<{ 'update:modelValue': [v: boolean] }>()

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
const platformLabel: Record<AgentPlatform, string> = { windows: 'Windows', linux: 'Linux', macos: 'macOS', openwrt: 'OpenWrt' }

// 设备访问 Gately 用的地址，默认为浏览器当前地址；修改后记在本机，下次打开沿用
const SERVER_KEY = 'wol.agentServer'
const server = ref('')
const insecure = ref(false)
const token = ref('')
const tokenShown = ref(false)
const regenerating = ref(false)
const platform = ref<AgentPlatform>('windows')
const installEl = ref<HTMLElement | null>(null)
const uninstallEl = ref<HTMLElement | null>(null)

const serverOk = computed(() => normalizeServer(server.value) !== '')
// 客户端脚本的 SHA-256，按文件名索引：安装命令先校验下载的脚本再执行
const scriptSums = ref<Record<string, string>>({})
const scriptName = computed(() => (platform.value === 'windows' ? 'gately-agent.ps1' : 'gately-agent.sh'))
const install = computed(() =>
  installCommand(platform.value, server.value, token.value, insecure.value && server.value.trim().startsWith('https://'), scriptSums.value[scriptName.value] ?? ''),
)
const uninstall = computed(() => uninstallCommand(platform.value))
const scriptHref = computed(() => `/api/wol/agent-script/${scriptName.value}`)
const installHelp = computed(() => {
  switch (platform.value) {
    case 'windows':
      return t('wol.agentHelpWindows')
    case 'openwrt':
      return t('wol.agentHelpOpenwrt')
    case 'macos':
      return t('wol.agentHelpMacos')
  }
  return t('wol.agentHelpLinux')
})

const statusBadge = computed((): { type: StatusType; text: string } => {
  if (props.info?.online) {
    return { type: 'running', text: t('wol.agentOnline') }
  }
  if (props.info) {
    return { type: 'off', text: t('wol.agentOffline') }
  }
  return { type: 'info', text: t('wol.agentNever') }
})

function fmtTime(ms: number) {
  return new Date(ms).toLocaleString()
}

async function loadToken() {
  token.value = ''
  if (!props.device?.key) {
    return
  }
  try {
    token.value = (await fetchSecret(`/api/wol/devices/${props.device.key}/secrets`, 'agentToken')) ?? ''
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function loadSums() {
  try {
    scriptSums.value = await api.get<Record<string, string>>('/api/wol/agent-scripts')
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

function onOpen() {
  server.value = localStorage.getItem(SERVER_KEY) || apiOrigin
  insecure.value = localStorage.getItem(SERVER_KEY + '.insecure') === '1'
  tokenShown.value = false
  platform.value = guessPlatform(props.info?.os)
  loadToken()
  loadSums()
}

// 记住修改过的地址与证书设置
function remember() {
  const s = normalizeServer(server.value)
  if (s && s !== apiOrigin) {
    localStorage.setItem(SERVER_KEY, s)
  } else if (s === apiOrigin) {
    localStorage.removeItem(SERVER_KEY)
  }
  localStorage.setItem(SERVER_KEY + '.insecure', insecure.value ? '1' : '0')
}

async function regenerate() {
  if (!props.device?.key) {
    return
  }
  try {
    await ElMessageBox.confirm(t('wol.agentRegenerateConfirm'), t('wol.agentRegenerate'), { type: 'warning' })
  } catch {
    return
  }
  regenerating.value = true
  try {
    const r = await api.post<{ agentToken: string }>(`/api/wol/devices/${props.device.key}/agent-token`)
    token.value = r.agentToken
    ElMessage.success(t('wol.agentRegenerated'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    regenerating.value = false
  }
}

// 地址或证书设置变化时立即记住，关闭对话框后再打开沿用
watch([server, insecure], remember)
</script>

<style scoped>
.agent-status {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.agent-host {
  font-size: 13px;
}
.agent-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 14px;
}
.agent-meta b {
  margin-left: 4px;
  font-weight: 500;
  color: var(--el-text-color-regular);
}
.agent-form {
  margin-top: 6px;
}
.token-row {
  display: flex;
  gap: 6px;
  width: 100%;
}
.token-row .el-button {
  margin-left: 0;
}
.token-input {
  flex: 1;
  min-width: 0;
}
.agent-tabs :deep(.el-tabs__header) {
  margin-bottom: 8px;
}
.cmd-title {
  font-size: 13px;
  font-weight: 500;
  margin: 10px 0 6px;
}
.cmd-box {
  position: relative;
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  padding: 10px 12px;
}
.cmd-box pre {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  /* 给右上角的复制按钮留位置 */
  padding-right: 64px;
  max-height: 180px;
  overflow: auto;
  user-select: all;
}
.cmd-box.empty pre {
  color: var(--el-text-color-secondary);
  user-select: auto;
}
.cmd-copy {
  position: absolute;
  top: 6px;
  right: 6px;
}
.form-help {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
  margin-top: 4px;
}
.form-help.warn {
  color: var(--el-color-warning);
}
.agent-note {
  margin-top: 14px;
}
.agent-download {
  margin-left: 6px;
  color: var(--el-color-primary);
}
</style>
