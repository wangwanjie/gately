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
