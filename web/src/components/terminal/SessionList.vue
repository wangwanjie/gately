<template>
  <el-dialog
    :model-value="modelValue"
    :title="t('termConn.sessions.title')"
    width="760px"
    :fullscreen="isMobile"
    class="term-dialog"
    @update:model-value="(v: boolean) => $emit('update:modelValue', v)"
    @open="onOpen"
    @closed="stopTimer"
  >
    <div class="top">
      <span class="meta">{{ t('termConn.sessions.intro', { count: list.length }) }}</span>
      <el-button size="small" :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
    </div>
    <el-empty v-if="!list.length && !loading" :description="t('termConn.sessions.empty')" :image-size="60" />
    <div v-for="s in list" :key="s.id" class="item">
      <div class="info">
        <div class="line1">
          <span class="name ellipsis">{{ s.connName || s.connKey }}</span>
          <el-tag size="small" effect="plain">{{ typeName(s.type) }}</el-tag>
          <el-tag v-if="openIds.includes(s.id)" size="small" type="success">{{ t('termConn.sessions.opened') }}</el-tag>
          <el-tag v-else-if="s.attached > 0" size="small" type="warning">{{ t('termConn.sessions.attachedCount', { count: s.attached }) }}</el-tag>
          <el-tag v-else size="small" type="info">{{ t('termConn.sessions.detached') }}</el-tag>
        </div>
        <div v-if="s.remark" class="remark">{{ t('termConn.sessions.remark', { remark: s.remark }) }}</div>
        <div class="meta">
          {{
            t('termConn.sessions.times', {
              created: fmtTime(s.createdAt),
              active: fmtTime(s.lastActive),
              cols: s.cols,
              rows: s.rows,
              out: formatBytes(s.bytesOut),
              in: formatBytes(s.bytesIn),
            })
          }}
        </div>
        <div v-if="stats[s.id]" class="stats meta">
          {{
            t('termConn.sessions.stats', {
              duration: formatDuration(stats[s.id].durationSec),
              idle: formatDuration(stats[s.id].idleSec),
              count: stats[s.id].attached,
            })
          }}
        </div>
      </div>
      <div class="ops">
        <el-button size="small" type="primary" plain :icon="Link2" @click="$emit('attach', s)">{{ openIds.includes(s.id) ? t('termConn.sessions.switch') : t('termConn.sessions.attach') }}</el-button>
        <el-button size="small" :icon="StickyNote" @click="editRemark(s)">{{ t('termConn.sessions.remarkBtn') }}</el-button>
        <el-button size="small" :icon="BarChart3" @click="showStats(s)">{{ t('termConn.sessions.statsBtn') }}</el-button>
        <el-button size="small" type="danger" plain :icon="Power" @click="kill(s)">{{ t('termConn.sessions.kill') }}</el-button>
      </div>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { BarChart3, Link2, Power, RefreshCw, StickyNote } from 'lucide-vue-next'
import { api, formatBytes, formatDuration } from '../../api'
import { useIsMobile } from '../../composables/useIsMobile'
import { usePolling } from '../../composables/usePolling'
import { fmtTime, type SessionInfo, type SessionStats } from './types'

defineProps<{ modelValue: boolean; openIds: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [v: boolean]; attach: [s: SessionInfo]; killed: [id: string]; remark: [id: string, remark: string] }>()

const { t } = useI18n()
const isMobile = useIsMobile()
const list = ref<SessionInfo[]>([])
const stats = ref<Record<string, SessionStats>>({})
const loading = ref(false)
// 连接类型显示名；未知类型原样显示
function typeName(type: string): string {
  return ['ssh', 'telnet', 'local'].includes(type) ? t(`termConn.type.${type}`) : type
}

// 打开期间每 5 秒自动刷新：标签页在后台时暂停，上一次请求未结束时跳过本轮
const { start: startPolling, stop: stopPolling } = usePolling(() => load(), 5000)

async function load() {
  loading.value = true
  try {
    list.value = (await api.get<SessionInfo[] | null>('/api/webterminal/sessions')) ?? []
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

function stopTimer() {
  stopPolling()
}

// 打开期间每 5 秒自动刷新一次
function onOpen() {
  stats.value = {}
  load()
  stopTimer()
  startPolling()
}

async function showStats(s: SessionInfo) {
  try {
    stats.value[s.id] = await api.get<SessionStats>(`/api/webterminal/sessions/${encodeURIComponent(s.id)}/stats`)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function editRemark(s: SessionInfo) {
  try {
    const { value } = await ElMessageBox.prompt(t('termConn.sessions.remarkPrompt'), t('termConn.sessions.remarkTitle'), { inputValue: s.remark, inputPlaceholder: t('termConn.sessions.remarkPlaceholder') })
    await api.put(`/api/webterminal/sessions/${encodeURIComponent(s.id)}/remark`, { remark: value ?? '' })
    s.remark = value ?? ''
    emit('remark', s.id, s.remark)
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error((e as Error).message)
    }
  }
}

async function kill(s: SessionInfo) {
  try {
    await ElMessageBox.confirm(t('termConn.sessions.killConfirm', { name: s.connName }), t('termConn.sessions.confirmTitle'), { type: 'warning' })
    await api.del(`/api/webterminal/sessions/${encodeURIComponent(s.id)}`)
    emit('killed', s.id)
    await load()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error((e as Error).message)
    }
  }
}

onBeforeUnmount(stopTimer)
</script>

<style scoped>
.top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}
.item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-top: 1px solid var(--g-border);
}
.info {
  flex: 1;
  min-width: 0;
}
.line1 {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.name {
  max-width: 220px;
  font-weight: 500;
}
.remark {
  margin-top: 2px;
  font-size: 13px;
  color: var(--el-text-color-regular);
}
.stats {
  margin-top: 2px;
  color: var(--el-color-primary);
}
.ops {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
  max-width: 250px;
}
.ops .el-button + .el-button {
  margin-left: 0;
}
@media (max-width: 767.98px) {
  .item {
    flex-direction: column;
    align-items: stretch;
  }
  .ops {
    max-width: none;
    justify-content: flex-start;
  }
}
</style>
