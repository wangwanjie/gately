// 管理 API 封装：会话通过 HttpOnly Cookie 维持，401 时跳转登录页。
import router from './router'
import { i18n, t } from './i18n'

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public data?: Record<string, unknown>,
  ) {
    super(message)
  }
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  // 带上界面语言，后端据此翻译错误说明与状态文案
  const init: RequestInit = { method, credentials: 'same-origin', headers: { 'Accept-Language': i18n.global.locale.value } }
  if (body instanceof FormData) {
    init.body = body
  } else if (body !== undefined) {
    ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }
  const resp = await fetch(url, init)
  const text = await resp.text()
  const data = text ? JSON.parse(text) : null
  if (resp.status === 401 && url !== '/api/login') {
    if (router.currentRoute.value.name !== 'login') {
      router.push({ name: 'login', query: { redirect: router.currentRoute.value.fullPath } })
    }
    throw new ApiError(data?.error ?? t('common.notLoggedIn'), 401, data)
  }
  if (!resp.ok) {
    throw new ApiError(data?.error ?? t('common.requestFailed', { status: resp.status }), resp.status, data)
  }
  return data as T
}

export const api = {
  get: <T>(url: string) => request<T>('GET', url),
  post: <T>(url: string, body?: unknown) => request<T>('POST', url, body),
  put: <T>(url: string, body?: unknown) => request<T>('PUT', url, body),
  // DELETE 允许携带 JSON 请求体（如批量删除文件）
  del: <T>(url: string, body?: unknown) => request<T>('DELETE', url, body),
}

export interface ModuleInfo {
  id: string
  title: string
  // 模块是否开启；关闭的模块没有摘要
  enabled: boolean
  summary?: { label: string; value: string; level?: string }[]
}

export interface Status {
  version: string
  commit: string
  buildDate: string
  goVersion: string
  os: string
  arch: string
  startTime: string
  goroutines: number
  memAlloc: number
  modules: ModuleInfo[]
  timeZone: string
  configDir: string
  host: HostStats
}

// HostStats 对应 /api/status 中的 host：主机与进程资源占用，不支持的字段缺省。
export interface HostStats {
  hostname: string
  os: string
  platform?: string
  kernel?: string
  arch: string
  inContainer: boolean
  bootTime?: number
  cpu: { model?: string; cores: number; usage?: number; load?: number[] }
  memory?: { total: number; used: number; available: number; swapTotal: number; swapUsed: number }
  disk?: { path: string; total: number; used: number; free: number }
  interfaces: NetInterface[]
  process: {
    pid: number
    rss?: number
    cpu?: number
    threads?: number
    fds?: number
    goroutines: number
    heapAlloc: number
    heapSys: number
    numGC: number
  }
  sampledAt: string
}

export interface NetInterface {
  name: string
  mac?: string
  mtu: number
  ipv4: string[]
  ipv6: string[]
  rxBytes: number
  txBytes: number
  rxRate?: number
  txRate?: number
}

export interface LogEntry {
  seq: number
  time: string
  level: 'debug' | 'info' | 'warn' | 'error'
  source?: string
  message: string
  // 可选的结构化字段：ip、host、site、status、method、path、rule 等，可按字段过滤
  fields?: Record<string, string>
}

export interface ImportReport {
  module: string
  title: string
  found: number
  imported: number
  warnings?: string[]
}

export interface ImportFileResult {
  file: string
  error?: string
  reports: ImportReport[]
}

// SystemSettings 对应 GET/PUT /api/system/settings。
// OAuthProvider 是第三方登录方式（Web 服务站点与管理界面共用）。
export interface OAuthProvider {
  key: string
  name: string
  type: string
  clientId: string
  clientSecret: string
  issuer: string
  // 服务器访问该提供方时使用的代理，留空直连
  proxy?: string
}

// ThirdLogin 是管理界面的第三方登录设置。
export interface ThirdLogin {
  enable: boolean
  callbackBase: string
  providers: OAuthProvider[]
  allowed: string[]
  skipTotp: boolean
}

export interface SystemSettings {
  listen: string
  httpsListen: string
  forceHTTPS: boolean
  safePath: string
  lanOnly: boolean
  // 经飞牛统一网关访问时，飞牛管理员自动登录
  gatewaySSO: boolean
  // 以统一网关方式运行（飞牛 fpk 包）时为 true，只读
  gatewayAvailable?: boolean
  // 为 true 时不信任本机反向代理传来的客户端地址
  disableProxyHeaders: boolean
  trustedProxies: string[] | null
  maxLoginFailures: number
  lockMinutes: number
  timeZone: string
  language: string
  dnsServers: string[] | null
  // 检查更新与在线升级访问 GitHub 时使用的代理，为空直连
  updateProxy: string
  logLines: number
  // 为 true 时运行日志只保留在内存；默认写入配置目录下的 logs/
  disableLogPersist: boolean
  // 每个日志文件的大小上限（MB），0 为默认 2 MB
  logFileMB: number
  gcPercent: number
  maxProcs: number
  thirdLogin: ThirdLogin
  // 关闭的功能模块 ID，修改后重启生效
  disabledModules: string[] | null
  // 全部功能模块及其依赖关系，只读
  modules?: ModuleOption[]
  sessionHours: number
  restartRequired: boolean
}

// ModuleOption 是设置页“功能模块”中的一项。
export interface ModuleOption {
  id: string
  title: string
  // 该模块依赖的模块 ID
  requires?: string[]
}

// SystemInfo 对应 GET /api/system/info。
export interface SystemInfo {
  version: string
  commit: string
  buildDate: string
  goVersion: string
  os: string
  arch: string
  hostname: string
  pid: number
  startTime: string
  configDir: string
  goroutines: number
  memSys: number
  memAlloc: number
  numCPU: number
  maxProcs: number
  timeZone: string
  inDocker: boolean
  selfUpgrade: SelfUpgrade
}

// SelfUpgrade 表示当前部署方式能否在线升级；不能时 reason 为 os / docker / fnos / opkg / apk / readonly。
export interface SelfUpgrade {
  supported: boolean
  reason?: 'os' | 'docker' | 'fnos' | 'opkg' | 'apk' | 'readonly'
  // 将被替换的可执行文件路径
  executable?: string
}

// UpgradeStatus 对应 GET /api/system/upgrade/status：在线升级任务的进度。
export interface UpgradeStatus {
  // 为空表示没有执行过在线升级
  state: '' | 'checking' | 'downloading' | 'verifying' | 'installing' | 'restarting' | 'failed' | 'canceled'
  from: string
  to?: string
  downloaded: number
  // 升级包总字节数，0 表示未知
  total: number
  error?: string
  startedAt: string
}

// UpgradeResult 对应上传升级包的返回。
export interface UpgradeResult {
  from: string
  to: string
  executable: string
  // 旧版本保留的位置
  previous: string
}

// UpdateInfo 对应 GET /api/system/update。
export interface UpdateInfo {
  current: string
  latest: string
  url: string
  notes: string
  publishedAt: string
  hasUpdate: boolean
  // 当前版本与最新版本号能否比较新旧（开发版等无法比较时为 false）
  comparable: boolean
  selfUpgrade: SelfUpgrade
}

// formatBytes 把字节数格式化为可读字符串。
export function formatBytes(n: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`
}

// formatRate 把字节/秒格式化为速率字符串。
export function formatRate(n: number | undefined): string {
  if (n === undefined) {
    return '-'
  }
  return formatBytes(Math.round(n)) + '/s'
}

// formatDuration 把秒数格式化为“x 天 x 小时”这类简短时长。
export function formatDuration(sec: number): string {
  let s = Math.max(0, Math.floor(sec))
  const d = Math.floor(s / 86400)
  s %= 86400
  const h = Math.floor(s / 3600)
  s %= 3600
  const m = Math.floor(s / 60)
  if (d > 0) {
    return t('common.durationDaysHours', { d, h })
  }
  if (h > 0) {
    return t('common.durationHoursMinutes', { h, m })
  }
  return t('common.durationMinutes', { m })
}

// TokenView 对应 GET /api/tokens 的一项（不含明文）。
export interface TokenView {
  id: string
  name: string
  // 令牌明文的前缀提示，便于识别
  hint: string
  scopes: string[]
  // 升级前的单令牌（拥有全部权限）
  legacy: boolean
  disabled: boolean
  expired: boolean
  createdAt: string
  expiresAt: string | null
  lastUsedAt: string | null
  lastUsedIP: string
}

// ScopeDomain 对应 GET /api/tokens/scopes 的一项：一个权限域。
export interface ScopeDomain {
  id: string
  title: string
  // 系统内置域（状态、日志、导入），其余为功能模块
  builtin: boolean
  // 仅有写权限
  writeOnly?: boolean
  // 仅有读权限
  readOnly?: boolean
  // 写权限可以在服务器上执行命令
  canExec?: boolean
}

// TokenInput 是创建/编辑令牌的请求体。
export interface TokenInput {
  name: string
  scopes: string[]
  expiresAt: string | null
  disabled?: boolean
}

// TokenSecret 是创建/重新生成令牌的响应，secret 明文只返回这一次。
export interface TokenSecret {
  token: TokenView
  secret: string
}
