// 后台任务跟踪：提交任务后按 1 秒间隔轮询 /tasks/{id}，状态全局共享，供 TaskPanel 展示。
import { reactive } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../../api'
import { t } from '../../i18n'
import type { DockerTask } from './types'

export interface TrackedTask extends DockerTask {
  // 用户是否已关闭该条浮条
  dismissed: boolean
}

export const trackedTasks = reactive<TrackedTask[]>([])
const callbacks = new Map<string, (task: DockerTask) => void>()
const timers = new Map<string, number>()
// 任务记录消失时的回调：升级 Gately 自身后进程重启，旧任务查不到，由调用方按“已在重启”处理
const lostCallbacks = new Map<string, () => void>()

async function poll(id: string) {
  try {
    const task = await api.get<DockerTask>(`/api/docker/tasks/${id}`)
    const cur = trackedTasks.find((x) => x.id === id)
    if (cur) {
      Object.assign(cur, task)
    }
    if (task.status === 'running') {
      timers.set(id, window.setTimeout(() => poll(id), 1000))
      return
    }
    timers.delete(id)
    const cb = callbacks.get(id)
    callbacks.delete(id)
    lostCallbacks.delete(id)
    if (task.status === 'error') {
      ElMessage.error(task.error || t('docker.task.failed', { title: task.title }))
    } else if (task.status === 'cancelled') {
      ElMessage.info(t('docker.task.cancelled', { title: task.title }))
    }
    if (cb) {
      cb(task)
    }
  } catch (e) {
    // 轮询失败（多为网络抖动）时稍后重试，任务不存在则放弃
    const msg = (e as Error).message
    if (/不存在|not found|404/i.test(msg)) {
      timers.delete(id)
      callbacks.delete(id)
      const lost = lostCallbacks.get(id)
      lostCallbacks.delete(id)
      if (lost) {
        const idx = trackedTasks.findIndex((x) => x.id === id)
        if (idx >= 0) {
          trackedTasks.splice(idx, 1)
        }
        lost()
        return
      }
      const cur = trackedTasks.find((x) => x.id === id)
      if (cur) {
        cur.status = 'error'
        cur.error = msg
      }
      return
    }
    timers.set(id, window.setTimeout(() => poll(id), 2000))
  }
}

// trackTask 开始跟踪一个任务，结束（成功或失败）后调用 onEnd；onLost 在任务记录不存在（如 Gately 已重启）时调用。
export function trackTask(id: string, title: string, onEnd?: (task: DockerTask) => void, onLost?: () => void) {
  trackedTasks.unshift({ id, kind: '', title, status: 'running', started: new Date().toISOString(), lines: [], dismissed: false })
  if (trackedTasks.length > 5) {
    trackedTasks.length = 5
  }
  if (onEnd) {
    callbacks.set(id, onEnd)
  }
  if (onLost) {
    lostCallbacks.set(id, onLost)
  }
  poll(id)
}

// cancelTask 请求取消进行中的任务；返回的任务快照同步到浮条，最终状态由轮询更新。
export async function cancelTask(id: string): Promise<DockerTask | undefined> {
  try {
    const task = await api.post<DockerTask>(`/api/docker/tasks/${encodeURIComponent(id)}/cancel`)
    const cur = trackedTasks.find((x) => x.id === id)
    if (cur) {
      Object.assign(cur, task)
    }
    return task
  } catch (e) {
    ElMessage.error((e as Error).message)
    return undefined
  }
}

// 恢复页面上仍在运行的任务（刷新页面后继续展示进度）。
export async function resumeRunningTasks() {
  try {
    const list = await api.get<DockerTask[]>('/api/docker/tasks')
    for (const task of list ?? []) {
      if (task.status === 'running' && !trackedTasks.some((x) => x.id === task.id)) {
        trackTask(task.id, task.title)
      }
    }
  } catch {
    // 无法连接 Docker 时由主页面给出提示
  }
}
