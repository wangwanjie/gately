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
  hold: boolean
  // errors 为修改生效以来模块报告的错误（例如监听启动失败）
  errors: string[]
}

// HoldItem 是批量修改中暂缓生效的规则或监听
export interface HoldItem {
  module: string
  key: string
  name?: string
}

export interface HoldState {
  items: HoldItem[]
  configs: string[]
}

export interface ConfirmStatus {
  enabled: boolean
  timeout: number
  pending: ConfirmPending | null
  rolledBack: { configs: string[]; reason: string; errors: string[] } | null
  hold: HoldState | null
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
  rolledBack: null as { configs: string[]; reason: string; errors: string[] } | null,
  // errors 为修改生效以来模块报告的错误（例如监听启动失败），确认不了时往往就是原因
  errors: [] as string[],
  // hold 为进行中的批量修改（修改已保存、暂缓生效），没有时为 null
  hold: null as HoldState | null,
  // reachable 为确认失败期间普通请求能否到达服务端：到不了时“立即回滚”同样发不出去，只能等自动回滚
  reachable: true,
})

// 本标签页等待过确认（保存后确认失败或经确认重启），重新打开页面后据此提示回滚结果、自动确认重启后的配置
const WATCH_KEY = 'gately-confirm-watch'
const RESTART_KEY = 'gately-confirm-restart'

const CONFIRM_MODULES = /^\/api\/(portforward|protoroute|webservice)\//

function pathOf(url: string): string {
  return url.split('?')[0]
}

// isProtectedWrite 判断写请求是否可能影响访问路径：三个模块的写请求、保存全局设置与重启
export function isProtectedWrite(method: string, url: string): boolean {
  if (method === 'GET' || method === 'HEAD') {
    return false
  }
  const p = pathOf(url)
  return CONFIRM_MODULES.test(p) || (method === 'PUT' && p === '/api/system/settings') || (method === 'POST' && p === '/api/system/restart')
}

// confirmHeaderFor 判断请求是否需要带确认请求头：受保护的写请求，且确认未关闭；
// 批量修改期间总是带上，服务端据此记录改动前的内容，放弃修改时恢复
export function confirmHeaderFor(method: string, url: string): boolean {
  return (confirmState.enabled || !!confirmState.hold) && isProtectedWrite(method, url)
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
  // 批量修改期间修改暂缓生效，不需要确认；刷新横幅上暂存的项目
  if (confirmState.hold) {
    void refreshStatus()
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
  // 弹窗已出现（确认不了或打开页面时发现待确认的变更）后再得到“没有待确认的变更”：变更已不在了，
  // 通常是服务端到时回滚并重启后访问恢复，也可能在别的页面确认了。不能当作确认成功，刷新页面看结果（回滚时会提示原因）
  if (status === 'none' && (confirmState.phase === 'failed' || confirmState.phase === 'found')) {
    confirmState.phase = 'rollingBack'
    sessionStorage.setItem(WATCH_KEY, '1')
    location.reload()
    return true
  }
  switch (status) {
    case 'confirmed':
    case 'none':
    case 'waitRestart':
    case 'hold':
      if (confirmState.phase === 'failed' || confirmState.phase === 'found') {
        confirmState.confirmedAfterPrompt++
      }
      confirmState.phase = 'idle'
      confirmState.reachable = true
      confirmState.errors = []
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
    let applying = 0
    for (let i = 0; i < 2 && !finished; i++) {
      const status = await socketConfirm()
      // applying：确认请求在修改生效完成前就到达（批量修改还在逐项生效），稍后经新连接重试，不计入静默确认次数
      if (status === 'applying' && applying < 20) {
        applying++
        i--
        await sleep(500)
        continue
      }
      finished = handleResult(status)
    }
    if (finished) {
      continue
    }
    confirmState.phase = 'failed'
    sessionStorage.setItem(WATCH_KEY, '1')
    // 普通请求可能还能经旧连接到达：取服务端给出的剩余时间，消除两边时钟差
    await refreshRemaining()
    while (!again) {
      if (confirmState.deadline && Date.now() > confirmState.deadline + 2000) {
        startRollingBack()
        break
      }
      if (handleResult(await socketConfirm())) {
        break
      }
      // 顺带检查普通请求能否到达服务端（决定“立即回滚”能否使用）
      await refreshRemaining()
      await sleep(3000)
    }
  } while (again)
}

async function refreshRemaining() {
  const ctrl = new AbortController()
  const timer = window.setTimeout(() => ctrl.abort(), 3000)
  try {
    const resp = await fetch('/api/system/confirm', { credentials: 'same-origin', cache: 'no-store', signal: ctrl.signal })
    if (!resp.ok) {
      throw new Error(String(resp.status))
    }
    const s = (await resp.json()) as ConfirmStatus
    confirmState.reachable = true
    if (s.pending && !s.pending.waitRestart && !s.pending.hold) {
      confirmState.configs = s.pending.configs
      confirmState.deadline = Date.now() + s.pending.remaining * 1000
      confirmState.errors = s.pending.errors ?? []
    }
  } catch {
    // 连不上服务端：按本地估计倒计时，“立即回滚”也发不出去
    confirmState.reachable = false
  } finally {
    window.clearTimeout(timer)
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

// rollbackNow 立即回滚：普通请求，只有还能连上服务端时才有效；连不上时标记不可达，界面改为提示等待自动回滚
export async function rollbackNow() {
  confirmState.busy = true
  try {
    await api.post('/api/system/confirm/rollback')
    startRollingBack()
  } catch (e) {
    if (e instanceof TypeError) {
      confirmState.reachable = false
    }
    throw e
  } finally {
    confirmState.busy = false
  }
}

// refreshStatus 刷新确认开关、时限与批量修改状态
export async function refreshStatus(): Promise<ConfirmStatus | null> {
  try {
    const s = await api.get<ConfirmStatus>('/api/system/confirm')
    confirmState.enabled = s.enabled
    confirmState.timeout = s.timeout || 90
    confirmState.hold = s.hold
    return s
  } catch {
    return null
  }
}

// beginHold 开始批量修改：此后端口转发、协议分流与 Web 服务的保存暂缓生效
export async function beginHold() {
  confirmState.hold = await api.post<HoldState>('/api/system/changeset/begin')
}

// HoldProblem 是全部生效后会启动失败的一项：监听地址在本机不可用
export interface HoldProblem {
  module: string
  key: string
  name?: string
  addr: string
  error: string
}

// checkHold 在全部生效前检查暂存的规则与监听，返回会启动失败的项
export async function checkHold(): Promise<HoldProblem[]> {
  const r = await api.post<{ problems: HoldProblem[] }>('/api/system/changeset/check')
  return r.problems ?? []
}

// applyHold 批量修改全部生效；需要确认时与单次保存一样经新连接确认，确认不了时整批回滚
export async function applyHold(): Promise<number> {
  // 记下这批涉及的模块：生效后连不上服务端时，倒计时弹窗也能说明改了哪里
  const mods = [...new Set((confirmState.hold?.items ?? []).map((it) => it.module))]
  let r: { applied: number; confirm: boolean; timeout: number }
  try {
    r = await api.post<{ applied: number; confirm: boolean; timeout: number }>('/api/system/changeset/apply')
  } catch (e) {
    if (!(e instanceof TypeError)) {
      throw e
    }
    // 响应丢失（生效重启了这次请求经过的监听）：修改可能已经生效，照常经新连接确认，确认不了时服务端到时回滚
    const n = confirmState.hold?.items.length ?? 0
    confirmState.hold = null
    confirmState.configs = mods
    scheduleCheck(Date.now() + confirmState.timeout * 1000)
    return n
  }
  confirmState.hold = null
  if (r.confirm) {
    confirmState.configs = mods
    scheduleCheck(Date.now() + r.timeout * 1000)
  }
  return r.applied
}

// discardHold 放弃批量修改：有改动时服务端恢复开始前的配置并重启，页面等它恢复后刷新
export async function discardHold() {
  const r = await api.post<{ restart: boolean }>('/api/system/changeset/discard')
  confirmState.hold = null
  if (r.restart) {
    startRollingBack()
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
  confirmState.hold = s.hold
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
  if (p.waitRestart || p.hold) {
    return
  }
  if (afterRestart) {
    scheduleCheck(Date.now() + p.remaining * 1000)
    return
  }
  confirmState.deadline = Date.now() + p.remaining * 1000
  confirmState.errors = p.errors ?? []
  confirmState.phase = 'found'
}
