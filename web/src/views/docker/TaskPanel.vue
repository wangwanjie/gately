<template>
  <!-- 后台任务浮条：展示最近的任务进度，结束后可手动关闭 -->
  <div v-if="visible.length" class="task-panel">
    <div v-for="task in visible" :key="task.id" class="task" :class="`is-${task.status}`">
      <div class="task-head">
        <Loader v-if="task.status === 'running'" :size="16" class="spin" />
        <CircleCheck v-else-if="task.status === 'done'" :size="16" class="ok" />
        <Ban v-else-if="task.status === 'cancelled'" :size="16" class="muted" />
        <CircleX v-else :size="16" class="bad" />
        <span class="task-title">{{ task.title || t('docker.taskPanel.defaultTitle') }}</span>
        <!-- 进行中的任务可以中断：拉取、构建等随即中止，批量升级在当前容器处理完后停止 -->
        <template v-if="task.status === 'running'">
          <span v-if="task.cancelling" class="stopping">{{ t('docker.task.stopping') }}</span>
          <el-popconfirm v-else :title="t('docker.task.cancelConfirm')" width="260" @confirm="cancelTask(task.id)">
            <template #reference>
              <el-button text size="small" type="danger" :icon="Square" class="task-btn">{{ t('docker.task.cancel') }}</el-button>
            </template>
          </el-popconfirm>
        </template>
        <el-button text size="small" class="task-btn" @click="toggle(task.id)">{{ open[task.id] ? t('docker.taskPanel.collapse') : t('common.details') }}</el-button>
        <el-button v-if="task.status !== 'running'" text size="small" :icon="X" class="task-btn" :aria-label="t('common.close')" @click="task.dismissed = true" />
      </div>
      <div v-if="task.status === 'error' && task.error" class="task-err">{{ task.error }}</div>
      <pre v-else-if="lastLine(task)" class="task-line mono">{{ lastLine(task) }}</pre>
      <pre v-if="open[task.id]" class="task-lines mono">{{ (task.lines ?? []).join('\n') || t('docker.taskPanel.noOutput') }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { Ban, CircleCheck, CircleX, Loader, Square, X } from 'lucide-vue-next'
import { cancelTask, trackedTasks, type TrackedTask } from './tasks'

const { t } = useI18n()

const open = reactive<Record<string, boolean>>({})
const visible = computed(() => trackedTasks.filter((task) => !task.dismissed).slice(0, 3))

function toggle(id: string) {
  open[id] = !open[id]
}

function lastLine(task: TrackedTask): string {
  const l = task.lines ?? []
  return l.length ? l[l.length - 1] : ''
}
</script>

<style scoped>
.task-panel {
  position: fixed;
  right: 16px;
  bottom: 16px;
  z-index: 2100;
  width: 380px;
  max-width: calc(100vw - 32px);
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.task {
  background: var(--el-bg-color-overlay);
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  box-shadow: 0 8px 24px -12px rgba(0, 0, 0, 0.35);
  padding: 8px 8px 8px 12px;
  min-width: 0;
}
.task.is-error {
  border-color: var(--el-color-danger-light-5);
}
.task-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 28px;
}
.task-title {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.task-btn {
  flex-shrink: 0;
}
.ok {
  color: var(--el-color-success);
}
.bad {
  color: var(--el-color-danger);
}
.muted {
  color: var(--el-text-color-secondary);
}
.stopping {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--el-color-warning);
}
.spin {
  color: var(--el-color-primary);
  animation: spin 1s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
.task-line,
.task-lines {
  margin: 4px 4px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  white-space: pre-wrap;
  word-break: break-all;
}
.task-line {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.task-lines {
  max-height: 200px;
  overflow: auto;
  background: var(--el-fill-color-lighter);
  border-radius: 6px;
  padding: 8px;
}
.task-err {
  margin: 4px 4px 0 0;
  font-size: 12px;
  color: var(--el-color-danger);
  word-break: break-all;
}
@media (max-width: 767.98px) {
  .task-panel {
    left: 8px;
    right: 8px;
    width: auto;
    max-width: none;
    bottom: calc(8px + env(safe-area-inset-bottom));
  }
  .task-btn {
    min-height: 36px;
  }
}
</style>
