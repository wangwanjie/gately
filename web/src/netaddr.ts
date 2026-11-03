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
