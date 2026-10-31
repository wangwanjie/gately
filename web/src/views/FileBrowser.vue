<template>
  <div>
    <PageHeader :title="t('filebrowser.title')" :desc="t('filebrowser.desc')" :icon="FolderOpen">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('groupSort.search')" />
      <ResetConfigButton base="/api/filebrowser" :desc="t('components.resetConfig.filebrowser')" @done="onReset" />
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('filebrowser.addInstance') }}</el-button>
    </PageHeader>

    <el-alert v-if="binError" type="warning" :title="binError" :closable="false" show-icon class="block-gap">
      <el-button size="small" type="primary" :icon="Download" @click="installer?.install()">{{ t('components.programInstall.install') }}</el-button>
    </el-alert>

    <GroupedList :gl="gl" :compact="compact" grid :empty-text="t('filebrowser.empty')">
      <template #item="{ it: row, item: it, sorting, bind }">
        <EntityCard v-bind="bind" :off="!it.enable" :compact="compact || sorting">
          <template v-if="sorting" #lead>
            <SortCheck :gl="gl" :id="row.id" />
          </template>
          <template #title>{{ it.name }}</template>
          <template #sub>{{ authLabel(it.authMethod) }}<template v-if="it.remark"> · {{ it.remark }}</template></template>
          <template #status><StatusBadge v-bind="badge(it)" /></template>
          <Field :label="t('filebrowser.accessUrl')" mono wide>
            <div class="url-list">
              <a v-for="u in accessUrls(it)" :key="u" :href="u" target="_blank" rel="noopener">{{ u }}</a>
            </div>
          </Field>
          <Field :label="t('filebrowser.sources')" wide>
            <span v-for="s in it.sources" :key="s.name" class="src-tag" :title="s.path">
              {{ s.name }}<span v-if="s.readOnly" class="muted"> · {{ t('filebrowser.readOnly') }}</span>
            </span>
          </Field>
          <Field v-if="it.authMethod !== 'noauth'" :label="t('filebrowser.adminUser')" mono>{{ it.adminUser }}</Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="row" />
          </template>
          <template v-else #actions>
            <el-switch :model-value="it.enable" size="small" @change="(v: boolean | string | number) => toggle(it, Boolean(v))" />
            <el-button text size="small" :icon="RefreshCw" :disabled="!it.enable" @click="restart(it)">{{ t('common.restart') }}</el-button>
            <el-button text size="small" :icon="Pencil" @click="openEditor(it)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('filebrowser.confirmDelete')" width="260" @confirm="remove(it)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <div class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Terminal :size="16" />{{ t('filebrowser.program') }} <span v-if="version" class="g-card-desc">{{ version }}</span></div>
        <el-button size="small" :icon="Save" @click="saveSettings">{{ t('common.save') }}</el-button>
      </div>
      <div class="g-card-body">
        <el-form label-width="110px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('filebrowser.binary')">
            <el-input v-model="binPath" :placeholder="binary || t('filebrowser.binaryPlaceholder')" class="mono" />
            <div class="form-help">{{ t('filebrowser.binaryHelp', { path: installPath }) }}</div>
          </el-form-item>
        </el-form>
      </div>
    </div>

    <ProgramInstall ref="installer" :key="resetCount" base="/api/filebrowser" :active-paths="[binary]" @changed="onInstalled" />

    <LogCard url="/api/filebrowser/logs" clearable />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('filebrowser.editInstance') : t('filebrowser.addInstance')" width="640px" :fullscreen="isMobile">
      <el-form :model="editing" label-width="120px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('filebrowser.listen')">
          <div class="listen-row">
            <el-input v-model="editing.listenIp" :placeholder="t('filebrowser.listenPlaceholder')" class="mono" />
            <el-input-number v-model="editing.port" :min="1" :max="65535" controls-position="right" />
          </div>
          <div class="form-help">{{ t('filebrowser.listenHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('filebrowser.baseUrl')">
          <el-input v-model="editing.baseUrl" placeholder="/" class="mono" />
          <div class="form-help">{{ t('filebrowser.baseUrlHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('filebrowser.accessUrl')">
          <div class="src-list">
            <div v-for="(_, i) in editing.accessUrls" :key="i" class="src-row">
              <el-input v-model="editing.accessUrls[i]" :placeholder="guessUrl(editing)" class="mono src-path" />
              <el-button text type="danger" :icon="X" @click="editing.accessUrls.splice(i, 1)" />
            </div>
            <el-button link type="primary" size="small" :icon="Plus" :disabled="editing.accessUrls.length >= 10" @click="editing.accessUrls.push('')">{{ t('filebrowser.addAccessUrl') }}</el-button>
          </div>
          <div class="form-help">{{ t('filebrowser.accessUrlHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('filebrowser.sources')">
          <div class="src-list">
            <div v-for="(s, i) in editing.sources" :key="i" class="src-row">
              <el-input v-model="s.path" :placeholder="t('filebrowser.sourcePath')" class="mono src-path" />
              <el-input v-model="s.name" :placeholder="t('filebrowser.sourceName')" class="src-name" />
              <el-checkbox v-model="s.readOnly">{{ t('filebrowser.readOnly') }}</el-checkbox>
              <el-button text type="danger" :icon="X" :disabled="editing.sources.length <= 1" @click="editing.sources.splice(i, 1)" />
            </div>
            <el-button link type="primary" size="small" :icon="FolderPlus" @click="editing.sources.push({ path: '', name: '', readOnly: false })">{{ t('filebrowser.addSource') }}</el-button>
          </div>
          <div class="form-help">{{ t('filebrowser.sourcesHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('filebrowser.authMethod')">
          <el-radio-group v-model="editing.authMethod">
            <el-radio-button value="password">{{ t('filebrowser.authPassword') }}</el-radio-button>
            <el-radio-button value="proxy">{{ t('filebrowser.authProxy') }}</el-radio-button>
            <el-radio-button value="noauth">{{ t('filebrowser.authNone') }}</el-radio-button>
          </el-radio-group>
          <el-alert v-if="editing.authMethod === 'noauth'" type="warning" :title="t('filebrowser.authNoneWarn')" :closable="false" show-icon class="auth-alert" />
          <div v-else-if="editing.authMethod === 'proxy'" class="form-help">{{ t('filebrowser.authProxyHelp') }}</div>
        </el-form-item>
        <el-form-item v-if="editing.authMethod === 'proxy'" :label="t('filebrowser.proxyHeader')">
          <el-input v-model="editing.proxyHeader" placeholder="Remote-User" class="mono" />
        </el-form-item>
        <template v-if="editing.authMethod !== 'noauth'">
          <el-form-item :label="t('filebrowser.adminUser')">
            <el-input v-model="editing.adminUser" placeholder="admin" />
          </el-form-item>
          <el-form-item v-if="editing.authMethod === 'password'" :label="t('filebrowser.adminPassword')">
            <SecretInput v-model="editing.adminPassword" autocomplete="new-password" :reveal="secretOf(editing.key && `/api/filebrowser/instances/${editing.key}/secrets`, 'adminPassword')" />
            <div class="form-help">{{ t('filebrowser.adminPasswordHelp') }}</div>
          </el-form-item>
        </template>
        <el-form-item :label="t('filebrowser.trustProxy')">
          <el-switch v-model="editing.trustProxyHeaders" />
          <div class="form-help">{{ t('filebrowser.trustProxyHelp') }}</div>
        </el-form-item>
        <el-form-item label="WebDAV">
          <el-switch :model-value="!editing.disableWebDav" @update:model-value="(v: boolean | string | number) => (editing.disableWebDav = !v)" />
          <div class="form-help">{{ t('filebrowser.webdavHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('filebrowser.locale')">
          <el-select v-model="editing.locale" clearable :placeholder="t('filebrowser.localeDefault')">
            <el-option v-for="l in fbLocales" :key="l.value" :label="l.label" :value="l.value" />
          </el-select>
          <div class="form-help">{{ t('filebrowser.localeHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('filebrowser.tokenHours')">
          <el-input-number v-model="editing.tokenHours" :min="1" :max="8760" controls-position="right" />
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Download, FolderOpen, FolderPlus, Pencil, Plus, RefreshCw, Save, Terminal, Trash2, X } from 'lucide-vue-next'
import { api } from '../api'
import SecretInput from '../components/SecretInput.vue'
import { secretOf } from '../secrets'
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
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import ProgramInstall from '../components/ProgramInstall.vue'
import ResetConfigButton from '../components/ResetConfigButton.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'

interface Source {
  path: string
  name: string
  readOnly: boolean
}
interface Instance {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  enable: boolean
  listenIp: string
  port: number
  baseUrl: string
  sources: Source[]
  authMethod: string
  proxyHeader: string
  adminUser: string
  adminPassword: string
  trustProxyHeaders: boolean
  disableWebDav: boolean
  locale: string
  tokenHours: number
  // accessUrls 为用户填写的访问地址，为空时按监听地址推测
  accessUrls: string[]
  remark: string
}
// InstanceView 是后端返回的实例，附带运行状态
interface InstanceView extends Instance {
  state: { running: boolean }
  error: string
}

// FileBrowser Quantum 支持的界面语言（取与 Gately 相同的 10 种）
const fbLocales = [
  { value: 'zh-cn', label: '简体中文' },
  { value: 'zh-tw', label: '繁體中文' },
  { value: 'en', label: 'English' },
  { value: 'ja', label: '日本語' },
  { value: 'ko', label: '한국어' },
  { value: 'es', label: 'Español' },
  { value: 'fr', label: 'Français' },
  { value: 'de', label: 'Deutsch' },
  { value: 'ru', label: 'Русский' },
  { value: 'pt-br', label: 'Português (Brasil)' },
]

const { t, locale } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
const instances = ref<InstanceView[]>([])
const compact = useCompactView('filebrowser')
// 列表的搜索、分组与排序；顺序与分组的修改不会重启实例
const gl = useGroupedList<InstanceView>({
  storageKey: 'filebrowser.groups',
  items: () => instances.value,
  base: '/api/filebrowser/instances',
  reload: loadInstances,
  matches: (it, q) => [it.name, it.group ?? '', it.remark, ...accessUrls(it), ...it.sources.map((s) => s.name)].join('\n').toLowerCase().includes(q),
})
const binPath = ref('')
const binary = ref('')
const version = ref('')
const quantum = ref(false)
const installPath = ref('')
const binError = ref('')
const installer = ref<InstanceType<typeof ProgramInstall>>()
const editorVisible = ref(false)
const editing = ref<Instance>(empty())
// 定时刷新，标签页在后台时暂停
const polling = usePolling(loadInstances, 4000)

// uiLocale 把 Gately 的界面语言换成 FileBrowser Quantum 的语言代码，作为新实例的默认值
function uiLocale(): string {
  const code = String(locale.value).toLowerCase()
  return fbLocales.some((l) => l.value === code) ? code : 'en'
}

function empty(): Instance {
  return {
    name: 'File Browser',
    enable: true,
    listenIp: '',
    port: 8080,
    baseUrl: '/',
    sources: [{ path: '', name: '', readOnly: false }],
    authMethod: 'password',
    proxyHeader: 'Remote-User',
    adminUser: 'admin',
    adminPassword: '',
    trustProxyHeaders: false,
    disableWebDav: false,
    locale: uiLocale(),
    tokenHours: 2,
    accessUrls: [],
    remark: '',
  }
}

function authLabel(m: string): string {
  if (m === 'proxy') {
    return t('filebrowser.authProxy')
  }
  if (m === 'noauth') {
    return t('filebrowser.authNone')
  }
  return t('filebrowser.authPassword')
}

// accessUrls 返回卡片上显示的访问地址：优先使用用户填写的地址，未填写时按监听地址推测
function accessUrls(it: Instance): string[] {
  return it.accessUrls?.length ? it.accessUrls : [guessUrl(it)]
}

// guessUrl 拼出推测的访问地址：监听全部地址时使用当前访问 Gately 的主机名
function guessUrl(it: Instance): string {
  let host = it.listenIp
  if (!host || host === '0.0.0.0' || host === '::') {
    host = window.location.hostname
  }
  if (host.includes(':') && !host.startsWith('[')) {
    host = `[${host}]`
  }
  const base = it.baseUrl || '/'
  return `http://${host}:${it.port}${base.endsWith('/') ? base : base + '/'}`
}

function badge(it: InstanceView): { type: StatusType; text: string; title?: string } {
  if (!it.enable) {
    return { type: 'off', text: t('filebrowser.deactivated') }
  }
  if (it.error) {
    return { type: 'error', text: t('filebrowser.startFailed'), title: it.error }
  }
  if (!it.state.running) {
    return { type: 'warn', text: t('filebrowser.notRunning') }
  }
  return { type: 'running', text: t('common.running') }
}

async function loadInstances() {
  try {
    instances.value = await api.get<InstanceView[]>('/api/filebrowser/instances')
  } catch {
    // 轮询失败时保留上一次的状态
  }
}

async function loadSettings() {
  const r = await api.get<{ settings: { binPath: string }; binary: string; version: string; quantum: boolean; installPath: string; error: string }>('/api/filebrowser/settings')
  binPath.value = r.settings.binPath
  binary.value = r.binary
  version.value = r.version
  quantum.value = r.quantum
  installPath.value = r.installPath
  binError.value = r.error
}

async function saveSettings() {
  try {
    await api.put('/api/filebrowser/settings', { binPath: binPath.value })
    ElMessage.success(t('common.saved'))
    await Promise.all([loadSettings(), loadInstances()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// onInstalled 在安装、回滚、卸载后刷新程序信息与实例状态
// resetCount 在恢复默认配置后递增，让在线安装卡片重新读取下载代理
const resetCount = ref(0)

async function onReset() {
  resetCount.value++
  await onInstalled()
}

async function onInstalled() {
  await Promise.all([loadSettings(), loadInstances()])
}

function openEditor(it?: InstanceView) {
  if (it) {
    // 去掉运行状态等只读字段，后端按未知字段拒绝请求
    const { state: _s, error: _e, ...inst } = it
    editing.value = JSON.parse(JSON.stringify(inst))
  } else {
    editing.value = empty()
  }
  if (!editing.value.sources?.length) {
    editing.value.sources = [{ path: '', name: '', readOnly: false }]
  }
  if (!editing.value.tokenHours) {
    editing.value.tokenHours = 2
  }
  if (!editing.value.accessUrls) {
    editing.value.accessUrls = []
  }
  if (!editing.value.proxyHeader) {
    editing.value.proxyHeader = 'Remote-User'
  }
  editorVisible.value = true
}

async function save() {
  try {
    const { key, ...body } = editing.value
    // 忽略未填写路径的空行
    body.sources = body.sources.filter((s) => s.path.trim() !== '' || s.name.trim() !== '')
    body.accessUrls = body.accessUrls.map((u) => u.trim()).filter((u) => u !== '')
    if (key) {
      await api.put(`/api/filebrowser/instances/${key}`, body)
    } else {
      await api.post('/api/filebrowser/instances', body)
    }
    editorVisible.value = false
    await loadInstances()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function toggle(it: Instance, enable: boolean) {
  try {
    await api.put(`/api/filebrowser/instances/${it.key}/enable`, { enable })
    await loadInstances()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function restart(it: Instance) {
  try {
    await api.post(`/api/filebrowser/instances/${it.key}/restart`)
    await loadInstances()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(it: Instance) {
  try {
    await api.del(`/api/filebrowser/instances/${it.key}`)
    await loadInstances()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(() => {
  loadSettings()
  loadInstances()
  polling.start()
})
</script>

<style scoped>
.block-gap {
  margin-bottom: 16px;
}
.block-gap :deep(.el-alert__description) {
  margin-top: 6px;
}
.svc-form {
  max-width: 640px;
}
.svc-form .el-form-item:last-child {
  margin-bottom: 0;
}
.muted {
  color: var(--el-text-color-secondary);
}
.card-grid + .g-card {
  margin-top: 16px;
}
.url-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.url-list a {
  overflow-wrap: anywhere;
}
.src-tag {
  display: inline-block;
  margin-right: 10px;
}
.listen-row {
  display: flex;
  gap: 8px;
  width: 100%;
}
.listen-row .el-input {
  flex: 1;
}
.src-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}
.src-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.src-path {
  flex: 3;
}
.src-name {
  flex: 2;
}
.auth-alert {
  margin-top: 8px;
}
@media (max-width: 640px) {
  .src-row {
    flex-wrap: wrap;
  }
  .src-path {
    flex-basis: 100%;
  }
}
</style>
