<template>
  <!-- 新版本弹窗：展示版本与更新内容，执行在线升级 / 上传升级包并显示进度 -->
  <el-dialog
    v-model="visible"
    :width="mode === 'release' ? 'min(640px, 94vw)' : 'min(520px, 94vw)'"
    :show-close="!busy"
    :close-on-click-modal="!busy"
    :close-on-press-escape="!busy"
    append-to-body
    class="update-dialog"
    @closed="onClosed"
  >
    <template #header>
      <div class="head">
        <AppLogo :size="44" />
        <div class="head-text">
          <template v-if="mode === 'release' && info">
            <div class="head-title">{{ info.security ? t('about.upgrade.foundSecurity') : t('about.upgrade.found') }}</div>
            <div class="head-ver mono">Gately {{ info.latest }}</div>
            <div class="head-meta">
              {{ t('about.upgrade.currentVersion', { current: info.current }) }}
              <template v-if="info.publishedAt"> · {{ t('about.publishedAt', { time: new Date(info.publishedAt).toLocaleString() }) }}</template>
            </div>
          </template>
          <template v-else>
            <div class="head-title">{{ kind === 'upload' ? t('about.upgrade.upload') : t('about.upgrade.progressTitle', { version: target || '' }) }}</div>
            <div class="head-meta">{{ t('about.upgrade.keepOpen') }}</div>
          </template>
        </div>
      </div>
    </template>

    <!-- 版本与更新内容 -->
    <template v-if="mode === 'release' && info">
      <SecurityNotice :info="info" class="security" />
      <div class="label">{{ t('about.upgrade.notes') }}</div>
      <!-- renderMarkdown 先整体转义再生成受控标签，链接仅限 http/https，可安全用于 v-html -->
      <div v-if="info.notes" class="notes md" v-html="notesHTML" />
      <div v-else class="notes empty">{{ t('about.upgrade.noNotes') }}</div>
      <div v-if="canUpgrade" class="note">{{ t('about.upgrade.confirm', { latest: info.latest, exe: info.selfUpgrade?.executable ?? 'gately' }) }}</div>
      <el-alert v-else-if="info.selfUpgrade?.reason" type="info" :closable="false" show-icon class="reason">
        {{ t(`about.upgrade.reason.${info.selfUpgrade.reason}`) }}
      </el-alert>
    </template>

    <!-- 升级进度 -->
    <template v-else>
      <el-steps :active="stepIndex" :process-status="phase === 'failed' ? 'error' : 'process'" finish-status="success" align-center class="steps">
        <el-step v-for="s in steps" :key="s" :title="s" />
      </el-steps>
      <template v-if="phase !== 'failed'">
        <el-progress :percentage="percent" :indeterminate="indeterminate" :duration="2" :show-text="!indeterminate" :stroke-width="10" class="bar" />
        <div class="phase">
          <span>{{ phaseText }}</span>
          <span v-if="phase === 'downloading' && speed > 0" class="mono muted">{{ formatBytes(speed) }}/s</span>
        </div>
      </template>
      <el-alert v-else type="error" :title="t('about.upgrade.failed')" :description="error" :closable="false" show-icon />
    </template>

    <template #footer>
      <div class="foot">
        <a v-if="mode === 'release' && info?.url" :href="info.url" target="_blank" rel="noopener" class="release-link">{{ t('about.viewRelease') }}</a>
        <span class="spacer" />
        <template v-if="mode === 'release' && canUpgrade">
          <el-button @click="visible = false">{{ t('about.upgrade.later') }}</el-button>
          <el-button type="primary" :icon="Download" @click="startOnline">{{ t('about.upgrade.now') }}</el-button>
        </template>
        <!-- 不能在线升级时只是告知，用“知道了”关闭 -->
        <el-button v-else-if="mode === 'release'" type="primary" @click="visible = false">{{ t('about.upgrade.gotIt') }}</el-button>
        <el-button v-else-if="cancelable" :loading="canceling" @click="cancel">{{ t('about.upgrade.cancel') }}</el-button>
        <template v-else-if="phase === 'failed'">
          <el-button @click="visible = false">{{ t('common.close') }}</el-button>
          <el-button v-if="kind === 'online'" type="primary" :icon="RotateCw" @click="startOnline">{{ t('about.upgrade.retry') }}</el-button>
        </template>
      </div>
    </template>
  </el-dialog>
  <RestartOverlay ref="overlay" />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Download, RotateCw } from 'lucide-vue-next'
import { api, ApiError, formatBytes, type UpdateInfo, type UpgradeResult, type UpgradeStatus } from '../api'
import { i18n } from '../i18n'
import { renderMarkdown } from '../markdown'
import AppLogo from './AppLogo.vue'
import RestartOverlay from './RestartOverlay.vue'
import SecurityNotice from './SecurityNotice.vue'

const props = defineProps<{ isAdmin: boolean }>()
const { t } = useI18n()

type Phase = 'uploading' | Exclude<UpgradeStatus['state'], ''>

const visible = ref(false)
const mode = ref<'release' | 'progress'>('release')
const kind = ref<'online' | 'upload'>('online')
const info = ref<UpdateInfo>()
const phase = ref<Phase>('checking')
const target = ref('')
const downloaded = ref(0)
const total = ref(0)
const speed = ref(0)
const uploadPct = ref(0)
const error = ref('')
const overlay = ref<InstanceType<typeof RestartOverlay>>()
const canceling = ref(false)
let timer: number | undefined
let xhr: XMLHttpRequest | null = null
let lastSample = { at: 0, bytes: 0 }

const notesHTML = computed(() => renderMarkdown(info.value?.notes ?? ''))
const canUpgrade = computed(() => props.isAdmin && !!info.value?.hasUpdate && !!info.value.selfUpgrade?.supported)
// 升级进行中不允许关闭弹窗
const busy = computed(() => mode.value === 'progress' && phase.value !== 'failed')
// 替换可执行文件之前都可以取消：在线升级到校验为止，上传升级包在上传完成之前
const cancelable = computed(() =>
  kind.value === 'upload' ? phase.value === 'uploading' : ['checking', 'downloading', 'verifying'].includes(phase.value),
)

const steps = computed(() =>
  kind.value === 'upload'
    ? [t('about.upgrade.steps.upload'), t('about.upgrade.steps.install'), t('about.upgrade.steps.restart')]
    : [t('about.upgrade.steps.download'), t('about.upgrade.steps.verify'), t('about.upgrade.steps.install'), t('about.upgrade.steps.restart')],
)

// 失败时停在失败前所处的步骤
const failedAt = ref(0)
const stepIndex = computed(() => {
  if (phase.value === 'failed') {
    return failedAt.value
  }
  const order: Phase[] = kind.value === 'upload' ? ['uploading', 'installing', 'restarting'] : ['downloading', 'verifying', 'installing', 'restarting']
  return Math.max(0, order.indexOf(phase.value === 'checking' ? 'downloading' : phase.value))
})

const indeterminate = computed(() => {
  if (phase.value === 'downloading') {
    return total.value <= 0
  }
  return phase.value !== 'uploading'
})

const percent = computed(() => {
  if (phase.value === 'downloading' && total.value > 0) {
    return Math.min(100, Math.floor((downloaded.value / total.value) * 100))
  }
  if (phase.value === 'uploading') {
    return uploadPct.value
  }
  return 100
})

const phaseText = computed(() => {
  switch (phase.value) {
    case 'checking':
      return t('about.upgrade.phase.checking')
    case 'downloading':
      return total.value > 0
        ? t('about.upgrade.phase.downloading', { done: formatBytes(downloaded.value), total: formatBytes(total.value) })
        : t('about.upgrade.phase.downloadingUnknown', { done: formatBytes(downloaded.value) })
    case 'verifying':
      return t('about.upgrade.phase.verifying')
    case 'installing':
      return t('about.upgrade.phase.installing')
    case 'uploading':
      return t('about.upgrade.phase.uploading', { pct: uploadPct.value })
    default:
      return ''
  }
})

function stopPolling() {
  window.clearTimeout(timer)
  timer = undefined
}

function resetProgress(k: 'online' | 'upload') {
  kind.value = k
  canceling.value = false
  mode.value = 'progress'
  error.value = ''
  downloaded.value = 0
  total.value = 0
  speed.value = 0
  uploadPct.value = 0
  lastSample = { at: 0, bytes: 0 }
}

function fail(msg: string) {
  failedAt.value = stepIndex.value
  phase.value = 'failed'
  error.value = msg
  stopPolling()
}

// restarting 关闭弹窗并显示全屏等待层，服务恢复后自动刷新到新版本
function restarting(version: string) {
  stopPolling()
  phase.value = 'restarting'
  visible.value = false
  overlay.value?.start(t('about.upgrade.restarting', { version }))
}

// apply 应用一次进度；下载速度按相邻两次采样估算
function apply(st: UpgradeStatus) {
  if (st.to) {
    target.value = st.to
  }
  downloaded.value = st.downloaded
  total.value = st.total
  const now = Date.now()
  if (st.state === 'downloading') {
    if (lastSample.at && now > lastSample.at) {
      const s = ((st.downloaded - lastSample.bytes) * 1000) / (now - lastSample.at)
      speed.value = speed.value ? speed.value * 0.6 + s * 0.4 : s
    }
    lastSample = { at: now, bytes: st.downloaded }
  }
  if (st.state === 'failed') {
    fail(st.error ?? '')
    return
  }
  if (st.state === 'restarting') {
    restarting(st.to ?? '')
    return
  }
  if (st.state === 'canceled') {
    canceled()
    return
  }
  if (st.state) {
    phase.value = st.state
  }
}

// poll 轮询升级进度。服务开始重启后请求会失败：已进入安装阶段时连续失败视为正在重启
function poll(failures = 0) {
  timer = window.setTimeout(async () => {
    try {
      apply(await api.get<UpgradeStatus>('/api/system/upgrade/status'))
      if (phase.value !== 'failed' && phase.value !== 'restarting' && phase.value !== 'canceled') {
        poll(0)
      }
    } catch (e) {
      if (e instanceof ApiError && e.status !== 502 && e.status !== 503) {
        fail(e.message)
      } else if (failures >= 2 && phase.value === 'installing') {
        restarting(target.value)
      } else if (failures >= 20) {
        fail((e as Error).message)
      } else {
        poll(failures + 1)
      }
    }
  }, 600)
}

// canceled 取消完成：在线升级回到版本信息，便于重新开始；上传升级包直接关闭
function canceled() {
  stopPolling()
  canceling.value = false
  phase.value = 'canceled'
  ElMessage.info(t('about.upgrade.canceled'))
  if (kind.value === 'online' && info.value) {
    mode.value = 'release'
  } else {
    visible.value = false
  }
}

// cancel 在线升级请求服务端中止，结果由轮询得到；上传升级包直接中止上传
async function cancel() {
  if (kind.value === 'upload') {
    xhr?.abort()
    return
  }
  canceling.value = true
  try {
    await api.post('/api/system/upgrade/cancel')
  } catch (e) {
    canceling.value = false
    ElMessage.warning((e as Error).message)
  }
}

async function startOnline() {
  resetProgress('online')
  target.value = info.value?.latest ?? target.value
  phase.value = 'checking'
  try {
    apply(await api.post<UpgradeStatus>('/api/system/upgrade'))
    poll()
  } catch (e) {
    fail((e as Error).message)
  }
}

// upload 上传升级包；用 XHR 获取上传进度，上传完成后服务端校验、试运行并替换
function upload(file: File) {
  resetProgress('upload')
  target.value = ''
  phase.value = 'uploading'
  visible.value = true
  const fd = new FormData()
  fd.append('file', file)
  const req = new XMLHttpRequest()
  xhr = req
  req.open('POST', '/api/system/upgrade/upload')
  req.setRequestHeader('Accept-Language', i18n.global.locale.value)
  req.upload.onprogress = (ev) => {
    if (ev.lengthComputable) {
      uploadPct.value = Math.floor((ev.loaded / ev.total) * 100)
    }
  }
  req.upload.onload = () => {
    phase.value = 'installing'
  }
  req.onload = () => {
    let data: (UpgradeResult & { error?: string }) | null = null
    try {
      data = JSON.parse(req.responseText)
    } catch {
      // 非 JSON 响应按失败处理
    }
    if (req.status === 200 && data) {
      restarting(data.to)
    } else {
      fail(data?.error ?? t('common.requestFailed', { status: req.status }))
    }
  }
  req.onerror = () => fail(t('common.requestFailed', { status: 0 }))
  req.onabort = () => canceled()
  req.send(fd)
}

// showRelease 打开新版本弹窗
function showRelease(u: UpdateInfo) {
  stopPolling()
  info.value = u
  target.value = u.latest
  mode.value = 'release'
  visible.value = true
}

// resume 页面打开时若已有进行中的在线升级，直接显示进度
function resume(st: UpgradeStatus) {
  resetProgress('online')
  target.value = st.to ?? ''
  visible.value = true
  apply(st)
  if (phase.value !== 'failed' && phase.value !== 'restarting' && phase.value !== 'canceled') {
    poll()
  }
}

function onClosed() {
  if (!busy.value) {
    mode.value = 'release'
  }
}

onBeforeUnmount(stopPolling)

defineExpose({ showRelease, upload, resume })
</script>

<style scoped>
.head {
  display: flex;
  align-items: center;
  gap: 14px;
}
.head-text {
  min-width: 0;
}
.head-title {
  font-size: 17px;
  font-weight: 700;
}
.head-ver {
  margin-top: 2px;
  font-size: 15px;
  font-weight: 600;
  color: var(--el-color-primary);
}
.head-meta {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.security {
  margin-bottom: 14px;
}
.label {
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-regular);
}
.notes {
  max-height: min(380px, 50vh);
  overflow: auto;
  padding: 12px 16px;
  border-radius: var(--g-radius);
  border: 1px solid var(--g-border);
  background: var(--el-fill-color-lighter);
  font-size: 13px;
  line-height: 1.75;
  color: var(--el-text-color-regular);
  overflow-wrap: anywhere;
}
.notes.empty {
  color: var(--el-text-color-secondary);
}
.md :deep(h1),
.md :deep(h2),
.md :deep(h3),
.md :deep(h4),
.md :deep(h5),
.md :deep(h6) {
  margin: 14px 0 6px;
  font-weight: 700;
  line-height: 1.4;
  color: var(--el-text-color-primary);
}
.md :deep(h1) {
  font-size: 17px;
}
.md :deep(h2) {
  font-size: 15px;
  padding-bottom: 4px;
  border-bottom: 1px solid var(--g-border);
}
.md :deep(h3),
.md :deep(h4),
.md :deep(h5),
.md :deep(h6) {
  font-size: 14px;
}
.md :deep(> :first-child) {
  margin-top: 0;
}
.md :deep(p) {
  margin: 6px 0;
}
.md :deep(ul),
.md :deep(ol) {
  margin: 6px 0;
  padding-left: 20px;
}
.md :deep(li) {
  margin: 4px 0;
}
.md :deep(li::marker) {
  color: var(--el-color-primary);
}
.md :deep(strong) {
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.md :deep(code) {
  font-family: var(--el-font-family-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 12px;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--el-fill-color);
}
.md :deep(pre) {
  margin: 8px 0;
  padding: 10px 12px;
  overflow-x: auto;
  border-radius: 6px;
  background: var(--el-fill-color);
}
.md :deep(pre code) {
  padding: 0;
  background: none;
}
.md :deep(blockquote) {
  margin: 8px 0;
  padding: 2px 12px;
  border-left: 3px solid var(--el-color-primary-light-5);
  color: var(--el-text-color-secondary);
}
.md :deep(hr) {
  margin: 12px 0;
  border: none;
  border-top: 1px solid var(--g-border);
}
.md :deep(a) {
  color: var(--el-color-primary);
  text-decoration: none;
  overflow-wrap: anywhere;
}
.md :deep(a:hover) {
  text-decoration: underline;
}
.note {
  margin-top: 12px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--el-text-color-secondary);
}
.reason {
  margin-top: 12px;
}
.steps {
  margin: 6px 0 22px;
}
.bar {
  margin-bottom: 10px;
}
.phase {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
  color: var(--el-text-color-regular);
}
.muted {
  color: var(--el-text-color-secondary);
}
.foot {
  display: flex;
  align-items: center;
  gap: 8px;
}
.spacer {
  flex: 1;
}
.release-link {
  font-size: 13px;
  text-decoration: none;
}
.foot .el-button + .el-button {
  margin-left: 0;
}
</style>
