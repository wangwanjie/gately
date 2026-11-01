<template>
  <div>
    <PageHeader :title="t('ddns.title')" :desc="t('ddns.desc')" :icon="Globe">
      <el-button v-if="tasks.length" :icon="allTasksCollapsed ? ChevronsUpDown : ChevronsDownUp" @click="setAll(tasks.map((tk) => tk.key), !allTasksCollapsed)">
        {{ allTasksCollapsed ? t('common.expandAll') : t('common.collapseAll') }}
      </el-button>
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('ddns.filterPlaceholder')" />
      <el-button :icon="SlidersHorizontal" @click="openSettings">{{ t('ddns.globalSettings') }}</el-button>
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('ddns.addTask') }}</el-button>
    </PageHeader>

    <ModuleBanner v-if="moduleOff" :title="t('ddns.moduleOffTitle')" :desc="t('ddns.moduleOffDesc')" :loading="enabling" @enable="enableModule" />

    <GroupedList :gl="gl" :loading="loading" :compact="compact" :empty-text="t('ddns.empty')" :empty-image-size="120" :count-text="(n: number) => t('ddns.groupCount', { count: n })">
      <template #item="{ it, item: tk, sorting, bind }">
        <EntityCard
          v-bind="bind"
          :compact="compact || sorting"
          :off="!tk.enable || moduleOff"
          :collapsible="!sorting"
          :collapsed="isCollapsed(tk.key)"
          :collapsed-hint="t('ddns.recordsCollapsed', { count: tk.records.length })"
          @update:collapsed="(v: boolean) => setCollapsed(tk.key, v)"
        >
          <template #lead>
            <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
            <el-switch v-else :model-value="tk.enable" @change="(v: string | number | boolean) => toggle(tk, Boolean(v))" />
          </template>
          <template #title>
            <span>{{ tk.name || t('ddns.unnamedTask') }}</span>
            <el-tag size="small" effect="plain">{{ providerTitle(tk.provider) }}</el-tag>
          </template>
          <template v-if="tk.remark" #sub>{{ tk.remark }}</template>
          <template #status>
            <StatusBadge v-bind="taskStatus(tk)" />
          </template>
          <Field label="IPv4" mono>{{ tk.ipv4.enable ? tk.state.ipv4 || (taskActive(tk) ? t('ddns.fetching') : '-') : t('ddns.notEnabled') }}</Field>
          <Field label="IPv6" mono>{{ tk.ipv6.enable ? tk.state.ipv6 || (taskActive(tk) ? t('ddns.fetching') : '-') : t('ddns.notEnabled') }}</Field>
          <Field :label="t('ddns.lastCheck')">{{ tk.state.lastRun ? fmtTime(tk.state.lastRun) : '-' }}</Field>
          <Field :label="t('ddns.nextCheck')">{{ taskActive(tk) && tk.state.nextRun ? fmtTime(tk.state.nextRun) : '-' }}</Field>
          <template v-if="!sorting" #extra>
            <el-alert v-if="tk.state.ipError && taskActive(tk)" type="error" :title="tk.state.ipError" :closable="false" show-icon class="ip-error" />
            <div class="records">
              <template v-for="sec in recordSections(tk)" :key="sec.name">
                <!-- 记录有分组时按分组显示，每组可单独收起；没有分组时直接列出 -->
                <GroupHeader
                  v-if="sec.grouped"
                  class="rec-group"
                  :name="sec.name"
                  :count-text="t('ddns.recordCount', { count: sec.items.length })"
                  :collapsed="recordGroupCollapsed(tk, sec.name)"
                  @toggle="toggleRecordGroup(tk, sec.name)"
                  @rename="renameRecordGroup(tk, sec.name)"
                />
                <template v-if="!sec.grouped || !recordGroupCollapsed(tk, sec.name)">
                  <div v-for="rec in sec.items" :key="rec.key" class="rec" :class="{ 'is-off': !rec.enable, 'is-grouped': sec.grouped }">
                    <div class="rec-name">
                      <span class="mono rec-domain">{{ fqdn(rec) }}</span>
                      <el-tag size="small" type="info" class="mono">{{ rec.type }}</el-tag>
                      <!-- 备注单独占一行，不挤占域名的宽度 -->
                      <span v-if="rec.remark" class="meta rec-remark" :title="rec.remark">{{ rec.remark }}</span>
                    </div>
                    <div class="rec-value mono">{{ (rec.key && tk.state.records?.[rec.key]?.value) || '-' }}</div>
                    <div class="rec-status">
                      <StatusBadge v-bind="recordStatus(tk, rec)" />
                      <span v-if="recordSync(tk, rec)" class="meta">{{ recordSync(tk, rec) }}</span>
                    </div>
                    <!-- 不进入编辑页直接删除单条记录；任务至少保留一条记录，最后一条时禁用 -->
                    <div class="rec-ops">
                      <el-button
                        v-if="rec.key && tk.records.length > 1"
                        text
                        type="danger"
                        size="small"
                        class="rec-del"
                        :icon="Trash2"
                        :aria-label="t('ddns.deleteRecord')"
                        :title="t('ddns.deleteRecord')"
                        @click="removeRecord(tk, rec)"
                      />
                      <el-button
                        v-else
                        text
                        size="small"
                        class="rec-del"
                        :icon="Trash2"
                        disabled
                        :aria-label="t('ddns.deleteRecord')"
                        :title="t('ddns.lastRecordHint')"
                      />
                    </div>
                  </div>
                </template>
              </template>
              <div v-if="!tk.records.length" class="meta">{{ t('ddns.noRecords') }}</div>
            </div>
          </template>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="RefreshCw" :disabled="!taskActive(tk) || !tk.state.running" @click="sync(tk, false)">{{ t('ddns.check') }}</el-button>
            <el-button text size="small" :icon="Zap" :disabled="!taskActive(tk) || !tk.state.running" @click="sync(tk, true)">{{ t('ddns.forceSync') }}</el-button>
            <el-button v-if="tk.records.length > 1" text size="small" :icon="ArrowUpDown" @click="openRecordSort(tk)">{{ t('ddns.sortRecords') }}</el-button>
            <el-button text size="small" :icon="Pencil" @click="openEditor(tk)">{{ t('common.edit') }}</el-button>
            <el-button text size="small" type="danger" :icon="Trash2" @click="remove(tk)">{{ t('common.delete') }}</el-button>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <SortDialog v-model="recordSort.visible" :title="recordSort.title" :entries="recordSort.entries" :save="recordSort.save" mono />

    <LogCard url="/api/ddns/logs" clearable />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('ddns.editTask') : t('ddns.addTask')" width="900px" top="5vh" :fullscreen="isMobile">
      <el-form :model="editing" label-width="120px" :label-position="labelPosition">
        <el-tabs v-model="tab">
          <el-tab-pane :label="t('ddns.tabBasic')" name="basic">
            <el-form-item :label="t('ddns.taskName')"><el-input v-model="editing.name" /></el-form-item>
            <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
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
            <el-form-item :label="t('common.remark')">
              <el-input v-model="editing.remark" />
              <span v-if="currentProvider?.supportsRemark" class="help">{{ t('ddns.taskRemarkHelp') }}</span>
            </el-form-item>
            <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>
          </el-tab-pane>

          <el-tab-pane :label="t('ddns.tabRecords')" name="records">
            <div class="rec-edit-head">
              <span>{{ t('common.enable') }}</span><span>{{ t('ddns.host') }}</span><span>{{ t('ddns.zone') }}</span><span>{{ t('ddns.type') }}</span><span>{{ t('ddns.value') }}</span><span>TTL</span>
              <span>{{ t('common.remark') }}</span><span>{{ currentProvider?.supportsLine ? t('ddns.line') : '' }}</span><span />
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
                <label class="re-label">{{ t('common.remark') }}</label>
                <el-input v-model="row.remark" size="small" :maxlength="100" :placeholder="editing.remark || t('ddns.remarkPlaceholder')" />
              </div>
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
              <el-button v-if="editing.records.length > 1" size="small" :icon="ArrowUpDown" @click="openEditingRecordSort">{{ t('ddns.sortRecords') }}</el-button>
            </div>
            <div v-if="editing.records.length" class="form-help">
              {{ currentProvider?.supportsRemark ? t('ddns.remarkHelp') : t('ddns.remarkLocalOnly') }}
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

    <el-dialog v-model="settingsVisible" :title="t('ddns.settingsTitle')" width="560px" :fullscreen="isMobile">
      <el-form :model="settingsForm" label-width="180px" :label-position="labelPosition">
        <el-form-item :label="t('ddns.enableModule')">
          <el-switch v-model="settingsForm.enable" />
          <span class="help">{{ t('ddns.moduleOffDesc') }}</span>
        </el-form-item>
        <el-form-item :label="t('ddns.deleteRemoteDefault')">
          <el-switch v-model="settingsForm.deleteRemote" />
          <span class="help">{{ t('ddns.deleteRemoteDefaultHelp') }}</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settingsVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="settingsSaving" @click="submitSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 删除记录或任务的确认框：服务商支持时可勾选同时删除服务商处的解析，默认值取全局设置 -->
    <el-dialog v-model="delDlg.visible" :title="delDlg.title" width="480px" :fullscreen="isMobile" append-to-body>
      <p class="del-msg">{{ delDlg.message }}</p>
      <ul v-if="delDlg.domains.length" class="del-list mono">
        <li v-for="d in delDlg.domains" :key="d">{{ d }}</li>
      </ul>
      <template v-if="delDlg.canRemote">
        <el-checkbox v-model="delDlg.remote">{{ t('ddns.deleteRemote') }}</el-checkbox>
        <div class="form-help">{{ delDlg.remote ? t('ddns.deleteRemoteHint') : t('ddns.deleteLocalOnly') }}</div>
      </template>
      <div v-else class="form-help">{{ t('ddns.deleteRemoteUnsupported', { provider: delDlg.provider }) }}</div>
      <template #footer>
        <el-button @click="delDlg.visible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :loading="delDlg.loading" @click="confirmDelete">{{ delDlg.confirmText }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { ArrowUpDown, ChevronsDownUp, ChevronsUpDown, Globe, Pencil, Plus, RefreshCw, SlidersHorizontal, Trash2, Zap } from 'lucide-vue-next'
import { api } from '../api'
import GroupHeader from '../components/GroupHeader.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortDialog from '../components/SortDialog.vue'
import SortMoves from '../components/SortMoves.vue'
import { useGroupRename, useGroupedList } from '../composables/useGroupSort'
import { buildSections, renameSection, type SortEntry } from '../groupSort'
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
  // supportsRemark 表示记录备注会写到服务商处，supportsDelete 表示删除时可同时删除服务商处的解析
  supportsRemark?: boolean
  supportsDelete?: boolean
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
  // group 为任务内记录列表中的分组名称，只影响展示
  group?: string
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
const settings = reactive({ enable: true, deleteRemote: false })
// 全局设置对话框的编辑副本，保存成功后才写回 settings
const settingsVisible = ref(false)
const settingsSaving = ref(false)
const settingsForm = reactive({ enable: true, deleteRemote: false })
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

// 任务内解析记录的分组：分组的收起状态按 任务key/分组名 持久化
const recordFold = useCollapsed('ddns.records')
const askRecordGroup = useGroupRename()
// recordSort 为记录排序对话框的状态，卡片上与编辑对话框中共用
const recordSort = reactive<{ visible: boolean; title: string; entries: SortEntry[]; save: (entries: SortEntry[]) => Promise<void> | void }>({
  visible: false,
  title: '',
  entries: [],
  save: () => {},
})

// recordSections 把任务的记录按分组归类；没有任何分组时只有一节且不显示标题
function recordSections(tk: Task) {
  const secs = buildSections(tk.records, (r) => r.group ?? '')
  const grouped = secs.some((s) => s.name)
  return secs.map((s) => ({ ...s, grouped }))
}

function recordGroupCollapsed(tk: TaskView, name: string) {
  return recordFold.isCollapsed(`${tk.key}/${name}`)
}

function toggleRecordGroup(tk: TaskView, name: string) {
  recordFold.setCollapsed(`${tk.key}/${name}`, !recordGroupCollapsed(tk, name))
}

// saveRecordLayout 保存记录的新顺序与分组，只改配置，不重启任务
async function saveRecordLayout(tk: TaskView, records: { key: string; group: string }[]) {
  await api.put(`/api/ddns/tasks/${tk.key}/records-layout`, { records })
  await load()
}

// renameRecordGroup 重命名任务内的记录分组，位置与顺序不变；与已有分组同名时合并
async function renameRecordGroup(tk: TaskView, name: string) {
  const secs = buildSections(tk.records, (r) => r.group ?? '')
  const to = await askRecordGroup(
    name,
    secs.map((s) => s.name).filter(Boolean),
  )
  if (to === null) {
    return
  }
  const next = renameSection(secs, name, to)
  if (!next) {
    return
  }
  try {
    await saveRecordLayout(tk, next.flatMap((sec) => sec.items.map((r) => ({ key: r.key ?? '', group: sec.name }))))
    recordFold.setCollapsed(`${tk.key}/${to}`, recordGroupCollapsed(tk, name) || recordGroupCollapsed(tk, to))
    recordFold.setCollapsed(`${tk.key}/${name}`, false)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// recordEntry 把记录转换为排序对话框中的一项
function recordEntry(r: DNSRecord, id: string): SortEntry {
  return { id, label: fqdn(r) || '-', sub: `${r.type} · ${r.value}`, group: r.group ?? '', off: !r.enable }
}

// openRecordSort 在卡片上调整已保存任务的记录顺序与分组，保存后立即生效
function openRecordSort(tk: TaskView) {
  recordSort.title = t('ddns.sortRecordsTitle', { name: tk.name || t('ddns.unnamedTask') })
  recordSort.entries = tk.records.map((r) => recordEntry(r, r.key ?? ''))
  recordSort.save = (entries) => saveRecordLayout(tk, entries.map((e) => ({ key: e.id, group: e.group })))
  recordSort.visible = true
}

// openEditingRecordSort 在编辑对话框中调整记录顺序与分组，随任务一起保存；新记录还没有 key，用位置标识
function openEditingRecordSort() {
  const rows = editing.value.records
  const ids = rows.map((r, i) => r.key || `#${i}`)
  recordSort.title = t('ddns.sortRecordsTitle', { name: editing.value.name || t('ddns.unnamedTask') })
  recordSort.entries = rows.map((r, i) => recordEntry(r, ids[i]))
  recordSort.save = (entries) => {
    const byId = new Map(rows.map((r, i) => [ids[i], r]))
    editing.value.records = entries.map((e) => ({ ...byId.get(e.id)!, group: e.group }))
  }
  recordSort.visible = true
}

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
  // 编辑已保存的任务时删掉了已有记录：先确认是否同时从服务商删除，再保存
  const stored = editing.value.key ? tasks.value.find((tk) => tk.key === editing.value.key) : undefined
  const kept = new Set(editing.value.records.map((r) => r.key).filter(Boolean))
  const removed = stored?.records.filter((r) => r.key && !kept.has(r.key)) ?? []
  if (stored && removed.length) {
    askDelete({
      title: t('ddns.removeRecordsTitle'),
      message: t('ddns.removeRecordsMsg', { count: removed.length }),
      domains: removed.map((r) => `${r.type} ${fqdn(r)}`),
      provider: stored.provider,
      confirmText: t('common.save'),
      run: async (remote) => {
        if (remote) {
          await api.post(`/api/ddns/tasks/${stored.key}/remote-delete`, { records: removed.map((r) => r.key) })
        }
        await doSave()
      },
    })
    return
  }
  try {
    await doSave()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function doSave() {
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

// delDlg 为删除确认框的状态；run 按是否勾选“同时从服务商删除”执行删除，失败时保留对话框
const delDlg = reactive({
  visible: false,
  loading: false,
  title: '',
  message: '',
  domains: [] as string[],
  provider: '',
  canRemote: false,
  remote: false,
  confirmText: '',
  run: (async () => {}) as (remote: boolean) => Promise<void>,
})

function askDelete(o: { title: string; message: string; domains: string[]; provider: string; confirmText?: string; run: (remote: boolean) => Promise<void> }) {
  const meta = providers.value.find((p) => p.name === o.provider)
  Object.assign(delDlg, {
    ...o,
    provider: meta?.title ?? o.provider,
    canRemote: !!meta?.supportsDelete,
    remote: !!meta?.supportsDelete && settings.deleteRemote,
    confirmText: o.confirmText ?? t('common.delete'),
    loading: false,
    visible: true,
  })
}

async function confirmDelete() {
  if (delDlg.loading) {
    return
  }
  delDlg.loading = true
  try {
    await delDlg.run(delDlg.canRemote && delDlg.remote)
    delDlg.visible = false
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    delDlg.loading = false
  }
}

function remove(tk: TaskView) {
  askDelete({
    title: t('ddns.deleteTaskTitle'),
    message: t('ddns.confirmDeleteTask', { name: tk.name || t('ddns.unnamedTask'), count: tk.records.length }),
    domains: tk.records.map((r) => `${r.type} ${fqdn(r)}`),
    provider: tk.provider,
    run: async (remote) => {
      if (remote) {
        await api.post(`/api/ddns/tasks/${tk.key}/remote-delete`, { records: [] })
      }
      await api.del(`/api/ddns/tasks/${tk.key}`)
      ElMessage.success(remote ? t('ddns.deletedRemote') : t('common.deleted'))
      await load()
    },
  })
}

function removeRecord(tk: TaskView, rec: DNSRecord) {
  if (!rec.key) {
    return
  }
  const key = rec.key
  askDelete({
    title: t('ddns.deleteRecord'),
    message: t('ddns.confirmDeleteRecord', { name: fqdn(rec) }),
    domains: [],
    provider: tk.provider,
    run: async (remote) => {
      if (remote) {
        await api.post(`/api/ddns/tasks/${tk.key}/remote-delete`, { records: [key] })
      }
      await api.del(`/api/ddns/tasks/${tk.key}/records/${encodeURIComponent(key)}`)
      ElMessage.success(remote ? t('ddns.deletedRemote') : t('common.deleted'))
      await load()
    },
  })
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

function openSettings() {
  Object.assign(settingsForm, settings)
  settingsVisible.value = true
}

async function submitSettings() {
  settingsSaving.value = true
  try {
    await api.put('/api/ddns/settings', settingsForm)
    Object.assign(settings, settingsForm)
    settingsVisible.value = false
    ElMessage.success(t('ddns.settingsSaved'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    settingsSaving.value = false
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
      api.get<{ enable: boolean; deleteRemote?: boolean }>('/api/ddns/settings'),
      api.get<TaskView[]>('/api/ddns/tasks'),
    ])
    providers.value = p
    interfaces.value = i
    Object.assign(defaults, d)
    settings.enable = s.enable
    settings.deleteRemote = !!s.deleteRemote
    settingsLoaded.value = true
    tasks.value = tk
  } finally {
    loading.value = false
  }
  polling.start()
})
</script>

<style scoped>
.del-msg {
  margin: 0 0 8px;
}
.del-list {
  margin: 0 0 12px;
  padding-left: 20px;
  max-height: 180px;
  overflow: auto;
  font-size: 13px;
}
.rec-remark {
  flex-basis: 100%;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
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
  grid-template-columns: minmax(0, 2fr) minmax(0, 1.4fr) minmax(0, 1.6fr) auto;
  gap: 6px 16px;
  align-items: center;
  padding: 9px 12px;
  font-size: 13px;
}
.records > * + * {
  border-top: 1px solid var(--g-border);
}
.rec.is-grouped {
  padding-left: 32px;
}
.rec.is-off .rec-name,
.rec.is-off .rec-value {
  opacity: 0.55;
}
.rec-name {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px 8px;
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
.rec-ops {
  display: flex;
  justify-content: flex-end;
}
.rec-del {
  margin: 0;
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
  grid-template-columns: 44px 100px minmax(0, 1.2fr) 80px minmax(0, 1.1fr) 60px minmax(0, 1fr) 80px 36px;
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
  /* 移动端记录纵向排列，删除按钮固定在右上角 */
  .rec {
    grid-template-columns: 1fr;
    gap: 4px;
    position: relative;
    padding-right: 40px;
  }
  .rec-ops {
    position: absolute;
    top: 6px;
    right: 6px;
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
