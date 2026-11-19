// 配置变更确认：保存端口转发、协议分流、Web 服务或全局设置后，用新建立的连接确认管理界面仍可访问，
// 确认不了时提示并倒计时，服务端到时自动回滚到修改前的配置（见后端 internal/app/confirm.go）。
//
// 浏览器总是为 WebSocket 新建连接，经过的是修改后的转发与监听；普通 fetch 可能复用修改前建立的长连接，
// 热更新后它仍连着旧目标，成功也不能说明新配置可用，因此确认只走 WebSocket。
import { reactive } from 'vue'
import { api } from './api'

export interface ConfirmPending {
  id: string
  configs: string[]
  remaining: number
  waitRestart: boolean
  rollback: boolean
}

export interface ConfirmStatus {
  enabled: boolean
  timeout: number
  pending: ConfirmPending | null
  rolledBack: { configs: string[]; reason: string } | null
}

// phase：idle 无需处理；checking 正在后台确认（不打扰用户）；failed 保存后确认不了；
// found 打开页面时发现别处留下的待确认变更；rollingBack 正在回滚并重启
export type ConfirmPhase = 'idle' | 'checking' | 'failed' | 'found' | 'rollingBack'

export const confirmState = reactive({
  // 未读取状态前按开启处理：服务端关闭时会忽略请求头，确认连接返回 none
  enabled: true,
  timeout: 90,
  phase: 'idle' as ConfirmPhase,
  configs: [] as string[],
  // deadline 为本地时钟下的自动回滚时间（毫秒），0 表示未知
  deadline: 0,
  // busy 为手动确认或回滚进行中
  busy: false,
  // confirmedAfterPrompt 为弹窗出现后才确认成功，界面据此提示一次
  confirmedAfterPrompt: 0,
  // rolledBack 为刚刚回滚掉的配置，界面据此提示一次
  rolledBack: null as { configs: string[]; reason: string } | null,
})

// 本标签页等待过确认（保存后确认失败或经确认重启），重新打开页面后据此提示回滚结果、自动确认重启后的配置
const WATCH_KEY = 'gately-confirm-watch'
const RESTART_KEY = 'gately-confirm-restart'

const CONFIRM_MODULES = /^\/api\/(portforward|protoroute|webservice)\//

function pathOf(url: string): string {
  return url.split('?')[0]
}

// confirmHeaderFor 判断请求是否需要带确认请求头：三个模块的写请求、保存全局设置与重启
export function confirmHeaderFor(method: string, url: string): boolean {
  if (method === 'GET' || method === 'HEAD' || !confirmState.enabled) {
    return false
  }
  const p = pathOf(url)
  return CONFIRM_MODULES.test(p) || (method === 'PUT' && p === '/api/system/settings') || (method === 'POST' && p === '/api/system/restart')
}

// afterConfirmWrite 在带确认请求头的请求结束后调用：ok 为收到响应（含校验失败），network 为连接中断或代理返回 502~504。
// 校验失败等未改动配置的情况由服务端判断，确认连接返回 none
export function afterConfirmWrite(method: string, url: string) {
  if (method === 'POST' && pathOf(url) === '/api/system/restart') {
    // 重启后才开始计时，由重启后的页面自动确认
    sessionStorage.setItem(RESTART_KEY, '1')
    sessionStorage.setItem(WATCH_KEY, '1')
    return
  }
  // 先按请求路径记下涉及的配置：连不上服务端时弹窗也能说明改了哪里，连得上时以服务端返回的为准
  const p = pathOf(url)
  const cfg = p === '/api/system/settings' ? 'gately' : (CONFIRM_MODULES.exec(p)?.[1] ?? '')
  if (cfg && !confirmState.configs.includes(cfg)) {
    confirmState.configs = confirmState.phase === 'idle' ? [cfg] : [...confirmState.configs, cfg]
  }
  scheduleCheck(Date.now() + confirmState.timeout * 1000)
}

function sleep(ms: number) {
  return new Promise((r) => window.setTimeout(r, ms))
}

// socketConfirm 经新建立的 WebSocket 确认，返回服务端的结果或 failed（连不上、超时）
export function socketConfirm(timeoutMs = 4000): Promise<string> {
  return new Promise((resolve) => {
    let done = false
    const finish = (status: string, ws?: WebSocket) => {
      if (done) {
        return
      }
      done = true
      window.clearTimeout(timer)
      try {
        ws?.close()
      } catch {
        // 已关闭
      }
      resolve(status)
    }
    let ws: WebSocket
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    try {
      ws = new WebSocket(`${proto}://${location.host}/api/system/confirm/connect`)
    } catch {
      resolve('failed')
      return
    }
    const timer = window.setTimeout(() => finish('failed', ws), timeoutMs)
    ws.onmessage = (e) => {
      let status = 'failed'
      try {
        status = String(JSON.parse(String(e.data)).status ?? 'failed')
      } catch {
        // 格式不对按失败处理
      }
      finish(status === 'error' ? 'failed' : status, ws)
    }
    ws.onerror = () => finish('failed', ws)
    ws.onclose = () => finish('failed')
  })
}

let running = false
let again = false
let nextDeadline = 0

function scheduleCheck(deadline: number) {
  nextDeadline = deadline
  if (running) {
    again = true
    return
  }
  running = true
  void runCheck().finally(() => {
    running = false
  })
}

// handleResult 处理一次确认的结果，返回是否结束等待
function handleResult(status: string): boolean {
  switch (status) {
    case 'confirmed':
    case 'none':
    case 'waitRestart':
      if (confirmState.phase === 'failed' || confirmState.phase === 'found') {
        confirmState.confirmedAfterPrompt++
      }
      confirmState.phase = 'idle'
      sessionStorage.removeItem(WATCH_KEY)
      return true
    case 'rollingBack':
      startRollingBack()
      return true
  }
  return false
}

async function runCheck() {
  do {
    again = false
    confirmState.deadline = nextDeadline
    if (confirmState.phase === 'idle') {
      confirmState.phase = 'checking'
    }
    // 等保存引起的监听重启、连接断开完成
    await sleep(400)
    // 先静默确认两次：访问正常时用户不会看到任何提示
    let finished = false
    for (let i = 0; i < 2 && !finished; i++) {
      finished = handleResult(await socketConfirm())
    }
    if (finished) {
      continue
    }
    confirmState.phase = 'failed'
    sessionStorage.setItem(WATCH_KEY, '1')
    // 普通请求可能还能经旧连接到达：取服务端给出的剩余时间，消除两边时钟差
    refreshRemaining()
    while (!again) {
      if (confirmState.deadline && Date.now() > confirmState.deadline + 2000) {
        startRollingBack()
        break
      }
      if (handleResult(await socketConfirm())) {
        break
      }
      await sleep(3000)
    }
  } while (again)
}

async function refreshRemaining() {
  try {
    const s = await api.get<ConfirmStatus>('/api/system/confirm')
    if (s.pending && !s.pending.waitRestart) {
      confirmState.configs = s.pending.configs
      confirmState.deadline = Date.now() + s.pending.remaining * 1000
    }
  } catch {
    // 旧连接也断了，按本地估计倒计时
  }
}

let reloadTimer: number | undefined

// startRollingBack 进入回滚等待：服务端重启并恢复配置，恢复后刷新页面。须先观察到一次失败或等满 6 秒，
// 避免旧进程尚未退出时误判
function startRollingBack() {
  confirmState.phase = 'rollingBack'
  sessionStorage.setItem(WATCH_KEY, '1')
  if (reloadTimer !== undefined) {
    return
  }
  const begin = Date.now()
  let sawDown = false
  const poll = async () => {
    try {
      const resp = await fetch('/api/session', { credentials: 'same-origin', cache: 'no-store' })
      if ((sawDown || Date.now() - begin > 6000) && (resp.status === 200 || resp.status === 401)) {
        location.reload()
        return
      }
      if (resp.status >= 500) {
        sawDown = true
      }
    } catch {
      sawDown = true
    }
    reloadTimer = window.setTimeout(poll, 2000)
  }
  reloadTimer = window.setTimeout(poll, 2000)
}

// confirmNow 由用户点击“确认保留”：同样经新建立的连接确认
export async function confirmNow(): Promise<boolean> {
  confirmState.busy = true
  try {
    return handleResult(await socketConfirm(6000))
  } finally {
    confirmState.busy = false
  }
}

// rollbackNow 立即回滚：普通请求，可能经修改前的连接到达
export async function rollbackNow() {
  confirmState.busy = true
  try {
    await api.post('/api/system/confirm/rollback')
    startRollingBack()
  } finally {
    confirmState.busy = false
  }
}

// loadConfirmStatus 在进入管理界面时读取状态：本标签页经确认重启的自动确认，别处留下的待确认变更弹窗询问，
// 刚刚回滚过且本标签页在等待时提示回滚结果
export async function loadConfirmStatus() {
  let s: ConfirmStatus
  try {
    s = await api.get<ConfirmStatus>('/api/system/confirm')
  } catch {
    return
  }
  confirmState.enabled = s.enabled
  confirmState.timeout = s.timeout || 90
  const watching = sessionStorage.getItem(WATCH_KEY) === '1'
  const afterRestart = sessionStorage.getItem(RESTART_KEY) === '1'
  sessionStorage.removeItem(RESTART_KEY)
  const p = s.pending
  if (!p) {
    if (watching && s.rolledBack) {
      confirmState.rolledBack = s.rolledBack
    }
    sessionStorage.removeItem(WATCH_KEY)
    return
  }
  confirmState.configs = p.configs
  if (p.rollback) {
    startRollingBack()
    return
  }
  if (p.waitRestart) {
    return
  }
  if (afterRestart) {
    scheduleCheck(Date.now() + p.remaining * 1000)
    return
  }
  confirmState.deadline = Date.now() + p.remaining * 1000
  confirmState.phase = 'found'
}
