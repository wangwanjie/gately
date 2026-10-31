<template>
  <div>
    <PageHeader :title="t('cloudflared.title')" :desc="t('cloudflared.desc')" :icon="Cloud">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('groupSort.search')" />
      <ResetConfigButton base="/api/cloudflared" :desc="t('components.resetConfig.cloudflared')" @done="onReset" />
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('cloudflared.addTunnel') }}</el-button>
    </PageHeader>

    <el-alert v-if="binError" type="warning" :title="binError" :closable="false" show-icon class="block-gap">
      <el-button size="small" type="primary" :icon="Download" @click="installer?.install()">{{ t('components.programInstall.install') }}</el-button>
    </el-alert>

    <GroupedList :gl="gl" :compact="compact" grid :empty-text="t('cloudflared.empty')">
      <template #item="{ it, item: tn, sorting, bind }">
        <EntityCard v-bind="bind" :off="!tn.enable" :compact="compact || sorting">
          <template v-if="sorting" #lead>
            <SortCheck :gl="gl" :id="it.id" />
          </template>
          <template #title>{{ tn.name }}</template>
          <template #sub>{{ tn.mode === 'token' ? t('cloudflared.modeToken') : t('cloudflared.modeQuick') }}<template v-if="tn.remark"> · {{ tn.remark }}</template></template>
          <template #status><StatusBadge v-bind="tunnelBadge(tn)" /></template>
          <Field v-if="tn.mode === 'quick'" :label="t('cloudflared.localService')" mono>{{ tn.service }}</Field>
          <Field v-if="tn.mode === 'quick'" :label="t('cloudflared.publicAddr')" mono wide>
            <a v-if="tn.url" :href="tn.url" target="_blank" rel="noopener">{{ tn.url }}</a>
            <span v-else class="muted">{{ tn.state.running ? t('cloudflared.requesting') : '—' }}</span>
          </Field>
          <Field v-if="tn.mode === 'token'" :label="t('cloudflared.edgeConnections')" mono>{{ tn.connections }}</Field>
          <Field v-if="tn.tunnelId" :label="t('cloudflared.tunnelId')" mono wide>{{ tn.tunnelId }}</Field>
          <Field :label="t('cloudflared.protocol')" mono>{{ tn.protocol || 'auto' }}</Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-switch :model-value="tn.enable" size="small" @change="(v: boolean | string | number) => toggle(tn, Boolean(v))" />
            <el-button text size="small" :icon="RefreshCw" :disabled="!tn.enable" @click="restart(tn)">{{ t('common.restart') }}</el-button>
            <el-button v-if="tn.mode === 'token'" text size="small" :icon="Route" :title="tn.apiToken ? '' : t('cloudflared.ingressNeedsApi')" @click="ingress?.open(tn.key!, tn.name)">{{ t('cloudflared.ingressButton') }}</el-button>
            <el-button text size="small" :icon="Pencil" @click="openEditor(tn)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('cloudflared.confirmDelete')" @confirm="remove(tn)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <div class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Terminal :size="16" />{{ t('cloudflared.program') }} <span v-if="version" class="g-card-desc">{{ version }}</span></div>
        <el-button size="small" :icon="Save" @click="saveSettings">{{ t('common.save') }}</el-button>
      </div>
      <div class="g-card-body">
        <el-form label-width="110px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('cloudflared.binary')">
            <el-input v-model="binPath" :placeholder="binary || t('cloudflared.binaryPlaceholder')" class="mono" />
          </el-form-item>
        </el-form>
      </div>
    </div>

    <ProgramInstall ref="installer" :key="resetCount" base="/api/cloudflared" :active-paths="[binary]" @changed="onInstalled" />

    <LogCard url="/api/cloudflared/logs" clearable />

    <CloudflaredIngress ref="ingress" />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('cloudflared.editTunnel') : t('cloudflared.addTunnel')" width="540px" :fullscreen="isMobile">
      <el-form :model="editing" label-width="100px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('cloudflared.mode')">
          <el-radio-group v-model="editing.mode">
            <el-radio-button value="token">{{ t('cloudflared.modeToken') }}</el-radio-button>
            <el-radio-button value="quick">{{ t('cloudflared.modeQuick') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="editing.mode === 'token'" :label="t('cloudflared.token')">
          <SecretInput v-model="editing.token" placeholder="eyJh…" :reveal="secretOf(editing.key && `/api/cloudflared/tunnels/${editing.key}/secrets`, 'token')" />
          <div class="form-help">{{ t('cloudflared.tokenHelp') }}</div>
        </el-form-item>
        <el-form-item v-if="editing.mode === 'token'" :label="t('cloudflared.apiToken')">
          <SecretInput v-model="editing.apiToken" :placeholder="t('cloudflared.apiTokenPh')" :reveal="secretOf(editing.key && `/api/cloudflared/tunnels/${editing.key}/secrets`, 'apiToken')" />
          <div class="form-help">{{ t('cloudflared.apiTokenHelp') }} <a href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" rel="noopener">dash.cloudflare.com</a></div>
        </el-form-item>
        <el-form-item v-else :label="t('cloudflared.localService')">
          <el-input v-model="editing.service" placeholder="http://localhost:8080" class="mono" />
          <div class="form-help">{{ t('cloudflared.serviceHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('cloudflared.protocol')">
          <el-select v-model="editing.protocol">
            <el-option label="auto" value="auto" />
            <el-option label="quic" value="quic" />
            <el-option label="http2" value="http2" />
          </el-select>
          <div class="form-help">{{ t('cloudflared.protocolHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('cloudflared.edgeIp')">
          <el-select v-model="editing.edgeIpVersion">
            <el-option label="auto" value="auto" />
            <el-option label="IPv4" value="4" />
            <el-option label="IPv6" value="6" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Cloud, Download, Pencil, Plus, RefreshCw, Route, Save, Terminal, Trash2 } from 'lucide-vue-next'
import { api } from '../api'
import SecretInput from '../components/SecretInput.vue'
import { secretOf } from '../secrets'
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
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import ProgramInstall from '../components/ProgramInstall.vue'
import ResetConfigButton from '../components/ResetConfigButton.vue'
import CloudflaredIngress from '../components/CloudflaredIngress.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'

interface Tunnel {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  enable: boolean
  mode: string
  token: string
  apiToken: string
  service: string
  protocol: string
  edgeIpVersion: string
  remark: string
}
// TunnelView 是后端返回的隧道，附带运行状态
interface TunnelView extends Tunnel {
  state: { running: boolean }
  error: string
  url?: string
  connections: number
  tunnelId?: string
}

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
const tunnels = ref<TunnelView[]>([])
const compact = useCompactView('cloudflared')
// 列表的搜索、分组与排序；顺序与分组的修改不会重启隧道
const gl = useGroupedList<TunnelView>({
  storageKey: 'cloudflared.groups',
  items: () => tunnels.value,
  base: '/api/cloudflared/tunnels',
  reload: loadTunnels,
  matches: (tn, q) => [tn.name, tn.group ?? '', tn.remark, tn.service, tn.url ?? '', tn.tunnelId ?? ''].join('\n').toLowerCase().includes(q),
})
const binPath = ref('')
const binary = ref('')
const version = ref('')
const binError = ref('')
const installer = ref<InstanceType<typeof ProgramInstall>>()
const ingress = ref<InstanceType<typeof CloudflaredIngress>>()
const editorVisible = ref(false)
const editing = ref<Tunnel>(empty())
// 定时刷新，标签页在后台时暂停
const polling = usePolling(loadTunnels, 4000)

function empty(): Tunnel {
  return { name: '', enable: true, mode: 'token', token: '', apiToken: '', service: 'http://localhost:8080', protocol: 'auto', edgeIpVersion: 'auto', remark: '' }
}

function tunnelBadge(tn: TunnelView): { type: StatusType; text: string; title?: string } {
  if (!tn.enable) {
    return { type: 'off', text: t('cloudflared.deactivated') }
  }
  if (tn.error) {
    return { type: 'error', text: t('cloudflared.startFailed'), title: tn.error }
  }
  if (!tn.state.running) {
    return { type: 'warn', text: t('cloudflared.notRunning') }
  }
  if (tn.mode === 'token' && tn.connections === 0) {
    return { type: 'pending', text: t('cloudflared.connecting') }
  }
  return { type: 'running', text: t('common.running') }
}

async function loadTunnels() {
  try {
    tunnels.value = await api.get<TunnelView[]>('/api/cloudflared/tunnels')
  } catch {
    // 轮询失败时保留上一次的状态
  }
}

async function loadSettings() {
  const r = await api.get<{ settings: { binPath: string }; binary: string; version: string; error: string }>('/api/cloudflared/settings')
  binPath.value = r.settings.binPath
  binary.value = r.binary
  version.value = r.version
  binError.value = r.error
}

// onInstalled 在安装、回滚、卸载后刷新程序信息与隧道状态
// resetCount 在恢复默认配置后递增，让在线安装卡片重新读取下载代理
const resetCount = ref(0)

async function onReset() {
  resetCount.value++
  await onInstalled()
}

async function onInstalled() {
  await Promise.all([loadSettings(), loadTunnels()])
}

async function saveSettings() {
  try {
    await api.put('/api/cloudflared/settings', { binPath: binPath.value })
    ElMessage.success(t('common.saved'))
    await Promise.all([loadSettings(), loadTunnels()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

function openEditor(tn?: TunnelView) {
  if (tn) {
    // 去掉运行状态、公网地址等只读字段，后端按未知字段拒绝请求
    const { state: _s, error: _e, url: _u, connections: _c, tunnelId: _i, ...tunnel } = tn
    editing.value = JSON.parse(JSON.stringify(tunnel))
  } else {
    editing.value = empty()
  }
  editorVisible.value = true
}

async function save() {
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/cloudflared/tunnels/${key}`, body)
    } else {
      await api.post('/api/cloudflared/tunnels', body)
    }
    editorVisible.value = false
    await loadTunnels()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function toggle(tn: Tunnel, enable: boolean) {
  try {
    await api.put(`/api/cloudflared/tunnels/${tn.key}/enable`, { enable })
    await loadTunnels()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function restart(tn: Tunnel) {
  try {
    await api.post(`/api/cloudflared/tunnels/${tn.key}/restart`)
    await loadTunnels()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(tn: Tunnel) {
  try {
    await api.del(`/api/cloudflared/tunnels/${tn.key}`)
    await loadTunnels()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(() => {
  loadSettings()
  loadTunnels()
  polling.start()
})
</script>

<style scoped>
.block-gap {
  margin-bottom: 16px;
}
.block-gap :deep(.el-alert__description) {
  margin-top: 6px;
}
.svc-form {
  max-width: 640px;
}
.svc-form .el-form-item:last-child {
  margin-bottom: 0;
}
.muted {
  color: var(--el-text-color-secondary);
}
.card-grid + .g-card {
  margin-top: 16px;
}
</style>
