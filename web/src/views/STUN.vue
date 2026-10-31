<template>
  <div>
    <PageHeader :title="t('stun.title')" :desc="t('stun.desc')" :icon="Radar">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('groupSort.search')" />
      <el-button :icon="SlidersHorizontal" @click="openSettings">{{ t('stun.globalSettings') }}</el-button>
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('stun.addRule') }}</el-button>
    </PageHeader>

    <ModuleBanner v-if="moduleOff" :title="t('stun.moduleOffTitle')" :loading="enabling" @enable="enableModule" />

    <el-alert type="info" :closable="false" class="intro" show-icon>
      {{ t('stun.intro') }}
    </el-alert>

    <GroupedList :gl="gl" :loading="loading" :compact="compact" :empty-text="t('stun.empty')">
      <template #item="{ it, item: r, sorting, bind }">
        <EntityCard v-bind="bind" :off="!r.enable || moduleOff" :compact="compact || sorting">
          <template #lead>
            <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
            <el-switch v-else :model-value="r.enable" @change="(v: string | number | boolean) => toggle(r, Boolean(v))" />
          </template>
          <template #title>
            <span>{{ r.name }}</span>
            <span class="tags">
              <el-tag size="small" effect="plain">{{ r.protocol === 'udp4' ? 'UDP' : 'TCP' }}</el-tag>
              <el-tag v-if="r.upnp" size="small" type="info">UPnP</el-tag>
              <el-tag v-if="r.natpmp" size="small" type="info">NAT-PMP</el-tag>
            </span>
          </template>
          <template v-if="r.remark" #sub>{{ r.remark }}</template>
          <template #status>
            <StatusBadge v-bind="ruleStatus(r)" />
          </template>
          <Field :label="t('stun.publicAddr')" mono>
            <template v-if="r.enable && !moduleOff && r.state.publicAddr">
              <span class="copyable" :title="t('stun.clickToCopy')" @click="copy(r.state.publicAddr, $event)">{{ r.state.publicAddr }}</span>
              <div v-if="r.state.since" class="meta">{{ t('stun.since', { time: fmtTime(r.state.since) }) }}</div>
            </template>
            <span v-else class="muted">-</span>
          </Field>
          <Field :label="t('stun.tabChannel')" mono>{{ r.state.localAddr || (r.localPort ? ':' + r.localPort : t('stun.randomPort')) }}</Field>
          <Field :label="t('stun.target')" mono>{{ r.disableForward ? t('stun.routerForward') : `${(r.targetAddresses ?? []).join(' | ')}:${r.targetPort}` }}</Field>
          <Field :label="t('stun.connTraffic')">
            {{ t('stun.connStats', { active: r.state.stats.activeConns, total: r.state.stats.totalConns }) }}
            <div class="meta">↑ {{ formatBytes(r.state.stats.bytesIn) }} · ↓ {{ formatBytes(r.state.stats.bytesOut) }}</div>
          </Field>
          <template v-if="!sorting && (r.enable && !moduleOff && (r.state.error || r.state.mapping))" #extra>
            <div v-if="r.state.error" class="err-line">{{ r.state.error }}</div>
            <div v-if="r.state.mapping" class="meta">{{ r.state.mapping }}</div>
          </template>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="RotateCw" :disabled="!r.enable || moduleOff" @click="restart(r)">{{ t('stun.reconnect') }}</el-button>
            <el-button text size="small" :icon="Pencil" @click="openEditor(r)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('stun.confirmDelete')" @confirm="remove(r)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <LogCard url="/api/stun/logs" clearable />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('stun.editRule') : t('stun.addRule')" width="780px" top="5vh" :fullscreen="isMobile">
      <el-form :model="editing" label-width="140px" :label-position="labelPosition">
        <el-tabs v-model="tab">
          <el-tab-pane :label="t('stun.tabBasic')" name="basic">
            <el-form-item :label="t('stun.ruleName')" required><el-input v-model="editing.name" :placeholder="t('stun.ruleNamePlaceholder')" /></el-form-item>
            <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
            <el-form-item :label="t('stun.protocolType')">
              <el-radio-group v-model="editing.protocol">
                <el-radio-button value="tcp4">TCP</el-radio-button>
                <el-radio-button value="udp4">UDP</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <el-form-item :label="t('stun.localPort')">
              <el-input-number v-model="editing.localPort" :min="0" :max="65535" />
              <span class="help">{{ t('stun.localPortHelp') }}</span>
            </el-form-item>
            <el-form-item :label="t('stun.disableForward')">
              <el-switch v-model="editing.disableForward" />
              <span class="help">{{ t('stun.disableForwardHelp') }}</span>
            </el-form-item>
            <template v-if="!editing.disableForward">
              <el-form-item :label="t('stun.targetAddr')" required>
                <el-input v-model="targetsText" :placeholder="t('stun.targetAddrPlaceholder')" />
              </el-form-item>
              <el-form-item :label="t('stun.targetPort')" required>
                <el-input-number v-model="editing.targetPort" :min="1" :max="65535" />
              </el-form-item>
            </template>
            <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
            <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>
          </el-tab-pane>

          <el-tab-pane :label="t('stun.tabChannel')" name="channel">
            <el-form-item :label="t('stun.bind')">
              <el-radio-group v-model="editing.bind.mode">
                <el-radio-button value="ip">{{ t('stun.bindIp') }}</el-radio-button>
                <el-radio-button value="interface">{{ t('stun.bindInterface') }}</el-radio-button>
              </el-radio-group>
              <span class="help">{{ t('stun.bindHelp') }}</span>
            </el-form-item>
            <el-form-item v-if="editing.bind.mode === 'ip'" :label="t('stun.channelIp')">
              <el-input v-model="editing.bind.ip" :placeholder="t('stun.channelIpPlaceholder')" />
            </el-form-item>
            <template v-else>
              <el-form-item :label="t('stun.nic')">
                <el-select v-model="editing.bind.interface" filterable allow-create style="width: 100%">
                  <el-option v-for="i in interfaces" :key="i.name" :label="i.name" :value="i.name">
                    <span>{{ i.name }}</span><span class="iface">{{ i.addrs.join('  ') }}</span>
                  </el-option>
                </el-select>
              </el-form-item>
              <el-form-item :label="t('stun.ipRule')">
                <el-input v-model="editing.bind.regex" :placeholder="t('stun.ipRulePlaceholder')" />
              </el-form-item>
            </template>
            <el-form-item :label="t('stun.useGlobalServers')">
              <el-switch v-model="editing.useGlobalServers" />
            </el-form-item>
            <el-form-item v-if="!editing.useGlobalServers" :label="t('stun.stunServers')">
              <el-input v-model="serversText" type="textarea" :rows="4" class="mono" :placeholder="t('stun.stunServersPlaceholder')" />
            </el-form-item>
            <el-form-item v-if="editing.protocol === 'tcp4'" :label="t('stun.keepAliveServers')">
              <el-input v-model="keepAliveText" type="textarea" :rows="2" class="mono" :placeholder="t('stun.keepAlivePlaceholder')" />
            </el-form-item>
          </el-tab-pane>

          <el-tab-pane :label="t('stun.tabMapping')" name="mapping">
            <el-form-item label="UPnP"><el-switch v-model="editing.upnp" /></el-form-item>
            <template v-if="editing.upnp">
              <el-form-item :label="t('stun.gatewayIp')"><el-input v-model="editing.upnpGateway" :placeholder="t('stun.gatewayIpPlaceholder')" /></el-form-item>
              <el-form-item :label="t('stun.localHostIp')"><el-input v-model="editing.upnpLocalHost" :placeholder="t('stun.localHostIpPlaceholder')" /></el-form-item>
              <el-form-item :label="t('stun.controlUrl')"><el-input v-model="editing.upnpControlUrl" :placeholder="t('stun.controlUrlPlaceholder')" /></el-form-item>
            </template>
            <el-form-item label="NAT-PMP"><el-switch v-model="editing.natpmp" /></el-form-item>
            <el-form-item v-if="editing.natpmp" :label="t('stun.natpmpGateway')"><el-input v-model="editing.natpmpGateway" :placeholder="t('stun.natpmpGatewayPlaceholder')" /></el-form-item>
            <el-form-item v-if="editing.upnp || editing.natpmp" :label="t('stun.internalPort')">
              <el-input-number v-model="editing.upnpInternalPort" :min="0" :max="65535" />
              <span class="help">{{ t('stun.internalPortHelp') }}</span>
            </el-form-item>
          </el-tab-pane>

          <el-tab-pane :label="t('stun.tabAdvanced')" name="advanced">
            <el-form-item :label="t('stun.heartbeat')"><el-input-number v-model="editing.heartbeatInterval" :min="500" :max="60000" :step="500" /></el-form-item>
            <el-form-item :label="t('stun.stunTimeout')"><el-input-number v-model="editing.stunTimeout" :min="500" :max="30000" :step="500" /></el-form-item>
            <el-form-item :label="t('stun.retryInterval')"><el-input-number v-model="editing.retryInterval" :min="500" :max="600000" :step="500" /></el-form-item>
            <el-form-item :label="t('stun.autoRetry')"><el-switch v-model="editing.autoRetry" /></el-form-item>
            <el-form-item :label="t('stun.disableCheck')">
              <el-switch v-model="editing.disableCheck" />
              <span class="help">{{ t('stun.disableCheckHelp') }}</span>
            </el-form-item>
            <el-form-item :label="t('stun.maxConnections')"><el-input-number v-model="editing.maxConnections" :min="0" /></el-form-item>
            <el-form-item v-if="editing.protocol === 'udp4'" :label="t('stun.udpTimeout')"><el-input-number v-model="editing.udpSessionTimeout" :min="0" /></el-form-item>
          </el-tab-pane>

          <el-tab-pane :label="t('stun.tabNotify')" name="notify">
            <el-form-item :label="t('stun.onlyOnChange')"><el-switch v-model="editing.webhookOnlyOnChange" /></el-form-item>
            <el-form-item :label="t('stun.useGlobalWebhook')"><el-switch v-model="editing.useGlobalWebhook" /></el-form-item>
            <WebhookEditor v-if="!editing.useGlobalWebhook" v-model="editing.webhook" :variables="vars" />
            <el-divider />
            <el-form-item :label="t('stun.runScript')"><el-switch v-model="editing.scriptEnable" /></el-form-item>
            <el-form-item v-if="editing.scriptEnable" :label="t('stun.scriptContent')">
              <el-input v-model="editing.script" type="textarea" :rows="5" class="mono" :placeholder="t('stun.scriptPlaceholder')" />
              <div class="form-help">
                <i18n-t keypath="stun.scriptHelp" tag="span">
                  <template #vars><span class="mono">ruleName time ip port ipAddr</span></template>
                </i18n-t>
              </div>
            </el-form-item>
          </el-tab-pane>
        </el-tabs>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="settingsVisible" :title="t('stun.settingsTitle')" width="700px" :fullscreen="isMobile">
      <el-form v-if="settings" :model="settings" label-width="140px" :label-position="labelPosition">
        <el-form-item :label="t('stun.enableModule')"><el-switch v-model="settings.enable" /></el-form-item>
        <el-form-item :label="t('stun.stunServers')">
          <el-input v-model="settingsServers" type="textarea" :rows="6" class="mono" :placeholder="t('stun.settingsServersPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('stun.keepAliveServers')">
          <el-input v-model="settingsKeepAlive" type="textarea" :rows="2" class="mono" />
        </el-form-item>
        <el-form-item :label="t('stun.addrCheckInterval')"><el-input-number v-model="settings.addrCheckInterval" :min="10" :max="3600" /></el-form-item>
        <el-divider content-position="left">{{ t('stun.globalWebhook') }}</el-divider>
        <WebhookEditor v-model="settings.webhook" :variables="vars" />
      </el-form>
      <template #footer>
        <el-button @click="settingsVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Pencil, Plus, Radar, RotateCw, SlidersHorizontal, Trash2 } from 'lucide-vue-next'
import { api, formatBytes } from '../api'
import PageHeader from '../components/PageHeader.vue'
import ModuleBanner from '../components/ModuleBanner.vue'
import CompactToggle from '../components/CompactToggle.vue'
import EntityCard from '../components/EntityCard.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import { useGroupedList } from '../composables/useGroupSort'
import { useCompactView } from '../composables/useCompactView'
import Field from '../components/Field.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { useListText, splitLinesOrCommas } from '../composables/useListText'
import WebhookEditor, { type WebhookConfig } from '../components/WebhookEditor.vue'
import type { NetInterface } from '../components/IPSourceEditor.vue'
import { usePolling } from '../composables/usePolling'
import { copyText } from '../composables/useClipboard'

interface Rule {
  key?: string
  name: string
  enable: boolean
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  protocol: string
  bind: { mode: string; ip: string; interface: string; regex: string }
  localPort: number
  useGlobalServers: boolean
  stunServers: string[] | null
  keepAliveServers: string[] | null
  disableForward: boolean
  targetAddresses: string[] | null
  targetPort: number
  upnp: boolean
  upnpGateway: string
  upnpLocalHost: string
  upnpControlUrl: string
  upnpInternalPort: number
  natpmp: boolean
  natpmpGateway: string
  heartbeatInterval: number
  stunTimeout: number
  retryInterval: number
  autoRetry: boolean
  disableCheck: boolean
  useGlobalWebhook: boolean
  webhook: WebhookConfig
  webhookOnlyOnChange: boolean
  scriptEnable: boolean
  script: string
  maxConnections: number
  udpSessionTimeout: number
  remark: string
}

interface RuleView extends Rule {
  key: string
  state: {
    status: string
    publicAddr?: string
    localAddr?: string
    since?: string
    error?: string
    mapping?: string
    stats: { activeConns: number; totalConns: number; bytesIn: number; bytesOut: number }
  }
}

interface Settings {
  enable: boolean
  stunServers: string[]
  keepAliveServers: string[]
  webhook: WebhookConfig
  addrCheckInterval: number
}

const vars = ['ruleName', 'time', 'ip', 'port', 'ipAddr']
const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
// moduleEnabled 为模块全局开关，undefined 表示尚未加载
const moduleEnabled = ref<boolean>()
const moduleOff = computed(() => moduleEnabled.value === false)
const enabling = ref(false)
const rules = ref<RuleView[]>([])
const interfaces = ref<NetInterface[]>([])
const loading = ref(true)
const compact = useCompactView('stun')
// 列表的搜索、分组与排序；顺序与分组的修改不会重启规则
const gl = useGroupedList<RuleView>({
  storageKey: 'stun.groups',
  items: () => rules.value,
  base: '/api/stun/rules',
  reload: load,
  matches: (r, q) =>
    [r.name, r.group ?? '', r.remark, r.state.publicAddr ?? '', ...(r.targetAddresses ?? [])].join('\n').toLowerCase().includes(q),
})
const editorVisible = ref(false)
const saving = ref(false)
const tab = ref('basic')
const editing = ref<Rule>(emptyRule())
const settingsVisible = ref(false)
const settings = ref<Settings>()
// 定时刷新，标签页在后台时暂停
const polling = usePolling(load, 3000)

// 输入框保留原始文本，回车、逗号可正常输入（见 useListText）
const lines = { split: splitLinesOrCommas }
const targetsText = useListText(() => editing.value.targetAddresses, (v) => (editing.value.targetAddresses = v), { ...lines, separator: ', ' })
const serversText = useListText(() => editing.value.stunServers, (v) => (editing.value.stunServers = v), lines)
const keepAliveText = useListText(() => editing.value.keepAliveServers, (v) => (editing.value.keepAliveServers = v), lines)
const settingsServers = useListText(
  () => settings.value?.stunServers,
  (v) => {
    if (settings.value) {
      settings.value.stunServers = v
    }
  },
  lines,
)
const settingsKeepAlive = useListText(
  () => settings.value?.keepAliveServers,
  (v) => {
    if (settings.value) {
      settings.value.keepAliveServers = v
    }
  },
  lines,
)

function emptyRule(): Rule {
  return {
    name: '',
    enable: true,
    protocol: 'tcp4',
    bind: { mode: 'ip', ip: '', interface: '', regex: '' },
    localPort: 0,
    useGlobalServers: true,
    stunServers: [],
    keepAliveServers: [],
    disableForward: false,
    targetAddresses: [],
    targetPort: 0,
    upnp: false,
    upnpGateway: '',
    upnpLocalHost: '',
    upnpControlUrl: '',
    upnpInternalPort: 0,
    natpmp: false,
    natpmpGateway: '',
    heartbeatInterval: 3000,
    stunTimeout: 3000,
    retryInterval: 3000,
    autoRetry: true,
    disableCheck: false,
    useGlobalWebhook: false,
    webhook: { enable: false, url: '', method: 'GET', headers: [], body: '', successContent: [], timeout: 0, insecureSkipVerify: false },
    webhookOnlyOnChange: true,
    scriptEnable: false,
    script: '',
    maxConnections: 0,
    udpSessionTimeout: 0,
    remark: '',
  }
}

// ruleStatus 计算规则展示状态：停用的规则只显示“已停用”
function ruleStatus(r: RuleView): { type: StatusType; text: string; title?: string } {
  if (moduleOff.value) {
    return { type: 'off', text: t('stun.moduleOff') }
  }
  if (!r.enable) {
    return { type: 'off', text: t('stun.deactivated') }
  }
  const map: Record<string, { type: StatusType; text: string }> = {
    ok: { type: 'running', text: t('stun.stNone') },
    connecting: { type: 'pending', text: t('stun.stConnecting') },
    retrying: { type: 'warn', text: t('stun.stRetrying') },
    failed: { type: 'error', text: t('stun.stFailed') },
    stopped: { type: 'warn', text: t('stun.stStopped') },
  }
  return { ...(map[r.state.status] ?? { type: 'info', text: r.state.status }), title: r.state.error }
}

function fmtTime(iso: string) {
  return new Date(iso).toLocaleString()
}

// 点击公网地址复制；自动复制失败时选中被点击的地址供手动复制
function copy(s: string, e: MouseEvent) {
  copyText(s, { el: e.currentTarget as HTMLElement, success: t('stun.copied', { text: s }) })
}

async function load() {
  try {
    rules.value = await api.get<RuleView[]>('/api/stun/rules')
  } finally {
    loading.value = false
  }
}

function openEditor(row?: RuleView) {
  if (row) {
    const { state: _s, ...rule } = row
    editing.value = JSON.parse(JSON.stringify(rule))
  } else {
    editing.value = emptyRule()
  }
  tab.value = 'basic'
  editorVisible.value = true
}

async function save() {
  saving.value = true
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/stun/rules/${key}`, body)
    } else {
      await api.post('/api/stun/rules', body)
    }
    editorVisible.value = false
    ElMessage.success(t('common.saved'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function toggle(row: RuleView, enable: boolean) {
  try {
    await api.put(`/api/stun/rules/${row.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function restart(row: RuleView) {
  try {
    await api.post(`/api/stun/rules/${row.key}/restart`)
    ElMessage.success(t('stun.reconnected'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(row: RuleView) {
  try {
    await api.del(`/api/stun/rules/${row.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function loadModule() {
  const s = await api.get<Settings>('/api/stun/settings')
  moduleEnabled.value = s.enable
  return s
}

async function openSettings() {
  settings.value = await loadModule()
  settingsVisible.value = true
}

// enableModule 在横幅中一键启用模块
async function enableModule() {
  enabling.value = true
  try {
    const s = await loadModule()
    await api.put('/api/stun/settings', { ...s, enable: true })
    ElMessage.success(t('stun.moduleEnabled'))
    await Promise.all([loadModule(), load()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    enabling.value = false
  }
}

async function saveSettings() {
  try {
    await api.put('/api/stun/settings', settings.value)
    settingsVisible.value = false
    ElMessage.success(t('stun.settingsSaved'))
    await Promise.all([loadModule(), load()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(async () => {
  loadModule()
  interfaces.value = await api.get<NetInterface[]>('/api/stun/netinterfaces')
  await load()
  polling.start()
})
</script>

<style scoped>
.intro {
  margin-bottom: 16px;
}
.err-line {
  font-size: 12px;
  color: var(--el-color-danger);
  word-break: break-all;
}
.iface {
  float: right;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-left: 16px;
}
</style>
