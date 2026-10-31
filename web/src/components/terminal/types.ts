// Web 终端前端共用的类型与数据规整函数，字段与 docs/webterminal-api.md 保持一致。
import { t } from '../../i18n'

export interface ProxyConf {
  enabled: boolean
  type: 'socks5' | 'http'
  host: string
  port: number
  username: string
  password: string
}

export interface SSHConf {
  host: string
  port: number
  username: string
  // credential 表示引用认证预设，凭据由 credentialKey 指向的预设提供
  authType: 'password' | 'key' | 'credential'
  credentialKey: string
  password: string
  privateKey: string
  keyFile: string
  passphrase: string
  hostKey: string
  keepAlive: number
  workDir: string
  timeout: number
  proxy: ProxyConf
}

// Credential 是可被多个 SSH 连接复用的认证预设。
export interface Credential {
  key: string
  name: string
  authType: 'password' | 'key'
  // username 为可选的默认用户名，连接未填写用户名时使用
  username: string
  password: string
  privateKey: string
  keyFile: string
  passphrase: string
  remark: string
  enable: boolean
}

export interface TelnetConf {
  host: string
  port: number
  username: string
  password: string
  timeout: number
  proxy: ProxyConf
}

export interface LocalConf {
  shell: string
  workDir: string
  env: Record<string, string>
}

export type ConnType = 'ssh' | 'telnet' | 'local'

export interface Conn {
  key: string
  name: string
  // group 为连接列表中的分组名称，只影响展示，留空为未分组
  group?: string
  type: ConnType
  remark: string
  enable: boolean
  quickAccessDirs: string[]
  ssh: SSHConf
  telnet: TelnetConf
  local: LocalConf
}

export interface SessionInfo {
  id: string
  connKey: string
  connName: string
  type: string
  remark: string
  createdAt: string | number
  lastActive: string | number
  attached: number
  cols: number
  rows: number
  bytesIn: number
  bytesOut: number
}

export interface SessionStats {
  bytesIn: number
  bytesOut: number
  durationSec: number
  idleSec: number
  attached: number
  cols: number
  rows: number
}

export interface Shortcut {
  name: string
  command: string
  sendEnter: boolean
}

export interface TermSettings {
  allowLocalShell: boolean
  maxSessions: number
  idleTimeout: number
  bufferSize: number
  heartbeatInterval: number
  sessionKeepAlive: number
}

export interface SecurityState {
  agreementAccepted: boolean
  twoFAEnable: boolean
  verified: boolean
}

export type PaneStatus = 'connecting' | 'connected' | 'reconnecting' | 'closed' | 'ended'

// TermTab 是一个终端标签（对应一个持久会话）。
export interface TermTab {
  id: string
  connKey: string
  connName: string
  type: string
  sessionId: string
  title: string
  remark: string
  status: PaneStatus
}

export type SplitMode = 'h' | 'v' | 'grid'

// SplitLayout 是保存到后端的分屏布局，后端原样存取。
export interface SplitLayout {
  version: 1
  mode: SplitMode
  items: { sessionId: string; title: string }[]
  active: string
}

export interface SftpEntry {
  name: string
  path: string
  isDir: boolean
  isLink: boolean
  size: number
  mode: number | string
  modTime: string | number
  owner?: string
}

export interface SftpList {
  path: string
  parent: string
  entries: SftpEntry[] | null
}

export const MASK = '********'

export function emptyCredential(): Credential {
  return { key: '', name: '', authType: 'password', username: '', password: '', privateKey: '', keyFile: '', passphrase: '', remark: '', enable: true }
}

export function emptyProxy(): ProxyConf {
  return { enabled: false, type: 'socks5', host: '', port: 1080, username: '', password: '' }
}

export function emptyConn(): Conn {
  return {
    key: '',
    name: '',
    group: '',
    type: 'ssh',
    remark: '',
    enable: true,
    quickAccessDirs: [],
    ssh: {
      host: '',
      port: 22,
      username: 'root',
      authType: 'password',
      credentialKey: '',
      password: '',
      privateKey: '',
      keyFile: '',
      passphrase: '',
      hostKey: '',
      keepAlive: 60,
      workDir: '',
      timeout: 15,
      proxy: emptyProxy(),
    },
    telnet: { host: '', port: 23, username: '', password: '', timeout: 15, proxy: emptyProxy() },
    local: { shell: '', workDir: '', env: {} },
  }
}

// normalizeConn 把后端返回的连接补全默认值（后端可能省略空结构或返回 null）。
export function normalizeConn(raw: Partial<Conn> | null | undefined): Conn {
  const base = emptyConn()
  const r = (raw ?? {}) as Partial<Conn>
  return {
    ...base,
    ...r,
    quickAccessDirs: r.quickAccessDirs ? [...r.quickAccessDirs] : [],
    ssh: { ...base.ssh, ...(r.ssh ?? {}), proxy: { ...base.ssh.proxy, ...(r.ssh?.proxy ?? {}) } },
    telnet: { ...base.telnet, ...(r.telnet ?? {}), proxy: { ...base.telnet.proxy, ...(r.telnet?.proxy ?? {}) } },
    local: { ...base.local, ...(r.local ?? {}), env: { ...(r.local?.env ?? {}) } },
  }
}

// connAddr 返回连接的简短地址描述，用于列表展示。
export function connAddr(c: Conn): string {
  if (c.type === 'local') {
    return c.local.shell ? t('terminal.type.localShellAddr', { shell: c.local.shell }) : t('terminal.type.localShell')
  }
  if (c.type === 'telnet') {
    return `${c.telnet.username ? c.telnet.username + '@' : ''}${c.telnet.host}:${c.telnet.port}`
  }
  // 使用认证预设时用户名可能留空，由预设提供
  return `${c.ssh.username ? c.ssh.username + '@' : ''}${c.ssh.host}:${c.ssh.port}`
}

// local 使用 getter，在读取时才翻译，切换语言后即时生效。
export const typeLabel: Record<string, string> = {
  ssh: 'SSH',
  telnet: 'Telnet',
  get local() {
    return t('terminal.type.local')
  },
}

// fmtTime 把后端的时间（ISO 字符串或 Unix 秒/毫秒）格式化为本地时间文本。
export function fmtTime(v: string | number | undefined | null): string {
  if (v === undefined || v === null || v === '' || v === 0) {
    return '-'
  }
  let d: Date
  if (typeof v === 'number') {
    d = new Date(v < 1e12 ? v * 1000 : v)
  } else {
    d = new Date(v)
  }
  if (isNaN(d.getTime())) {
    return String(v)
  }
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}
