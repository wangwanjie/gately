// 端口转发与动态域名空状态的场景模板：点一下预填新建表单（不依赖界面，便于单元测试）。
// 名称与说明在各页面命名空间的 templates.<id> 下。

// PortForwardTemplate 预填端口转发规则：目标地址留空由用户填写设备 IP
export interface PortForwardTemplate {
  id: 'ssh' | 'rdp' | 'web'
  protocols: string[]
  listenPorts: string
  targetPorts: string
}

export const PORTFORWARD_TEMPLATES: PortForwardTemplate[] = [
  // 转发 NAS 的 SSH：对外用非标准端口，减少扫描
  { id: 'ssh', protocols: ['tcp'], listenPorts: '2222', targetPorts: '22' },
  // Windows 远程桌面（RDP 同时使用 TCP 与 UDP）
  { id: 'rdp', protocols: ['tcp', 'udp'], listenPorts: '3389', targetPorts: '3389' },
  // 内网设备的网页管理界面
  { id: 'web', protocols: ['tcp'], listenPorts: '8080', targetPorts: '80' },
]

// DDNSTemplate 预填动态域名任务：服务商与要更新的记录类型
export interface DDNSTemplate {
  id: 'cloudflare4' | 'alidns46' | 'dnspod6'
  provider: string
  ipv4: boolean
  ipv6: boolean
  // types 为预先添加的解析记录类型（A 对应 IPv4，AAAA 对应 IPv6）
  types: ('A' | 'AAAA')[]
}

export const DDNS_TEMPLATES: DDNSTemplate[] = [
  // Cloudflare，家宽公网 IPv4
  { id: 'cloudflare4', provider: 'cloudflare', ipv4: true, ipv6: false, types: ['A'] },
  // 阿里云，IPv4 与 IPv6 双栈
  { id: 'alidns46', provider: 'alidns', ipv4: true, ipv6: true, types: ['A', 'AAAA'] },
  // DNSPod，仅 IPv6（没有公网 IPv4 的家宽）
  { id: 'dnspod6', provider: 'dnspod', ipv4: false, ipv6: true, types: ['AAAA'] },
]
