<template>
  <div>
    <PageHeader :title="t('portForward.title')" :desc="t('portForward.desc')" :icon="ArrowLeftRight">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('portForward.filterPlaceholder')" />
      <el-button :icon="SlidersHorizontal" @click="openSettings">{{ t('portForward.globalSettings') }}</el-button>
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('portForward.addRule') }}</el-button>
    </PageHeader>

    <ModuleBanner v-if="settings && !settings.enable" :title="t('portForward.moduleOffTitle')" :loading="enabling" @enable="enableModule" />

    <GroupedList :gl="gl" :loading="loading" :compact="compact" :empty-text="t('portForward.empty')" :count-text="(n: number) => t('portForward.groupCount', { count: n })">
      <template #item="{ it, sorting, bind }">
      <EntityCard
        v-bind="bind"
        :compact="compact || sorting"
        :off="!it.item.enable || moduleOff"
      >
        <template #lead>
          <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
          <el-switch v-else :model-value="it.item.enable" @change="(v: string | number | boolean) => toggle(it.item, Boolean(v))" />
        </template>
        <template #title>
          <span>{{ it.item.name || t('portForward.unnamed') }}</span>
          <span class="tags">
            <el-tag v-if="it.item.mode === 'sni'" size="small" type="success" effect="plain">{{ t('portForward.modeSni') }}</el-tag>
            <el-tag v-for="p in it.item.protocols" :key="p" size="small" effect="plain" class="mono">{{ p.toUpperCase() }}</el-tag>
          </span>
        </template>
        <template v-if="it.item.remark" #sub>{{ it.item.remark }}</template>
        <template #status>
          <el-tooltip v-if="it.item.details?.length && it.item.enable" placement="top">
            <template #content>
              <div v-for="d in it.item.details" :key="d">{{ d }}</div>
            </template>
            <TriangleAlert :size="16" class="detail" />
          </el-tooltip>
          <StatusBadge v-bind="ruleStatus(it.item)" />
        </template>
        <Field :label="t('portForward.listen')" mono>{{ listenText(it.item) }}</Field>
        <template v-if="it.item.mode === 'sni'">
          <Field :label="t('portForward.sniRoutes')">
            <div v-for="(rt, i) in it.item.sniRoutes" :key="i" class="route-line">
              <span class="route-name">{{ rt.name || rt.domains[0] }}</span>
              <span class="mono">→ {{ routeTargetText(it.item, rt) }}</span>
              <el-tag v-if="rt.proxyProtocol" size="small" effect="plain">{{ rt.proxyProtocol }}</el-tag>
              <span class="route-meta">{{ t('portForward.routeMeta', { domains: rt.domains.length, hits: it.item.stats.routeConns?.[i] ?? 0 }) }}</span>
            </div>
          </Field>
          <Field :label="t('portForward.defaultTarget')" mono>
            <template v-if="it.item.targetAddresses?.length">
              <div v-for="addr in it.item.targetAddresses" :key="addr">
                {{ addr }}:{{ it.item.targetPorts || it.item.listenPorts }}
                <el-tag v-if="it.item.proxyProtocol" size="small" effect="plain">{{ it.item.proxyProtocol }}</el-tag>
              </div>
              <span class="route-meta">{{ t('portForward.defaultHits', { hits: it.item.stats.defaultConns ?? 0 }) }}</span>
            </template>
            <span v-else class="route-meta">{{ t('portForward.defaultTargetNone', { rejected: it.item.stats.sniRejected ?? 0 }) }}</span>
          </Field>
        </template>
        <Field v-else :label="t('portForward.target')" mono>
          <div v-for="addr in it.item.targetAddresses" :key="addr">{{ addr }}:{{ it.item.targetPorts || it.item.listenPorts }}</div>
        </Field>
        <Field :label="t('portForward.connections')">{{ t('portForward.connectionsValue', { active: it.item.stats.activeConns, total: it.item.stats.totalConns }) }}</Field>
        <Field :label="t('portForward.traffic')">↑ {{ formatBytes(it.item.stats.bytesIn) }} · ↓ {{ formatBytes(it.item.stats.bytesOut) }}</Field>
        <template v-if="sorting" #actions>
          <SortMoves :gl="gl" :it="it" />
        </template>
        <template v-else #actions>
          <el-button text size="small" :icon="Pencil" @click="openEditor(it.item)">{{ t('common.edit') }}</el-button>
          <el-popconfirm :title="t('portForward.deleteConfirm')" @confirm="remove(it.item)">
            <template #reference>
              <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
            </template>
          </el-popconfirm>
        </template>
      </EntityCard>
      </template>
    </GroupedList>

    <LogCard url="/api/portforward/logs" clearable />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('portForward.editRule') : t('portForward.addRule')" :width="editing.mode === 'sni' ? '760px' : '600px'" :fullscreen="isMobile">
      <el-form :model="editing" label-width="110px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')">
          <el-input v-model="editing.name" :placeholder="t('portForward.namePlaceholder')" />
        </el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('portForward.mode')">
          <el-radio-group v-model="editing.mode" @change="onModeChange">
            <el-radio-button value="">{{ t('portForward.modeForward') }}</el-radio-button>
            <el-radio-button value="sni">{{ t('portForward.modeSni') }}</el-radio-button>
          </el-radio-group>
          <div class="form-help">{{ editing.mode === 'sni' ? t('portForward.modeSniHelp') : t('portForward.modeForwardHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('portForward.protocol')" required>
          <el-checkbox-group v-model="editing.protocols">
            <el-checkbox v-for="p in availableProtocols" :key="p.value" :value="p.value">{{ p.label }}</el-checkbox>
          </el-checkbox-group>
          <div class="form-help">{{ t('portForward.protocolHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('portForward.listenAddress')">
          <el-input v-model="editing.listenAddress" :placeholder="t('portForward.listenAddressPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('portForward.listenPorts')" required>
          <el-input v-model="editing.listenPorts" :placeholder="t('portForward.portsPlaceholder')" />
        </el-form-item>
        <template v-if="editing.mode === 'sni'">
          <el-form-item :label="t('portForward.sniRoutes')" required>
            <div class="routes">
              <div v-for="(rt, i) in routeForms" :key="rt.id" class="route-card">
                <div class="route-head">
                  <el-input v-model="rt.name" size="small" :placeholder="t('portForward.routeNamePlaceholder')" class="route-title" />
                  <el-button text size="small" :icon="ArrowUp" :disabled="i === 0" @click="moveRoute(i, -1)" />
                  <el-button text size="small" :icon="ArrowDown" :disabled="i === routeForms.length - 1" @click="moveRoute(i, 1)" />
                  <el-button text size="small" type="danger" :icon="Trash2" @click="routeForms.splice(i, 1)" />
                </div>
                <el-input v-model="rt.domainsText" type="textarea" :autosize="{ minRows: 2, maxRows: 8 }" :placeholder="t('portForward.routeDomainsPlaceholder')" />
                <div class="route-row">
                  <el-input v-model="rt.targetsText" :placeholder="t('portForward.routeTargetsPlaceholder')" class="route-targets" />
                  <el-input-number v-model="rt.targetPort" :min="0" :max="65535" :controls="false" :placeholder="t('portForward.routePortPlaceholder')" class="route-port" />
                  <el-select v-model="rt.proxyProtocol" class="route-proxy">
                    <el-option :label="t('portForward.proxyProtocolOff')" value="" />
                    <el-option label="PROXY v1" value="v1" />
                    <el-option label="PROXY v2" value="v2" />
                  </el-select>
                </div>
              </div>
              <el-button :icon="Plus" size="small" @click="addRoute">{{ t('portForward.addRoute') }}</el-button>
              <div class="form-help">{{ t('portForward.sniRoutesHelp') }}</div>
            </div>
          </el-form-item>
          <el-form-item :label="t('portForward.defaultTarget')">
            <el-input v-model="targetsText" type="textarea" :rows="2" :placeholder="t('portForward.defaultTargetPlaceholder')" />
          </el-form-item>
          <el-form-item :label="t('portForward.targetPorts')">
            <el-input v-model="editing.targetPorts" :placeholder="t('portForward.targetPortsPlaceholder')" />
          </el-form-item>
        </template>
        <template v-else>
          <el-form-item :label="t('portForward.targetAddress')" required>
            <el-input v-model="targetsText" type="textarea" :rows="2" :placeholder="t('portForward.targetAddressPlaceholder')" />
          </el-form-item>
          <el-form-item :label="t('portForward.targetPorts')">
            <el-input v-model="editing.targetPorts" :placeholder="t('portForward.targetPortsPlaceholder')" />
          </el-form-item>
        </template>
        <el-form-item :label="t('portForward.maxConnections')">
          <el-input-number v-model="editing.maxConnections" :min="0" :max="100000" />
          <span class="help">{{ t('portForward.useGlobalDefault') }}</span>
        </el-form-item>
        <el-form-item v-if="editing.mode !== 'sni'" :label="t('portForward.udpTimeout')">
          <el-input-number v-model="editing.udpSessionTimeout" :min="0" :max="3600" />
          <span class="help">{{ t('portForward.useGlobalDefault') }}</span>
        </el-form-item>
        <el-form-item :label="editing.mode === 'sni' ? t('portForward.defaultProxyProtocol') : t('portForward.proxyProtocol')">
          <el-select v-model="editing.proxyProtocol" style="width: 160px">
            <el-option :label="t('portForward.proxyProtocolOff')" value="" />
            <el-option label="PROXY v1" value="v1" />
            <el-option label="PROXY v2" value="v2" />
          </el-select>
          <div class="help">{{ t('portForward.proxyProtocolHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('common.remark')">
          <el-input v-model="editing.remark" />
        </el-form-item>
        <el-form-item :label="t('common.enable')">
          <el-switch v-model="editing.enable" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="settingsVisible" :title="t('portForward.settingsTitle')" width="520px" :fullscreen="isMobile">
      <el-form v-if="settingsForm" :model="settingsForm" label-width="150px" :label-position="labelPosition">
        <el-form-item :label="t('portForward.enableModule')"><el-switch v-model="settingsForm.enable" /></el-form-item>
        <el-form-item :label="t('portForward.defaultMaxConnections')"><el-input-number v-model="settingsForm.defaultMaxConnections" :min="1" /></el-form-item>
        <el-form-item :label="t('portForward.defaultUdpTimeout')"><el-input-number v-model="settingsForm.defaultUDPSessionTimeout" :min="1" /></el-form-item>
        <el-form-item :label="t('portForward.maxUdpSessions')"><el-input-number v-model="settingsForm.maxUDPSessions" :min="1" /></el-form-item>
        <el-form-item :label="t('portForward.dialTimeout')"><el-input-number v-model="settingsForm.dialTimeout" :min="1" :max="60" /></el-form-item>
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
import {
  ArrowDown,
  ArrowLeftRight,
  ArrowUp,
  Pencil,
  Plus,
  SlidersHorizontal,
  Trash2,
  TriangleAlert,
} from 'lucide-vue-next'
import { api, formatBytes } from '../api'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import { useGroupedList } from '../composables/useGroupSort'
import PageHeader from '../components/PageHeader.vue'
import ModuleBanner from '../components/ModuleBanner.vue'
import CompactToggle from '../components/CompactToggle.vue'
import EntityCard from '../components/EntityCard.vue'
import { useCompactView } from '../composables/useCompactView'
import Field from '../components/Field.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'

interface Rule {
  key?: string
  name: string
  enable: boolean
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  protocols: string[]
  listenAddress: string
  listenPorts: string
  targetAddresses: string[]
  targetPorts: string
  maxConnections: number
  udpSessionTimeout: number
  // 发送给目标的 PROXY 协议版本，空为不发送（仅 TCP）
  proxyProtocol: string
  // 空为普通转发，sni 为按 TLS SNI 分流；SNI 模式下 targetAddresses 等字段作为默认目标
  mode: string
  sniRoutes: SNIRoute[]
  remark: string
}

interface SNIRoute {
  name: string
  domains: string[]
  targetAddresses: string[]
  // 0 表示与监听端口相同
  targetPort: number
  proxyProtocol: string
}

// RouteForm 是编辑中的路由，域名与目标以文本形式编辑
interface RouteForm {
  id: number
  name: string
  domainsText: string
  targetsText: string
  targetPort: number | undefined
  proxyProtocol: string
}

interface RuleView extends Rule {
  running: boolean
  error?: string
  details?: string[]
  stats: {
    activeConns: number
    totalConns: number
    bytesIn: number
    bytesOut: number
    // 仅 SNI 规则：各路由累计连接、默认目标累计连接、未命中被断开的连接
    routeConns?: number[]
    defaultConns?: number
    sniRejected?: number
  }
}

interface Settings {
  enable: boolean
  defaultMaxConnections: number
  defaultUDPSessionTimeout: number
  maxUDPSessions: number
  dialTimeout: number
}

const protocols = [
  { value: 'tcp', label: 'TCP' },
  { value: 'udp', label: 'UDP' },
  { value: 'tcp4', label: 'TCP4' },
  { value: 'udp4', label: 'UDP4' },
  { value: 'tcp6', label: 'TCP6' },
  { value: 'udp6', label: 'UDP6' },
]

const { t } = useI18n()
const compact = useCompactView('portforward')
const { isMobile, labelPosition } = useFormLayout()
const rules = ref<RuleView[]>([])
const loading = ref(true)
const editorVisible = ref(false)
const saving = ref(false)
const editing = ref<Rule>(emptyRule())
const targetsText = ref('')
const routeForms = ref<RouteForm[]>([])
let routeSeq = 0
const settings = ref<Settings>()
const settingsForm = ref<Settings>()
const settingsVisible = ref(false)
const enabling = ref(false)
// 定时刷新，标签页在后台时暂停
const polling = usePolling(load, 3000)

const moduleOff = computed(() => settings.value?.enable === false)
// 列表的筛选、分组与排序；顺序与分组的修改不会重启转发
const gl = useGroupedList<RuleView>({
  storageKey: 'portforward.groups',
  items: () => rules.value,
  base: '/api/portforward/rules',
  reload: load,
  matches: (r, q) =>
    [r.name, r.group ?? '', r.remark, listenText(r), ...(r.targetAddresses ?? []), ...(r.sniRoutes ?? []).flatMap((rt) => [rt.name, ...rt.domains])]
      .join('\n')
      .toLowerCase()
      .includes(q),
})
// SNI 分流只处理 TCP
const availableProtocols = computed(() => (editing.value.mode === 'sni' ? protocols.filter((p) => p.value.startsWith('tcp')) : protocols))

function emptyRule(): Rule {
  return {
    name: '',
    enable: true,
    group: '',
    protocols: ['tcp'],
    listenAddress: '',
    listenPorts: '',
    targetAddresses: [],
    targetPorts: '',
    maxConnections: 0,
    udpSessionTimeout: 0,
    proxyProtocol: '',
    mode: '',
    sniRoutes: [],
    remark: '',
  }
}

// splitList 按换行、逗号、空格拆分文本，用于域名与地址列表
function splitList(text: string) {
  return text
    .split(/[\s,，]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}

function toRouteForm(rt?: SNIRoute): RouteForm {
  return {
    id: ++routeSeq,
    name: rt?.name ?? '',
    domainsText: (rt?.domains ?? []).join('\n'),
    targetsText: (rt?.targetAddresses ?? []).join(', '),
    targetPort: rt?.targetPort || undefined,
    proxyProtocol: rt?.proxyProtocol ?? '',
  }
}

function addRoute() {
  routeForms.value.push(toRouteForm())
}

function moveRoute(i: number, delta: number) {
  const list = routeForms.value
  const [item] = list.splice(i, 1)
  list.splice(i + delta, 0, item)
}

// onModeChange 切到 SNI 时去掉 UDP 协议，并预置一条空路由
function onModeChange() {
  if (editing.value.mode === 'sni') {
    editing.value.protocols = editing.value.protocols.filter((p) => p.startsWith('tcp'))
    if (!editing.value.protocols.length) {
      editing.value.protocols = ['tcp']
    }
    if (!routeForms.value.length) {
      addRoute()
    }
  }
}

function routeTargetText(r: Rule, rt: SNIRoute) {
  const port = rt.targetPort || r.listenPorts
  return rt.targetAddresses.map((a) => `${a.includes(':') ? `[${a}]` : a}:${port}`).join(' | ')
}

function listenText(r: Rule) {
  const host = r.listenAddress ? (r.listenAddress.includes(':') ? `[${r.listenAddress}]` : r.listenAddress) : '*'
  return `${host}:${r.listenPorts}`
}

// ruleStatus 计算规则的展示状态：停用的规则只显示“已停用”，不显示为异常
function ruleStatus(r: RuleView): { type: StatusType; text: string; title?: string } {
  if (moduleOff.value) {
    return { type: 'off', text: t('portForward.moduleOff') }
  }
  if (!r.enable) {
    return { type: 'off', text: t('portForward.statusOff') }
  }
  if (r.error) {
    return { type: 'error', text: r.error, title: r.error }
  }
  if (r.running) {
    return { type: 'running', text: t('common.running') }
  }
  return { type: 'warn', text: t('portForward.statusNotRunning') }
}

async function load() {
  try {
    rules.value = await api.get<RuleView[]>('/api/portforward/rules')
  } finally {
    loading.value = false
  }
}

async function loadSettings() {
  settings.value = await api.get<Settings>('/api/portforward/settings')
}

function openEditor(row?: RuleView) {
  if (row) {
    const { running: _r, error: _e, details: _d, stats: _s, ...rule } = row
    editing.value = JSON.parse(JSON.stringify(rule))
  } else {
    editing.value = emptyRule()
  }
  editing.value.mode = editing.value.mode || ''
  targetsText.value = (editing.value.targetAddresses ?? []).join('\n')
  routeForms.value = (editing.value.sniRoutes ?? []).map(toRouteForm)
  editorVisible.value = true
}

async function save() {
  editing.value.targetAddresses = targetsText.value
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean)
  editing.value.sniRoutes =
    editing.value.mode === 'sni'
      ? routeForms.value.map((f) => ({
          name: f.name.trim(),
          domains: splitList(f.domainsText),
          targetAddresses: splitList(f.targetsText),
          targetPort: f.targetPort || 0,
          proxyProtocol: f.proxyProtocol,
        }))
      : []
  saving.value = true
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/portforward/rules/${key}`, body)
    } else {
      await api.post('/api/portforward/rules', body)
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
    await api.put(`/api/portforward/rules/${row.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(row: RuleView) {
  try {
    await api.del(`/api/portforward/rules/${row.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function openSettings() {
  await loadSettings()
  settingsForm.value = JSON.parse(JSON.stringify(settings.value))
  settingsVisible.value = true
}

async function saveSettings() {
  try {
    await api.put('/api/portforward/settings', settingsForm.value)
    settingsVisible.value = false
    ElMessage.success(t('portForward.savedRestarted'))
    await Promise.all([loadSettings(), load()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// enableModule 在横幅中一键启用模块
async function enableModule() {
  if (!settings.value) {
    return
  }
  enabling.value = true
  try {
    await api.put('/api/portforward/settings', { ...settings.value, enable: true })
    ElMessage.success(t('portForward.moduleEnabled'))
    await Promise.all([loadSettings(), load()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    enabling.value = false
  }
}

onMounted(() => {
  load()
  loadSettings()
  // 定时刷新连接数与流量
  polling.start()
})
</script>

<style scoped>
.w-260 {
  width: 260px;
}
@media (max-width: 767.98px) {
  .w-260 {
    width: 100%;
  }
}
.detail {
  color: var(--el-color-warning);
}
.route-line {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.route-name {
  font-weight: 500;
}
.route-meta {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.routes {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
}
.route-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
}
.route-head,
.route-row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.route-title,
.route-targets {
  flex: 1;
  min-width: 0;
}
.route-port {
  width: 110px;
}
.route-proxy {
  width: 120px;
}
</style>
