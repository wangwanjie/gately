// Web 服务拓扑视图的数据类型与纯函数：布局、链路高亮、IP 筛选与打码。
// 组件 WebTopology.vue 只负责渲染与交互，这里的函数可在 node --test 中直接测试。

export interface Rate {
  qps: number
  bps: number
  qps60: number
}

export interface RatePoint {
  t: number
  requests: number
  bytes: number
}

export interface TopoUpstream {
  id: string
  url: string
  scope?: string
  match?: string
  weight: number
  backup?: boolean
  healthy?: boolean
  conns: number
  requests: number
  bytes: number
  rate: Rate
}

export interface TopoSite {
  id: string
  key: string
  name: string
  group?: string
  type: string
  domains: string[]
  default?: boolean
  enable: boolean
  running: boolean
  target?: string
  guards: string[]
  requests: number
  active: number
  errors: number
  closed: number
  rate: Rate
  status: Record<string, number>
  blocked: Record<string, number>
  upstreams: TopoUpstream[]
}

export interface TopoAction {
  id: string
  kind: 'redirect' | 'notFound'
  requests: number
  rate: Rate
}

export interface TopoListener {
  id: string
  key: string
  name: string
  enable: boolean
  running: boolean
  error?: string
  network: string
  listenIp: string
  port: number
  tls: boolean
  http3: boolean
  plainHttp: string
  proxyProtocol: boolean
  requests: number
  active: number
  rate: Rate
  sites: TopoSite[]
  actions: TopoAction[]
}

export interface TopoIP {
  ip: string
  nodes: string[]
  // active 为进行中请求总数，activeAt 为按站点、后端节点分的进行中请求数
  active: number
  activeAt?: Record<string, number>
  last: number
}

export interface TopologyView {
  time: number
  window: number
  enabled: boolean
  requests: number
  active: number
  rate: Rate
  listeners: TopoListener[]
  ips: TopoIP[]
  ipTotal: number
  ipTruncated: boolean
  focus?: string
  series?: RatePoint[]
}

// 节点种类：internet 互联网、gately 本实例、listener 监听、site 站点、action 监听级动作、
// upstream 后端、terminal 非代理站点的去向（跳转、静态目录、固定内容、断开连接）
export type NodeKind = 'internet' | 'gately' | 'listener' | 'site' | 'action' | 'upstream' | 'terminal'

export interface LayoutNode {
  id: string
  kind: NodeKind
  col: number
  x: number
  y: number
  w: number
  h: number
  parent?: string
  listener?: TopoListener
  site?: TopoSite
  action?: TopoAction
  upstream?: TopoUpstream
  // collapsed 表示监听已折叠，hidden 为折叠或筛选后未显示的子节点数
  collapsed?: boolean
  hidden?: number
}

export interface LayoutEdge {
  id: string
  from: string
  to: string
  d: string
  // rate 为流经这条连线的速率（取下游节点的速率）
  rate: Rate
  // labelX、labelY 为连线标注的位置（曲线中点）
  labelX: number
  labelY: number
}

export interface Layout {
  width: number
  height: number
  nodes: LayoutNode[]
  edges: LayoutEdge[]
  byId: Map<string, LayoutNode>
  children: Map<string, string[]>
}

// 画布尺寸：列的左边缘与宽度，行高
export const ROW = 60
export const PAD_Y = 16
export const COLS = [
  { x: 18, w: 56 }, // 互联网（圆形）
  { x: 112, w: 64 }, // Gately（圆形）
  { x: 220, w: 188 }, // 监听
  { x: 460, w: 226 }, // 站点 / 监听级动作
  { x: 740, w: 236 }, // 后端 / 去向
]
export const WIDTH = 1000
const NODE_H = 46

// siteMatches 判断站点是否匹配搜索词（名称、域名、分组、后端地址）。
export function siteMatches(s: TopoSite, q: string): boolean {
  if (!q) {
    return true
  }
  const k = q.toLowerCase()
  if ((s.name || '').toLowerCase().includes(k) || (s.group || '').toLowerCase().includes(k)) {
    return true
  }
  if (s.domains.some((d) => d.toLowerCase().includes(k))) {
    return true
  }
  return s.upstreams.some((u) => u.url.toLowerCase().includes(k))
}

// terminalId 返回非代理站点的去向节点编号。
export function terminalId(siteId: string): string {
  return 't:' + siteId.slice(2)
}

// curve 生成从 (x1,y1) 到 (x2,y2) 的水平三次贝塞尔曲线。
function curve(x1: number, y1: number, x2: number, y2: number): string {
  const dx = Math.max(24, (x2 - x1) * 0.5)
  return `M${x1},${y1} C${x1 + dx},${y1} ${x2 - dx},${y2} ${x2},${y2}`
}

const zero: Rate = { qps: 0, bps: 0, qps60: 0 }

// buildLayout 按“互联网 → Gately → 监听 → 站点 / 动作 → 后端 / 去向”的树形结构排版：
// 每个叶子占一行，父节点位于子节点的垂直中点。collapsed 为折叠的监听 key，query 为站点搜索词。
export function buildLayout(v: TopologyView, collapsed: Set<string>, query: string): Layout {
  const nodes: LayoutNode[] = []
  const edges: LayoutEdge[] = []
  const byId = new Map<string, LayoutNode>()
  const children = new Map<string, string[]>()
  let row = 0
  const rowY = (r: number) => PAD_Y + r * ROW + ROW / 2
  const add = (n: LayoutNode) => {
    nodes.push(n)
    byId.set(n.id, n)
    if (n.parent) {
      const list = children.get(n.parent) ?? []
      list.push(n.id)
      children.set(n.parent, list)
    }
    return n
  }
  const card = (col: number, y: number) => ({ col, x: COLS[col].x, w: COLS[col].w, y: y - NODE_H / 2, h: NODE_H })

  const listenerNodes: LayoutNode[] = []
  for (const l of v.listeners) {
    const sites = l.sites.filter((s) => siteMatches(s, query))
    // 搜索时隐藏没有匹配站点的监听（监听名称或端口匹配时仍显示）
    const selfMatch = !query || (l.name || '').toLowerCase().includes(query.toLowerCase()) || String(l.port) === query
    if (query && !sites.length && !selfMatch) {
      continue
    }
    const shown = selfMatch && query && !sites.length ? l.sites : sites
    const isCollapsed = collapsed.has(l.key)
    const startRow = row
    const childYs: number[] = []
    if (!isCollapsed) {
      for (const s of shown) {
        const leaves = s.type === 'proxy' ? Math.max(1, s.upstreams.length) : 1
        const first = row
        row += leaves
        const sy = (rowY(first) + rowY(row - 1)) / 2
        childYs.push(sy)
        add({ id: s.id, kind: 'site', ...card(3, sy), parent: l.id, site: s })
        if (s.type === 'proxy') {
          s.upstreams.forEach((u, i) => {
            add({ id: u.id, kind: 'upstream', ...card(4, rowY(first + i)), parent: s.id, upstream: u, site: s })
          })
        } else {
          add({ id: terminalId(s.id), kind: 'terminal', ...card(4, rowY(first)), parent: s.id, site: s })
        }
      }
      // 搜索时只在监听本身匹配时列出监听级动作（它们没有域名可匹配）
      for (const a of query && !selfMatch ? [] : l.actions) {
        const ay = rowY(row)
        row++
        childYs.push(ay)
        add({ id: a.id, kind: 'action', ...card(3, ay), parent: l.id, action: a })
      }
    }
    if (row === startRow) {
      row++
    }
    const ly = childYs.length ? (childYs[0] + childYs[childYs.length - 1]) / 2 : rowY(startRow)
    const hidden = isCollapsed ? l.sites.length + l.actions.length : l.sites.length - shown.length + (query && !selfMatch ? l.actions.length : 0)
    listenerNodes.push(add({ id: l.id, kind: 'listener', ...card(2, ly), parent: 'gately', listener: l, collapsed: isCollapsed, hidden }))
  }
  if (row === 0) {
    row = 1
  }
  const height = PAD_Y * 2 + row * ROW
  const midY = listenerNodes.length ? (listenerNodes[0].y + listenerNodes[listenerNodes.length - 1].y) / 2 + NODE_H / 2 : rowY(0)
  add({ id: 'internet', kind: 'internet', col: 0, x: COLS[0].x, w: COLS[0].w, y: midY - COLS[0].w / 2, h: COLS[0].w })
  add({ id: 'gately', kind: 'gately', col: 1, x: COLS[1].x, w: COLS[1].w, y: midY - COLS[1].w / 2, h: COLS[1].w, parent: 'internet' })

  // 连线：从上游节点右缘中点连到下游节点左缘中点
  for (const n of nodes) {
    if (!n.parent) {
      continue
    }
    const p = byId.get(n.parent)
    if (!p) {
      continue
    }
    const x1 = p.x + p.w
    const y1 = p.y + p.h / 2
    const x2 = n.x
    const y2 = n.y + n.h / 2
    const rate = n.kind === 'gately' ? v.rate : (n.listener?.rate ?? n.action?.rate ?? n.upstream?.rate ?? n.site?.rate ?? zero)
    edges.push({ id: p.id + '>' + n.id, from: p.id, to: n.id, d: curve(x1, y1, x2, y2), rate, labelX: (x1 + x2) / 2, labelY: (y1 + y2) / 2 - 5 })
  }
  return { width: WIDTH, height, nodes, edges, byId, children }
}

// ancestors 返回节点及其全部上游节点。
export function ancestors(layout: Layout, id: string): string[] {
  const out: string[] = []
  let cur: string | undefined = id
  while (cur) {
    out.push(cur)
    cur = layout.byId.get(cur)?.parent
  }
  return out
}

// descendants 返回节点的全部下游节点（不含自身）。
export function descendants(layout: Layout, id: string): string[] {
  const out: string[] = []
  const stack = [...(layout.children.get(id) ?? [])]
  while (stack.length) {
    const c = stack.pop()!
    out.push(c)
    stack.push(...(layout.children.get(c) ?? []))
  }
  return out
}

// pathOf 返回选中节点所在的整条链路：上游一直到互联网，下游一直到后端。
// 互联网与 Gately 节点覆盖全部链路，返回 null 表示不需要变暗。
export function pathOf(layout: Layout, id: string): Set<string> | null {
  if (!layout.byId.has(id) || id === 'internet' || id === 'gately') {
    return null
  }
  return new Set([...ancestors(layout, id), ...descendants(layout, id)])
}

// ipPath 返回一个访客地址走过的链路。地址的节点列表包含站点、监听级动作，以及进行中请求所在的后端；
// 站点下没有记录到具体后端时，高亮该站点的全部后端（或去向）。
export function ipPath(layout: Layout, ip: TopoIP | undefined): Set<string> {
  const out = new Set<string>()
  if (!ip) {
    return out
  }
  const ups = new Set(ip.nodes.filter((n) => n.startsWith('u:')))
  for (const n of ip.nodes) {
    if (!layout.byId.has(n)) {
      continue
    }
    for (const a of ancestors(layout, n)) {
      out.add(a)
    }
    if (n.startsWith('s:')) {
      const kids = layout.children.get(n) ?? []
      const known = kids.filter((k) => ups.has(k))
      for (const k of known.length ? known : kids) {
        out.add(k)
      }
    }
  }
  return out
}

// nodeCovers 判断地址的节点列表是否经过节点 id（id 为监听、互联网或 Gately 时按包含关系判断）。
export function nodeCovers(ip: TopoIP, id: string): boolean {
  if (id === 'internet' || id === 'gately') {
    return true
  }
  if (id.startsWith('l:')) {
    const key = id.slice(2)
    return ip.nodes.some((n) => n.startsWith('s:' + key + '/') || n.startsWith('a:' + key + '/'))
  }
  if (id.startsWith('t:')) {
    return ip.nodes.includes('s:' + id.slice(2))
  }
  return ip.nodes.includes(id)
}

// activeAt 返回地址在节点 id 上进行中的请求数：互联网与 Gately 为总数，监听为其下各站点之和，去向节点取所属站点。
export function activeAt(ip: TopoIP, id: string): number {
  if (id === 'internet' || id === 'gately') {
    return ip.active
  }
  const m = ip.activeAt ?? {}
  if (id.startsWith('l:')) {
    const prefix = 's:' + id.slice(2) + '/'
    return Object.entries(m).reduce((n, [k, v]) => (k.startsWith(prefix) ? n + v : n), 0)
  }
  if (id.startsWith('t:')) {
    return m['s:' + id.slice(2)] ?? 0
  }
  return m[id] ?? 0
}

// maskIP 把 IPv4 末段、IPv6 后 64 位（接口标识）替换为 *，用于截图分享时隐藏访客与内网地址。
export function maskIP(ip: string): string {
  if (!ip) {
    return ip
  }
  if (ip.includes(':')) {
    // 展开 :: 后保留前 4 组（/64 前缀）
    const [head, tail] = ip.split('::')
    const h = head ? head.split(':') : []
    let groups = h
    if (tail !== undefined) {
      const tl = tail ? tail.split(':') : []
      groups = [...h, ...Array(Math.max(0, 8 - h.length - tl.length)).fill('0'), ...tl]
    }
    return groups.slice(0, 4).join(':') + ':*'
  }
  const p = ip.split('.')
  if (p.length === 4) {
    return `${p[0]}.${p[1]}.${p[2]}.*`
  }
  return ip
}

const ipv4Re = /^\d{1,3}(\.\d{1,3}){3}$/

// isIPLiteral 判断主机名是否为 IP 地址（IPv6 可带方括号）。
export function isIPLiteral(host: string): boolean {
  const h = host.replace(/^\[|\]$/g, '')
  return ipv4Re.test(h) || (h.includes(':') && /^[0-9a-fA-F:.]+$/.test(h))
}

// maskHost 打码地址中的 IP（主机名不变）。
export function maskHost(host: string): string {
  if (!isIPLiteral(host)) {
    return host
  }
  if (host.startsWith('[')) {
    return '[' + maskIP(host.slice(1, -1)) + ']'
  }
  return maskIP(host)
}

// maskURL 打码后端地址中的 IP 主机（保留协议、端口与路径）。
export function maskURL(raw: string): string {
  const m = /^([a-z][a-z0-9+.-]*:\/\/)(\[[^\]]+\]|[^/:?#]+)(.*)$/i.exec(raw)
  if (!m) {
    return raw
  }
  return m[1] + maskHost(m[2]) + m[3]
}

// formatQps 把每秒请求数格式化为简短字符串。
export function formatQps(n: number): string {
  if (!n) {
    return '0'
  }
  if (n < 0.1) {
    return '<0.1'
  }
  if (n < 10) {
    return n.toFixed(1)
  }
  if (n < 1000) {
    return Math.round(n).toString()
  }
  return (n / 1000).toFixed(n < 10000 ? 1 : 0) + 'k'
}

// formatCount 把累计数格式化为简短字符串。
export function formatCount(n: number): string {
  if (n < 10000) {
    return String(n)
  }
  if (n < 1e6) {
    return (n / 1000).toFixed(n < 1e5 ? 1 : 0) + 'k'
  }
  if (n < 1e9) {
    return (n / 1e6).toFixed(n < 1e7 ? 1 : 0) + 'M'
  }
  return (n / 1e9).toFixed(1) + 'G'
}

// flowLevel 把 QPS 映射为连线动画档位：0 不动，1 慢，2 中，3 快。
export function flowLevel(r: Rate): number {
  const q = Math.max(r.qps, r.qps60)
  if (q <= 0 && r.bps <= 0) {
    return 0
  }
  if (q < 1) {
    return 1
  }
  if (q < 20) {
    return 2
  }
  return 3
}

// edgeWidth 按 QPS 对数增长的连线宽度。
export function edgeWidth(r: Rate): number {
  return 1.4 + Math.min(4, Math.log10(1 + Math.max(r.qps, r.qps60)) * 1.6)
}
