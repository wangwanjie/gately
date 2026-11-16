// 监听地址相关的纯函数（不依赖界面，便于单元测试）

// listensBeyondLocal 判断 “主机:端口” 形式的监听地址是否会对本机以外开放：
// 主机为空（如 “:1080”，监听全部地址）、0.0.0.0、:: 或其他非回环地址时返回 true；
// 整个值为空表示使用程序默认值（默认只监听本机），返回 false。
export function listensBeyondLocal(addr: string): boolean {
  const v = addr.trim()
  if (!v) {
    return false
  }
  let host = v
  if (v.startsWith('[')) {
    // [IPv6]:端口
    const end = v.indexOf(']')
    host = end > 0 ? v.slice(1, end) : v
  } else if ((v.match(/:/g) || []).length === 1) {
    host = v.slice(0, v.indexOf(':'))
  }
  host = host.toLowerCase()
  if (host === 'localhost' || host === '::1' || /^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host)) {
    return false
  }
  return true
}

// forwardsBeyondLocal 判断多行端口转发规则（协议://绑定地址:端口/目标地址:端口）中是否有绑定到本机以外的地址：
// 转发入口没有认证，绑定到非回环地址时同网络里的设备都能经它进入组网。空行与 # 注释行忽略。
export function forwardsBeyondLocal(text: string): boolean {
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (!line || line.startsWith('#')) {
      continue
    }
    const rest = line.includes('://') ? line.slice(line.indexOf('://') + 3) : line
    const bind = rest.split('/')[0]
    if (listensBeyondLocal(bind)) {
      return true
    }
  }
  return false
}

// maskProxyPassword 把代理地址 “协议://用户名:密码@主机:端口” 中的密码换成 ***，没有密码时原样返回。
// 代理地址不带路径，按最后一个 @ 划分用户信息，密码里含未编码的 @、/、: 时也能整体隐藏
export function maskProxyPassword(p: string): string {
  const m = /^([a-z0-9+.-]+:\/\/)(.*)$/i.exec(p)
  if (!m) {
    return p
  }
  const rest = m[2]
  const at = rest.lastIndexOf('@')
  if (at < 0) {
    return p
  }
  const colon = rest.indexOf(':')
  if (colon < 0 || colon > at) {
    return p
  }
  return `${m[1]}${rest.slice(0, colon)}:***${rest.slice(at)}`
}

// TargetCheck 是转发目标地址的检查结果：
// split 为地址里写了端口且目标端口为空（保存时端口会被拆到目标端口）；conflict 为地址里的端口与已填的目标端口不同；
// mixed 为多个地址写了不同端口；invalid 为写法无法识别（带 http:// 或路径、多冒号又不是 IPv6、端口无效）。
export interface TargetCheck {
  kind: '' | 'split' | 'conflict' | 'mixed' | 'invalid'
  // addr 为触发提示的那一行地址
  addr: string
  // port 为地址里写的端口
  port: number
  // hosts 为去掉端口后的主机列表（kind 为 split 时用于一键拆分）
  hosts: string[]
}

// splitTarget 解析单个目标地址，返回主机与端口（没写端口时为 0），无法识别时返回 null。与后端 l4proxy.SplitTargets 规则一致。
export function splitTarget(raw: string): { host: string; port: number } | null {
  const t = raw.trim()
  if (t.includes('/')) {
    return null
  }
  const portOf = (p: string) => (/^\d+$/.test(p) && +p >= 1 && +p <= 65535 ? +p : -1)
  if (t.startsWith('[')) {
    if (t.endsWith(']')) {
      return { host: t.slice(1, -1), port: 0 }
    }
    const m = /^\[([^\]]+)\]:([^:]*)$/.exec(t)
    if (!m) {
      return null
    }
    const port = portOf(m[2])
    return port < 0 ? null : { host: m[1], port }
  }
  const colons = (t.match(/:/g) || []).length
  if (colons === 0) {
    return { host: t, port: 0 }
  }
  if (colons === 1) {
    const i = t.indexOf(':')
    const port = portOf(t.slice(i + 1))
    if (i === 0 || port < 0) {
      return null
    }
    return { host: t.slice(0, i), port }
  }
  // 多个冒号只能是不带方括号的 IPv6 地址（可带 %网卡）
  return isIPv6(t) ? { host: t, port: 0 } : null
}

// isIPv6 粗略判断不带方括号的 IPv6 地址（允许 :: 缩写、内嵌 IPv4 与 %网卡后缀）。
function isIPv6(s: string): boolean {
  const addr = s.split('%')[0]
  if (!/^[0-9a-fA-F:.]+$/.test(addr) || (addr.match(/::/g) || []).length > 1) {
    return false
  }
  const groups = addr.split(':')
  if (groups.length < 3 || groups.length > 8) {
    return false
  }
  return groups.every((g, i) => g === '' || /^[0-9a-fA-F]{1,4}$/.test(g) || (i === groups.length - 1 && /^\d{1,3}(\.\d{1,3}){3}$/.test(g)))
}

// checkTargets 检查多行目标地址与目标端口（字符串，如 "8080"、"8080-8090"，空为未填），用于表单即时提示。
export function checkTargets(lines: string[], targetPorts: string): TargetCheck {
  const hosts: string[] = []
  let port = 0
  let addr = ''
  for (const raw of lines) {
    const line = raw.trim()
    if (!line) {
      continue
    }
    const r = splitTarget(line)
    if (!r) {
      return { kind: 'invalid', addr: line, port: 0, hosts: [] }
    }
    if (r.port) {
      if (port && port !== r.port) {
        return { kind: 'mixed', addr: line, port: r.port, hosts: [] }
      }
      port = r.port
      addr = addr || line
    }
    hosts.push(r.host)
  }
  if (!port) {
    return { kind: '', addr: '', port: 0, hosts }
  }
  const spec = targetPorts.trim()
  if (!spec) {
    return { kind: 'split', addr, port, hosts }
  }
  if (/^\d+$/.test(spec) && +spec === port) {
    return { kind: '', addr: '', port, hosts }
  }
  return { kind: 'conflict', addr, port, hosts }
}
