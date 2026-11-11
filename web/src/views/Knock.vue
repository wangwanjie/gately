<template>
  <div>
    <PageHeader :title="t('knock.title')" :desc="t('knock.desc')" :icon="KeyRound">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('groupSort.search')" />
      <el-button :icon="Settings2" @click="openSettings">{{ t('knock.settings.button') }}</el-button>
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('knock.rule.add') }}</el-button>
    </PageHeader>

    <!-- 运行状态：敲门端口、防火墙拦截与报文统计 -->
    <div v-if="status" class="g-card status-card">
      <div class="g-card-head">
        <div class="g-card-title"><Activity :size="16" />{{ t('knock.status.title') }}</div>
        <div class="head-actions">
          <el-button v-if="status.firewall.active" size="small" :icon="FileCode" @click="showRules">{{ t('knock.status.viewRules') }}</el-button>
        </div>
      </div>
      <div class="g-card-body">
        <div class="status-rows">
          <div class="status-row">
            <span class="status-label">{{ t('knock.status.listen') }}</span>
            <StatusBadge v-bind="listenBadge" />
            <span v-if="status.openStatus" class="muted">{{ status.openStatus }}</span>
          </div>
          <div class="status-row">
            <span class="status-label">{{ t('knock.status.firewall') }}</span>
            <StatusBadge v-bind="fwBadge" />
            <span v-if="status.firewall.active" class="muted mono">{{ status.firewall.backend }} · {{ status.firewall.ports.join(', ') }}</span>
          </div>
        </div>
        <el-alert v-if="listenProblem" type="error" :title="listenProblem" :closable="false" show-icon class="status-alert" />
        <el-alert v-if="fwProblem" :type="fwProblem.type" :title="fwProblem.title" :closable="false" show-icon class="status-alert">
          <div>{{ fwProblem.hint }}</div>
          <div v-if="status.firewall.error" class="mono err-detail">{{ status.firewall.error }}</div>
        </el-alert>
        <!-- iptables 后端没有 ip6tables 时只拦截 IPv4，受保护端口经 IPv6 仍可直接访问 -->
        <el-alert v-if="status.firewall.active && status.firewall.noIPv6" type="warning" :title="t('knock.status.noIPv6')" :closable="false" show-icon class="status-alert">
          <div>{{ t('knock.status.noIPv6Hint') }}</div>
        </el-alert>
        <div class="stats">
          <div class="stat">
            <div class="stat-value">{{ status.counters.accepted }}</div>
            <div class="stat-label">{{ t('knock.status.accepted') }}</div>
          </div>
          <div class="stat">
            <div class="stat-value">{{ status.counters.rejected }}</div>
            <div class="stat-label">{{ t('knock.status.rejected') }}</div>
          </div>
          <div class="stat">
            <div class="stat-value">{{ status.firewall.dropped }}</div>
            <div class="stat-label">{{ t('knock.status.dropped') }}</div>
          </div>
          <div class="stat">
            <div class="stat-value">{{ status.firewall.allowed }}</div>
            <div class="stat-label">{{ t('knock.status.allowed') }}</div>
          </div>
        </div>
      </div>
    </div>

    <!-- 当前有效的授权 -->
    <div v-if="status" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Unlock :size="16" />{{ t('knock.grants.title', { count: status.grants.length }) }}</div>
      </div>
      <div class="g-card-body">
        <div v-if="!status.grants.length" class="empty-hint">{{ t('knock.grants.empty') }}</div>
        <div v-else class="grants">
          <div v-for="g in status.grants" :key="g.addr + g.rule" class="grant">
            <div class="grant-main">
              <span class="mono grant-addr">{{ g.addr }}</span>
              <el-tag size="small" effect="plain">{{ g.ruleName || g.rule }}</el-tag>
            </div>
            <div class="grant-meta">
              <span v-if="g.ports.length" class="mono">{{ g.ports.join(', ') }}</span>
              <span v-else class="muted">{{ t('knock.grants.gatelyOnly') }}</span>
              <span class="remain">{{ t('knock.grants.remain', { t: remainText(g.until) }) }}</span>
            </div>
            <el-popconfirm :title="t('knock.grants.revokeConfirm')" @confirm="revoke(g)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Lock" class="grant-revoke">{{ t('knock.grants.revoke') }}</el-button>
              </template>
            </el-popconfirm>
          </div>
        </div>
      </div>
    </div>

    <GroupedList :gl="gl" :compact="compact" grid :empty-text="t('knock.rule.empty')">
      <template #item="{ it, item: r, sorting, bind }">
        <EntityCard v-bind="bind" :off="!r.enable" :compact="compact || sorting">
          <template #lead>
            <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
            <el-switch v-else :model-value="r.enable" @change="(v: string | number | boolean) => toggle(r, Boolean(v))" />
          </template>
          <template #title>
            <span>{{ r.name }}</span>
            <el-tag v-if="r.firewall" size="small" type="warning" effect="plain">{{ t('knock.rule.firewallTag') }}</el-tag>
          </template>
          <template v-if="r.remark" #sub>{{ r.remark }}</template>
          <Field :label="t('knock.rule.ports')" mono>{{ r.ports.join(', ') }}</Field>
          <Field :label="t('knock.rule.timeout')">{{ timeoutText(r) }}</Field>
          <Field v-if="r.user" :label="t('knock.rule.user')" mono>{{ r.user }}</Field>
          <Field :label="t('knock.rule.knocks')">
            {{ t('knock.rule.knockStats', { ok: r.stats.knocks, bad: r.stats.rejected }) }}
            <div v-if="r.stats.lastAt" class="muted">{{ t('knock.rule.lastKnock', { addr: r.stats.lastAddr, time: formatTime(r.stats.lastAt) }) }}</div>
          </Field>
          <Field v-if="r.usedBy.length" :label="t('knock.rule.usedBy')" wide>
            <div v-for="u in r.usedBy" :key="u" class="used-by">{{ u }}</div>
          </Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="Pencil" @click="openEditor(r)">{{ t('common.edit') }}</el-button>
            <el-button text size="small" :icon="CopyPlus" @click="openEditor(r, true)">{{ t('common.duplicate') }}</el-button>
            <el-popconfirm :title="t('knock.rule.confirmDelete')" @confirm="remove(r)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
            <el-button type="primary" size="small" :icon="Terminal" class="client-btn" @click="openClient(r)">{{ t('knock.client.button') }}</el-button>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <LogCard url="/api/knock/logs" clearable />

    <!-- 规则编辑 -->
    <el-dialog v-model="editorVisible" :title="editing.key ? t('knock.rule.edit') : t('knock.rule.add')" width="600px" :fullscreen="isMobile">
      <CopyHint :from="copyFrom" />
      <el-form :model="editing" label-width="130px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" :placeholder="t('knock.rule.namePlaceholder')" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('knock.rule.ports')">
          <el-input v-model="portsText" class="mono" placeholder="tcp/22, udp/51820" />
          <div class="form-help">{{ t('knock.rule.portsHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.firewall')">
          <el-switch v-model="editing.firewall" />
          <div class="form-help">{{ t('knock.rule.firewallHelp') }}</div>
          <div v-if="editing.firewall" class="form-help warn">{{ t('knock.rule.firewallWarn') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.encKey')">
          <div class="key-row">
            <SecretInput v-model="editing.encKey" class="mono" autocomplete="off" :placeholder="t('knock.rule.keyAuto')" :reveal="secretOf(editing.key && !copyFrom && `/api/knock/rules/${editing.key}/secrets`, 'encKey')" />
            <el-button :icon="RefreshCw" @click="editing.encKey = randomKey(32)">{{ t('knock.rule.generate') }}</el-button>
          </div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.hmacKey')">
          <div class="key-row">
            <SecretInput v-model="editing.hmacKey" class="mono" autocomplete="off" :placeholder="t('knock.rule.keyAuto')" :reveal="secretOf(editing.key && !copyFrom && `/api/knock/rules/${editing.key}/secrets`, 'hmacKey')" />
            <el-button :icon="RefreshCw" @click="editing.hmacKey = randomKey(64)">{{ t('knock.rule.generate') }}</el-button>
          </div>
          <div class="form-help">{{ t('knock.rule.keysHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.hmacType')">
          <el-select v-model="editing.hmacType" class="w-200">
            <el-option v-for="h in hmacTypes" :key="h" :label="h.toUpperCase()" :value="h" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('knock.rule.timeout')">
          <el-input-number v-model="editing.timeout" :min="5" :max="86400" />
          <span class="unit">{{ t('knock.unit.seconds') }}</span>
          <div class="form-help">{{ t('knock.rule.timeoutHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.maxTimeout')">
          <el-input-number v-model="editing.maxTimeout" :min="0" :max="86400" />
          <span class="unit">{{ t('knock.unit.seconds') }}</span>
          <div class="form-help">{{ t('knock.rule.maxTimeoutHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.user')">
          <el-input v-model="editing.user" class="mono" :placeholder="t('knock.rule.userPlaceholder')" />
          <div class="form-help">{{ t('knock.rule.userHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.rule.requireAddr')">
          <el-switch v-model="editing.requireAddr" />
          <div class="form-help">{{ t('knock.rule.requireAddrHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 全局设置 -->
    <el-dialog v-model="settingsVisible" :title="t('knock.settings.title')" width="560px" :fullscreen="isMobile">
      <el-form v-if="settingsForm" :model="settingsForm" label-width="140px" :label-position="labelPosition">
        <el-form-item :label="t('knock.settings.port')">
          <el-input-number v-model="settingsForm.port" :min="0" :max="65535" />
          <div class="form-help">{{ t('knock.settings.portHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.settings.autoFirewall')">
          <el-switch v-model="settingsForm.autoFirewall" />
          <div class="form-help">{{ t('knock.settings.autoFirewallHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.settings.allowLan')">
          <el-switch v-model="settingsForm.allowLan" />
          <div class="form-help">{{ t('knock.settings.allowLanHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.settings.allowList')">
          <el-input v-model="allowText" type="textarea" :rows="3" class="mono" placeholder="203.0.113.0/24" />
          <div class="form-help">{{ t('knock.settings.allowListHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.settings.maxAge')">
          <el-input-number v-model="settingsForm.maxAge" :min="10" :max="3600" />
          <span class="unit">{{ t('knock.unit.seconds') }}</span>
          <div class="form-help">{{ t('knock.settings.maxAgeHelp') }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settingsVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="saveSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 客户端配置：生成 fwknop 命令与 .fwknoprc 配置段 -->
    <el-dialog v-model="clientVisible" :title="t('knock.client.title', { name: clientRule?.name })" width="680px" :fullscreen="isMobile">
      <el-form label-width="120px" :label-position="labelPosition">
        <el-form-item :label="t('knock.client.server')">
          <el-input v-model="client.server" class="mono" :placeholder="t('knock.client.serverPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('knock.client.via')">
          <el-radio-group v-model="client.via">
            <el-radio-button value="udp">{{ t('knock.client.viaUdp', { port: settings?.port || 62201 }) }}</el-radio-button>
            <el-radio-button value="tcp">{{ t('knock.client.viaTcp') }}</el-radio-button>
          </el-radio-group>
          <div v-if="client.via === 'tcp'" class="tcp-port">
            <el-input-number v-model="client.tcpPort" :min="1" :max="65535" />
            <span class="form-help">{{ t('knock.client.tcpHelp') }}</span>
          </div>
          <div v-else-if="!settings?.port" class="form-help warn">{{ t('knock.client.udpOff') }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.client.addr')">
          <el-radio-group v-model="client.addr">
            <el-radio-button value="resolve">{{ t('knock.client.addrResolve') }}</el-radio-button>
            <el-radio-button value="source" :disabled="clientRule?.requireAddr">{{ t('knock.client.addrSource') }}</el-radio-button>
            <el-radio-button value="fixed">{{ t('knock.client.addrFixed') }}</el-radio-button>
          </el-radio-group>
          <el-input v-if="client.addr === 'fixed'" v-model="client.fixed" class="mono fixed-ip" placeholder="203.0.113.10" />
          <div class="form-help">{{ t(`knock.client.addrHelp.${client.addr}`) }}</div>
        </el-form-item>
        <el-form-item :label="t('knock.client.ports')">
          <el-checkbox-group v-model="client.ports">
            <el-checkbox v-for="p in clientRule?.ports ?? []" :key="p" :value="p" class="mono">{{ p }}</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
      </el-form>
      <div v-if="clientError" class="form-help warn">{{ clientError }}</div>
      <template v-else>
        <div class="snippet-head">
          <span>{{ t('knock.client.command') }}</span>
          <el-button size="small" :icon="Copy" @click="copyText(commandText)">{{ t('common.copy') }}</el-button>
        </div>
        <pre class="snippet mono">{{ commandText }}</pre>
        <div class="snippet-head">
          <span>{{ t('knock.client.rc') }}</span>
          <el-button size="small" :icon="Copy" @click="copyText(rcText)">{{ t('common.copy') }}</el-button>
        </div>
        <pre class="snippet mono">{{ rcText }}</pre>
        <div class="form-help">{{ t('knock.client.rcHelp', { name: stanzaName }) }}</div>
      </template>
      <div class="form-help">{{ t('knock.client.install') }}</div>
    </el-dialog>

    <el-dialog v-model="rulesVisible" :title="t('knock.status.viewRules')" width="760px" :fullscreen="isMobile">
      <pre class="rules mono">{{ rulesText }}</pre>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Activity, Copy, CopyPlus, FileCode, KeyRound, Lock, Pencil, Plus, RefreshCw, Settings2, Terminal, Trash2, Unlock } from 'lucide-vue-next'
import { api } from '../api'
import { fetchSecret, secretOf } from '../secrets'
import SecretInput from '../components/SecretInput.vue'
import PageHeader from '../components/PageHeader.vue'
import EntityCard from '../components/EntityCard.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import CompactToggle from '../components/CompactToggle.vue'
import Field from '../components/Field.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import CopyHint from '../components/CopyHint.vue'
import { useGroupedList } from '../composables/useGroupSort'
import { useCompactView } from '../composables/useCompactView'
import { useFormLayout } from '../composables/useIsMobile'
import { useListText, splitLines } from '../composables/useListText'
import { usePolling } from '../composables/usePolling'
import { copyText } from '../composables/useClipboard'
import { createItem, uniqueName, type CopySource } from '../copyDraft'

interface Rule {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  enable: boolean
  // ports 为可以请求放行的端口，如 tcp/22
  ports: string[]
  // firewall 为 true 时在系统防火墙中拦截这些端口
  firewall: boolean
  encKey: string
  hmacKey: string
  hmacType: string
  timeout: number
  maxTimeout: number
  user: string
  requireAddr: boolean
  remark: string
}

interface RuleView extends Rule {
  key: string
  stats: { knocks: number; rejected: number; lastAt?: string; lastAddr?: string }
  // usedBy 为引用该规则的端口转发、协议分流、STUN 规则与路由
  usedBy: string[]
}

interface KnockSettings {
  port: number
  autoFirewall: boolean
  allowLan: boolean
  allowList: string[]
  maxAge: number
}

interface Grant {
  addr: string
  rule: string
  ruleName: string
  until: string
  // ports 为写入防火墙的端口，只用于 Gately 自身入口的授权为空
  ports: string[]
}

interface Status {
  listening: boolean
  port: number
  listenError?: string
  openStatus?: string
  firewall: {
    needed: boolean
    active: boolean
    reason?: string
    error?: string
    backend?: string
    version?: string
    // noIPv6 为 true 时防火墙拦截只覆盖 IPv4
    noIPv6?: boolean
    ports: string[]
    dropped: number
    allowed: number
  }
  grants: Grant[]
  counters: { accepted: number; rejected: number; ignored: number }
  os: string
  now: string
}

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
const compact = useCompactView('knock')
const rules = ref<RuleView[]>([])
const status = ref<Status>()
const settings = ref<KnockSettings>()
// serverOffset 为服务器时间与本机时间之差（毫秒），计算授权剩余时间时校正浏览器时钟
const serverOffset = ref(0)
const tick = ref(0)
const gl = useGroupedList<RuleView>({
  storageKey: 'knock.groups',
  items: () => rules.value,
  base: '/api/knock/rules',
  reload: load,
  matches: (r, q) => [r.name, r.group ?? '', r.remark, r.user, ...r.ports].join('\n').toLowerCase().includes(q),
})
const hmacTypes = ['sha256', 'sha384', 'sha512']
const saving = ref(false)
const editorVisible = ref(false)
const editing = ref<Rule>(empty())
const copyFrom = ref<CopySource | null>(null)
const portsText = useListText(
  () => editing.value.ports,
  (v) => (editing.value.ports = v),
  { split: (v) => v.split(/[\s,]+/).map((s) => s.trim()).filter(Boolean), separator: ', ' },
)
const settingsVisible = ref(false)
const settingsForm = ref<KnockSettings>()
const allowText = useListText(
  () => settingsForm.value?.allowList ?? [],
  (v) => {
    if (settingsForm.value) {
      settingsForm.value.allowList = v
    }
  },
  { split: splitLines },
)
const rulesVisible = ref(false)
const rulesText = ref('')
const clientVisible = ref(false)
const clientRule = ref<RuleView>()
const clientKeys = ref<{ encKey: string; hmacKey: string }>()
const client = reactive({ server: '', via: 'udp', tcpPort: 443, addr: 'resolve', fixed: '', ports: [] as string[] })

// 每 5 秒刷新状态与授权；剩余时间每秒更新
usePolling(load, 5000)
usePolling(() => {
  tick.value++
}, 1000)

function empty(): Rule {
  return {
    name: '',
    enable: true,
    ports: ['tcp/22'],
    firewall: true,
    encKey: '',
    hmacKey: '',
    hmacType: 'sha256',
    timeout: 30,
    maxTimeout: 300,
    user: '',
    requireAddr: false,
    remark: '',
    group: '',
  }
}

async function load() {
  const [r, s, st] = await Promise.all([
    api.get<RuleView[]>('/api/knock/rules'),
    api.get<Status>('/api/knock/status'),
    api.get<KnockSettings>('/api/knock/settings'),
  ])
  rules.value = r
  status.value = s
  settings.value = st
  serverOffset.value = new Date(s.now).getTime() - Date.now()
}

const listenBadge = computed((): { type: StatusType; text: string } => {
  const s = status.value
  if (!s) {
    return { type: 'pending', text: '' }
  }
  if (s.listenError) {
    return { type: 'error', text: t('knock.status.listenFailed', { port: s.port }) }
  }
  if (s.listening) {
    return { type: 'running', text: t('knock.status.listening', { port: s.port }) }
  }
  if (!s.port) {
    return { type: 'info', text: t('knock.status.udpOff') }
  }
  return { type: 'off', text: t('knock.status.noRules') }
})

const listenProblem = computed(() => (status.value?.listenError ? t('knock.status.listenError', { err: status.value.listenError }) : ''))

const fwBadge = computed((): { type: StatusType; text: string } => {
  const f = status.value?.firewall
  if (!f || !f.needed) {
    return { type: 'off', text: t('knock.status.fwNotNeeded') }
  }
  if (f.active) {
    return { type: 'running', text: t('knock.status.fwActive') }
  }
  return { type: 'warn', text: t('knock.status.fwInactive') }
})

const fwProblem = computed(() => {
  const f = status.value?.firewall
  if (!f || !f.needed || f.active) {
    return null
  }
  const reason = f.reason || 'error'
  return {
    type: reason === 'os' || reason === 'unsupported' ? ('info' as const) : ('warning' as const),
    title: t(`knock.reason.${reason}.title`),
    hint: t(`knock.reason.${reason}.hint`),
  }
})

function formatTime(s?: string) {
  return s ? new Date(s).toLocaleString() : '-'
}

// remainText 返回授权剩余时间，按服务器时间计算
function remainText(until: string) {
  void tick.value
  const sec = Math.max(0, Math.round((new Date(until).getTime() - Date.now() - serverOffset.value) / 1000))
  if (sec >= 3600) {
    return t('knock.unit.hm', { h: Math.floor(sec / 3600), m: Math.floor((sec % 3600) / 60) })
  }
  if (sec >= 60) {
    return t('knock.unit.ms', { m: Math.floor(sec / 60), s: sec % 60 })
  }
  return t('knock.unit.s', { s: sec })
}

function timeoutText(r: Rule) {
  if (r.maxTimeout > 0) {
    return t('knock.rule.timeoutWithMax', { n: r.timeout, max: r.maxTimeout })
  }
  return t('knock.rule.timeoutValue', { n: r.timeout })
}

// randomKey 在浏览器中生成 n 字节随机密钥（base64），与 fwknop --key-gen 的长度一致
function randomKey(n: number) {
  const b = new Uint8Array(n)
  crypto.getRandomValues(b)
  return btoa(String.fromCharCode(...b))
}

function openEditor(r?: RuleView, duplicate = false) {
  copyFrom.value = null
  if (!r) {
    editing.value = empty()
  } else {
    const draft: Rule = JSON.parse(JSON.stringify({ ...r, stats: undefined, usedBy: undefined }))
    if (duplicate) {
      // 复制的规则留空密钥，由服务端生成新密钥：共用密钥时敲门只会匹配到前一条规则
      copyFrom.value = { key: r.key, name: r.name }
      draft.key = undefined
      draft.name = uniqueName(r.name, rules.value.map((x) => x.name))
      draft.encKey = ''
      draft.hmacKey = ''
    }
    editing.value = draft
  }
  editorVisible.value = true
}

async function save() {
  saving.value = true
  try {
    const body = { ...editing.value }
    if (body.key && !copyFrom.value) {
      await api.put(`/api/knock/rules/${body.key}`, body)
    } else {
      await createItem('/api/knock/rules', body, copyFrom.value)
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

async function toggle(r: RuleView, enable: boolean) {
  try {
    await api.put(`/api/knock/rules/${r.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(r: RuleView) {
  try {
    await api.del(`/api/knock/rules/${r.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function revoke(g: Grant) {
  try {
    await api.del('/api/knock/grants', { addr: g.addr, rule: g.rule })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

function openSettings() {
  if (!settings.value) {
    return
  }
  settingsForm.value = JSON.parse(JSON.stringify(settings.value))
  settingsVisible.value = true
}

async function saveSettings() {
  if (!settingsForm.value) {
    return
  }
  saving.value = true
  try {
    await api.put('/api/knock/settings', settingsForm.value)
    settingsVisible.value = false
    ElMessage.success(t('common.saved'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function showRules() {
  try {
    rulesText.value = (await api.get<{ rules: string }>('/api/knock/firewall/rules')).rules
    rulesVisible.value = true
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function openClient(r: RuleView) {
  clientRule.value = r
  clientKeys.value = undefined
  client.server = client.server || window.location.hostname
  client.ports = r.ports.slice(0, 1)
  if (r.requireAddr && client.addr === 'source') {
    client.addr = 'resolve'
  }
  clientVisible.value = true
  try {
    const [enc, mac] = await Promise.all([
      fetchSecret(`/api/knock/rules/${r.key}/secrets`, 'encKey'),
      fetchSecret(`/api/knock/rules/${r.key}/secrets`, 'hmacKey'),
    ])
    clientKeys.value = { encKey: enc ?? '', hmacKey: mac ?? '' }
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// stanzaName 为 .fwknoprc 中的配置段名称，只保留字母数字与连字符
const stanzaName = computed(() => {
  const base = (clientRule.value?.name ?? '').toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/^-+|-+$/g, '')
  return 'gately-' + (base || clientRule.value?.key || 'knock')
})

const clientError = computed(() => {
  if (!clientKeys.value) {
    return t('knock.client.loading')
  }
  if (!client.server.trim()) {
    return t('knock.client.needServer')
  }
  if (!client.ports.length) {
    return t('knock.client.needPorts')
  }
  if (client.addr === 'fixed' && !client.fixed.trim()) {
    return t('knock.client.needFixed')
  }
  return ''
})

const clientPort = computed(() => (client.via === 'tcp' ? client.tcpPort : settings.value?.port || 62201))

// shellQuote 为命令行参数加单引号，密钥中的 + / = 不需要转义，但服务器地址等由用户填写
function shellQuote(s: string) {
  return /^[A-Za-z0-9_./:+=@%-]+$/.test(s) ? s : `'${s.replace(/'/g, `'\\''`)}'`
}

const commandText = computed(() => {
  const r = clientRule.value
  const k = clientKeys.value
  if (!r || !k) {
    return ''
  }
  const args = ['fwknop', '-A', client.ports.join(','), '-D', shellQuote(client.server.trim()), '-p', String(clientPort.value)]
  if (client.via === 'tcp') {
    args.push('-P', 'tcp')
  }
  if (client.addr === 'resolve') {
    args.push('-R')
  } else if (client.addr === 'source') {
    args.push('-s')
  } else {
    args.push('-a', shellQuote(client.fixed.trim()))
  }
  if (r.user) {
    args.push('-U', shellQuote(r.user))
  }
  args.push('--use-hmac', '--key-base64-rijndael', k.encKey, '--key-base64-hmac', k.hmacKey)
  if (r.hmacType !== 'sha256') {
    args.push('--hmac-digest-type', r.hmacType)
  }
  return args.join(' ')
})

const rcText = computed(() => {
  const r = clientRule.value
  const k = clientKeys.value
  if (!r || !k) {
    return ''
  }
  const lines = [`[${stanzaName.value}]`, `SPA_SERVER          ${client.server.trim()}`, `SPA_SERVER_PORT     ${clientPort.value}`, `ACCESS              ${client.ports.join(',')}`]
  if (client.via === 'tcp') {
    lines.push('SPA_SERVER_PROTO    tcp')
  }
  if (client.addr === 'resolve') {
    lines.push('ALLOW_IP            resolve')
  } else if (client.addr === 'source') {
    lines.push('ALLOW_IP            source')
  } else {
    lines.push(`ALLOW_IP            ${client.fixed.trim()}`)
  }
  if (r.user) {
    lines.push(`SPOOF_USER          ${r.user}`)
  }
  lines.push(`KEY_BASE64          ${k.encKey}`, `HMAC_KEY_BASE64     ${k.hmacKey}`, 'USE_HMAC            Y')
  if (r.hmacType !== 'sha256') {
    lines.push(`HMAC_DIGEST_TYPE    ${r.hmacType.toUpperCase()}`)
  }
  return lines.join('\n')
})

onMounted(load)
</script>

<style scoped>
.status-card {
  margin-bottom: 16px;
}
.head-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.status-rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}
.status-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.status-label {
  min-width: 96px;
  white-space: nowrap;
  flex-shrink: 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.status-alert {
  margin-bottom: 12px;
}
.err-detail {
  margin-top: 4px;
  font-size: 12px;
  word-break: break-all;
}
.stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}
.stat {
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
}
.stat-value {
  font-size: 22px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.stat-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.muted {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.form-help.warn {
  color: var(--el-color-warning);
}
.empty-hint {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.grants {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 10px;
}
.grant {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border: 1px solid var(--g-border);
  border-radius: 8px;
  min-width: 0;
}
.grant-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.grant-addr {
  font-weight: 600;
  word-break: break-all;
}
.grant-meta {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 12px;
}
.remain {
  color: var(--el-color-success);
  font-variant-numeric: tabular-nums;
}
.grant-revoke {
  align-self: flex-end;
}
.used-by {
  word-break: break-all;
}
.client-btn {
  margin-left: auto;
}
.key-row {
  display: flex;
  gap: 8px;
  width: 100%;
}
.key-row > :first-child {
  flex: 1;
  min-width: 0;
}
.w-200 {
  width: 200px;
}
.unit {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
}
.tcp-port {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 8px;
}
.fixed-ip {
  margin-top: 8px;
  max-width: 260px;
}
.snippet-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 12px 0 6px;
  font-weight: 600;
}
.snippet {
  margin: 0;
  padding: 10px 12px;
  border-radius: 6px;
  background: var(--el-fill-color-light);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
.rules {
  margin: 0;
  max-height: 65vh;
  overflow: auto;
  font-size: 12px;
  white-space: pre;
}
@media (max-width: 767.98px) {
  .stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .grants {
    grid-template-columns: 1fr;
  }
  .key-row {
    flex-direction: column;
  }
}
</style>
