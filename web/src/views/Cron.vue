<template>
  <div>
    <PageHeader :title="t('cron.title')" :desc="t('cron.desc')" :icon="CalendarClock">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('groupSort.search')" />
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('cron.addTask') }}</el-button>
    </PageHeader>

    <GroupedList :gl="gl" :compact="compact" :empty-text="t('cron.empty')">
      <template #item="{ it, item: tk, sorting, bind }">
        <EntityCard v-bind="bind" :off="!tk.enable" :compact="compact || sorting">
          <template #lead>
            <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
            <el-switch v-else :model-value="tk.enable" @change="(v: string | number | boolean) => toggle(tk, Boolean(v))" />
          </template>
          <template #title>
            <span>{{ tk.name }}</span>
            <el-tag size="small" effect="plain">{{ tk.type === 'http' ? t('cron.typeHttp') : t('cron.typeCommand') }}</el-tag>
          </template>
          <template v-if="tk.remark" #sub>{{ tk.remark }}</template>
          <template #status>
            <StatusBadge v-bind="taskStatus(tk)" />
          </template>
          <Field :label="t('cron.schedule')" mono>{{ tk.spec }}</Field>
          <Field :label="t('cron.nextRun')">{{ tk.enable && tk.nextRun ? new Date(tk.nextRun).toLocaleString() : '-' }}</Field>
          <Field :label="t('cron.lastRun')">
            <template v-if="last(tk)">
              <span :class="last(tk)!.ok ? 'text-ok' : 'text-danger'">{{ last(tk)!.ok ? t('cron.success') : t('cron.failure') }}</span>
              <span class="meta"> · {{ new Date(last(tk)!.start).toLocaleString() }} · {{ last(tk)!.durationMs }}ms</span>
            </template>
            <span v-else class="muted">{{ t('cron.neverRun') }}</span>
          </Field>
          <Field :label="tk.type === 'http' ? t('cron.request') : t('cron.typeCommand')" mono>
            <span class="ellipsis-2">{{ tk.type === 'http' ? `${tk.http.method} ${tk.http.url}` : tk.command }}</span>
          </Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="Play" :disabled="tk.running" @click="runNow(tk)">{{ t('cron.runNow') }}</el-button>
            <el-button text size="small" :icon="History" @click="showHistory(tk)">{{ t('cron.history') }}</el-button>
            <el-button text size="small" :icon="Pencil" @click="openEditor(tk)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('cron.confirmDelete')" @confirm="remove(tk)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <LogCard url="/api/cron/logs" clearable />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('cron.editTask') : t('cron.addTask')" width="680px" :fullscreen="isMobile">
      <el-form :model="editing" label-width="110px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('cron.schedule')">
          <el-input v-model="editing.spec" class="mono" :placeholder="t('cron.specPlaceholder')" @blur="preview" />
          <div v-if="nextRuns.length" class="form-help">{{ t('cron.nextRuns', { list: nextRuns.map((d) => new Date(d).toLocaleString()).join('；') }) }}</div>
          <div v-if="specError" class="form-help text-danger">{{ specError }}</div>
        </el-form-item>
        <el-form-item :label="t('cron.type')">
          <el-radio-group v-model="editing.type">
            <el-radio-button value="command">{{ t('cron.typeCommand') }}</el-radio-button>
            <el-radio-button value="http">{{ t('cron.typeHttp') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="editing.type === 'command'" :label="t('cron.typeCommand')">
          <el-input v-model="editing.command" type="textarea" :rows="4" class="mono" :placeholder="t('cron.commandPlaceholder')" />
        </el-form-item>
        <WebhookEditor v-else v-model="editing.http" :variables="['time', 'taskName']" />
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
        <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="historyVisible" :title="t('cron.historyTitle')" width="760px" :fullscreen="isMobile">
      <el-timeline v-if="history.length">
        <el-timeline-item
          v-for="(r, i) in history"
          :key="i"
          :type="r.ok ? 'success' : 'danger'"
          :timestamp="new Date(r.start).toLocaleString() + ` · ${r.durationMs}ms` + (r.manual ? ' · ' + t('cron.manual') : '')"
        >
          <pre class="output mono">{{ r.output || t('cron.noOutput') }}</pre>
        </el-timeline-item>
      </el-timeline>
      <el-empty v-else :description="t('cron.noHistory')" />
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { CalendarClock, History, Pencil, Play, Plus, Trash2 } from 'lucide-vue-next'
import { api } from '../api'
import PageHeader from '../components/PageHeader.vue'
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
import WebhookEditor, { type WebhookConfig } from '../components/WebhookEditor.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'

interface Run {
  start: string
  durationMs: number
  ok: boolean
  output: string
  manual: boolean
}

interface Task {
  key?: string
  name: string
  enable: boolean
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  spec: string
  type: string
  command: string
  http: WebhookConfig
  remark: string
}

interface TaskView extends Task {
  key: string
  running: boolean
  nextRun?: string
  history: Run[] | null
}

const { t } = useI18n()
const compact = useCompactView('cron')
const { isMobile, labelPosition } = useFormLayout()
const tasks = ref<TaskView[]>([])
// 列表的搜索、分组与排序；顺序与分组的修改不影响任务的调度
const gl = useGroupedList<TaskView>({
  storageKey: 'cron.groups',
  items: () => tasks.value,
  base: '/api/cron/tasks',
  reload: load,
  matches: (tk, q) => [tk.name, tk.group ?? '', tk.remark, tk.spec, tk.command, tk.http.url].join('\n').toLowerCase().includes(q),
})
const editorVisible = ref(false)
const editing = ref<Task>(empty())
const nextRuns = ref<string[]>([])
const specError = ref('')
const historyVisible = ref(false)
const history = ref<Run[]>([])
// 定时刷新，标签页在后台时暂停
const polling = usePolling(load, 5000)

function empty(): Task {
  return {
    name: '',
    enable: true,
    spec: '0 3 * * *',
    type: 'command',
    command: '',
    http: { enable: true, url: '', method: 'GET', headers: [], body: '', successContent: [], timeout: 0, insecureSkipVerify: false },
    remark: '',
  }
}

function last(tk: TaskView) {
  const h = tk.history ?? []
  return h.length ? h[h.length - 1] : undefined
}

// taskStatus 计算任务展示状态：停用的任务只显示“已停用”
function taskStatus(tk: TaskView): { type: StatusType; text: string } {
  if (tk.running) {
    return { type: 'pending', text: t('cron.running') }
  }
  if (!tk.enable) {
    return { type: 'off', text: t('cron.deactivated') }
  }
  const l = last(tk)
  if (l && !l.ok) {
    return { type: 'error', text: t('cron.lastFailed') }
  }
  return { type: 'running', text: t('cron.scheduled') }
}

async function load() {
  tasks.value = await api.get('/api/cron/tasks')
}

async function preview() {
  try {
    const r = await api.post<{ next: string[] }>('/api/cron/parse', { spec: editing.value.spec })
    nextRuns.value = r.next
    specError.value = ''
  } catch (e) {
    nextRuns.value = []
    specError.value = (e as Error).message
  }
}

function openEditor(tk?: TaskView) {
  if (tk) {
    const { running: _r, nextRun: _n, history: _h, ...task } = tk
    editing.value = JSON.parse(JSON.stringify(task))
  } else {
    editing.value = empty()
  }
  editorVisible.value = true
  preview()
}

async function save() {
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/cron/tasks/${key}`, body)
    } else {
      await api.post('/api/cron/tasks', body)
    }
    editorVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function toggle(tk: TaskView, enable: boolean) {
  try {
    await api.put(`/api/cron/tasks/${tk.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function runNow(tk: TaskView) {
  try {
    await api.post(`/api/cron/tasks/${tk.key}/run`)
    ElMessage.success(t('cron.started'))
    window.setTimeout(load, 1000)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

function showHistory(tk: TaskView) {
  history.value = [...(tk.history ?? [])].reverse()
  historyVisible.value = true
}

async function remove(tk: TaskView) {
  try {
    await api.del(`/api/cron/tasks/${tk.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(() => {
  load()
  polling.start()
})
</script>

<style scoped>
.ellipsis-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.output {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 12px;
  max-height: 200px;
  overflow: auto;
  background: var(--el-fill-color-lighter);
  border: 1px solid var(--g-border);
  border-radius: 6px;
  padding: 8px 10px;
}
</style>
