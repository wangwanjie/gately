<template>
  <el-card shadow="never" class="bk">
    <template #header>
      <div class="head">
        <span class="title"><CalendarClock :size="16" />{{ t('backupSchedule.title') }}</span>
        <el-switch :model-value="settings.enable" :loading="saving" @change="(v: string | number | boolean) => toggle(Boolean(v))" />
      </div>
    </template>

    <el-form :model="settings" label-width="110px" :label-position="labelPosition">
      <el-form-item :label="t('backupSchedule.schedule')">
        <el-input v-model="settings.schedule" class="mono" placeholder="0 3 * * *" />
        <div class="form-help">{{ t('backupSchedule.scheduleHelp') }}</div>
      </el-form-item>
      <el-form-item :label="t('backupSchedule.keep')">
        <el-input-number v-model="settings.keep" :min="1" :max="365" />
        <span class="inline-help">{{ t('backupSchedule.keepHelp') }}</span>
      </el-form-item>
      <el-form-item :label="t('backupSchedule.password')">
        <SecretInput v-model="settings.password" :placeholder="settings.hasPassword ? t('backupSchedule.passwordSet') : t('backupSchedule.passwordEmpty')" :reveal="secretOf('/api/backup/secrets', 'password')" />
        <div class="form-help">{{ t('backupSchedule.passwordHelp') }}</div>
      </el-form-item>
      <el-form-item :label="t('backupSchedule.localDir')">
        <el-switch v-model="settings.localEnable" />
        <span class="inline-help">{{ t('backupSchedule.localDirHelp') }}</span>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="saving" @click="save">{{ t('backupSchedule.saveSettings') }}</el-button>
        <el-button :icon="Play" :loading="running" @click="runNow">{{ t('backupSchedule.runNow') }}</el-button>
      </el-form-item>
    </el-form>

    <div class="sub">
      <span>{{ t('backupSchedule.remoteTargets') }}</span>
      <el-button text size="small" :icon="Plus" @click="openTarget()">{{ t('common.add') }}</el-button>
    </div>
    <el-empty v-if="!targets.length" :image-size="40" :description="t('backupSchedule.targetsEmpty')" />
    <div v-for="tg in targets" :key="tg.key" class="target">
      <el-switch :model-value="tg.enable" size="small" @change="(v: string | number | boolean) => toggleTarget(tg, Boolean(v))" />
      <div class="t-main">
        <div class="t-name">{{ tg.name }}</div>
        <div class="t-sub mono">{{ mountName(tg.mount) }}:/{{ tg.dir }}</div>
      </div>
      <el-button text size="small" :icon="Pencil" :title="t('common.edit')" :aria-label="t('common.edit')" @click="openTarget(tg)" />
      <el-button text size="small" :icon="CopyPlus" :title="t('common.duplicate')" :aria-label="t('common.duplicate')" @click="openTarget(tg, true)" />
      <el-popconfirm :title="t('backupSchedule.confirmDeleteTarget')" @confirm="removeTarget(tg)">
        <template #reference>
          <el-button text size="small" type="danger" :icon="Trash2" />
        </template>
      </el-popconfirm>
    </div>

    <div class="sub"><span>{{ t('backupSchedule.lastRun') }}</span></div>
    <div v-if="status.lastRun" class="last">
      <div class="meta">
        {{ new Date(status.lastRun).toLocaleString() }}{{ t('backupSchedule.lastRunMode', { mode: status.manual ? t('backupSchedule.manual') : t('backupSchedule.scheduled') }) }}
        <span v-if="status.nextRun">{{ t('backupSchedule.nextRun', { time: new Date(status.nextRun).toLocaleString() }) }}</span>
      </div>
      <div v-for="r in status.results" :key="r.target" class="res">
        <StatusBadge :type="r.ok ? 'running' : 'error'" :text="r.ok ? t('backupSchedule.success') : t('backupSchedule.failed')" />
        <span class="r-name">{{ r.target }}</span>
        <span v-if="r.error" class="r-err">{{ r.error }}</span>
        <span v-else-if="r.pruned" class="meta">{{ t('backupSchedule.pruned', { count: r.pruned }) }}</span>
      </div>
    </div>
    <div v-else class="meta">{{ t('backupSchedule.neverRun') }}<span v-if="status.nextRun">{{ t('backupSchedule.nextRun', { time: new Date(status.nextRun).toLocaleString() }) }}</span></div>

    <div v-if="localFiles.length" class="sub"><span>{{ t('backupSchedule.localFiles') }}</span></div>
    <div v-for="f in localFiles" :key="f.name" class="file">
      <span class="mono f-name">{{ f.name }}</span>
      <span class="meta">{{ formatSize(f.size) }} · {{ new Date(f.modTime).toLocaleString() }}</span>
      <a :href="`/api/backup/local/${encodeURIComponent(f.name)}`" download><el-button text size="small" :icon="Download" /></a>
    </div>

    <el-dialog v-model="targetVisible" :title="editingTarget.key ? t('backupSchedule.editTarget') : t('backupSchedule.addTarget')" width="460px" :fullscreen="isMobile" append-to-body>
      <CopyHint :from="copyTargetFrom" />
      <el-form :model="editingTarget" label-width="90px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editingTarget.name" /></el-form-item>
        <el-form-item :label="t('backupSchedule.mount')">
          <el-select v-model="editingTarget.mount" style="width: 100%" :placeholder="t('backupSchedule.mountPlaceholder')">
            <el-option v-for="m in mounts" :key="m.key" :label="t('backupSchedule.mountLabel', { name: m.name, type: m.type === 'webdav' ? 'WebDAV' : m.type === 'smb' ? 'SMB' : t('backupSchedule.localType') })" :value="m.key" :disabled="m.readOnly || !m.enable" />
          </el-select>
          <div v-if="!mounts.length" class="form-help">{{ t('backupSchedule.noMounts') }}</div>
        </el-form-item>
        <el-form-item :label="t('backupSchedule.dir')"><el-input v-model="editingTarget.dir" :placeholder="t('backupSchedule.dirPlaceholder')" /></el-form-item>
        <el-form-item :label="t('common.enable')"><el-switch v-model="editingTarget.enable" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="targetVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveTarget">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { CalendarClock, CopyPlus, Download, Pencil, Play, Plus, Trash2 } from 'lucide-vue-next'
import { api } from '../api'
import CopyHint from './CopyHint.vue'
import { createItem, uniqueName, type CopySource } from '../copyDraft'
import StatusBadge from './StatusBadge.vue'
import SecretInput from './SecretInput.vue'
import { secretOf } from '../secrets'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'

interface Settings {
  enable: boolean
  schedule: string
  keep: number
  password: string
  hasPassword?: boolean
  localEnable: boolean
}
interface Target {
  key?: string
  name: string
  enable: boolean
  mount: string
  dir: string
}
interface Mount {
  key: string
  name: string
  type: string
  enable: boolean
  readOnly: boolean
}
interface Result {
  target: string
  ok: boolean
  file?: string
  error?: string
  pruned?: number
}
interface Status {
  running: boolean
  lastRun?: string
  nextRun?: string
  manual: boolean
  results: Result[]
}
interface LocalFile {
  name: string
  size: number
  modTime: string
}

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
const settings = reactive<Settings>({ enable: false, schedule: '0 3 * * *', keep: 7, password: '', localEnable: true })
const targets = ref<Target[]>([])
const mounts = ref<Mount[]>([])
const status = ref<Status>({ running: false, manual: false, results: [] })
const localFiles = ref<LocalFile[]>([])
const saving = ref(false)
const running = ref(false)
const targetVisible = ref(false)
const editingTarget = ref<Target>(emptyTarget())
// 复制备份目的地时的模板，普通新增与编辑时为 null
const copyTargetFrom = ref<CopySource | null>(null)
// 定时刷新，标签页在后台时暂停
const polling = usePolling(loadLive, 15000)

function emptyTarget(): Target {
  return { name: '', enable: true, mount: '', dir: 'gately-backups' }
}

function mountName(key: string) {
  return mounts.value.find((m) => m.key === key)?.name ?? t('backupSchedule.mountDeleted')
}

function formatSize(n: number) {
  if (n < 1024) {
    return `${n} B`
  }
  if (n < 1024 * 1024) {
    return `${(n / 1024).toFixed(1)} KB`
  }
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

async function loadSettings() {
  Object.assign(settings, await api.get<Settings>('/api/backup/settings'))
}

async function loadLive() {
  const [tgs, s, f] = await Promise.all([
    api.get<Target[]>('/api/backup/targets'),
    api.get<Status>('/api/backup/status'),
    api.get<LocalFile[]>('/api/backup/local'),
  ])
  targets.value = tgs
  status.value = s
  localFiles.value = f
}

async function put(form: Settings) {
  saving.value = true
  try {
    // hasPassword 是 GET 返回的只读字段，后端拒绝未知字段，提交前剔除
    const { hasPassword: _h, ...body } = form
    Object.assign(settings, await api.put<Settings>('/api/backup/settings', body))
    await loadLive()
    return true
  } catch (e) {
    ElMessage.error((e as Error).message)
    return false
  } finally {
    saving.value = false
  }
}

async function save() {
  if (await put({ ...settings })) {
    ElMessage.success(t('common.saved'))
  }
}

async function toggle(enable: boolean) {
  await put({ ...settings, enable })
}

async function runNow() {
  running.value = true
  try {
    status.value = await api.post<Status>('/api/backup/run')
    const failed = status.value.results.filter((r) => !r.ok).length
    if (failed) {
      ElMessage.error(t('backupSchedule.targetsFailed', { count: failed }))
    } else {
      ElMessage.success(t('backupSchedule.done'))
    }
    await loadLive()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    running.value = false
  }
}

// openTarget 打开目的地编辑框：copy 为 true 时以 tg 为模板新建副本
function openTarget(tg?: Target, copy = false) {
  editingTarget.value = tg ? { ...tg } : emptyTarget()
  copyTargetFrom.value = null
  if (tg && copy) {
    copyTargetFrom.value = { key: tg.key ?? '', name: tg.name }
    editingTarget.value.key = ''
    editingTarget.value.name = uniqueName(tg.name, targets.value.map((x) => x.name))
  }
  targetVisible.value = true
}

async function saveTarget() {
  try {
    const { key, ...body } = editingTarget.value
    if (key) {
      await api.put(`/api/backup/targets/${key}`, body)
    } else {
      await createItem('/api/backup/targets', body, copyTargetFrom.value)
    }
    targetVisible.value = false
    await loadLive()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function toggleTarget(tg: Target, enable: boolean) {
  try {
    await api.put(`/api/backup/targets/${tg.key}/enable`, { enable })
    await loadLive()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function removeTarget(tg: Target) {
  try {
    await api.del(`/api/backup/targets/${tg.key}`)
    await loadLive()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(async () => {
  try {
    mounts.value = await api.get<Mount[]>('/api/backup/mounts')
    await loadSettings()
    await loadLive()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
  polling.start()
})
</script>

<style scoped>
.bk {
  margin-bottom: 16px;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
}
.inline-help {
  margin-left: 10px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.sub {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 14px 0 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-regular);
}
.target,
.file,
.res {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 0;
}
.t-main {
  flex: 1;
  min-width: 0;
}
.t-name {
  font-size: 13px;
}
.t-sub,
.meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.r-err {
  color: var(--el-color-danger);
  font-size: 12px;
  word-break: break-all;
}
.f-name {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
