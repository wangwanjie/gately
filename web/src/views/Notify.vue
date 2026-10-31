<template>
  <div>
    <PageHeader :title="t('notify.title')" :desc="t('notify.desc')" :icon="Bell">
      <el-button :icon="Plus" @click="openRule()">{{ t('notify.addRule') }}</el-button>
      <el-button type="primary" :icon="Plus" @click="openChannel()">{{ t('notify.addChannel') }}</el-button>
    </PageHeader>

    <div class="section-head">
      <h3 class="section">{{ t('notify.channels') }}</h3>
      <div class="section-tools">
        <ListTools :gl="glChannels" size="small" />
        <CompactToggle v-model="compactChannels" size="small" />
      </div>
    </div>
    <GroupedList :gl="glChannels" :compact="compactChannels" grid :empty-text="t('notify.noChannels')">
      <template #item="{ it, item: c, sorting, bind }">
        <EntityCard v-bind="bind" :off="!c.enable" :compact="compactChannels || sorting">
          <template #lead>
            <SortCheck v-if="sorting" :gl="glChannels" :id="it.id" />
            <el-switch v-else :model-value="c.enable" @change="(v: string | number | boolean) => toggleChannel(c, Boolean(v))" />
          </template>
          <template #title>{{ c.name }}</template>
          <template #sub>{{ kindLabel(c.kind) }}</template>
          <Field :label="t('notify.target')" mono>{{ channelTarget(c) }}</Field>
          <Field v-if="c.remark" :label="t('common.remark')">{{ c.remark }}</Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="glChannels" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="Send" :loading="testing[c.key ?? '']" @click="test(c)">{{ t('common.test') }}</el-button>
            <el-button text size="small" :icon="Pencil" @click="openChannel(c)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('notify.confirmDeleteChannel')" @confirm="removeChannel(c)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <div class="section-head">
      <h3 class="section">{{ t('notify.rules') }}</h3>
      <div class="section-tools">
        <ListTools :gl="glRules" size="small" />
        <CompactToggle v-model="compactRules" size="small" />
      </div>
    </div>
    <GroupedList :gl="glRules" :compact="compactRules" grid :empty-text="t('notify.noRules')">
      <template #item="{ it, item: r, sorting, bind }">
        <EntityCard v-bind="bind" :off="!r.enable" :compact="compactRules || sorting">
          <template #lead>
            <SortCheck v-if="sorting" :gl="glRules" :id="it.id" />
            <el-switch v-else :model-value="r.enable" @change="(v: string | number | boolean) => toggleRule(r, Boolean(v))" />
          </template>
          <template #title>{{ r.name }}</template>
          <template #sub>{{ t('notify.levelAtLeast', { level: levelLabel(r.minLevel) }) }}<span v-if="r.silence">{{ t('notify.silenceSuffix', { seconds: r.silence }) }}</span></template>
          <Field :label="t('notify.events')">
            <div class="tags">
              <el-tag v-for="e in r.events" :key="e" size="small" effect="plain">{{ eventName(e) }}</el-tag>
            </div>
          </Field>
          <Field :label="t('notify.channel')">{{ r.channels.map(channelName).join(t('notify.separator')) || '-' }}</Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="glRules" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="Pencil" @click="openRule(r)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('notify.confirmDeleteRule')" @confirm="removeRule(r)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <h3 class="section">{{ t('notify.recentDeliveries') }}</h3>
    <el-table :data="history" size="small" stripe :empty-text="t('notify.noDeliveries')">
      <el-table-column :label="t('notify.time')" width="170">
        <template #default="{ row }">{{ new Date(row.time).toLocaleString() }}</template>
      </el-table-column>
      <el-table-column :label="t('notify.event')" min-width="200" prop="title" show-overflow-tooltip />
      <el-table-column :label="t('notify.rule')" width="140" prop="rule" show-overflow-tooltip />
      <el-table-column :label="t('notify.channel')" width="140" prop="channel" show-overflow-tooltip />
      <el-table-column :label="t('notify.result')" width="90">
        <template #default="{ row }">
          <StatusBadge v-if="row.ok" type="running" :text="t('notify.success')" />
          <StatusBadge v-else type="error" :text="t('notify.failed')" :title="row.error" />
        </template>
      </el-table-column>
    </el-table>

    <LogCard url="/api/notify/logs" clearable />

    <el-dialog v-model="channelVisible" :title="editingChannel.key ? t('notify.editChannel') : t('notify.addChannel')" width="560px" :fullscreen="isMobile">
      <el-form :model="editingChannel" label-width="100px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editingChannel.name" /></el-form-item>
        <GroupInput v-model="editingChannel.group" :groups="glChannels.groupNames.value" />
        <el-form-item :label="t('notify.type')">
          <el-select v-model="editingChannel.kind" :disabled="!!editingChannel.key" style="width: 100%">
            <el-option v-for="k in kinds" :key="k.value" :label="k.label" :value="k.value" />
          </el-select>
        </el-form-item>

        <template v-if="editingChannel.kind === 'webhook'">
          <el-form-item :label="t('notify.requestUrl')"><el-input v-model="editingChannel.webhook.url" placeholder="https://example.com/hook" /></el-form-item>
          <el-form-item :label="t('notify.requestMethod')">
            <el-select v-model="editingChannel.webhook.method" style="width: 120px">
              <el-option v-for="m in ['GET', 'POST', 'PUT', 'PATCH']" :key="m" :label="m" :value="m" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('notify.requestHeaders')"><el-input v-model="headersText" type="textarea" :rows="2" :placeholder="t('notify.headersPlaceholder')" /></el-form-item>
          <el-form-item v-if="editingChannel.webhook.method !== 'GET'" :label="t('notify.requestBody')">
            <el-input v-model="editingChannel.webhook.body" type="textarea" :rows="4" class="mono" />
          </el-form-item>
          <el-form-item :label="t('notify.variables')">
            <div class="form-help mono">{title} {message} {text} {type} {source} {level} {time}</div>
          </el-form-item>
        </template>

        <template v-else-if="editingChannel.kind === 'telegram'">
          <el-form-item label="Bot Token"><SecretInput v-model="editingChannel.secret" :reveal="secretReveal" :placeholder="secretHint" /></el-form-item>
          <el-form-item label="Chat ID"><el-input v-model="editingChannel.target" /></el-form-item>
          <el-form-item :label="t('notify.apiUrl')"><el-input v-model="editingChannel.url" :placeholder="t('notify.telegramUrlPlaceholder')" /></el-form-item>
        </template>

        <template v-else-if="editingChannel.kind === 'bark'">
          <el-form-item :label="t('notify.deviceKey')"><SecretInput v-model="editingChannel.secret" :reveal="secretReveal" :placeholder="secretHint" /></el-form-item>
          <el-form-item :label="t('notify.serviceUrl')"><el-input v-model="editingChannel.url" :placeholder="t('notify.barkUrlPlaceholder')" /></el-form-item>
        </template>

        <template v-else-if="editingChannel.kind === 'ntfy'">
          <el-form-item :label="t('notify.serviceUrl')"><el-input v-model="editingChannel.url" :placeholder="t('notify.ntfyUrlPlaceholder')" /></el-form-item>
          <el-form-item :label="t('notify.topic')"><el-input v-model="editingChannel.target" /></el-form-item>
          <el-form-item :label="t('notify.accessToken')"><SecretInput v-model="editingChannel.secret" :reveal="secretReveal" :placeholder="secretHint" /></el-form-item>
        </template>

        <template v-else-if="editingChannel.kind === 'gotify'">
          <el-form-item :label="t('notify.serviceUrl')"><el-input v-model="editingChannel.url" placeholder="https://gotify.example.com" /></el-form-item>
          <el-form-item :label="t('notify.appToken')"><SecretInput v-model="editingChannel.secret" :reveal="secretReveal" :placeholder="secretHint" /></el-form-item>
        </template>

        <template v-else-if="editingChannel.kind === 'pushover'">
          <el-form-item :label="t('notify.appToken')"><SecretInput v-model="editingChannel.secret" :reveal="secretReveal" :placeholder="secretHint" /></el-form-item>
          <el-form-item :label="t('notify.userKey')"><el-input v-model="editingChannel.target" /></el-form-item>
        </template>

        <template v-else-if="editingChannel.kind === 'discord' || editingChannel.kind === 'slack'">
          <el-form-item :label="t('notify.webhookUrl')"><el-input v-model="editingChannel.url" placeholder="https://..." /></el-form-item>
        </template>

        <template v-else-if="editingChannel.kind === 'serverchan'">
          <el-form-item label="SendKey"><SecretInput v-model="editingChannel.secret" :reveal="secretReveal" :placeholder="secretHint" /></el-form-item>
        </template>

        <template v-else-if="editingChannel.kind === 'pushplus'">
          <el-form-item label="Token"><SecretInput v-model="editingChannel.secret" :reveal="secretReveal" :placeholder="secretHint" /></el-form-item>
        </template>

        <template v-else-if="editingChannel.kind === 'smtp'">
          <el-form-item :label="t('notify.server')"><el-input v-model="editingChannel.host" placeholder="smtp.example.com" /></el-form-item>
          <el-form-item :label="t('notify.security')">
            <el-select v-model="editingChannel.security" style="width: 180px">
              <el-option :label="t('notify.starttls')" value="starttls" />
              <el-option :label="t('notify.tls')" value="tls" />
              <el-option :label="t('notify.noEncryption')" value="none" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('notify.port')"><el-input-number v-model="editingChannel.port" :min="0" :max="65535" /></el-form-item>
          <el-form-item :label="t('notify.user')"><el-input v-model="editingChannel.user" /></el-form-item>
          <el-form-item :label="t('notify.password')"><SecretInput v-model="editingChannel.secret" :reveal="secretReveal" :placeholder="secretHint" /></el-form-item>
          <el-form-item :label="t('notify.from')"><el-input v-model="editingChannel.from" :placeholder="t('notify.fromPlaceholder')" /></el-form-item>
          <el-form-item :label="t('notify.to')"><el-input v-model="editingChannel.target" :placeholder="t('notify.toPlaceholder')" /></el-form-item>
        </template>

        <template v-else>
          <el-form-item :label="t('notify.botUrl')"><el-input v-model="editingChannel.url" placeholder="https://..." /></el-form-item>
          <el-form-item v-if="editingChannel.kind !== 'wecom'" :label="t('notify.signSecret')">
            <SecretInput v-model="editingChannel.secret" :reveal="secretReveal" :placeholder="secretHint" />
            <div class="form-help">{{ t('notify.signHelp') }}</div>
          </el-form-item>
        </template>

        <el-form-item :label="t('common.remark')"><el-input v-model="editingChannel.remark" /></el-form-item>
        <el-form-item :label="t('common.enable')"><el-switch v-model="editingChannel.enable" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="channelVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveChannel">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="ruleVisible" :title="editingRule.key ? t('notify.editRule') : t('notify.addRule')" width="560px" :fullscreen="isMobile">
      <el-form :model="editingRule" label-width="100px" :label-position="labelPosition">
        <el-form-item v-if="!editingRule.key" :label="t('notify.template')">
          <div class="tpl-list">
            <el-tooltip v-for="tpl in ruleTemplates" :key="tpl.id" :content="tpl.desc" placement="top">
              <el-button size="small" round @click="applyTemplate(tpl)">{{ tpl.name }}</el-button>
            </el-tooltip>
          </div>
        </el-form-item>
        <el-form-item :label="t('common.name')"><el-input v-model="editingRule.name" /></el-form-item>
        <GroupInput v-model="editingRule.group" :groups="glRules.groupNames.value" />
        <el-form-item :label="t('notify.events')">
          <el-select v-model="editingRule.events" multiple filterable allow-create default-first-option style="width: 100%" :placeholder="t('notify.eventsPlaceholder')">
            <el-option :label="t('notify.allEvents')" value="*" />
            <el-option v-for="et in eventTypes" :key="et.type" :label="et.name" :value="et.type" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('notify.minLevel')">
          <el-select v-model="editingRule.minLevel" style="width: 140px">
            <el-option :label="t('notify.levelInfo')" value="info" />
            <el-option :label="t('notify.levelWarn')" value="warn" />
            <el-option :label="t('notify.levelError')" value="error" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('notify.channel')">
          <el-select v-model="editingRule.channels" multiple style="width: 100%" :placeholder="t('notify.channelsPlaceholder')">
            <el-option v-for="c in channels" :key="c.key" :label="c.name" :value="c.key ?? ''" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('notify.silence')">
          <el-input-number v-model="editingRule.silence" :min="0" :step="60" />
          <div class="form-help">{{ t('notify.silenceHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('common.enable')"><el-switch v-model="editingRule.enable" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ruleVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveRule">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Bell, Pencil, Plus, Send, Trash2 } from 'lucide-vue-next'
import { api } from '../api'
import { SECRET_MASK, secretOf } from '../secrets'
import SecretInput from '../components/SecretInput.vue'
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
import StatusBadge from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { useListText, splitLines } from '../composables/useListText'
import { usePolling } from '../composables/usePolling'

interface WebhookCfg {
  enable: boolean
  url: string
  method: string
  headers: string[]
  body: string
  successContent: string[]
  timeout: number
  insecureSkipVerify: boolean
}

interface Channel {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  enable: boolean
  kind: string
  url: string
  secret: string
  hasSecret?: boolean
  target: string
  host: string
  port: number
  security: string
  user: string
  from: string
  webhook: WebhookCfg
  remark: string
}

interface Rule {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  enable: boolean
  events: string[]
  minLevel: string
  channels: string[]
  silence: number
}

interface EventType {
  type: string
  name: string
  level: string
}

interface DeliveryRecord {
  time: string
  type: string
  title: string
  rule: string
  channel: string
  ok: boolean
  error?: string
}

const { t } = useI18n()
// 渠道类型选项：标题随界面语言变化，因此用 computed
const kinds = computed(() => [
  { value: 'webhook', label: 'Webhook' },
  { value: 'telegram', label: 'Telegram' },
  { value: 'bark', label: 'Bark' },
  { value: 'wecom', label: t('notify.kindWecom') },
  { value: 'dingtalk', label: t('notify.kindDingtalk') },
  { value: 'feishu', label: t('notify.kindFeishu') },
  { value: 'smtp', label: t('notify.kindSmtp') },
  { value: 'ntfy', label: 'ntfy' },
  { value: 'gotify', label: 'Gotify' },
  { value: 'pushover', label: 'Pushover' },
  { value: 'discord', label: 'Discord' },
  { value: 'slack', label: 'Slack' },
  { value: 'serverchan', label: 'Server酱' },
  { value: 'pushplus', label: 'PushPlus' },
])

// 内置规则模板：点击后预填表单，仍需选择渠道后保存
const ruleTemplates = computed(() => [
  { id: 'critical', name: t('notify.tpl.critical.name'), desc: t('notify.tpl.critical.desc'), events: ['ddns.failed', 'ssl.renew_failed', 'ssl.deploy_failed', 'upstream.down', 'process.exited', 'backup.failed', 'docker.exited'], minLevel: 'warn', silence: 600 },
  { id: 'cert', name: t('notify.tpl.cert.name'), desc: t('notify.tpl.cert.desc'), events: ['ssl.expiring', 'ssl.renew_failed', 'ssl.deploy_failed'], minLevel: 'warn', silence: 86400 },
  { id: 'address', name: t('notify.tpl.address.name'), desc: t('notify.tpl.address.desc'), events: ['ddns.changed', 'stun.changed'], minLevel: 'info', silence: 0 },
  { id: 'health', name: t('notify.tpl.health.name'), desc: t('notify.tpl.health.desc'), events: ['upstream.down', 'upstream.up', 'docker.exited', 'process.exited'], minLevel: 'info', silence: 300 },
  { id: 'security', name: t('notify.tpl.security.name'), desc: t('notify.tpl.security.desc'), events: ['auth.failed', 'auth.banned', 'guard.banned', 'login.success', 'audit.access'], minLevel: 'info', silence: 0 },
  { id: 'changes', name: t('notify.tpl.changes.name'), desc: t('notify.tpl.changes.desc'), events: ['audit.change'], minLevel: 'info', silence: 0 },
])

function applyTemplate(tpl: { name: string; events: string[]; minLevel: string; silence: number }) {
  editingRule.value.name = tpl.name
  editingRule.value.events = [...tpl.events]
  editingRule.value.minLevel = tpl.minLevel
  editingRule.value.silence = tpl.silence
}

const { isMobile, labelPosition } = useFormLayout()
const channels = ref<Channel[]>([])
const rules = ref<Rule[]>([])
// 渠道与规则各自的视图、分组与排序
const compactChannels = useCompactView('notify.channels')
const compactRules = useCompactView('notify.rules')
const glChannels = useGroupedList<Channel>({
  storageKey: 'notify.channels.groups',
  items: () => channels.value,
  base: '/api/notify/channels',
  reload: load,
  matches: (c, q) => [c.name, c.group ?? '', c.remark, kindLabel(c.kind)].join('\n').toLowerCase().includes(q),
})
const glRules = useGroupedList<Rule>({
  storageKey: 'notify.rules.groups',
  items: () => rules.value,
  base: '/api/notify/rules',
  reload: load,
  matches: (r, q) => [r.name, r.group ?? ''].join('\n').toLowerCase().includes(q),
})
const eventTypes = ref<EventType[]>([])
const history = ref<DeliveryRecord[]>([])
const testing = reactive<Record<string, boolean>>({})
const channelVisible = ref(false)
const ruleVisible = ref(false)
const editingChannel = ref<Channel>(emptyChannel())
const editingRule = ref<Rule>(emptyRule())
// 定时刷新，标签页在后台时暂停
const polling = usePolling(load, 15000)

// secretReveal 取回已保存渠道的密钥明文，新渠道没有可取回的内容
const secretReveal = computed(() => secretOf(editingChannel.value.key && `/api/notify/channels/${editingChannel.value.key}/secrets`, 'secret'))
const secretHint = computed(() => (editingChannel.value.hasSecret ? t('notify.secretSet') : ''))
// 输入框保留原始文本，回车可正常换行（见 useListText）
const headersText = useListText(() => editingChannel.value.webhook.headers, (v) => (editingChannel.value.webhook.headers = v), { split: splitLines })

function emptyChannel(): Channel {
  return {
    name: '',
    enable: true,
    kind: 'telegram',
    url: '',
    secret: '',
    target: '',
    host: '',
    port: 0,
    security: 'starttls',
    user: '',
    from: '',
    webhook: { enable: true, url: '', method: 'POST', headers: [], body: '{"text":"{title}\\n{text}"}', successContent: [], timeout: 0, insecureSkipVerify: false },
    remark: '',
  }
}

function emptyRule(): Rule {
  return { name: '', enable: true, events: [], minLevel: 'info', channels: [], silence: 300 }
}

function kindLabel(k: string) {
  return kinds.value.find((x) => x.value === k)?.label ?? k
}

function levelLabel(l: string) {
  return ({ info: t('notify.levelInfo'), warn: t('notify.levelWarn'), error: t('notify.levelError') } as Record<string, string>)[l] ?? l
}

function eventName(e: string) {
  if (e === '*') {
    return t('notify.allEvents')
  }
  return eventTypes.value.find((et) => et.type === e)?.name ?? e
}

function channelName(key: string) {
  return channels.value.find((c) => c.key === key)?.name ?? t('notify.deleted')
}

function channelTarget(c: Channel) {
  switch (c.kind) {
    case 'webhook':
      return c.webhook.url
    case 'telegram':
      return `Chat ${c.target}`
    case 'bark':
      return c.url || 'api.day.app'
    case 'smtp':
      return `${c.target} via ${c.host}:${c.port}`
    case 'ntfy':
      return `${c.url || 'ntfy.sh'}/${c.target}`
    case 'pushover':
    case 'serverchan':
    case 'pushplus':
      return kindLabel(c.kind)
    default:
      return c.url
  }
}

async function load() {
  const [c, r, h] = await Promise.all([
    api.get<Channel[]>('/api/notify/channels'),
    api.get<Rule[]>('/api/notify/rules'),
    api.get<DeliveryRecord[]>('/api/notify/history'),
  ])
  channels.value = c
  rules.value = r
  history.value = h
}

function openChannel(c?: Channel) {
  editingChannel.value = c ? { ...emptyChannel(), ...JSON.parse(JSON.stringify(c)) } : emptyChannel()
  // 接口不返回已保存的密钥（留空表示保持原值）；用占位符表示“已设置”，点击显示时再取回明文
  if (c?.hasSecret) {
    editingChannel.value.secret = SECRET_MASK
  }
  channelVisible.value = true
}

function openRule(r?: Rule) {
  editingRule.value = r ? JSON.parse(JSON.stringify(r)) : emptyRule()
  ruleVisible.value = true
}

async function saveChannel() {
  try {
    const { key, hasSecret: _h, ...body } = editingChannel.value
    // 未查看也未修改的密钥仍是占位符，按原接口约定提交空值以保持原值
    if (body.secret === SECRET_MASK) {
      body.secret = ''
    }
    if (key) {
      await api.put(`/api/notify/channels/${key}`, body)
    } else {
      await api.post('/api/notify/channels', body)
    }
    channelVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function saveRule() {
  try {
    const { key, ...body } = editingRule.value
    if (key) {
      await api.put(`/api/notify/rules/${key}`, body)
    } else {
      await api.post('/api/notify/rules', body)
    }
    ruleVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function toggleChannel(c: Channel, enable: boolean) {
  try {
    await api.put(`/api/notify/channels/${c.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function toggleRule(r: Rule, enable: boolean) {
  try {
    await api.put(`/api/notify/rules/${r.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function removeChannel(c: Channel) {
  try {
    await api.del(`/api/notify/channels/${c.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function removeRule(r: Rule) {
  try {
    await api.del(`/api/notify/rules/${r.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function test(c: Channel) {
  const k = c.key ?? ''
  testing[k] = true
  try {
    await api.post(`/api/notify/channels/${k}/test`)
    ElMessage.success(t('notify.testSent'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    testing[k] = false
    await load()
  }
}

onMounted(async () => {
  eventTypes.value = await api.get('/api/notify/event-types')
  await load()
  polling.start()
})
</script>

<style scoped>
.section-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px 12px;
}
.section-tools {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.tpl-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.section {
  margin: 22px 0 10px;
  font-size: 15px;
  font-weight: 600;
}
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
</style>
