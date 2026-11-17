// 快速建站向导的纯函数（不依赖界面，便于单元测试）：域名与后端地址的规范化、应用模板、证书匹配与监听的复用或新建。

// SiteTemplate 为常见应用的预设（数据见 siteTemplates.json）：预填后端协议与端口，site 为推荐的站点选项
export interface SiteTemplate {
  id: string
  name: string
  // icon 为导航面板内置图标编号，color 为可选的底色
  icon: string
  color?: string
  scheme: 'http' | 'https'
  port: number
  // https 为是否推荐对外使用 HTTPS
  https: boolean
  site: {
    forwardedHeaders?: boolean
    insecureSkipVerify?: boolean
    clientTimeouts?: { readIdle: number; writeIdle: number }
  }
}

// normalizeDomain 去掉首尾空白、协议、路径与末尾的点，转为小写
export function normalizeDomain(input: string): string {
  let d = input.trim().toLowerCase()
  d = d.replace(/^[a-z][a-z0-9+.-]*:\/\//, '')
  d = d.split(/[/?#]/)[0]
  // 去掉端口（IPv6 字面量不作为站点域名）
  d = d.replace(/:\d+$/, '')
  return d.replace(/\.+$/, '')
}

// domainError 校验域名：返回空为合法，否则为错误类别。允许首段通配（*.example.com）
export function domainError(d: string): '' | 'empty' | 'format' {
  if (!d) {
    return 'empty'
  }
  const labels = d.split('.')
  if (labels.length < 2 && d !== 'localhost') {
    return 'format'
  }
  for (let i = 0; i < labels.length; i++) {
    const l = labels[i]
    if (i === 0 && l === '*') {
      continue
    }
    if (!/^[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?$/.test(l) && !/^xn--[a-z0-9-]+$/.test(l)) {
      return 'format'
    }
  }
  return ''
}

// normalizeTarget 规范化后端地址：没写协议时补 http://（端口为 443 时补 https://），去掉末尾的斜杠
export function normalizeTarget(input: string): string {
  let s = input.trim().replace(/\/+$/, '')
  if (!s) {
    return ''
  }
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(s)) {
    s = (/:443$/.test(s) ? 'https://' : 'http://') + s
  }
  return s
}

// targetError 校验后端地址：只接受 http(s)://主机[:端口]
export function targetError(s: string): '' | 'empty' | 'format' {
  if (!s) {
    return 'empty'
  }
  try {
    const u = new URL(s)
    if ((u.protocol !== 'http:' && u.protocol !== 'https:') || !u.hostname || u.username || u.password) {
      return 'format'
    }
  } catch {
    return 'format'
  }
  return ''
}

// templateTarget 按模板生成后端地址：host 为用户填写的主机（可带端口），没有端口时用模板端口
export function templateTarget(t: SiteTemplate, host: string): string {
  let h = host.trim().replace(/^[a-z][a-z0-9+.-]*:\/\//i, '').replace(/\/.*$/, '')
  if (!h) {
    h = '127.0.0.1'
  }
  const hasPort = /^\[.*\]:\d+$/.test(h) || (/^[^:]+:\d+$/.test(h) && !h.startsWith('['))
  return `${t.scheme}://${hasPort ? h : `${h}:${t.port}`}`
}

// hostOfTarget 取后端地址里的主机部分（切换模板时保留用户已填的主机）
export function hostOfTarget(s: string): string {
  try {
    return new URL(normalizeTarget(s)).hostname
  } catch {
    return ''
  }
}

// certCovers 判断证书域名是否覆盖 host：精确匹配，或 *.example.com 覆盖 example.com 下一级的任意名称
export function certCovers(certDomains: string[], host: string): boolean {
  const h = host.toLowerCase()
  return certDomains.some((raw) => {
    const d = raw.trim().toLowerCase()
    if (d === h) {
      return true
    }
    if (d.startsWith('*.')) {
      const rest = d.slice(2)
      const dot = h.indexOf('.')
      return dot > 0 && h.slice(dot + 1) === rest && h.slice(0, dot) !== '*'
    }
    return false
  })
}

// CertLike 为证书列表接口中判断覆盖所需的字段：已签发的看证书实际域名，尚未签发的看申请的域名
export interface CertLike {
  key: string
  remark?: string
  enable: boolean
  acme?: { domains: string[] | null } | null
  info?: { domains: string[]; notAfter?: string } | null
}

// findCert 返回启用且覆盖 host 的证书，已签发的优先
export function findCert(certs: CertLike[], host: string): CertLike | null {
  const on = certs.filter((c) => c.enable)
  return (
    on.find((c) => c.info?.domains?.length && certCovers(c.info.domains, host)) ??
    on.find((c) => certCovers(c.acme?.domains ?? [], host)) ??
    null
  )
}

// ListenerBrief 为选择监听所需的字段
export interface ListenerBrief {
  key: string
  name: string
  enable: boolean
  tls: boolean
  port: number
  listenIp?: string
  httpPort?: number
}

// listenerChoices 返回可复用的监听：协议一致（HTTPS 或 HTTP），已启用的排前，标准端口（443 / 80）排最前
export function listenerChoices(listeners: ListenerBrief[], tls: boolean): ListenerBrief[] {
  const std = tls ? 443 : 80
  return listeners
    .filter((l) => l.tls === tls)
    .sort((a, b) => Number(b.enable) - Number(a.enable) || Number(b.port === std) - Number(a.port === std))
}

// portTaken 判断端口是否已被某个监听（含其额外 HTTP 端口）占用
export function portTaken(listeners: ListenerBrief[], port: number): boolean {
  return listeners.some((l) => l.port === port || (!!l.httpPort && l.httpPort === port))
}

// NewListenerPlan 为新建监听的参数
export interface NewListenerPlan {
  name: string
  port: number
  tls: boolean
  // httpPort 为 HTTPS 监听同时开启的明文端口（跳转到 HTTPS），0 为不开启
  httpPort: number
}

// planListener 规划新建的监听：HTTPS 默认 443 并在 80 上跳转到 HTTPS（80 已被其他监听占用时不开）；HTTP 默认 80
export function planListener(listeners: ListenerBrief[], tls: boolean, port?: number): NewListenerPlan {
  const p = port || (tls ? 443 : 80)
  const httpPort = tls && p !== 80 && !portTaken(listeners, 80) ? 80 : 0
  return { name: `${tls ? 'HTTPS' : 'HTTP'} ${p}`, port: p, tls, httpPort }
}

// listenerPayload 生成新建监听的请求体（未列出的字段由后端取默认值）
export function listenerPayload(plan: NewListenerPlan, sites: object[]): Record<string, unknown> {
  return {
    name: plan.name,
    enable: true,
    network: 'tcp',
    listenIp: '',
    port: plan.port,
    tls: plan.tls,
    minTls: '1.2',
    httpPort: plan.httpPort,
    httpPortMode: plan.httpPort ? 'redirect' : '',
    sites,
  }
}

// sitePayload 生成站点的请求体：反向代理到 target，带模板的推荐选项与用户勾选的跳过证书校验
export function sitePayload(domain: string, target: string, tpl: SiteTemplate | null, insecureSkipVerify: boolean): Record<string, unknown> {
  const s: Record<string, unknown> = {
    name: tpl ? tpl.name : domain,
    enable: true,
    domains: [domain],
    type: 'proxy',
    targets: [target],
    forwardedHeaders: tpl?.site.forwardedHeaders ?? true,
    insecureSkipVerify: insecureSkipVerify && target.startsWith('https://'),
    keepPath: true,
    redirectCode: 302,
    statusCode: 200,
  }
  if (tpl?.site.clientTimeouts) {
    s.clientTimeouts = { ...tpl.site.clientTimeouts }
  }
  return s
}

// siteURL 返回站点的访问地址：标准端口省略
export function siteURL(domain: string, tls: boolean, port: number): string {
  const std = tls ? 443 : 80
  const host = domain.startsWith('*.') ? `www.${domain.slice(2)}` : domain
  return `${tls ? 'https' : 'http'}://${host}${port === std ? '' : `:${port}`}`
}
