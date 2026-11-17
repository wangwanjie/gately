<template>
  <div>
    <PageHeader :title="t('portForward.title')" doc="modules/portforward" :desc="t('portForward.desc')" :icon="ArrowLeftRight">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('portForward.filterPlaceholder')" />
      <el-button :icon="SlidersHorizontal" @click="openSettings">{{ t('portForward.globalSettings') }}</el-button>
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('portForward.addRule') }}</el-button>
    </PageHeader>

    <ModuleBanner v-if="settings && !settings.enable" :title="t('portForward.moduleOffTitle')" :loading="enabling" @enable="enableModule" />

    <GroupedList :gl="gl" :error="loadError" @retry="load" :loading="loading" :compact="compact" :empty-text="t('portForward.empty')" :count-text="(n: number) => t('portForward.groupCount', { count: n })">
      <template #empty>
        <div class="tpl-empty">
          <div class="tpl-empty-title">{{ t('portForward.templates.title') }}</div>
          <div class="tpl-empty-list">
            <el-button v-for="tp in PORTFORWARD_TEMPLATES" :key="tp.id" :data-tpl="tp.id" @click="openTemplate(tp)">{{ t(`portForward.templates.${tp.id}`) }}</el-button>
          </div>
        </div>
      </template>
      <template #item="{ it, sorting, bind }">
      <EntityCard
        v-bind="bind"
        :compact="compact || sorting"
        :off="!it.item.enable || moduleOff"
      >
        <template #lead>
          <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
          <el-switch v-else :model-value="it.item.enable" :aria-label="t('common.toggleItem', { name: it.item.name || '-' })" @change="(v: string | number | boolean) => toggle(it.item, Boolean(v))" />
        </template>
        <template #title>
          <span>{{ it.item.name || t('portForward.unnamed') }}</span>
          <span class="tags">
            <el-tag v-for="p in it.item.protocols" :key="p" size="small" effect="plain" class="mono">{{ p.toUpperCase() }}</el-tag>
            <el-tag v-if="it.item.autoFirewall" size="small" :type="it.item.firewallFailed ? 'warning' : 'info'">{{ t('common.firewallTag') }}</el-tag>
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
        <!-- 自动放行防火墙的结果：放行失败（如没有权限）时用警告色 -->
        <Field v-if="it.item.enable && it.item.autoFirewall && it.item.firewall" :label="t('common.firewallTag')" wide>
          <span class="fw-status" :class="{ 'is-failed': it.item.firewallFailed }">{{ it.item.firewall }}</span>
        </Field>
        <Field :label="t('portForward.target')" mono>
          <div v-for="addr in it.item.targetAddresses" :key="addr">{{ addr }}:{{ it.item.targetPorts || it.item.listenPorts }}</div>
        </Field>
        <Field :label="t('portForward.connections')">{{ t('portForward.connectionsValue', { active: it.item.stats.activeConns, total: it.item.stats.totalConns }) }}</Field>
        <Field :label="t('portForward.traffic')">↑ {{ formatBytes(it.item.stats.bytesIn) }} · ↓ {{ formatBytes(it.item.stats.bytesOut) }}</Field>
        <Field v-if="it.item.ipMode" :label="t('ipAccessEditor.title')">{{ it.item.ipMode === 'whitelist' ? t('ipAccessEditor.whitelist') : t('ipAccessEditor.blacklist') }}</Field>
        <Field v-if="it.item.ipDenyList?.length || it.item.ipDenySets?.length" :label="t('ipAccessEditor.ipDeny')" mono>{{ (it.item.ipDenyList?.length ?? 0) + (it.item.ipDenySets?.length ?? 0) }}</Field>
        <Field v-if="it.item.knock" :label="t('ipAccessEditor.knock')">{{ knockRuleName(it.item.knock) }}</Field>
        <Field v-if="it.item.stats.denied" :label="t('ipAccessEditor.denied')" mono>{{ it.item.stats.denied }}</Field>
        <template v-if="sorting" #actions>
          <SortMoves :gl="gl" :it="it" />
        </template>
        <template v-else #actions>
          <el-button text size="small" :icon="Pencil" @click="openEditor(it.item)">{{ t('common.edit') }}</el-button>
          <el-button text size="small" :icon="CopyPlus" @click="openEditor(it.item, true)">{{ t('common.duplicate') }}</el-button>
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

    <el-dialog v-model="editorVisible" :title="editing.key ? t('portForward.editRule') : t('portForward.addRule')" width="600px" :fullscreen="isMobile">
      <CopyHint :from="copyFrom" />
      <el-form ref="formRef" :model="editing" :rules="formRules" label-width="110px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')">
          <el-input v-model="editing.name" :placeholder="t('portForward.namePlaceholder')" />
        </el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('portForward.protocol')" prop="protocols" required>
          <el-checkbox-group v-model="editing.protocols">
            <el-checkbox v-for="p in protocols" :key="p.value" :value="p.value">{{ p.label }}</el-checkbox>
          </el-checkbox-group>
          <div class="form-help">{{ t('portForward.protocolHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('portForward.listenAddress')">
          <el-input v-model="editing.listenAddress" :placeholder="t('portForward.listenAddressPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('portForward.listenPorts')" prop="listenPorts" required>
          <el-input v-model="editing.listenPorts" :placeholder="t('portForward.portsPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('portForward.targetAddress')" prop="targetAddresses" required>
          <el-input v-model="targetsText" type="textarea" :rows="2" :placeholder="t('portForward.targetAddressPlaceholder')" />
          <TargetHint :lines="targetLines" :ports="editing.targetPorts" @split="splitTargets" />
        </el-form-item>
        <el-form-item :label="t('portForward.targetPorts')">
          <el-input v-model="editing.targetPorts" :placeholder="t('portForward.targetPortsPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('common.enable')">
          <el-switch v-model="editing.enable" />
        </el-form-item>
        <!-- 常用的是协议、监听与目标，其余收在「高级设置」里；打开已有规则时有改动的高级项会自动展开 -->
        <AdvancedToggle v-model="advOpen" :count="advCount" />
        <div v-show="advOpen">
          <el-form-item :label="t('common.autoFirewall')">
            <el-switch v-model="editing.autoFirewall" />
            <div class="form-help">{{ t('portForward.autoFirewallHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('portForward.maxConnections')">
            <el-input-number v-model="editing.maxConnections" :min="0" :max="100000" />
            <span class="help">{{ t('portForward.useGlobalDefault') }}</span>
          </el-form-item>
          <el-form-item :label="t('portForward.maxConnsPerSource')">
            <el-input-number v-model="editing.maxConnsPerSource" :min="0" :max="100000" />
            <span class="help">{{ t('portForward.useGlobalDefault') }}</span>
          </el-form-item>
          <el-form-item :label="t('portForward.udpTimeout')">
            <el-input-number v-model="editing.udpSessionTimeout" :min="0" :max="3600" />
            <span class="help">{{ t('portForward.useGlobalDefault') }}</span>
          </el-form-item>
          <el-form-item :label="t('portForward.proxyProtocol')">
            <el-select v-model="editing.proxyProtocol" style="width: 160px" :empty-values="[null, undefined]">
              <el-option :label="t('portForward.proxyProtocolOff')" value="" />
              <el-option label="PROXY v1" value="v1" />
              <el-option label="PROXY v2" value="v2" />
            </el-select>
            <div class="help">{{ t('portForward.proxyProtocolHelp') }}</div>
          </el-form-item>
          <IPAccessEditor v-model:mode="editing.ipMode" v-model:list="editing.ipList" v-model:sets="editing.ipSets" v-model:deny-list="editing.ipDenyList" v-model:deny-sets="editing.ipDenySets" v-model:knock="editing.knock" :note="t('portForward.ipAccessNote')" />
          <el-form-item :label="t('common.remark')">
            <el-input v-model="editing.remark" />
          </el-form-item>
        </div>
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
        <el-form-item :label="t('portForward.defaultMaxConnsPerSource')">
          <el-input-number v-model="settingsForm.defaultMaxConnsPerSource" :min="0" />
          <div class="form-help">{{ t('portForward.defaultMaxConnsPerSourceHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('portForward.defaultUdpTimeout')"><el-input-number v-model="settingsForm.defaultUDPSessionTimeout" :min="1" /></el-form-item>
        <el-form-item :label="t('portForward.maxUdpSessions')"><el-input-number v-model="settingsForm.maxUDPSessions" :min="1" /></el-form-item>
        <el-form-item :label="t('portForward.dialTimeout')"><el-input-number v-model="settingsForm.dialTimeout" :min="1" :max="60" /></el-form-item>
        <el-form-item :label="t('portForward.firstDataTimeout')">
          <el-input-number v-model="settingsForm.firstDataTimeout" :min="1" :max="3600" />
          <div class="form-help">{{ t('portForward.firstDataTimeoutHelp') }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settingsVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { ArrowLeftRight, CopyPlus, Pencil, Plus, SlidersHorizontal, Trash2, TriangleAlert } from 'lucide-vue-next'
import { api, errorText, formatBytes } from '../api'
import GroupedList from '../components/GroupedList.vue'
import { PORTFORWARD_TEMPLATES, type PortForwardTemplate } from '../emptyTemplates'
import GroupInput from '../components/GroupInput.vue'
import IPAccessEditor from '../components/IPAccessEditor.vue'
import { knockRuleName, useKnockRules } from '../composables/useKnockRules'
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
import CopyHint from '../components/CopyHint.vue'
import TargetHint from '../components/TargetHint.vue'
import AdvancedToggle from '../components/AdvancedToggle.vue'
import { createItem, uniqueName, type CopySource } from '../copyDraft'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'
import { validateForm } from '../composables/useFormValidate'

interface Rule {
  key?: string
  name: string
  enable: boolean
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  protocols: string[]
  listenAddress: string
  listenPorts: string
  // 在本机防火墙自动放行监听成功的端口（仅 Linux）
  autoFirewall?: boolean
  targetAddresses: string[]
  targetPorts: string
  maxConnections: number
  // 单个来源（IPv4 地址、IPv6 /64）的最大并发连接数，0 使用全局默认值
  maxConnsPerSource?: number
  udpSessionTimeout: number
  // 发送给目标的 PROXY 协议版本，空为不发送（仅 TCP）
  proxyProtocol: string
  // 来源 IP 访问控制：ipMode 为 ''（关闭）、whitelist、blacklist；ipSets 为引用的订阅 IP 列表 key
  ipMode: string
  ipList: string[]
  ipSets: string[]
  // 优先拒绝名单：先于白名单、黑名单判断，与模式无关
  ipDenyList?: string[]
  ipDenySets?: string[]
  // knock 为端口敲门规则 key，非空时来源地址还必须敲门成功
  knock?: string
  remark: string
}

interface RuleView extends Rule {
  running: boolean
  error?: string
  details?: string[]
  // 自动放行防火墙的结果说明与是否失败
  firewall?: string
  firewallFailed?: boolean
  stats: {
    activeConns: number
    totalConns: number
    bytesIn: number
    bytesOut: number
    // denied 为不满足来源 IP 访问控制而被拒绝的次数（TCP 按连接、UDP 按数据包）
    denied?: number
  }
}

interface Settings {
  enable: boolean
  defaultMaxConnections: number
  // 单个来源的默认最大并发连接数，0 为最大连接数的一半
  defaultMaxConnsPerSource: number
  defaultUDPSessionTimeout: number
  maxUDPSessions: number
  dialTimeout: number
  // TCP 连接建立后等待任一方发送数据的秒数，超时断开空连接
  firstDataTimeout: number
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
// 列表最近一次加载失败的原因，空为成功；接口出错时列表显示错误而不是空状态
const loadError = ref('')
// 读取端口敲门规则目录，卡片上显示“需要敲门”的规则名称
useKnockRules()
const compact = useCompactView('portforward')
const { isMobile, labelPosition } = useFormLayout()
const rules = ref<RuleView[]>([])
const loading = ref(true)
const editorVisible = ref(false)
const saving = ref(false)
const editing = ref<Rule>(emptyRule())
// 复制条目时的模板，普通新增与编辑时为 null
const copyFrom = ref<CopySource | null>(null)
const targetsText = ref('')
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
    [r.name, r.group ?? '', r.remark, listenText(r), ...(r.targetAddresses ?? [])]
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
    autoFirewall: false,
    targetAddresses: [],
    targetPorts: '',
    maxConnections: 0,
    maxConnsPerSource: 0,
    udpSessionTimeout: 0,
    proxyProtocol: '',
    ipMode: '',
    ipList: [],
    ipSets: [],
    ipDenyList: [],
    ipDenySets: [],
    knock: '',
    remark: '',
  }
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
    loadError.value = ''
  } catch (e) {
    loadError.value = errorText(e)
  } finally {
    loading.value = false
  }
}

async function loadSettings() {
  settings.value = await api.get<Settings>('/api/portforward/settings')
}

// 编辑框的前端校验：协议、监听端口与目标地址必填（目标地址的写法问题由 TargetHint 即时提示）
const formRef = ref<FormInstance>()
const formRules = computed<FormRules>(() => ({
  protocols: [{ type: 'array', required: true, min: 1, message: t('portForward.protocolRequired'), trigger: 'change' }],
  listenPorts: [{ required: true, whitespace: true, message: t('portForward.listenPortsRequired'), trigger: 'blur' }],
  // 目标地址的输入框绑定的是文本，按拆分后的地址判断
  targetAddresses: [{ trigger: 'blur', validator: (_r: unknown, _v: unknown, cb: (e?: Error) => void) => cb(targetLines.value.length ? undefined : new Error(t('portForward.targetRequired'))) }],
}))

// advOpen 为编辑框是否展开「高级设置」；advCount 为与默认值不同的高级字段数
const advOpen = ref(false)
const advCount = computed(() => {
  const r = editing.value
  return [
    r.autoFirewall,
    r.maxConnections > 0,
    (r.maxConnsPerSource ?? 0) > 0,
    r.udpSessionTimeout > 0,
    !!r.proxyProtocol,
    !!r.ipMode || !!r.ipDenyList?.length || !!r.ipDenySets?.length || !!r.knock,
    !!r.remark?.trim(),
  ].filter(Boolean).length
})

// openEditor 打开编辑框：copy 为 true 时以 row 为模板新建副本
// openTemplate 按场景模板打开新建表单：预填名称、协议与端口，目标地址由用户填写
function openTemplate(tp: PortForwardTemplate) {
  openEditor()
  editing.value.name = t(`portForward.templates.${tp.id}`)
  editing.value.protocols = [...tp.protocols]
  editing.value.listenPorts = tp.listenPorts
  editing.value.targetPorts = tp.targetPorts
}

function openEditor(row?: RuleView, copy = false) {
  copyFrom.value = null
  if (row) {
    const { running: _r, error: _e, details: _d, stats: _s, firewall: _f, firewallFailed: _ff, ...rule } = row
    editing.value = JSON.parse(JSON.stringify(rule))
    // 后端空名单返回 null，编辑时统一为数组
    editing.value.ipMode = editing.value.ipMode ?? ''
    editing.value.ipList = editing.value.ipList ?? []
    editing.value.ipSets = editing.value.ipSets ?? []
    editing.value.ipDenyList = editing.value.ipDenyList ?? []
    editing.value.ipDenySets = editing.value.ipDenySets ?? []
    editing.value.knock = editing.value.knock ?? ''
    if (copy) {
      copyFrom.value = { key: row.key ?? '', name: row.name }
      editing.value.key = ''
      editing.value.name = uniqueName(row.name, rules.value.map((r) => r.name))
    }
  } else {
    editing.value = emptyRule()
  }
  targetsText.value = (editing.value.targetAddresses ?? []).join('\n')
  advOpen.value = advCount.value > 0
  editorVisible.value = true
  void nextTick(() => formRef.value?.clearValidate())
}

// targetLines 为目标地址输入框按行（或逗号）拆开的地址
const targetLines = computed(() =>
  targetsText.value
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean),
)

// splitTargets 把写成「主机:端口」的目标地址拆开：主机留在目标地址，端口填到目标端口
function splitTargets(v: { hosts: string[]; port: number }) {
  targetsText.value = v.hosts.join('\n')
  editing.value.targetPorts = String(v.port)
}

async function save() {
  editing.value.targetAddresses = targetLines.value
  // 必填项先在前端检查，未填时标红并聚焦
  if (!(await validateForm(formRef.value))) {
    return
  }
  saving.value = true
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/portforward/rules/${key}`, body)
    } else {
      await createItem('/api/portforward/rules', body, copyFrom.value)
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
/* 空状态的场景模板按钮 */
.tpl-empty-title {
  margin-bottom: 10px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.tpl-empty-list {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
}
.tpl-empty-list .el-button {
  margin-left: 0;
}
.fw-status {
  overflow-wrap: anywhere;
}
.fw-status.is-failed {
  color: var(--el-color-warning);
}
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
</style>
