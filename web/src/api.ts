// 管理 API 封装：会话通过 HttpOnly Cookie 维持，401 时跳转登录页。
import router from './router'
import { i18n, t } from './i18n'
import { afterConfirmWrite, beginHold, confirmHeaderFor, confirmState, isProtectedWrite } from './configConfirm'
import { askRisk, checkAccess } from './accessPath'

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
  // 修改端口转发、协议分流、Web 服务与全局设置可能切断当前访问路径：保存前请服务端判断是否影响当前访问，
  // 有影响时先让用户确认（见 accessPath.ts）；重启与上传文件不检查
  // 批量修改期间修改暂缓生效，不会影响当前访问，不再提醒
  if (isProtectedWrite(method, url) && !url.startsWith('/api/system/restart') && !(body instanceof FormData) && !confirmState.hold) {
    const check = await checkAccess(method, url, body)
    if (check && check.risks.length > 0) {
      const answer = await askRisk(check)
      if (answer === 'cancel') {
        throw new ApiError(t('accessPath.cancelled'), 0)
      }
      if (answer === 'hold') {
        // 先开始批量修改，这次保存随之暂缓生效
        await beginHold()
      }
    }
  }
  // 带上确认请求头，保存后经新连接确认，确认不了时服务端到时自动回滚（见 configConfirm.ts）
  const needConfirm = confirmHeaderFor(method, url)
  if (needConfirm) {
    ;(init.headers as Record<string, string>)['X-Gately-Confirm'] = '1'
  }
  let resp: Response
  try {
    resp = await fetch(url, init)
  } catch (e) {
    // 保存的响应可能因监听重启、连接被切断而丢失，配置却已改动
    if (needConfirm) {
      afterConfirmWrite(method, url)
    }
    throw e
  }
  // 成功，或前置代理因后端断开返回 502~504：都可能已经改动了配置；校验失败等 4xx 没有改动
  if (needConfirm && (resp.ok || resp.status === 502 || resp.status === 503 || resp.status === 504)) {
    afterConfirmWrite(method, url)
  }
  const text = await resp.text()
  // 接口只返回 JSON；收到 HTML 等其他内容说明请求被安全入口或“未授权访问响应”拦截（例如会话过期后入口 Cookie 也已失效），
  // 自定义页面与空白页的状态码可能是 200，不能当作成功
  const contentType = resp.headers.get('Content-Type') ?? ''
  const unexpected = text ? !contentType.includes('json') : resp.ok && contentType.startsWith('text/html')
  const data = text && !unexpected ? JSON.parse(text) : null
  if (resp.status === 401 && url !== '/api/login') {
    if (router.currentRoute.value.name !== 'login') {
      router.push({ name: 'login', query: { redirect: router.currentRoute.value.fullPath } })
    }
    throw new ApiError(data?.error ?? t('common.notLoggedIn'), 401, data)
  }
  if (!resp.ok) {
    throw new ApiError(data?.error ?? t('common.requestFailed', { status: resp.status }), resp.status, data)
  }
  if (unexpected) {
    throw new ApiError(t('common.unexpectedResponse'), resp.status)
  }
  return data as T
}

// errorText 返回请求失败的说明，用于列表的加载失败提示：网络中断时 fetch 抛出的 TypeError 只有英文的 "Failed to fetch"，
// 换成本地化的提示；接口返回的错误使用后端给出的说明
export function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    return e.message
  }
  if (e instanceof TypeError) {
    return t('common.networkError')
  }
  return e instanceof Error ? e.message : String(e)
}

// ConditionalResult 是条件请求的结果：status 为 304 时内容没有变化，data 为 null
export interface ConditionalResult<T> {
  status: number
  etag: string
  data: T | null
}

// getIfChanged 带 If-None-Match 发起 GET（etag 为空时不带），用于轮询支持 ETag 的接口：
// 304 时返回 data 为 null，调用方跳过解析与赋值；其他错误与 api.get 相同（抛出 ApiError，401 跳转登录页）
export async function getIfChanged<T>(url: string, etag: string): Promise<ConditionalResult<T>> {
  const headers: Record<string, string> = { 'Accept-Language': i18n.global.locale.value }
  if (etag) {
    headers['If-None-Match'] = etag
  }
  const resp = await fetch(url, { method: 'GET', credentials: 'same-origin', headers })
  if (resp.status === 304) {
    return { status: 304, etag: resp.headers.get('ETag') ?? etag, data: null }
  }
  if (!resp.ok) {
    // 错误响应交给通用请求函数处理（解析错误说明、401 跳转登录页）
    return { status: resp.status, etag: '', data: await request<T>('GET', url) }
  }
  const text = await resp.text()
  const contentType = resp.headers.get('Content-Type') ?? ''
  if (!contentType.includes('json')) {
    throw new ApiError(t('common.unexpectedResponse'), resp.status)
  }
  return { status: resp.status, etag: resp.headers.get('ETag') ?? '', data: JSON.parse(text) as T }
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
  // 托管的外部进程（frpc、tailscaled、File Browser 等），旧版本后端没有该字段
  managed?: ManagedProcess[]
  sampledAt: string
}

// ManagedProcess 是一个托管进程及其资源占用；cpu 按整机核数归一化，rss 为字节，取不到时缺省
export interface ManagedProcess {
  module: string
  name: string
  program: string
  pid: number
  since: string
  restarts: number
  cpu?: number | null
  rss?: number
  threads?: number
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
  // 未授权访问的响应方式：'' 默认、notfound、blank、close、drop、custom
  denyMode: '' | 'notfound' | 'blank' | 'close' | 'drop' | 'custom'
  // denyMode 为 custom 时返回的状态码与 HTML
  denyStatus: number
  denyPage: string
  // 经飞牛统一网关访问时，飞牛管理员自动登录
  gatewaySSO: boolean
  // 以统一网关方式运行（飞牛 fpk 包）时为 true，只读
  gatewayAvailable?: boolean
  // 为 true 时不信任本机反向代理传来的客户端地址
  disableProxyHeaders: boolean
  trustedProxies: string[] | null
  maxLoginFailures: number
  lockMinutes: number
  // 陌生来源 10 分钟内累计的登录失败上限：0 为默认 20，负数为关闭
  loginGlobalLimit: number
  // 修改端口转发、协议分流、Web 服务与全局设置后确认访问正常的时限（秒）：0 为默认 90，负数关闭
  confirmTimeout: number
  timeZone: string
  language: string
  dnsServers: string[] | null
  // 检查更新与在线升级访问 GitHub 时使用的代理，为空直连
  updateProxy: string
  // 为 true 时，管理员配置的出站请求拒绝访问内网与链路本地地址
  restrictOutbound: boolean
  // 为 true 时不再定期检查更新与安全公告，只在点击“检查更新”时访问 GitHub
  disableUpdateCheck: boolean
  logLines: number
  // 为 true 时运行日志只保留在内存；默认写入配置目录下的 logs/
  disableLogPersist: boolean
  // 每个日志文件的大小上限（MB），0 为默认 2 MB
  logFileMB: number
  // 访问日志（Web 服务）归档的保留天数与总大小（MB），0 为默认值
  accessLogDays: number
  accessLogMB: number
  // 运行日志目录（绝对路径），为空时使用配置目录下的 logs/
  logDir: string
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
  build?: BuildProfile
}

// BuildProfile 描述程序的构建内容：定制构建去掉了部分功能模块或界面语言。
export interface BuildProfile {
  custom: boolean
  // 编译进来的功能模块 ID
  modules: string[]
  // 没有编译进来的功能模块
  absent: { id: string; title: string }[]
  // 编译进来的语言
  languages: string[]
}

// SelfUpgrade 表示当前部署方式能否在线升级；不能时 reason 为 os / docker / fnos / opkg / apk / readonly。
export interface SelfUpgrade {
  supported: boolean
  reason?: 'os' | 'docker' | 'fnos' | 'opkg' | 'apk' | 'readonly'
  // 将被替换的可执行文件路径
  executable?: string
  // 定制构建：可以上传升级包，但不提供在线下载升级（发布页的完整版本会装回全部功能）
  custom?: boolean
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
  // 目标版本没有发布签名（签名上线之前的版本），只核对了 SHA-256
  unsigned?: boolean
}

// UpgradeResult 对应上传升级包的返回。
export interface UpgradeResult {
  from: string
  to: string
  executable: string
  // 旧版本保留的位置
  previous: string
}

// SecurityAdvisory 是一条影响当前版本的已公开安全公告（GitHub Security Advisories）。
export interface SecurityAdvisory {
  id: string
  cve?: string
  summary: string
  // low / medium / high / critical，未填写时为空
  severity?: string
  url: string
  publishedAt: string
  // 受影响的版本范围与修复版本，写法同 GitHub，例如 "<= 0.6.0.4"
  ranges: { vulnerable: string; patched?: string }[]
}

// ReleaseBrief 是一个发布版本的摘要。
export interface ReleaseBrief {
  version: string
  url: string
  publishedAt: string
}

// UpdateInfo 对应 GET /api/system/update（立即检查）与 GET /api/system/update/state（上次检查的结果）。
export interface UpdateInfo {
  current: string
  // 从未成功检查过时为空
  latest: string
  url: string
  notes: string
  publishedAt?: string
  hasUpdate: boolean
  // 当前版本与最新版本号能否比较新旧（开发版等无法比较时为 false）
  comparable: boolean
  // 当前版本受已公开的安全问题影响：advisories 为影响当前版本的安全公告，securityReleases 为比当前版本新、带有安全修复的版本
  security: boolean
  advisories: SecurityAdvisory[]
  securityReleases: ReleaseBrief[]
  // 是否开启自动检查、最近一次成功检查的时间，以及最近一次失败的原因与时间
  autoCheck: boolean
  checkedAt?: string
  error?: string
  failedAt?: string
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
  // group 为列表中的分组名称，只影响展示
  group?: string
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
  // 写权限可以读取或修改服务器上的文件（把目录提供出去，或把配置备份写到存储挂载点）
  canFiles?: boolean
}

// TokenInput 是创建/编辑令牌的请求体。
export interface TokenInput {
  name: string
  scopes: string[]
  expiresAt: string | null
  disabled?: boolean
  // group 不提供时保持原分组
  group?: string
}

// TokenSecret 是创建/重新生成令牌的响应，secret 明文只返回这一次。
export interface TokenSecret {
  token: TokenView
  secret: string
}
