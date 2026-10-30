<template>
  <div>
    <PageHeader :title="t('ddns.title')" :desc="t('ddns.desc')" :icon="Globe">
      <el-button v-if="tasks.length" :icon="allTasksCollapsed ? ChevronsUpDown : ChevronsDownUp" @click="setAll(tasks.map((tk) => tk.key), !allTasksCollapsed)">
        {{ allTasksCollapsed ? t('common.expandAll') : t('common.collapseAll') }}
      </el-button>
      <el-input
        v-if="tasks.length"
        v-model="gl.filterText.value"
        class="list-filter"
        clearable
        :disabled="gl.sortMode.value"
        :prefix-icon="Search"
        :placeholder="t('ddns.filterPlaceholder')"
      />
      <CompactToggle v-model="compact" />
      <el-button v-if="tasks.length > 1" :type="gl.sortMode.value ? 'primary' : 'default'" :icon="ArrowUpDown" @click="gl.toggleSortMode()">
        {{ gl.sortMode.value ? t('groupSort.sortDone') : t('groupSort.sort') }}
      </el-button>
      <div class="module-toggle">
        <span class="meta">{{ t('ddns.module') }}</span>
        <el-switch v-model="settings.enable" inline-prompt :active-text="t('ddns.switchOn')" :inactive-text="t('ddns.switchOff')" @change="saveSettings" />
      </div>
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('ddns.addTask') }}</el-button>
    </PageHeader>

    <ModuleBanner v-if="moduleOff" :title="t('ddns.moduleOffTitle')" :desc="t('ddns.moduleOffDesc')" :loading="enabling" @enable="enableModule" />

    <SortToolbar
      v-if="gl.sortMode.value"
      :selected-count="gl.selected.value.size"
      :total="tasks.length"
      :groups="gl.groupNames.value"
      :busy="gl.busy.value"
      @select-all="gl.selectAll()"
      @clear="gl.clearSelection()"
      @set-group="gl.setGroup"
    />

    <div v-loading="loading" class="card-list" :class="{ 'is-compact': compact || gl.sortMode.value, 'is-sorting': gl.sortMode.value }">
      <el-empty v-if="!loading && !tasks.length" :description="t('ddns.empty')" />
      <el-empty v-else-if="!loading && !gl.items.value.length" :description="t('groupSort.noMatched')" :image-size="60" />
      <template v-for="it in gl.items.value" :key="it.id">
      <GroupHeader
        v-if="it.kind === 'group'"
        :name="it.name"
        :count-text="t('ddns.groupCount', { count: it.count })"
        :collapsed="it.collapsed"
        :sorting="gl.sortMode.value"
        :can-up="it.canUp"
        :can-down="it.canDown"
        :checked="it.checked"
        :indeterminate="it.indeterminate"
        :class="gl.dropClass(it.id)"
        @toggle="gl.toggleGroup(it.name)"
        @rename="gl.renameGroup(it.name)"
        @up="gl.moveGroup(it.index, -1)"
        @down="gl.moveGroup(it.index, 1)"
        @check="(v: boolean) => gl.setSelected(it.ids, v)"
        @dragover.prevent="gl.dragOver(it.id, $event, false)"
        @dragleave="gl.dragLeave(it.id)"
        @drop.prevent="gl.drop()"
      />
      <EntityCard
        v-else
        :compact="compact || gl.sortMode.value"
        :off="!it.item.enable || moduleOff"
        :collapsible="!gl.sortMode.value"
        :collapsed="isCollapsed(it.item.key)"
        :collapsed-hint="t('ddns.recordsCollapsed', { count: it.item.records.length })"
        :class="[gl.dropClass(it.id), { 'is-grouped': it.grouped }]"
        :draggable="gl.sortMode.value && !isMobile"
        @update:collapsed="(v: boolean) => setCollapsed(it.item.key, v)"
        @dragstart="gl.dragStart(it.id, $event)"
        @dragover.prevent="gl.dragOver(it.id, $event)"
        @dragleave="gl.dragLeave(it.id)"
        @drop.prevent="gl.drop()"
        @dragend="gl.resetDrag()"
      >
        <template #lead>
          <el-checkbox v-if="gl.sortMode.value" :model-value="gl.isSelected(it.id)" @change="(v: string | number | boolean) => gl.setSelected([it.id], Boolean(v))" />
          <el-switch v-else :model-value="it.item.enable" @change="(v: string | number | boolean) => toggle(it.item, Boolean(v))" />
        </template>
        <template #title>
          <span>{{ it.item.name || t('ddns.unnamedTask') }}</span>
          <el-tag size="small" effect="plain">{{ providerTitle(it.item.provider) }}</el-tag>
        </template>
        <template v-if="it.item.remark" #sub>{{ it.item.remark }}</template>
        <template #status>
          <StatusBadge v-bind="taskStatus(it.item)" />
        </template>
        <Field label="IPv4" mono>{{ it.item.ipv4.enable ? it.item.state.ipv4 || (taskActive(it.item) ? t('ddns.fetching') : '-') : t('ddns.notEnabled') }}</Field>
        <Field label="IPv6" mono>{{ it.item.ipv6.enable ? it.item.state.ipv6 || (taskActive(it.item) ? t('ddns.fetching') : '-') : t('ddns.notEnabled') }}</Field>
        <Field :label="t('ddns.lastCheck')">{{ it.item.state.lastRun ? fmtTime(it.item.state.lastRun) : '-' }}</Field>
        <Field :label="t('ddns.nextCheck')">{{ taskActive(it.item) && it.item.state.nextRun ? fmtTime(it.item.state.nextRun) : '-' }}</Field>
        <template v-if="!gl.sortMode.value" #extra>
          <el-alert v-if="it.item.state.ipError && taskActive(it.item)" type="error" :title="it.item.state.ipError" :closable="false" show-icon class="ip-error" />
          <div class="records">
            <div v-for="rec in it.item.records" :key="rec.key" class="rec" :class="{ 'is-off': !rec.enable }">
              <div class="rec-name">
                <span class="mono rec-domain">{{ fqdn(rec) }}</span>
                <el-tag size="small" type="info" class="mono">{{ rec.type }}</el-tag>
              </div>
              <div class="rec-value mono">{{ (rec.key && it.item.state.records?.[rec.key]?.value) || '-' }}</div>
              <div class="rec-status">
                <StatusBadge v-bind="recordStatus(it.item, rec)" />
                <span v-if="recordSync(it.item, rec)" class="meta">{{ recordSync(it.item, rec) }}</span>
              </div>
            </div>
            <div v-if="!it.item.records.length" class="meta">{{ t('ddns.noRecords') }}</div>
          </div>
        </template>
        <template v-if="gl.sortMode.value" #actions>
          <GripVertical v-if="!isMobile" :size="16" class="grip" />
          <el-button text size="small" :icon="ChevronUp" :disabled="!it.canUp" :aria-label="t('groupSort.moveUp')" :title="t('groupSort.moveUp')" @click="gl.moveItem(it.id, -1)" />
          <el-button text size="small" :icon="ChevronDown" :disabled="!it.canDown" :aria-label="t('groupSort.moveDown')" :title="t('groupSort.moveDown')" @click="gl.moveItem(it.id, 1)" />
        </template>
        <template v-else #actions>
          <el-button text size="small" :icon="RefreshCw" :disabled="!taskActive(it.item) || !it.item.state.running" @click="sync(it.item, false)">{{ t('ddns.check') }}</el-button>
          <el-button text size="small" :icon="Zap" :disabled="!taskActive(it.item) || !it.item.state.running" @click="sync(it.item, true)">{{ t('ddns.forceSync') }}</el-button>
          <el-button text size="small" :icon="Pencil" @click="openEditor(it.item)">{{ t('common.edit') }}</el-button>
          <el-popconfirm :title="t('ddns.confirmDelete')" width="240" @confirm="remove(it.item)">
            <template #reference>
              <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
            </template>
          </el-popconfirm>
        </template>
      </EntityCard>
      </template>
    </div>

    <LogCard url="/api/ddns/logs" clearable />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('ddns.editTask') : t('ddns.addTask')" width="900px" top="5vh" :fullscreen="isMobile">
      <el-form :model="editing" label-width="120px" :label-position="labelPosition">
        <el-tabs v-model="tab">
          <el-tab-pane :label="t('ddns.tabBasic')" name="basic">
            <el-form-item :label="t('ddns.taskName')"><el-input v-model="editing.name" /></el-form-item>
            <el-form-item :label="t('groupSort.group')">
              <el-autocomplete
                v-model="editing.group"
                class="w-260"
                clearable
                :maxlength="64"
                :fetch-suggestions="(q: string, cb: (v: { value: string }[]) => void) => cb(groupSuggestions(gl.groupNames.value, q))"
                :placeholder="t('groupSort.groupPlaceholder')"
              />
            </el-form-item>
            <el-form-item :label="t('ddns.dnsProvider')">
              <el-select v-model="editing.provider" filterable class="w-260" @change="onProviderChange">
                <el-option v-for="p in providers" :key="p.name" :label="p.title" :value="p.name" />
              </el-select>
              <el-link v-if="currentProvider?.docUrl" :href="currentProvider.docUrl" target="_blank" type="primary" class="doc">{{ t('ddns.getCredentials') }}</el-link>
            </el-form-item>
            <el-form-item v-if="currentProvider?.idLabel" :label="currentProvider.idLabel">
              <el-input v-model="editing.credentials.id" />
            </el-form-item>
            <el-form-item v-if="currentProvider?.secretLabel" :label="currentProvider.secretLabel">
              <SecretInput v-model="editing.credentials.secret" :placeholder="t('ddns.secretPlaceholder')" :reveal="secretOf(editing.key && `/api/ddns/tasks/${editing.key}/secrets`, 'secret')" />
            </el-form-item>
            <el-form-item v-for="f in currentProvider?.extra ?? []" :key="f.key" :label="f.label" :required="f.required">
              <el-input v-model="extra[f.key]" :type="f.multiline ? 'textarea' : 'text'" :rows="3" :placeholder="f.placeholder" />
            </el-form-item>
            <el-form-item :label="t('ddns.interval')">
              <el-input-number v-model="editing.interval" :min="30" :max="86400" />
            </el-form-item>
            <el-form-item :label="t('ddns.forceInterval')">
              <el-input-number v-model="editing.forceInterval" :min="0" :max="604800" />
              <span class="help">{{ t('ddns.forceHelp') }}</span>
            </el-form-item>
            <el-form-item :label="t('ddns.httpTimeout')">
              <el-input-number v-model="editing.httpTimeout" :min="1" :max="120" />
            </el-form-item>
            <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
            <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>
          </el-tab-pane>

          <el-tab-pane :label="t('ddns.tabRecords')" name="records">
            <div class="rec-edit-head">
              <span>{{ t('common.enable') }}</span><span>{{ t('ddns.host') }}</span><span>{{ t('ddns.zone') }}</span><span>{{ t('ddns.type') }}</span><span>{{ t('ddns.value') }}</span><span>TTL</span>
              <span>{{ currentProvider?.supportsLine ? t('ddns.line') : '' }}</span><span />
            </div>
            <div v-for="(row, i) in editing.records" :key="i" class="rec-edit">
              <div class="re-cell re-enable"><label class="re-label">{{ t('common.enable') }}</label><el-switch v-model="row.enable" size="small" /></div>
              <div class="re-cell"><label class="re-label">{{ t('ddns.host') }}</label><el-input v-model="row.sub" size="small" placeholder="@" /></div>
              <div class="re-cell"><label class="re-label">{{ t('ddns.zone') }}</label><el-input v-model="row.zone" size="small" placeholder="example.com" /></div>
              <div class="re-cell">
                <label class="re-label">{{ t('ddns.type') }}</label>
                <el-select v-model="row.type" size="small" @change="onTypeChange(row)">
                  <el-option v-for="ty in ['A', 'AAAA', 'CNAME', 'TXT']" :key="ty" :label="ty" :value="ty" />
                </el-select>
              </div>
              <div class="re-cell"><label class="re-label">{{ t('ddns.value') }}</label><el-input v-model="row.value" size="small" class="mono" /></div>
              <div class="re-cell"><label class="re-label">TTL</label><el-input-number v-model="row.ttl" size="small" :min="0" :controls="false" class="re-ttl" /></div>
              <div class="re-cell">
                <template v-if="currentProvider?.supportsLine">
                  <label class="re-label">{{ t('ddns.line') }}</label>
                  <el-input v-model="row.line" size="small" :placeholder="t('ddns.default')" />
                </template>
              </div>
              <div class="re-cell re-del">
                <el-button text type="danger" size="small" :icon="Trash2" :aria-label="t('ddns.deleteRecord')" @click="editing.records.splice(i, 1)" />
              </div>
            </div>
            <el-empty v-if="!editing.records.length" :description="t('ddns.noRecordsYet')" :image-size="60" />
            <div class="records-ops">
              <el-button size="small" :icon="Plus" @click="addRecord('A')">{{ t('ddns.addV4') }}</el-button>
              <el-button size="small" :icon="Plus" @click="addRecord('AAAA')">{{ t('ddns.addV6') }}</el-button>
            </div>
            <div class="form-help">
              <i18n-t keypath="ddns.recordValueHelp" tag="span">
                <template #v4><span class="mono">{ipv4Addr}</span></template>
                <template #v6><span class="mono">{ipv6Addr}</span></template>
                <template #stun>
                  <span class="mono">{{ `{STUN_${t('ddns.stunRuleName')}_PORT} {STUN_${t('ddns.stunRuleName')}_IP} {STUN_${t('ddns.stunRuleName')}_ADDR}` }}</span>
                </template>
              </i18n-t>
            </div>
          </el-tab-pane>

          <el-tab-pane :label="t('ddns.tabIp')" name="ip">
            <IPSourceEditor v-model="editing.ipv4" :family="4" :interfaces="interfaces" />
            <el-divider />
            <IPSourceEditor v-model="editing.ipv6" :family="6" :interfaces="interfaces" />
          </el-tab-pane>

          <el-tab-pane label="Webhook" name="webhook">
            <WebhookEditor
              v-model="editing.webhook"
              :variables="['ipAddr', 'ipv4Addr', 'ipv6Addr', 'successDomains', 'failedDomains', 'successDomainsLine', 'failedDomainsLine', 'taskName', 'time']"
            />
          </el-tab-pane>
        </el-tabs>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { ArrowUpDown, ChevronDown, ChevronsDownUp, ChevronsUpDown, ChevronUp, Globe, GripVertical, Pencil, Plus, RefreshCw, Search, Trash2, Zap } from 'lucide-vue-next'
import { api } from '../api'
import GroupHeader from '../components/GroupHeader.vue'
import SortToolbar from '../components/SortToolbar.vue'
import { useGroupedList } from '../composables/useGroupSort'
import { groupSuggestions } from '../groupSort'
import SecretInput from '../components/SecretInput.vue'
import { secretOf } from '../secrets'
import PageHeader from '../components/PageHeader.vue'
import ModuleBanner from '../components/ModuleBanner.vue'
import CompactToggle from '../components/CompactToggle.vue'
import EntityCard from '../components/EntityCard.vue'
import { useCompactView } from '../composables/useCompactView'
import { useCollapsed } from '../composables/useCollapsed'
import Field from '../components/Field.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import WebhookEditor, { type WebhookConfig } from '../components/WebhookEditor.vue'
import IPSourceEditor, { type IPSource, type NetInterface } from '../components/IPSourceEditor.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'

interface ProviderMeta {
  name: string
  title: string
  idLabel?: string
  secretLabel?: string
  supportsLine?: boolean
  docUrl?: string
  extra?: { key: string; label: string; placeholder?: string; multiline?: boolean; required?: boolean }[]
}

interface DNSRecord {
  key?: string
  enable: boolean
  zone: string
  sub: string
  type: string
  value: string
  ttl: number
  line: string
  remark: string
}

interface Task {
  key?: string
  name: string
  enable: boolean
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  ipv4: IPSource
  ipv6: IPSource
  provider: string
  credentials: { id: string; secret: string; extra?: Record<string, string> }
  records: DNSRecord[]
  interval: number
  forceInterval: number
  httpTimeout: number
  webhook: WebhookConfig
  remark: string
}

interface RecordState {
  value?: string
  lastSync?: string
  lastAction?: string
  lastError?: string
}

interface TaskView extends Task {
  key: string
  state: {
    ipv4?: string
    ipv6?: string
    ipError?: string
    lastRun?: string
    nextRun?: string
    running: boolean
    records: Record<string, RecordState> | null
  }
}

type Badge = { type: StatusType; text: string; title?: string }

const { t } = useI18n()
const compact = useCompactView('ddns')
const { isMobile, labelPosition } = useFormLayout()
const tasks = ref<TaskView[]>([])
// 每个任务的解析记录列表可单独收起，状态按任务 key 持久化
const { isCollapsed, setCollapsed, setAll, allCollapsed } = useCollapsed('ddns')
const allTasksCollapsed = allCollapsed(() => tasks.value.map((tk) => tk.key))
const providers = ref<ProviderMeta[]>([])
const interfaces = ref<NetInterface[]>([])
const defaults = reactive({ ipv4URLs: [] as string[], ipv6URLs: [] as string[] })
const settings = reactive({ enable: true })
const settingsLoaded = ref(false)
const loading = ref(true)
const editorVisible = ref(false)
const saving = ref(false)
const enabling = ref(false)
const tab = ref('basic')
const editing = ref<Task>(emptyTask())
const extra = ref<Record<string, string>>({})
// 定时刷新，标签页在后台时暂停
const polling = usePolling(load, 5000)

// 列表的筛选、分组与排序；顺序与分组的修改不会重启任务
const gl = useGroupedList<TaskView>({
  storageKey: 'ddns.groups',
  items: () => tasks.value,
  base: '/api/ddns/tasks',
  reload: load,
  matches: (tk, q) =>
    [tk.name, tk.group ?? '', tk.remark, providerTitle(tk.provider), ...tk.records.map(fqdn)].join('\n').toLowerCase().includes(q),
})

const currentProvider = computed(() => providers.value.find((p) => p.name === editing.value.provider))
const moduleOff = computed(() => settingsLoaded.value && !settings.enable)

function emptySource(urls: string[]): IPSource {
  return { enable: false, type: 'url', urls: [...urls], interface: '', regex: '', command: '' }
}

function emptyTask(): Task {
  return {
    name: '',
    enable: true,
    group: '',
    ipv4: { ...emptySource(defaults.ipv4URLs), enable: true },
    ipv6: emptySource(defaults.ipv6URLs),
    provider: 'alidns',
    credentials: { id: '', secret: '' },
    records: [],
    interval: 300,
    forceInterval: 3600,
    httpTimeout: 20,
    webhook: { enable: false, url: '', method: 'GET', headers: [], body: '', successContent: [], timeout: 0, insecureSkipVerify: false },
    remark: '',
  }
}

function providerTitle(name: string) {
  return providers.value.find((p) => p.name === name)?.title ?? name
}

function fqdn(r: DNSRecord) {
  return !r.sub || r.sub === '@' ? r.zone : `${r.sub}.${r.zone}`
}

function fmtTime(iso: string) {
  const d = new Date(iso)
  // 当天只显示时间，其余显示日期与时间
  return d.toDateString() === new Date().toDateString() ? d.toLocaleTimeString() : d.toLocaleString()
}

function actionText(a?: string) {
  return ({ created: t('ddns.actCreated'), updated: t('ddns.actUpdated'), unchanged: t('ddns.actUnchanged') } as Record<string, string>)[a ?? ''] ?? t('ddns.actSynced')
}

// taskActive 表示任务实际处于运行条件：模块与任务均已启用
function taskActive(tk: TaskView) {
  return !moduleOff.value && tk.enable
}

function recState(tk: TaskView, r: DNSRecord): RecordState | undefined {
  return r.key ? tk.state.records?.[r.key] : undefined
}

// taskStatus 汇总任务状态。停用的任务只显示“已停用”，不会显示“等待检查”
function taskStatus(tk: TaskView): Badge {
  if (moduleOff.value) {
    return { type: 'off', text: t('ddns.moduleOff') }
  }
  if (!tk.enable) {
    return { type: 'off', text: t('ddns.deactivated') }
  }
  if (tk.state.ipError) {
    return { type: 'error', text: t('ddns.ipFailed'), title: tk.state.ipError }
  }
  const enabled = tk.records.filter((r) => r.enable)
  const failed = enabled.filter((r) => recState(tk, r)?.lastError).length
  if (failed) {
    return { type: 'error', text: t('ddns.syncFailed', { count: failed }) }
  }
  if (!tk.state.running) {
    return { type: 'pending', text: t('ddns.starting') }
  }
  if (!enabled.length) {
    return { type: 'warn', text: t('ddns.noEnabledRecords') }
  }
  if (enabled.every((r) => recState(tk, r))) {
    return { type: 'ok', text: t('ddns.ok') }
  }
  return { type: 'pending', text: t('ddns.waiting') }
}

// recordStatus 计算单条记录状态：模块停用、任务停用、记录停用都显示为“已停用”
function recordStatus(tk: TaskView, r: DNSRecord): Badge {
  if (moduleOff.value) {
    return { type: 'off', text: t('ddns.moduleOff') }
  }
  if (!tk.enable) {
    return { type: 'off', text: t('ddns.deactivated'), title: t('ddns.taskDeactivated') }
  }
  if (!r.enable) {
    return { type: 'off', text: t('ddns.deactivated'), title: t('ddns.recordDeactivated') }
  }
  const st = recState(tk, r)
  if (st?.lastError) {
    return { type: 'error', text: st.lastError, title: st.lastError }
  }
  if (st) {
    return { type: 'ok', text: actionText(st.lastAction) }
  }
  return { type: 'pending', text: t('ddns.waiting') }
}

function recordSync(tk: TaskView, r: DNSRecord) {
  const st = recState(tk, r)
  if (!taskActive(tk) || !r.enable || !st?.lastSync) {
    return ''
  }
  return fmtTime(st.lastSync)
}

async function load() {
  try {
    tasks.value = await api.get<TaskView[]>('/api/ddns/tasks')
  } finally {
    loading.value = false
  }
}

function openEditor(tk?: TaskView) {
  if (tk) {
    const { state: _state, ...task } = tk
    editing.value = JSON.parse(JSON.stringify(task))
  } else {
    editing.value = emptyTask()
    addRecord('A')
  }
  extra.value = { ...(editing.value.credentials.extra ?? {}) }
  tab.value = 'basic'
  editorVisible.value = true
}

function onProviderChange() {
  extra.value = {}
}

function addRecord(type: 'A' | 'AAAA') {
  const last = editing.value.records[editing.value.records.length - 1]
  editing.value.records.push({
    enable: true,
    zone: last?.zone ?? '',
    sub: '',
    type,
    value: type === 'A' ? '{ipv4Addr}' : '{ipv6Addr}',
    ttl: 0,
    line: '',
    remark: '',
  })
  if (type === 'AAAA') {
    editing.value.ipv6.enable = true
  } else {
    editing.value.ipv4.enable = true
  }
}

function onTypeChange(r: DNSRecord) {
  if (r.type === 'A' && (r.value === '' || r.value === '{ipv6Addr}')) {
    r.value = '{ipv4Addr}'
  } else if (r.type === 'AAAA' && (r.value === '' || r.value === '{ipv4Addr}')) {
    r.value = '{ipv6Addr}'
  }
}

async function save() {
  editing.value.credentials.extra = { ...extra.value }
  saving.value = true
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/ddns/tasks/${key}`, body)
    } else {
      await api.post('/api/ddns/tasks', body)
    }
    editorVisible.value = false
    ElMessage.success(body.enable && settings.enable ? t('ddns.savedChecking') : t('common.saved'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function toggle(tk: TaskView, enable: boolean) {
  try {
    await api.put(`/api/ddns/tasks/${tk.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function sync(tk: TaskView, force: boolean) {
  try {
    await api.post(`/api/ddns/tasks/${tk.key}/sync${force ? '?force=1' : ''}`)
    ElMessage.success(t('ddns.triggered'))
    window.setTimeout(load, 1500)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(tk: TaskView) {
  try {
    await api.del(`/api/ddns/tasks/${tk.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function saveSettings() {
  try {
    await api.put('/api/ddns/settings', settings)
    await load()
  } catch (e) {
    settings.enable = !settings.enable
    ElMessage.error((e as Error).message)
  }
}

async function enableModule() {
  enabling.value = true
  settings.enable = true
  await saveSettings()
  enabling.value = false
}

onMounted(async () => {
  // 任务列表与元数据一起并行请求，首屏只需一次往返；数据一并赋值，避免服务商名称先显示内部名再跳成标题
  try {
    const [p, i, d, s, tk] = await Promise.all([
      api.get<ProviderMeta[]>('/api/ddns/providers'),
      api.get<NetInterface[]>('/api/ddns/netinterfaces'),
      api.get<{ ipv4URLs: string[]; ipv6URLs: string[] }>('/api/ddns/defaults'),
      api.get<{ enable: boolean }>('/api/ddns/settings'),
      api.get<TaskView[]>('/api/ddns/tasks'),
    ])
    providers.value = p
    interfaces.value = i
    Object.assign(defaults, d)
    settings.enable = s.enable
    settingsLoaded.value = true
    tasks.value = tk
  } finally {
    loading.value = false
  }
  polling.start()
})
</script>

<style scoped>
.list-filter {
  width: 220px;
}
.card-list > .group-header {
  border-radius: var(--g-radius);
}
.card-list > .is-grouped {
  margin-left: 16px;
}
.card-list.is-sorting > .entity[draggable='true'] {
  cursor: grab;
}
.card-list > .entity.dragging {
  opacity: 0.4;
}
.card-list > .drop-before {
  box-shadow: 0 -3px 0 var(--el-color-primary);
}
.card-list > .drop-after {
  box-shadow: 0 3px 0 var(--el-color-primary);
}
.grip {
  color: var(--el-text-color-secondary);
  cursor: grab;
}
.module-toggle {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 10px;
  border: 1px solid var(--g-border);
  border-radius: 8px;
  background: var(--el-bg-color);
}
.ip-error {
  margin-bottom: 10px;
}
.records {
  border: 1px solid var(--g-border);
  border-radius: 8px;
  overflow: hidden;
}
.records > .meta {
  padding: 10px 12px;
}
.rec {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 1.4fr) minmax(0, 1.6fr);
  gap: 6px 16px;
  align-items: center;
  padding: 9px 12px;
  font-size: 13px;
}
.rec + .rec {
  border-top: 1px solid var(--g-border);
}
.rec.is-off .rec-name,
.rec.is-off .rec-value {
  opacity: 0.55;
}
.rec-name {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.rec-domain {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rec-value {
  color: var(--el-text-color-regular);
  word-break: break-all;
}
.rec-status {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.rec-status :deep(.status-badge) {
  max-width: 220px;
}
.doc {
  margin-left: 12px;
}
.w-260 {
  width: 260px;
}
/* 记录编辑行：桌面为表格式网格，移动端为卡片 */
.rec-edit-head,
.rec-edit {
  display: grid;
  grid-template-columns: 44px 110px minmax(0, 1.3fr) 90px minmax(0, 1.2fr) 70px 90px 36px;
  gap: 8px;
  align-items: center;
}
.rec-edit-head {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  padding: 0 4px 6px;
}
.rec-edit {
  padding: 6px 4px;
  border-top: 1px solid var(--g-border);
}
.re-label {
  display: none;
}
.re-ttl {
  width: 100%;
}
.records-ops {
  margin-top: 12px;
}
@media (max-width: 767.98px) {
  .list-filter {
    width: 160px;
  }
  .card-list > .is-grouped {
    margin-left: 8px;
  }
  .rec {
    grid-template-columns: 1fr;
    gap: 4px;
  }
  .w-260 {
    width: 100%;
  }
  .doc {
    margin: 6px 0 0;
  }
  .rec-edit-head {
    display: none;
  }
  .rec-edit {
    grid-template-columns: 1fr 1fr;
    gap: 8px 10px;
    padding: 12px;
    margin-bottom: 10px;
    border: 1px solid var(--g-border);
    border-radius: 8px;
    position: relative;
  }
  .re-label {
    display: block;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-bottom: 2px;
  }
  .re-cell:empty {
    display: none;
  }
  .re-enable {
    grid-column: 1 / -1;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .re-enable .re-label {
    margin: 0;
  }
  .re-del {
    position: absolute;
    top: 6px;
    right: 6px;
  }
  .re-del + * {
    display: none;
  }
}
</style>
