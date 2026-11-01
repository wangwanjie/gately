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
