<template>
  <div>
    <PageHeader :title="t('protoRoute.title')" :desc="t('protoRoute.desc')" :icon="Split">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('protoRoute.filterPlaceholder')" />
      <el-button :icon="SlidersHorizontal" @click="openSettings">{{ t('protoRoute.globalSettings') }}</el-button>
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('protoRoute.addRule') }}</el-button>
    </PageHeader>

    <ModuleBanner v-if="settings && !settings.enable" :title="t('protoRoute.moduleOffTitle')" :loading="enabling" @enable="enableModule" />

    <GroupedList :gl="gl" :loading="loading" :compact="compact" :empty-text="t('protoRoute.empty')" :count-text="(n: number) => t('protoRoute.groupCount', { count: n })">
      <template #item="{ it, sorting, bind }">
        <EntityCard v-bind="bind" :compact="compact || sorting" :off="!it.item.enable || moduleOff">
          <template #lead>
            <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
            <el-switch v-else :model-value="it.item.enable" @change="(v: string | number | boolean) => toggle(it.item, Boolean(v))" />
          </template>
          <template #title>
            <span>{{ it.item.name || t('protoRoute.unnamed') }}</span>
            <span class="tags">
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
          <Field :label="t('protoRoute.listen')" mono>{{ listenText(it.item) }}</Field>
          <Field :label="t('protoRoute.defaultTarget')" mono>
            <template v-if="it.item.targetAddresses?.length">
              <div v-for="addr in it.item.targetAddresses" :key="addr">
                {{ addr }}:{{ it.item.targetPorts || it.item.listenPorts }}
                <el-tag v-if="it.item.proxyProtocol" size="small" effect="plain">{{ it.item.proxyProtocol }}</el-tag>
              </div>
              <span class="route-meta">{{ t('protoRoute.hits', { hits: it.item.stats.defaultConns ?? 0 }) }}</span>
            </template>
            <span v-else class="route-meta">{{ t('protoRoute.defaultTargetNone', { rejected: it.item.stats.rejected ?? 0 }) }}</span>
          </Field>
          <Field :label="t('protoRoute.connections')">{{ t('protoRoute.connectionsValue', { active: it.item.stats.activeConns, total: it.item.stats.totalConns }) }}</Field>
          <Field :label="t('protoRoute.traffic')">↑ {{ formatBytes(it.item.stats.bytesIn) }} · ↓ {{ formatBytes(it.item.stats.bytesOut) }}</Field>
          <Field :label="t('protoRoute.routes')" wide>
            <div v-for="(rt, i) in it.item.routes" :key="i" class="route-line">
              <el-tag size="small" type="info" effect="plain">{{ protoShort(rt.protocol) }}</el-tag>
              <span class="route-name">{{ routeTitle(rt) }}</span>
              <span class="mono">→ {{ routeTargetText(it.item, rt) }}</span>
              <el-tag v-if="rt.proxyProtocol" size="small" effect="plain">{{ rt.proxyProtocol }}</el-tag>
              <el-tag v-for="a in rt.alpn ?? []" :key="a" size="small" effect="plain">{{ a }}</el-tag>
              <span class="route-meta">{{
                rt.domains?.length
                  ? t('protoRoute.routeMeta', { domains: rt.domains.length, hits: it.item.stats.routeConns?.[i] ?? 0 })
                  : t('protoRoute.hits', { hits: it.item.stats.routeConns?.[i] ?? 0 })
              }}</span>
            </div>
            <span v-if="!it.item.routes?.length" class="route-meta">{{ t('protoRoute.noRoutes') }}</span>
          </Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="Pencil" @click="openEditor(it.item)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('protoRoute.deleteConfirm')" @confirm="remove(it.item)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <LogCard url="/api/protoroute/logs" clearable />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('protoRoute.editRule') : t('protoRoute.addRule')" width="760px" :fullscreen="isMobile">
      <el-form :model="editing" label-width="110px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')">
          <el-input v-model="editing.name" :placeholder="t('protoRoute.namePlaceholder')" />
        </el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('protoRoute.protocol')" required>
          <el-checkbox-group v-model="editing.protocols">
            <el-checkbox v-for="p in protocols" :key="p.value" :value="p.value">{{ p.label }}</el-checkbox>
          </el-checkbox-group>
          <div class="form-help">{{ t('protoRoute.protocolHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('protoRoute.listenAddress')">
          <el-input v-model="editing.listenAddress" :placeholder="t('protoRoute.listenAddressPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('protoRoute.listenPorts')" required>
          <el-input v-model="editing.listenPorts" :placeholder="t('protoRoute.portsPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('protoRoute.routes')">
          <div class="routes">
            <div v-for="(rt, i) in routeForms" :key="rt.id" class="route-card">
              <div class="route-head">
                <span class="route-index">{{ i + 1 }}</span>
                <el-select v-model="rt.protocol" size="small" class="route-proto">
                  <el-option v-for="p in routeProtocols" :key="p" :label="protoLabel(p)" :value="p" />
                </el-select>
                <el-input v-model="rt.name" size="small" :placeholder="t('protoRoute.routeNamePlaceholder')" class="route-title" />
                <el-button text size="small" :icon="ArrowUp" :disabled="i === 0" @click="moveRoute(i, -1)" />
                <el-button text size="small" :icon="ArrowDown" :disabled="i === routeForms.length - 1" @click="moveRoute(i, 1)" />
                <el-button text size="small" type="danger" :icon="Trash2" @click="routeForms.splice(i, 1)" />
              </div>
              <el-input
                v-if="usesDomains(rt.protocol)"
                v-model="rt.domainsText"
                type="textarea"
                :autosize="{ minRows: 2, maxRows: 8 }"
                :placeholder="domainsPlaceholder(rt.protocol)"
              />
              <el-input v-if="rt.protocol === 'tls'" v-model="rt.alpnText" :placeholder="t('protoRoute.routeAlpnPlaceholder')" />
              <div v-if="protoHelp(rt.protocol)" class="form-help">{{ protoHelp(rt.protocol) }}</div>
              <div class="route-row">
                <el-input v-model="rt.targetsText" :placeholder="t('protoRoute.routeTargetsPlaceholder')" class="route-targets" />
                <el-input-number v-model="rt.targetPort" :min="0" :max="65535" :controls="false" :placeholder="t('protoRoute.routePortPlaceholder')" class="route-port" />
                <el-select v-model="rt.proxyProtocol" class="route-proxy" :empty-values="[null, undefined]">
                  <el-option :label="t('protoRoute.proxyProtocolOff')" value="" />
                  <el-option label="PROXY v1" value="v1" />
                  <el-option label="PROXY v2" value="v2" />
                </el-select>
              </div>
            </div>
            <el-button :icon="Plus" size="small" @click="addRoute">{{ t('protoRoute.addRoute') }}</el-button>
            <div class="form-help">{{ t('protoRoute.routesHelp') }}</div>
          </div>
        </el-form-item>
        <el-form-item :label="t('protoRoute.defaultTarget')">
          <el-input v-model="targetsText" type="textarea" :rows="2" :placeholder="t('protoRoute.defaultTargetPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('protoRoute.targetPorts')">
          <el-input v-model="editing.targetPorts" :placeholder="t('protoRoute.targetPortsPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('protoRoute.defaultProxyProtocol')">
          <el-select v-model="editing.proxyProtocol" style="width: 160px" :empty-values="[null, undefined]">
            <el-option :label="t('protoRoute.proxyProtocolOff')" value="" />
            <el-option label="PROXY v1" value="v1" />
            <el-option label="PROXY v2" value="v2" />
          </el-select>
          <div class="help">{{ t('protoRoute.proxyProtocolHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('protoRoute.sniffTimeout')">
          <el-input-number v-model="editing.sniffTimeout" :min="0" :max="60" :step="0.1" :precision="1" />
          <div class="form-help">{{ t('protoRoute.sniffTimeoutHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('protoRoute.maxConnections')">
          <el-input-number v-model="editing.maxConnections" :min="0" :max="100000" />
          <span class="help">{{ t('protoRoute.useGlobalDefault') }}</span>
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

    <el-dialog v-model="settingsVisible" :title="t('protoRoute.settingsTitle')" width="520px" :fullscreen="isMobile">
      <el-form v-if="settingsForm" :model="settingsForm" label-width="150px" :label-position="labelPosition">
        <el-form-item :label="t('protoRoute.enableModule')"><el-switch v-model="settingsForm.enable" /></el-form-item>
        <el-form-item :label="t('protoRoute.defaultMaxConnections')"><el-input-number v-model="settingsForm.defaultMaxConnections" :min="1" /></el-form-item>
        <el-form-item :label="t('protoRoute.dialTimeout')"><el-input-number v-model="settingsForm.dialTimeout" :min="1" :max="60" /></el-form-item>
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
import { ArrowDown, ArrowUp, Pencil, Plus, SlidersHorizontal, Split, Trash2, TriangleAlert } from 'lucide-vue-next'
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
  routes: Route[]
  // 默认目标：未命中路由或无法识别协议时使用，地址留空表示直接断开
  targetAddresses: string[]
  targetPorts: string
  proxyProtocol: string
  // 等待客户端发来可识别数据的秒数，可带一位小数，0 为默认 5 秒
  sniffTimeout: number
  maxConnections: number
  remark: string
}

interface Route {
  name: string
  // 路由匹配的协议，见 routeProtocols
  protocol: string
  // 仅 tls（SNI）与 http（Host）使用，留空匹配该协议的全部连接
  domains: string[]
  // 仅 tls 使用，客户端声明其中任一 ALPN 时匹配
  alpn: string[]
  targetAddresses: string[]
  // 0 表示与监听端口相同
  targetPort: number
  proxyProtocol: string
}

// RouteForm 是编辑中的路由，域名、ALPN 与目标以文本形式编辑
interface RouteForm {
  id: number
  name: string
  protocol: string
  domainsText: string
  alpnText: string
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
    // 各路由累计连接、默认目标累计连接、未命中且没有默认目标被断开的连接
    routeConns?: number[]
    defaultConns?: number
    rejected?: number
  }
}

interface Settings {
  enable: boolean
  defaultMaxConnections: number
  dialTimeout: number
}

// 分流路由可识别的协议，顺序即下拉列表的顺序
const routeProtocols = [
  'tls',
  'http',
  'ssh',
  'rdp',
  'socks',
  'openvpn',
  'minecraft',
  'rtsp',
  'sip',
  'xmpp',
  'mqtt',
  'amqp',
  'postgresql',
  'redis',
  'smb',
  'git',
  'irc',
  'adb',
  'bittorrent',
  'timeout',
]

// 协议分流只处理 TCP
const protocols = [
  { value: 'tcp', label: 'TCP' },
  { value: 'tcp4', label: 'TCP4' },
  { value: 'tcp6', label: 'TCP6' },
]

const { t, te } = useI18n()
const compact = useCompactView('protoroute')
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
// 列表的筛选、分组与排序；顺序与分组的修改不会重启监听
const gl = useGroupedList<RuleView>({
  storageKey: 'protoroute.groups',
  items: () => rules.value,
  base: '/api/protoroute/rules',
  reload: load,
  matches: (r, q) =>
    [
      r.name,
      r.group ?? '',
      r.remark,
      listenText(r),
      ...(r.targetAddresses ?? []),
      ...(r.routes ?? []).flatMap((rt) => [rt.name, protoLabel(rt.protocol), ...(rt.domains ?? []), ...(rt.alpn ?? []), ...(rt.targetAddresses ?? [])]),
    ]
      .join('\n')
      .toLowerCase()
      .includes(q),
})

function emptyRule(): Rule {
  return {
    name: '',
    enable: true,
    group: '',
    protocols: ['tcp'],
    listenAddress: '',
    listenPorts: '',
    routes: [],
    targetAddresses: [],
    targetPorts: '',
    proxyProtocol: '',
    sniffTimeout: 0,
    maxConnections: 0,
    remark: '',
  }
}

// protoLabel 返回路由协议在下拉列表中的完整名称
function protoLabel(p?: string) {
  return t(`protoRoute.proto.${p || 'tls'}`)
}

// protoShort 是卡片上路由的协议短名，等待超时需要翻译，其他协议名直接显示
function protoShort(p?: string) {
  const names: Record<string, string> = {
    tls: 'TLS',
    http: 'HTTP',
    ssh: 'SSH',
    rdp: 'RDP',
    socks: 'SOCKS',
    openvpn: 'OpenVPN',
    xmpp: 'XMPP',
    mqtt: 'MQTT',
    postgresql: 'PostgreSQL',
    rtsp: 'RTSP',
    sip: 'SIP',
    minecraft: 'Minecraft',
    redis: 'Redis',
    amqp: 'AMQP',
    smb: 'SMB',
    git: 'Git',
    bittorrent: 'BitTorrent',
    adb: 'ADB',
    irc: 'IRC',
  }
  return p === 'timeout' ? t('protoRoute.protoTimeoutShort') : (names[p || 'tls'] ?? p)
}

// protoHelp 返回编辑路由时该协议的说明，没有说明时为空
function protoHelp(p: string) {
  const key = `protoRoute.protoHelp.${p}`
  return te(key) ? t(key) : ''
}

// usesDomains 判断协议是否按域名匹配：TLS 看 SNI，HTTP 看 Host，Minecraft 看握手包中的服务器地址
function usesDomains(p: string) {
  return p === 'tls' || p === 'http' || p === 'minecraft'
}

// domainsPlaceholder 返回各协议域名输入框的提示
function domainsPlaceholder(p: string) {
  if (p === 'http') {
    return t('protoRoute.routeHostsPlaceholder')
  }
  if (p === 'minecraft') {
    return t('protoRoute.routeMinecraftPlaceholder')
  }
  return t('protoRoute.routeDomainsPlaceholder')
}

// routeTitle 是卡片上路由的名称：未命名时取第一个域名，再没有则用协议名
function routeTitle(rt: Route) {
  return rt.name || rt.domains?.[0] || protoLabel(rt.protocol)
}

// splitList 按换行、逗号、空格拆分文本，用于域名、ALPN 与地址列表
function splitList(text: string) {
  return text
    .split(/[\s,，]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}

function toRouteForm(rt?: Route): RouteForm {
  return {
    id: ++routeSeq,
    name: rt?.name ?? '',
    protocol: rt?.protocol || 'tls',
    domainsText: (rt?.domains ?? []).join('\n'),
    alpnText: (rt?.alpn ?? []).join(', '),
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

function routeTargetText(r: Rule, rt: Route) {
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
    return { type: 'off', text: t('protoRoute.moduleOff') }
  }
  if (!r.enable) {
    return { type: 'off', text: t('protoRoute.statusOff') }
  }
  if (r.error) {
    return { type: 'error', text: r.error, title: r.error }
  }
  if (r.running) {
    return { type: 'running', text: t('common.running') }
  }
  return { type: 'warn', text: t('protoRoute.statusNotRunning') }
}

async function load() {
  try {
    rules.value = await api.get<RuleView[]>('/api/protoroute/rules')
  } finally {
    loading.value = false
  }
}

async function loadSettings() {
  settings.value = await api.get<Settings>('/api/protoroute/settings')
}

function openEditor(row?: RuleView) {
  if (row) {
    const { running: _r, error: _e, details: _d, stats: _s, ...rule } = row
    editing.value = JSON.parse(JSON.stringify(rule))
  } else {
    editing.value = emptyRule()
  }
  editing.value.sniffTimeout = editing.value.sniffTimeout || 0
  targetsText.value = (editing.value.targetAddresses ?? []).join('\n')
  routeForms.value = (editing.value.routes ?? []).map(toRouteForm)
  // 新规则预置一条空路由
  if (!routeForms.value.length && !row) {
    addRoute()
  }
  editorVisible.value = true
}

async function save() {
  editing.value.targetAddresses = targetsText.value
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean)
  editing.value.routes = routeForms.value.map((f) => ({
    name: f.name.trim(),
    protocol: f.protocol,
    // 不按域名匹配的协议不提交域名与 ALPN，避免切换协议后残留的输入导致校验失败
    domains: usesDomains(f.protocol) ? splitList(f.domainsText) : [],
    alpn: f.protocol === 'tls' ? splitList(f.alpnText) : [],
    targetAddresses: splitList(f.targetsText),
    targetPort: f.targetPort || 0,
    proxyProtocol: f.proxyProtocol,
  }))
  saving.value = true
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/protoroute/rules/${key}`, body)
    } else {
      await api.post('/api/protoroute/rules', body)
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
    await api.put(`/api/protoroute/rules/${row.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(row: RuleView) {
  try {
    await api.del(`/api/protoroute/rules/${row.key}`)
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
    await api.put('/api/protoroute/settings', settingsForm.value)
    settingsVisible.value = false
    ElMessage.success(t('protoRoute.savedRestarted'))
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
    await api.put('/api/protoroute/settings', { ...settings.value, enable: true })
    ElMessage.success(t('protoRoute.moduleEnabled'))
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
  border-left: 3px solid var(--el-color-primary-light-5);
  border-radius: 8px;
  background: var(--el-fill-color-blank);
}
/* 相邻路由交替底色与左侧色条，便于在长列表中分辨每条路由的边界 */
.route-card:nth-child(even) {
  background: var(--el-fill-color-light);
  border-left-color: var(--el-color-success-light-5);
}
.route-index {
  flex-shrink: 0;
  min-width: 22px;
  height: 22px;
  padding: 0 4px;
  border-radius: 11px;
  background: var(--el-color-primary);
  color: var(--el-color-white);
  font-size: 12px;
  font-weight: 600;
  line-height: 22px;
  text-align: center;
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
.route-proto {
  width: 170px;
  flex-shrink: 0;
}
/* 窄屏时协议下拉与目标地址各占一行，名称、端口等放到下一行 */
@media (max-width: 767.98px) {
  .route-head,
  .route-row {
    flex-wrap: wrap;
  }
  /* 序号与协议下拉占第一行，名称和操作按钮换到下一行 */
  .route-proto {
    flex: 1 1 calc(100% - 32px);
  }
  .route-targets {
    flex-basis: 100%;
  }
}
</style>
