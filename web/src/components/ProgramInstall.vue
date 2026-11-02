<template>
  <!-- 托管程序的在线安装、更新与回滚（frp、cloudflared、Tailscale、File Browser 共用） -->
  <div class="g-card program-install">
    <div class="g-card-head">
      <div class="g-card-title">
        <PackageCheck :size="16" />{{ title ?? t('components.programInstall.title', { name: info.program }) }}
        <span v-if="info.installed.present" class="g-card-desc">{{ info.installed.version || t('components.programInstall.unknownVersion') }}</span>
        <el-tag v-if="info.latest?.hasUpdate" size="small" type="success" effect="light">{{ t('components.programInstall.newVersion', { version: info.latest.version }) }}</el-tag>
      </div>
      <div class="head-actions">
        <el-button size="small" type="primary" :icon="Download" :loading="running" :disabled="!!info.unsupported" @click="install()">
          {{ info.installed.present ? t('components.programInstall.update') : t('components.programInstall.install') }}
        </el-button>
        <el-button size="small" :icon="RefreshCw" :loading="checking" :disabled="running || !!info.unsupported" @click="checkLatest">{{ t('components.programInstall.checkUpdate') }}</el-button>
        <el-button size="small" :icon="Tag" :disabled="running || !!info.unsupported" @click="installVersion">{{ t('components.programInstall.installVersion') }}</el-button>
        <el-button size="small" type="danger" plain :icon="PackageX" :disabled="running || (!info.installed.present && !info.backups.length)" @click="uninstall">
          {{ t('components.programInstall.uninstall') }}
        </el-button>
      </div>
    </div>
    <div class="g-card-body">
      <el-alert v-if="info.unsupported" type="info" :title="info.unsupported" :closable="false" show-icon class="gap" />
      <el-form label-width="110px" :label-position="labelPosition" class="svc-form">
        <el-form-item :label="t('components.programInstall.installed')">
          <div class="line">
            <template v-if="info.installed.present">
              <span class="mono">{{ info.mainPath }}</span>
              <span v-if="info.installed.missing?.length" class="warn">{{ t('components.programInstall.missing', { files: info.installed.missing.join(', ') }) }}</span>
            </template>
            <span v-else class="muted">{{ t('components.programInstall.notInstalled', { dir: info.binDir }) }}</span>
          </div>
          <div v-if="info.latest" class="form-help">
            <template v-if="info.latest.version">{{ t('components.programInstall.latestVersion', { version: info.latest.version, time: new Date(info.latest.checkedAt).toLocaleString() }) }}</template>
            <span v-if="info.latest.error" class="warn">{{ t('components.programInstall.checkFailed', { error: info.latest.error }) }}</span>
          </div>
          <div v-if="notUsed" class="form-help warn">{{ t('components.programInstall.notUsed', { path: notUsed }) }}</div>
        </el-form-item>
        <el-form-item :label="t('components.programInstall.proxy')">
          <el-input v-model="proxy" :placeholder="t('components.programInstall.proxyPlaceholder')" class="mono" clearable />
          <div class="form-help">{{ t('components.programInstall.proxyHelp') }}</div>
        </el-form-item>
        <el-form-item v-if="showState" :label="t('components.programInstall.progress')">
          <div class="state-box">
            <el-progress v-if="running" :percentage="percent" :indeterminate="!info.state.total" :format="() => progressText" />
            <el-alert v-else-if="info.state.stage === 'failed'" type="error" :title="info.state.error" :closable="false" show-icon />
            <el-alert v-else-if="info.state.stage === 'uptodate'" type="success" :title="t('components.programInstall.upToDate', { version: info.state.version })" :closable="false" show-icon />
            <el-alert v-else-if="info.state.stage === 'done'" type="success" :title="t('components.programInstall.installDone', { name: info.program, version: info.state.version })" :closable="false" show-icon />
          </div>
        </el-form-item>
        <el-form-item :label="t('components.programInstall.backups')">
          <div class="backups">
            <div class="form-help">{{ t('components.programInstall.backupsHelp', { count: 3 }) }}</div>
            <el-table v-if="info.backups.length" :data="info.backups" size="small">
              <el-table-column :label="t('components.programInstall.version')" min-width="120">
                <template #default="{ row }"><span class="mono">{{ row.version || t('components.programInstall.unknownVersion') }}</span></template>
              </el-table-column>
              <el-table-column :label="t('components.programInstall.savedAt')" min-width="160">
                <template #default="{ row }">{{ new Date(row.savedAt).toLocaleString() }}</template>
              </el-table-column>
              <el-table-column :label="t('common.actions')" width="170" align="right">
                <template #default="{ row }">
                  <el-popconfirm :title="t('components.programInstall.confirmRollback', { version: row.version || t('components.programInstall.unknownVersion') })" width="280" @confirm="rollback(row)">
                    <template #reference>
                      <el-button text size="small" type="primary" :icon="Undo2" :disabled="running">{{ t('components.programInstall.rollback') }}</el-button>
                    </template>
                  </el-popconfirm>
                  <el-popconfirm :title="t('components.programInstall.confirmDelete')" width="220" @confirm="removeBackup(row)">
                    <template #reference>
                      <el-button text size="small" type="danger" :icon="Trash2" :aria-label="t('common.delete')" />
                    </template>
                  </el-popconfirm>
                </template>
              </el-table-column>
            </el-table>
            <div v-else class="muted">{{ t('components.programInstall.noBackups') }}</div>
          </div>
        </el-form-item>
      </el-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Download, PackageCheck, PackageX, RefreshCw, Tag, Trash2, Undo2 } from 'lucide-vue-next'
import { api } from '../api'
import { useFormLayout } from '../composables/useIsMobile'

interface InstallState {
  running: boolean
  stage: string
  version: string
  downloaded: number
  total: number
  error: string
}
interface Backup {
  id: string
  version: string
  savedAt: string
}
// Latest 是最近一次检查更新查询到的上游最新版本
interface Latest {
  version?: string
  checkedAt: string
  error?: string
  hasUpdate: boolean
}
interface InstallView {
  state: InstallState
  program: string
  files: string[]
  binDir: string
  mainPath: string
  installed: { present: boolean; version: string; missing?: string[] }
  backups: Backup[]
  proxy: string
  platform: string
  unsupported: string
  // 从未检查过更新时为空
  latest?: Latest
}

const props = defineProps<{
  // base 为模块接口前缀，例如 /api/frp
  base: string
  title?: string
  // activePaths 为模块实际使用的可执行文件，不在安装目录时提示安装的版本不会生效
  activePaths?: (string | undefined)[]
}>()
const emit = defineEmits<{ changed: [] }>()

const { t } = useI18n()
const { labelPosition } = useFormLayout()
const info = ref<InstallView>({
  state: { running: false, stage: '', version: '', downloaded: 0, total: 0, error: '' },
  program: '',
  files: [],
  binDir: '',
  mainPath: '',
  installed: { present: false, version: '' },
  backups: [],
  proxy: '',
  platform: '',
  unsupported: '',
})
const proxy = ref('')
// showState 只在本页发起过安装或进入页面时安装仍在进行时显示结果，避免展示很久以前的失败
const showState = ref(false)
let timer: ReturnType<typeof setInterval> | undefined

const running = computed(() => info.value.state.running)
// notUsed 为不在安装目录中的可执行文件（填写了其他路径），安装的版本对它不生效
const notUsed = computed(() => {
  if (!info.value.installed.present) {
    return ''
  }
  return (props.activePaths ?? []).find((p) => p && p.slice(0, Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))) !== info.value.binDir) ?? ''
})
const percent = computed(() => {
  const s = info.value.state
  return s.total ? Math.min(100, Math.round((s.downloaded / s.total) * 100)) : 0
})
const progressText = computed(() => {
  const s = info.value.state
  if (s.stage === 'downloading' && s.total) {
    return `${(s.downloaded / 1048576).toFixed(1)} / ${(s.total / 1048576).toFixed(1)} MB`
  }
  return t(`components.programInstall.stage.${s.stage || 'checking'}`)
})

async function load(initial = false) {
  const v = await api.get<InstallView>(`${props.base}/install`)
  info.value = v
  if (initial) {
    proxy.value = v.proxy
    showState.value = v.state.running
  }
  return v
}

function stopPolling() {
  if (timer) {
    clearInterval(timer)
    timer = undefined
  }
}

// poll 每秒查询一次进度，结束后提示结果并通知页面刷新程序信息
function poll() {
  stopPolling()
  timer = setInterval(async () => {
    let v: InstallView
    try {
      v = await load()
    } catch {
      return
    }
    if (v.state.running) {
      return
    }
    stopPolling()
    if (v.state.stage === 'done') {
      ElMessage.success(t('components.programInstall.installDone', { name: v.program, version: v.state.version }))
      emit('changed')
    }
  }, 1000)
}

// install 开始安装 version，为空时安装最新正式版
async function install(version = '') {
  try {
    info.value = await api.post<InstallView>(`${props.base}/install`, { version, proxy: proxy.value.trim() })
    showState.value = true
    poll()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// checkLatest 查询上游最新版本，不下载；开启自动检查更新时 Gately 也会定期检查已安装的程序
const checking = ref(false)
async function checkLatest() {
  checking.value = true
  try {
    const v = await api.post<InstallView>(`${props.base}/install/check`)
    info.value = v
    const l = v.latest
    if (l?.hasUpdate) {
      ElMessage.success(t('components.programInstall.newVersion', { version: l.version }))
    } else if (l?.version) {
      ElMessage.info(v.installed.present ? t('components.programInstall.upToDate', { version: l.version }) : t('components.programInstall.latestOnly', { version: l.version }))
    }
  } catch (e) {
    ElMessage.error((e as Error).message)
    await load().catch(() => undefined)
  } finally {
    checking.value = false
  }
}

async function installVersion() {
  let value: string
  try {
    const r = await ElMessageBox.prompt(t('components.programInstall.versionPrompt'), t('components.programInstall.installVersion'), {
      inputPlaceholder: info.value.installed.version || 'v1.0.0',
      inputPattern: /^\S+$/,
      inputErrorMessage: t('components.programInstall.versionRequired'),
    })
    value = r.value.trim()
  } catch {
    return
  }
  await install(value)
}

async function rollback(b: Backup) {
  try {
    info.value = await api.post<InstallView>(`${props.base}/install/rollback`, { id: b.id })
    showState.value = false
    ElMessage.success(t('components.programInstall.rolledBack', { version: b.version || t('components.programInstall.unknownVersion') }))
    await load()
    emit('changed')
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// uninstall 删除安装的程序与全部旧版本，模块配置保留
async function uninstall() {
  try {
    await ElMessageBox.confirm(
      t('components.programInstall.confirmUninstall', { files: info.value.files.join(', '), dir: info.value.binDir }),
      t('components.programInstall.uninstall'),
      { type: 'warning', confirmButtonText: t('components.programInstall.uninstall'), confirmButtonClass: 'el-button--danger' },
    )
  } catch {
    return
  }
  try {
    info.value = await api.del<InstallView>(`${props.base}/install`)
    showState.value = false
    ElMessage.success(t('components.programInstall.uninstalled', { name: info.value.program }))
    emit('changed')
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function removeBackup(b: Backup) {
  try {
    await api.del(`${props.base}/install/backups/${encodeURIComponent(b.id)}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(async () => {
  try {
    const v = await load(true)
    if (v.state.running) {
      poll()
    }
  } catch {
    // 模块关闭或接口不可用时保持空白
  }
})
onUnmounted(stopPolling)

defineExpose({ install })
</script>

<style scoped>
.program-install {
  margin-top: 16px;
}
.gap {
  margin-bottom: 12px;
}
.svc-form {
  max-width: 640px;
}
.svc-form .el-form-item:last-child {
  margin-bottom: 0;
}
.line {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  min-height: 32px;
  word-break: break-all;
}
.muted {
  color: var(--el-text-color-secondary);
}
.warn {
  color: var(--el-color-warning);
}
.state-box,
.backups {
  width: 100%;
}
.head-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
</style>
