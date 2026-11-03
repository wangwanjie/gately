<template>
  <div>
    <div class="toolbar g-sticky-bar g-sticky-desktop">
      <el-input v-model="keyword" :placeholder="t('docker.compose.tab.search')" clearable :prefix-icon="Search" class="t-search" />
      <CompactToggle v-model="compact" />
      <div class="t-ops">
        <el-button type="primary" :icon="Plus" @click="newProject">{{ t('docker.compose.tab.newProject') }}</el-button>
        <el-button :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </div>
    </div>
    <el-alert v-if="info && !info.composeCli" type="warning" :closable="false" show-icon class="gap" :title="t('docker.compose.tab.noCli')">
      {{ t('docker.compose.tab.noCliDesc') }}
    </el-alert>

    <div v-loading="loading && !projects.length" class="card-list" :class="{ 'is-compact': compact }">
      <el-empty v-if="!loading && !filtered.length" :description="t('docker.compose.tab.empty')" />
      <EntityCard :compact="compact" v-for="p in filtered" :key="p.name" :off="p.status === 'stopped'">
        <template #lead>
          <div class="ico" :class="{ on: p.status !== 'stopped' }"><Layers :size="18" /></div>
        </template>
        <template #title>
          <span>{{ p.name }}</span>
          <el-tag v-if="p.managed" size="small" type="success">{{ t('docker.compose.tab.managed') }}</el-tag>
        </template>
        <template #sub>{{ p.workingDir || '-' }}</template>
        <template #status>
          <StatusBadge v-bind="projBadge(p)" />
        </template>
        <Field :label="t('docker.compose.tab.containers')" wide>{{ t('docker.compose.tab.runningCount', { running: p.running, total: p.total }) }}</Field>
        <Field :label="t('docker.compose.tab.services')" wide>
          <div v-for="s in p.services" :key="s.name" class="svc">
            <span class="svc-dot" :class="s.state === 'running' ? 'on' : ''" />
            <b>{{ s.name }}</b>
            <span class="muted mono svc-img">{{ s.image }}</span>
            <span v-for="port in s.ports ?? []" :key="port" class="mono svc-port">{{ port }}</span>
          </div>
          <span v-if="!p.services?.length" class="muted">-</span>
        </Field>
        <template #actions>
          <el-button v-if="p.status !== 'running'" text type="primary" :icon="Play" class="btn" :loading="busy[p.name]" @click="simple(p, 'start')">{{ t('common.start') }}</el-button>
          <el-button v-if="p.status !== 'stopped'" text type="danger" :icon="Square" class="btn" :loading="busy[p.name]" :disabled="p.self" :title="p.self ? t('docker.compose.tab.selfHint') : ''" @click="stop(p)">{{ t('common.stop') }}</el-button>
          <el-button text :icon="RotateCw" class="btn" :loading="busy[p.name]" @click="simple(p, 'restart')">{{ t('common.restart') }}</el-button>
          <el-button text :icon="ScrollText" class="btn" @click="showLogs(p)">{{ t('docker.compose.tab.logs') }}</el-button>
          <el-dropdown trigger="click" @command="(c: string) => onCommand(p, c)">
            <el-button text :icon="Ellipsis" class="btn">{{ t('common.more') }}</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="up" :icon="Rocket" :disabled="p.self" :title="p.self ? t('docker.compose.tab.selfUpHint') : ''">{{ t('docker.compose.tab.up') }}</el-dropdown-item>
                <el-dropdown-item command="pull" :icon="CloudDownload">{{ t('docker.compose.tab.pull') }}</el-dropdown-item>
                <el-dropdown-item command="edit" :icon="FilePen">{{ t('docker.compose.tab.editFile') }}</el-dropdown-item>
                <el-dropdown-item v-if="!p.self" command="down" :icon="ArrowDownToLine" divided>{{ t('docker.compose.tab.down') }}</el-dropdown-item>
                <el-dropdown-item v-if="p.managed && !p.self" command="delete" :icon="Trash2">{{ t('docker.compose.tab.deleteProject') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </EntityCard>
    </div>

    <ComposeEditor v-model="editorVisible" :project="editorProject" @done="load" />

    <el-dialog v-model="logsVisible" :title="t('docker.compose.tab.logsTitle', { name: logsName })" width="900px" :fullscreen="isMobile" top="5vh" append-to-body>
      <pre class="logs mono">{{ logs }}</pre>
      <template #footer>
        <el-button :icon="RefreshCw" @click="refreshLogs">{{ t('common.refresh') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="downVisible" :title="t('docker.compose.tab.downTitle')" width="440px" append-to-body>
      <i18n-t keypath="docker.compose.tab.downText" tag="p" class="dlg-text">
        <template #name><b>{{ downTarget?.name }}</b></template>
      </i18n-t>
      <el-checkbox v-model="downVolumes">{{ t('docker.compose.tab.downVolumes') }}</el-checkbox>
      <template #footer>
        <el-button @click="downVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :loading="downBusy" @click="doDown">down</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDownToLine, CloudDownload, Ellipsis, FilePen, Layers, Play, Plus, RefreshCw, Rocket, RotateCw, ScrollText, Search, Square, Trash2 } from 'lucide-vue-next'
import { api, ApiError } from '../../api'
import CompactToggle from '../../components/CompactToggle.vue'
import EntityCard from '../../components/EntityCard.vue'
import { useCompactView } from '../../composables/useCompactView'
import Field from '../../components/Field.vue'
import StatusBadge, { type StatusType } from '../../components/StatusBadge.vue'
import { useIsMobile } from '../../composables/useIsMobile'
import ComposeEditor from './ComposeEditor.vue'
import { dockerInfo, reportError } from './state'
import { trackTask } from './tasks'
import type { ComposeProject } from './types'

const { t } = useI18n()
const compact = useCompactView('docker.compose')

const isMobile = useIsMobile()
const info = dockerInfo
const projects = ref<ComposeProject[]>([])
const loading = ref(false)
const keyword = ref('')
const busy = reactive<Record<string, boolean>>({})
const editorVisible = ref(false)
const editorProject = ref<string>()
const logsVisible = ref(false)
const logsName = ref('')
const logs = ref('')
const downVisible = ref(false)
const downTarget = ref<ComposeProject>()
const downVolumes = ref(false)
const downBusy = ref(false)

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  return k ? projects.value.filter((p) => p.name.toLowerCase().includes(k) || (p.services ?? []).some((s) => s.name.toLowerCase().includes(k))) : projects.value
})

function projBadge(p: ComposeProject): { type: StatusType; text: string } {
  if (p.status === 'running') {
    return { type: 'running', text: t('common.running') }
  }
  if (p.status === 'partial') {
    return { type: 'warn', text: t('docker.compose.tab.partial') }
  }
  return { type: 'off', text: t('common.stopped') }
}

// 失败时对 501（缺少 compose CLI）给出友好提示
function fail(e: unknown) {
  if (e instanceof ApiError && e.status === 501) {
    ElMessageBox.alert(t('docker.compose.tab.unsupportedMsg', { message: e.message }), t('docker.compose.tab.unsupported'), { type: 'warning' })
    return
  }
  ElMessage.error((e as Error).message)
}

function needCli(): boolean {
  if (info.value && !info.value.composeCli) {
    ElMessageBox.alert(t('docker.compose.tab.needCli'), t('docker.compose.tab.unsupported'), { type: 'warning' })
    return false
  }
  return true
}

async function load() {
  loading.value = true
  try {
    projects.value = (await api.get<ComposeProject[]>('/api/docker/compose/projects')) ?? []
    reportError('')
  } catch (e) {
    reportError((e as Error).message)
  } finally {
    loading.value = false
  }
}

const url = (p: ComposeProject, tail = '') => `/api/docker/compose/${encodeURIComponent(p.name)}${tail}`

async function simple(p: ComposeProject, action: string) {
  busy[p.name] = true
  try {
    await api.post(url(p, `/${action}`))
    ElMessage.success(t('docker.compose.tab.executed'))
    await load()
  } catch (e) {
    fail(e)
  } finally {
    busy[p.name] = false
  }
}

async function stop(p: ComposeProject) {
  try {
    await ElMessageBox.confirm(t('docker.compose.tab.stopConfirm', { name: p.name }), t('docker.compose.tab.stopTitle'), { type: 'warning' })
  } catch {
    return
  }
  await simple(p, 'stop')
}

async function runTask(p: ComposeProject, action: string, title: string) {
  if (!needCli()) {
    return
  }
  try {
    const r = await api.post<{ task: string }>(url(p, `/${action}`))
    trackTask(r.task, title, () => load())
  } catch (e) {
    fail(e)
  }
}

async function onCommand(p: ComposeProject, cmd: string) {
  if (cmd === 'up') {
    await runTask(p, 'up', `up ${p.name}`)
  } else if (cmd === 'pull') {
    await runTask(p, 'pull', t('docker.compose.tab.taskPull', { name: p.name }))
  } else if (cmd === 'edit') {
    if (needCli()) {
      editorProject.value = p.name
      editorVisible.value = true
    }
  } else if (cmd === 'down') {
    downTarget.value = p
    downVolumes.value = false
    downVisible.value = true
  } else if (cmd === 'delete') {
    try {
      await ElMessageBox.confirm(t('docker.compose.tab.deleteConfirm', { name: p.name }), t('docker.compose.tab.deleteProject'), { type: 'warning' })
      await api.del(url(p))
      ElMessage.success(t('common.deleted'))
      await load()
    } catch (e) {
      if (e !== 'cancel' && e !== 'close') {
        fail(e)
      }
    }
  }
}

async function doDown() {
  if (!downTarget.value) {
    return
  }
  downBusy.value = true
  try {
    await api.post(url(downTarget.value, `/down${downVolumes.value ? '?volumes=1' : ''}`))
    downVisible.value = false
    ElMessage.success(t('docker.compose.tab.downDone'))
    await load()
  } catch (e) {
    fail(e)
  } finally {
    downBusy.value = false
  }
}

function newProject() {
  if (!needCli()) {
    return
  }
  editorProject.value = undefined
  editorVisible.value = true
}

let logsProject: ComposeProject | undefined
async function showLogs(p: ComposeProject) {
  logsProject = p
  logsName.value = p.name
  logs.value = t('common.loading')
  logsVisible.value = true
  await refreshLogs()
}

async function refreshLogs() {
  if (!logsProject) {
    return
  }
  try {
    const r = await api.get<{ logs: string }>(url(logsProject, '/logs?tail=300'))
    logs.value = r.logs || t('docker.compose.tab.noLogs')
  } catch (e) {
    logs.value = (e as Error).message
  }
}

defineExpose({ reload: load })
onMounted(load)
</script>

<style scoped>
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  margin-bottom: 12px;
}
.t-search {
  width: 240px;
}
.t-ops {
  display: flex;
  gap: 8px;
  margin-left: auto;
}
.gap {
  margin-bottom: 12px;
}
.ico {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-secondary);
}
.ico.on {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}
.svc {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px 8px;
  padding: 2px 0;
}
.svc-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--el-text-color-placeholder);
  flex-shrink: 0;
}
.svc-dot.on {
  background: var(--el-color-success);
}
.svc-img,
.svc-port {
  font-size: 12px;
}
.svc-port {
  color: var(--el-text-color-secondary);
}
.logs {
  margin: 0;
  max-height: 65vh;
  overflow: auto;
  background: var(--el-fill-color-lighter);
  border: 1px solid var(--g-border);
  padding: 10px;
  border-radius: 8px;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
.dlg-text {
  margin: 0 0 12px;
  line-height: 1.6;
  word-break: break-all;
}
@media (max-width: 767.98px) {
  .t-search {
    width: 100%;
  }
  .t-ops {
    width: 100%;
    margin-left: 0;
  }
  .t-ops .el-button {
    flex: 1;
    min-height: 40px;
  }
  .btn {
    min-height: 40px;
    padding: 0 8px;
  }
  .logs {
    max-height: none;
  }
  .toolbar :deep(input) {
    font-size: 16px;
  }
}
</style>
