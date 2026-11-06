// 站点访问地址的拼接与校验：把 Web 服务的域名和监听的协议、端口组成浏览器可打开的地址。
// 只认合法的主机名或 IP，通配域名和带有 / @ : 等字符的值返回 null，由调用方按纯文本显示，
// 避免配置里混入的字符被拼成指向别处的链接。

export interface AccessListener {
  port: number
  tls: boolean
  // 对外访问协议，空为按 tls 推算
  accessScheme?: string
  // 对外访问端口，0 或未设置时使用监听端口
  accessPort?: number
}

// 主机名每一段由字母、数字、- 和 _ 组成，首尾不能是 - 或 _（允许 Unicode 国际化域名）
const labelRe = /^[\p{L}\p{N}](?:[\p{L}\p{N}_-]{0,61}[\p{L}\p{N}])?$/u

// hostPart 返回可放进地址的主机部分（IPv6 加方括号），不能作为具体主机时返回 null。
export function hostPart(domain: string): string | null {
  const d = domain.trim().toLowerCase()
  if (!d || d.length > 253 || d.includes('*')) {
    return null
  }
  if (d.includes(':')) {
    // 只有 IPv6 字面量会含冒号，交给 URL 解析校验
    const v6 = d.replace(/^\[|\]$/g, '')
    if (!/^[0-9a-f:.]+$/.test(v6)) {
      return null
    }
    try {
      new URL(`http://[${v6}]/`)
    } catch {
      return null
    }
    return `[${v6}]`
  }
  if (!d.split('.').every((l) => labelRe.test(l))) {
    return null
  }
  return d
}

// accessScheme 返回浏览器访问监听使用的协议。
export function accessScheme(l: AccessListener): 'http' | 'https' {
  if (l.accessScheme === 'http' || l.accessScheme === 'https') {
    return l.accessScheme
  }
  return l.tls ? 'https' : 'http'
}

// accessPort 返回浏览器访问监听使用的端口。
export function accessPort(l: AccessListener): number {
  return l.accessPort && l.accessPort > 0 ? l.accessPort : l.port
}

export interface SiteLink {
  // href 为链接地址，主机名经 URL 规范化（国际化域名转为 punycode）
  href: string
  // text 为复制与显示用的地址，保留原始域名写法
  text: string
}

// siteLink 按监听的访问协议与端口拼出站点地址；协议默认端口（http 80、https 443）省略。
// 域名不是具体主机（通配、格式不对）时返回 null。
export function siteLink(domain: string, l: AccessListener): SiteLink | null {
  const host = hostPart(domain)
  if (!host) {
    return null
  }
  const scheme = accessScheme(l)
  const port = accessPort(l)
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    return null
  }
  const defaultPort = scheme === 'https' ? 443 : 80
  const suffix = port === defaultPort ? '' : `:${port}`
  let u: URL
  try {
    u = new URL(`${scheme}://${host}${suffix}/`)
  } catch {
    return null
  }
  // 解析后主机、路径必须与拼接的完全对应，防止出现用户信息、路径等额外成分
  if (u.username || u.password || u.pathname !== '/' || u.search || u.hash) {
    return null
  }
  return { href: `${u.protocol}//${u.host}`, text: `${scheme}://${host}${suffix}` }
}

// safeHttpUrl 检查地址是否为 http(s) 绝对地址，返回规范化后的地址，否则返回 null。
// 用于渲染外部链接前的兜底校验，挡住 javascript: 等协议。
export function safeHttpUrl(raw: string): string | null {
  try {
    const u = new URL(raw)
    if ((u.protocol === 'http:' || u.protocol === 'https:') && u.hostname) {
      return u.href
    }
  } catch {
    // 不是绝对地址
  }
  return null
}

// ---- 经四层转发进入的监听：自动推算对外访问端口 ----

// ForwardRoute 是协议分流规则里的一条路由（只取推算入口需要的字段）。
export interface ForwardRoute {
  disabled?: boolean
  protocol?: string
  domains?: string[] | null
  alpn?: string[] | null
  targetAddresses?: string[] | null
  // 0 表示与监听端口相同
  targetPort?: number
}

// ForwardRule 是端口转发或协议分流的一条规则；端口转发没有 routes。
export interface ForwardRule {
  name: string
  enable: boolean
  listenPorts: string
  targetAddresses?: string[] | null
  targetPorts?: string
  routes?: ForwardRoute[] | null
}

// ForwardSource 是一个模块的规则列表，kind 用于界面说明入口来自哪个模块。
export interface ForwardSource {
  kind: 'protoroute' | 'portforward'
  rules: ForwardRule[]
}

export interface ForwardEntry {
  kind: ForwardSource['kind']
  rule: string
  port: number
}

// parsePorts 与后端 l4proxy.ParsePorts 相同：支持 80、80,443、8000-8010 及其组合，格式不对返回 null。
export function parsePorts(spec: string): number[] | null {
  const parts = spec.split(/[,，\s]+/).filter(Boolean)
  if (!parts.length) {
    return null
  }
  const out: number[] = []
  for (const part of parts) {
    const m = /^(\d+)(?:-(\d+))?$/.exec(part)
    if (!m) {
      return null
    }
    const lo = Number(m[1])
    const hi = m[2] === undefined ? lo : Number(m[2])
    if (lo < 1 || hi > 65535 || hi < lo || out.length + hi - lo + 1 > 1024) {
      return null
    }
    for (let p = lo; p <= hi; p++) {
      out.push(p)
    }
  }
  return out
}

// mapPorts 与后端 l4proxy.MapPorts 相同：目标端口一一对应、只有一个或留空（与监听端口相同）。
function mapPorts(listenSpec: string, targetSpec = ''): { listen: number; target: number }[] {
  const listen = parsePorts(listenSpec)
  if (!listen) {
    return []
  }
  const target = targetSpec.trim() ? parsePorts(targetSpec) : listen
  if (!target || (target.length !== 1 && target.length !== listen.length)) {
    return []
  }
  return listen.map((p, i) => ({ listen: p, target: target.length === 1 ? target[0] : target[i] }))
}

function bareIp(s: string): string {
  return s.trim().toLowerCase().replace(/^\[|\]$/g, '')
}

function isLoopback(ip: string): boolean {
  return ip === 'localhost' || ip === '::1' || /^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(ip)
}

// targetsListener 判断转发目标地址是否连到这个监听：监听全部地址时目标为回环或未指定地址，
// 监听具体地址时目标必须就是该地址（例如监听 127.0.0.2，转发到 127.0.0.1 连不上）。
// 目标为本机的局域网地址时无法在前端判断，不计入，需要时手动填写访问端口。
function targetsListener(addrs: string[] | null | undefined, listenIp: string): boolean {
  const lip = bareIp(listenIp)
  const any = lip === '' || lip === '0.0.0.0' || lip === '::'
  return (addrs ?? []).some((a) => {
    const t = bareIp(a)
    if (any) {
      return isLoopback(t) || t === '0.0.0.0' || t === '::'
    }
    return t === lip
  })
}

// routeFor 按后端协议分流的匹配顺序（精确域名 → 通配 → * → 不限域名）找到该域名命中的路由；
// 只看与监听协议对应的 TLS（按 SNI）或 HTTP（按 Host）路由，跳过停用路由和不接受浏览器 ALPN 的路由。
// 没有命中返回 null，表示走默认目标。
function routeFor(routes: ForwardRoute[], domain: string, tls: boolean): ForwardRoute | null {
  const proto = tls ? 'tls' : 'http'
  const host = domain.trim().toLowerCase()
  const usable = routes.filter((r) => {
    if (r.disabled || (r.protocol || 'tls') !== proto) {
      return false
    }
    const alpn = r.alpn ?? []
    return !alpn.length || alpn.some((a) => a === 'h2' || a === 'http/1.1')
  })
  const domainsOf = (r: ForwardRoute) => (r.domains ?? []).map((d) => d.toLowerCase())
  const exact = usable.find((r) => domainsOf(r).includes(host))
  if (exact) {
    return exact
  }
  const wild = usable.find((r) => domainsOf(r).some((d) => d.startsWith('*.') && host.endsWith(d.slice(1))))
  if (wild) {
    return wild
  }
  return usable.find((r) => domainsOf(r).includes('*')) ?? usable.find((r) => !domainsOf(r).length) ?? null
}

// forwardedEntry 在已启用的协议分流与端口转发规则里，找到把该域名的连接转到这个监听的入口端口，
// 用于监听只对本机开放、外网经四层转发进入时生成正确的站点链接。按规则顺序取第一个，找不到返回 null。
// domain 为空时只看端口转发与协议分流的默认目标（用于表单示例）。
export function forwardedEntry(
  l: { listenIp?: string; port: number; tls: boolean },
  domain: string,
  sources: ForwardSource[],
): ForwardEntry | null {
  for (const src of sources) {
    for (const r of src.rules) {
      if (!r.enable) {
        continue
      }
      const route = src.kind === 'protoroute' && domain ? routeFor(r.routes ?? [], domain, l.tls) : null
      if (route) {
        if (!targetsListener(route.targetAddresses, l.listenIp ?? '')) {
          continue
        }
        const ports = parsePorts(r.listenPorts) ?? []
        const p = ports.find((lp) => (route.targetPort || lp) === l.port)
        if (p) {
          return { kind: src.kind, rule: r.name, port: p }
        }
        continue
      }
      if (!targetsListener(r.targetAddresses, l.listenIp ?? '')) {
        continue
      }
      const pair = mapPorts(r.listenPorts, r.targetPorts).find((x) => x.target === l.port)
      if (pair) {
        return { kind: src.kind, rule: r.name, port: pair.listen }
      }
    }
  }
  return null
}
