import { hierarchy, pack, treemap, treemapSquarify, type HierarchyNode } from 'd3-hierarchy'

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
  // accessLog 表示站点开启了访问日志
  accessLog?: boolean
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
  // 额外的明文 HTTP 端口与处理方式，未开启时省略
  httpPort?: number
  httpPortMode?: string
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
  // seriesStep 为曲线每个点覆盖的秒数，只在回放快照中出现（实时快照每秒一个点）
  seriesStep?: number
  // replay 只在回放快照中出现：这一帧在录制数据中的位置
  replay?: ReplayMeta
}

// ReplayMeta 为回放帧的位置：全部数据与所在段的起止时刻、上一个与下一个采样时刻（没有时为 0）
export interface ReplayMeta {
  start: number
  end: number
  frameStart: number
  frameEnd: number
  prev: number
  next: number
  step: number
}

// 节点种类：internet 互联网、gately 本实例、listener 监听、site 站点、action 监听级动作、
// upstream 后端、terminal 非代理站点的去向（跳转、静态目录、固定内容、断开连接）、
// more 子节点过多时收起其余子节点的汇总节点（展开后作为“收起”入口）
export type NodeKind = 'internet' | 'gately' | 'listener' | 'site' | 'action' | 'upstream' | 'terminal' | 'more'

// 布局方式：tree 从左到右的树形，vertical 从上到下的竖向树，radial 以 Gately 为圆心的径向，
// sankey 按流量分配连线粗细的桑基图，sunburst 同心环形分区的旭日图，
// treemap 嵌套矩形的矩形树图，pack 嵌套圆形的气泡图
export type LayoutMode = 'tree' | 'vertical' | 'radial' | 'sankey' | 'sunburst' | 'treemap' | 'pack'
export const LAYOUT_MODES: LayoutMode[] = ['tree', 'vertical', 'radial', 'sankey', 'sunburst', 'treemap', 'pack']

// isColumnar 判断布局是否按列从左到右排列（默认适应宽度、从顶部开始显示）
export function isColumnar(mode: LayoutMode): boolean {
  return mode === 'tree' || mode === 'sankey'
}

// MoreInfo 描述汇总节点：owner 为被收起子节点的父节点，ids 为收起的直接子节点，
// expanded 表示已展开全部（此时节点用于收起），rate 与 active 为被收起子节点的合计
export interface MoreInfo {
  owner: string
  ids: string[]
  count: number
  expanded: boolean
  // childKind 为被收起子节点的层级：site 为站点，upstream 为后端
  childKind: 'site' | 'upstream'
  rate: Rate
  active: number
}

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
  more?: MoreInfo
  // collapsed 表示监听已折叠，hidden 为折叠或筛选后未显示的子节点数
  collapsed?: boolean
  hidden?: number
  // 径向布局：angle 为节点所在方向（弧度，0 指向右侧、顺时针增大），leaf 表示没有子节点（文字朝外）
  angle?: number
  leaf?: boolean
  // 旭日图：shape 为环形扇区的路径（绝对坐标），label 为文字中心、方向与可用长度（扇区太窄时 show 为 false）
  shape?: string
  label?: { x: number; y: number; angle: number; max: number; show: boolean }
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
  // 桑基图：band 表示 d 为填充的流量带，flow 为带中线（流动动画沿它播放）
  band?: boolean
  flow?: string
}

export interface Layout {
  mode: LayoutMode
  width: number
  height: number
  nodes: LayoutNode[]
  edges: LayoutEdge[]
  byId: Map<string, LayoutNode>
  children: Map<string, string[]>
  // alias 把被收起的节点编号映射到代替它显示的汇总节点，用于 IP 链路高亮与活动 IP 计数
  alias: Map<string, string>
}

export interface LayoutOptions {
  mode?: LayoutMode
  // expanded 为已展开全部子节点的父节点编号
  expanded?: Set<string>
  // limit 为每个监听最多显示的站点数，upstreamLimit 为每个站点最多显示的后端数
  limit?: number
  upstreamLimit?: number
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

// 默认每个监听显示 8 个站点、每个站点显示 6 个后端，其余收进汇总节点，避免单个分支把画布拉得很高
export const SITE_LIMIT = 8
export const UPSTREAM_LIMIT = 6

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

// TopoFilter 为拓扑筛选条件：listeners 只显示这些监听（key，为空时显示全部），
// issues 只看运行异常（监听启动失败、站点未运行、后端不健康），blocked 只看出现过这些原因拦截的站点（自启动以来计数），
// active 只看活动中的站点、监听级动作与后端（见 siteActive）。
export interface TopoFilter {
  listeners?: string[]
  issues?: boolean
  blocked?: string[]
  active?: boolean
}

// rateActive 判断速率是否表示近期有流量：60 秒均值覆盖“最近 60 秒有过请求”，与活动 IP 的时间窗口一致，
// 只看 10 秒均值时偶发请求的服务会在画面上反复出现、消失
function rateActive(r: Rate): boolean {
  return r.qps > 0 || r.qps60 > 0 || r.bps > 0
}

// seenNodes 返回活动 IP 经过的节点编号（站点、监听级动作与进行中请求所在的后端）。
// 地址列表可能被截断，只作为速率之外的补充：低频请求的 60 秒均值可能被四舍五入为 0
export function seenNodes(v: TopologyView): Set<string> {
  const out = new Set<string>()
  for (const ip of v.ips) {
    for (const n of ip.nodes) {
      out.add(n)
    }
  }
  return out
}

// siteActive 判断站点是否活动中：最近 60 秒有请求、有进行中的请求，或有活动 IP 经过。
export function siteActive(s: TopoSite, seen?: Set<string>): boolean {
  return s.active > 0 || rateActive(s.rate) || !!seen?.has(s.id)
}

// upstreamActive 判断后端是否活动中：最近 60 秒有转发、有进行中的连接，或有进行中的请求经过。
export function upstreamActive(u: TopoUpstream, seen?: Set<string>): boolean {
  return u.conns > 0 || rateActive(u.rate) || !!seen?.has(u.id)
}

// actionActive 判断监听级动作（跳转 HTTPS、未匹配站点）最近 60 秒是否处理过请求。
export function actionActive(a: TopoAction, seen?: Set<string>): boolean {
  return rateActive(a.rate) || !!seen?.has(a.id)
}

// countActive 统计视图中活动中的站点数（用于“聚焦活动”按钮上的计数），query 为站点搜索词。
export function countActive(v: TopologyView, query = ''): number {
  const seen = seenNodes(v)
  let n = 0
  for (const l of v.listeners) {
    for (const s of l.sites) {
      if (siteMatches(s, query) && siteActive(s, seen)) {
        n++
      }
    }
  }
  return n
}

// BLOCK_REASONS 为站点拦截计数的原因，与后端 SiteStats.blocked 的键一致
export const BLOCK_REASONS = ['ban', 'ip', 'country', 'rateLimit', 'waf']

// filterActive 判断筛选条件是否生效（用于按钮高亮与空结果提示）。
export function filterActive(f: TopoFilter): boolean {
  return !!(f.listeners?.length || f.issues || f.blocked?.length || f.active)
}

// listenerFailed 判断监听是否启动失败（已启用但未运行或有错误）。
export function listenerFailed(l: TopoListener): boolean {
  return l.enable && (!l.running || !!l.error)
}

// siteIssue 判断站点是否运行异常：已启用却未运行，或有后端被判定为不健康。
export function siteIssue(s: TopoSite): boolean {
  if (!s.enable) {
    return false
  }
  return !s.running || s.upstreams.some((u) => u.healthy === false)
}

// filterView 按筛选条件裁剪拓扑数据，返回新的视图（不修改原数据），再交给 buildLayout 排版。
// 各条件同时生效：先按监听筛选；只看异常时，启动失败的监听保留全部站点，其余监听只保留异常站点；
// 按拦截原因筛选时只保留出现过所选原因拦截的站点。站点被筛掉的监听连同监听级动作一起隐藏。
// 只看活动时只保留活动中的站点与监听级动作，代理站点只保留活动中的后端（都不活动时站点作为末端显示），
// 没有活动的监听隐藏；与其他条件同时生效。
export function filterView(v: TopologyView, f: TopoFilter): TopologyView {
  if (!filterActive(f)) {
    return v
  }
  const out = filterStatic(v, f)
  if (!f.active) {
    return out
  }
  const seen = seenNodes(v)
  const listeners: TopoListener[] = []
  for (const l of out.listeners) {
    const sites = l.sites.filter((s) => siteActive(s, seen)).map((s) => (s.type === 'proxy' ? { ...s, upstreams: s.upstreams.filter((u) => upstreamActive(u, seen)) } : s))
    const actions = l.actions.filter((a) => actionActive(a, seen))
    if (!sites.length && !actions.length) {
      continue
    }
    listeners.push({ ...l, sites, actions })
  }
  return { ...out, listeners }
}

// filterStatic 按监听、运行异常与拦截原因筛选（filterView 的前半部分）。
function filterStatic(v: TopologyView, f: TopoFilter): TopologyView {
  if (!(f.listeners?.length || f.issues || f.blocked?.length)) {
    return v
  }
  const only = f.listeners?.length ? new Set(f.listeners) : undefined
  const reasons = f.blocked ?? []
  const listeners: TopoListener[] = []
  for (const l of v.listeners) {
    if (only && !only.has(l.key)) {
      continue
    }
    const failed = listenerFailed(l)
    let sites = l.sites
    if (f.issues && !failed) {
      // 停用的监听下站点都不运行，属于预期状态，不算异常
      sites = l.enable ? sites.filter(siteIssue) : []
    }
    if (reasons.length) {
      sites = sites.filter((s) => reasons.some((r) => (s.blocked?.[r] ?? 0) > 0))
    }
    // 只按监听筛选时原样保留；其余条件下没有剩余站点的监听隐藏（启动失败的监听在只看异常且未按原因筛选时保留）
    const narrowed = f.issues || reasons.length > 0
    if (narrowed && !sites.length && !(f.issues && failed && !reasons.length)) {
      continue
    }
    listeners.push(narrowed ? { ...l, sites, actions: [] } : l)
  }
  return { ...v, listeners }
}

// terminalId 返回非代理站点的去向节点编号。
export function terminalId(siteId: string): string {
  return 't:' + siteId.slice(2)
}

// moreId 返回父节点下汇总节点的编号。
export function moreId(owner: string): string {
  return 'm:' + owner
}

// curve 生成从 (x1,y1) 到 (x2,y2) 的水平三次贝塞尔曲线。
function curve(x1: number, y1: number, x2: number, y2: number): string {
  const dx = Math.max(24, (x2 - x1) * 0.5)
  return `M${x1},${y1} C${x1 + dx},${y1} ${x2 - dx},${y2} ${x2},${y2}`
}

const zero: Rate = { qps: 0, bps: 0, qps60: 0 }

// TNode 为排版前的抽象树节点：先按搜索、折叠与数量上限决定显示哪些节点，再交给具体布局计算坐标
type TNode = Omit<LayoutNode, 'col' | 'x' | 'y' | 'w' | 'h'> & { kids: TNode[] }

// nodeRate 返回流经节点的速率（去向节点取所属站点的）。
function nodeRate(n: Omit<LayoutNode, 'col' | 'x' | 'y' | 'w' | 'h'>): Rate {
  return n.listener?.rate ?? n.action?.rate ?? n.upstream?.rate ?? n.more?.rate ?? n.site?.rate ?? zero
}

// buildTree 按搜索词、折叠状态与数量上限生成要显示的监听子树，alias 收集被收起的节点。
function buildTree(v: TopologyView, collapsed: Set<string>, query: string, opts: LayoutOptions, alias: Map<string, string>): TNode[] {
  const expanded = opts.expanded ?? new Set<string>()
  const limit = opts.limit ?? SITE_LIMIT
  const upLimit = opts.upstreamLimit ?? UPSTREAM_LIMIT

  // aliasAll 把被收起的子树全部映射到汇总节点
  const aliasAll = (t: TNode, to: string) => {
    alias.set(t.id, to)
    for (const k of t.kids) {
      aliasAll(k, to)
    }
  }
  const seen = seenNodes(v)
  // busy 判断站点或后端是否活动中，收起多余节点时活动中的始终显示
  const busy = (t: TNode) => (t.kind === 'upstream' ? upstreamActive(t.upstream!, seen) : t.kind === 'site' ? siteActive(t.site!, seen) : false)
  // cap 在子节点超过上限时只保留前 limit 个，其余空闲的收进汇总节点（活动中的仍按原顺序显示，不必展开查找）；
  // 只多出一个时直接显示，避免出现“还有 1 个”
  const cap = (owner: string, list: TNode[], max: number, childKind: MoreInfo['childKind']): TNode[] => {
    if (list.length <= max + 1) {
      return list
    }
    const rest = list.slice(max).filter((t) => !busy(t))
    if (rest.length <= 1) {
      return list
    }
    const sum = rest.reduce((a, t) => {
      const r = nodeRate(t)
      return { qps: a.qps + r.qps, bps: a.bps + r.bps, qps60: a.qps60 + r.qps60 }
    }, zero)
    const active = rest.reduce((a, t) => a + (t.kind === 'upstream' ? (t.upstream?.conns ?? 0) : (t.site?.active ?? 0)), 0)
    const isOpen = expanded.has(owner)
    const m: TNode = {
      id: moreId(owner),
      kind: 'more',
      parent: owner,
      more: { owner, ids: rest.map((t) => t.id), count: rest.length, expanded: isOpen, childKind, rate: sum, active },
      kids: [],
    }
    if (isOpen) {
      return [...list, m]
    }
    const hidden = new Set(rest)
    for (const t of rest) {
      aliasAll(t, m.id)
    }
    return [...list.filter((t) => !hidden.has(t)), m]
  }

  const roots: TNode[] = []
  for (const l of v.listeners) {
    const sites = l.sites.filter((s) => siteMatches(s, query))
    // 搜索时隐藏没有匹配站点的监听（监听名称或端口匹配时仍显示）
    const selfMatch = !query || (l.name || '').toLowerCase().includes(query.toLowerCase()) || String(l.port) === query
    if (query && !sites.length && !selfMatch) {
      continue
    }
    const shown = selfMatch && query && !sites.length ? l.sites : sites
    const isCollapsed = collapsed.has(l.key)
    let kids: TNode[] = []
    if (!isCollapsed) {
      for (const s of shown) {
        let leaves: TNode[]
        if (s.type === 'proxy') {
          leaves = s.upstreams.map((u) => ({ id: u.id, kind: 'upstream' as const, parent: s.id, upstream: u, site: s, kids: [] }))
          leaves = cap(s.id, leaves, upLimit, 'upstream')
        } else {
          leaves = [{ id: terminalId(s.id), kind: 'terminal', parent: s.id, site: s, kids: [] }]
        }
        kids.push({ id: s.id, kind: 'site', parent: l.id, site: s, kids: leaves })
      }
      // 只收起站点；监听级动作最多两个，始终显示在汇总节点之后
      kids = cap(l.id, kids, limit, 'site')
      // 搜索时只在监听本身匹配时列出监听级动作（它们没有域名可匹配）
      for (const a of query && !selfMatch ? [] : l.actions) {
        kids.push({ id: a.id, kind: 'action', parent: l.id, action: a, kids: [] })
      }
    }
    const hidden = isCollapsed ? l.sites.length + l.actions.length : l.sites.length - shown.length + (query && !selfMatch ? l.actions.length : 0)
    roots.push({ id: l.id, kind: 'listener', parent: 'gately', listener: l, collapsed: isCollapsed, hidden, kids })
  }
  return roots
}

// buildLayout 生成拓扑布局：collapsed 为折叠的监听 key，query 为站点搜索词，opts 选择布局方式与展开状态。
// 树形布局按“互联网 → Gately → 监听 → 站点 / 动作 → 后端 / 去向”从左到右排列，每个叶子占一行，父节点位于子节点的垂直中点；
// 径向布局以 Gately 为圆心，监听、站点、后端依次分布在三个同心圆上，叶子均匀占据角度。
export function buildLayout(v: TopologyView, collapsed: Set<string>, query: string, opts: LayoutOptions = {}): Layout {
  const alias = new Map<string, string>()
  const roots = buildTree(v, collapsed, query, opts, alias)
  const out: Layout = { mode: opts.mode ?? 'tree', width: WIDTH, height: 0, nodes: [], edges: [], byId: new Map(), children: new Map(), alias }
  if (out.mode === 'radial') {
    layoutRadial(v, roots, out)
  } else if (out.mode === 'sankey') {
    layoutSankey(v, roots, out)
  } else if (out.mode === 'sunburst') {
    layoutSunburst(roots, out)
  } else if (out.mode === 'vertical') {
    layoutVertical(v, roots, out)
  } else if (out.mode === 'treemap' || out.mode === 'pack') {
    layoutNested(roots, out)
  } else {
    layoutTree(v, roots, out)
  }
  return out
}

// addNode 把节点加入布局并登记父子关系。
function addNode(out: Layout, n: LayoutNode): LayoutNode {
  out.nodes.push(n)
  out.byId.set(n.id, n)
  if (n.parent) {
    const list = out.children.get(n.parent) ?? []
    list.push(n.id)
    out.children.set(n.parent, list)
  }
  return n
}

// strip 去掉抽象树节点的子节点列表，得到布局节点的基础字段。
function strip(t: TNode): Omit<LayoutNode, 'col' | 'x' | 'y' | 'w' | 'h'> {
  const { kids: _kids, ...base } = t
  return base
}

function layoutTree(v: TopologyView, roots: TNode[], out: Layout) {
  let row = 0
  const rowY = (r: number) => PAD_Y + r * ROW + ROW / 2
  const card = (col: number, y: number) => ({ col, x: COLS[col].x, w: COLS[col].w, y: y - NODE_H / 2, h: NODE_H })
  // place 先排子节点再排自身，返回节点中心的纵坐标
  const place = (t: TNode, depth: number): number => {
    let y: number
    if (t.kids.length) {
      const ys = t.kids.map((k) => place(k, depth + 1))
      y = (ys[0] + ys[ys.length - 1]) / 2
    } else {
      y = rowY(row)
      row++
    }
    addNode(out, { ...strip(t), ...card(2 + depth, y), leaf: !t.kids.length })
    return y
  }
  const listenerYs = roots.map((t) => place(t, 0))
  const height = PAD_Y * 2 + Math.max(1, row) * ROW
  const midY = listenerYs.length ? (listenerYs[0] + listenerYs[listenerYs.length - 1]) / 2 : rowY(0)
  addNode(out, { id: 'internet', kind: 'internet', col: 0, x: COLS[0].x, w: COLS[0].w, y: midY - COLS[0].w / 2, h: COLS[0].w })
  addNode(out, { id: 'gately', kind: 'gately', col: 1, x: COLS[1].x, w: COLS[1].w, y: midY - COLS[1].w / 2, h: COLS[1].w, parent: 'internet' })
  out.width = WIDTH
  out.height = height

  // 连线：从上游节点右缘中点连到下游节点左缘中点
  for (const n of out.nodes) {
    const p = n.parent ? out.byId.get(n.parent) : undefined
    if (!p) {
      continue
    }
    const x1 = p.x + p.w
    const y1 = p.y + p.h / 2
    const x2 = n.x
    const y2 = n.y + n.h / 2
    const rate = n.kind === 'gately' ? v.rate : nodeRate(n)
    out.edges.push({ id: p.id + '>' + n.id, from: p.id, to: n.id, d: curve(x1, y1, x2, y2), rate, labelX: (x1 + x2) / 2, labelY: (y1 + y2) / 2 - 5 })
  }
}

// 径向布局参数：三层圆环的基础半径、外圈文字预留宽度、留给互联网节点的扇区（弧度）
export const RADIAL_RINGS = [180, 350, 500]
export const RADIAL_LABEL = 210
const RADIAL_GAP = 1.1
// 外圈每个叶子至少占的弧长（px），叶子很多时整体放大圆环
const RADIAL_MIN_ARC = 22
// 相邻监听之间额外留出的角度（以叶子数计），便于区分分组
const RADIAL_GROUP_GAP = 0.6

function layoutRadial(v: TopologyView, roots: TNode[], out: Layout) {
  const leafCount = (t: TNode): number => (t.kids.length ? t.kids.reduce((n, k) => n + leafCount(k), 0) : 1)
  const units = roots.reduce((n, t) => n + leafCount(t), 0) + Math.max(0, roots.length - 1) * RADIAL_GROUP_GAP
  const span = Math.PI * 2 - RADIAL_GAP
  const k = Math.max(1, (RADIAL_MIN_ARC * units) / span / RADIAL_RINGS[2])
  // 放大时内圈放大得少一些，避免圆心附近过于空旷
  const rings = [RADIAL_RINGS[0] * Math.min(k, 1.6), RADIAL_RINGS[1] * k, RADIAL_RINGS[2] * k]
  const half = Math.round(rings[2] + RADIAL_LABEL)
  const c = half
  const step = units ? span / units : 0
  // 叶子从互联网扇区的下边缘开始顺时针排列，互联网扇区在左侧（角度 π）
  const start = Math.PI + RADIAL_GAP / 2
  let pos = 0
  const place = (t: TNode, depth: number): number => {
    let a: number
    if (t.kids.length) {
      const as = t.kids.map((kid) => place(kid, depth + 1))
      a = (as[0] + as[as.length - 1]) / 2
    } else {
      a = start + (pos + 0.5) * step
      pos++
    }
    const r = rings[depth]
    const nr = t.kind === 'listener' ? 15 : t.kind === 'more' ? 11 : 12
    const x = c + r * Math.cos(a)
    const y = c + r * Math.sin(a)
    addNode(out, { ...strip(t), col: 2 + depth, x: x - nr, y: y - nr, w: nr * 2, h: nr * 2, angle: a, leaf: !t.kids.length })
    return a
  }
  roots.forEach((t, i) => {
    if (i > 0) {
      pos += RADIAL_GROUP_GAP
    }
    place(t, 0)
  })
  const gr = 34
  const ir = 24
  const ia = Math.PI
  const id = rings[0] * 0.56
  addNode(out, { id: 'internet', kind: 'internet', col: 0, x: c + id * Math.cos(ia) - ir, y: c + id * Math.sin(ia) - ir, w: ir * 2, h: ir * 2, angle: ia })
  addNode(out, { id: 'gately', kind: 'gately', col: 1, x: c - gr, y: c - gr, w: gr * 2, h: gr * 2, parent: 'internet' })
  out.width = half * 2
  out.height = half * 2

  // 连线：在极坐标中插值的三次曲线，控制点位于两层半径的中间，分别沿父、子节点方向
  const center = (n: LayoutNode) => ({ x: n.x + n.w / 2, y: n.y + n.h / 2 })
  for (const n of out.nodes) {
    const p = n.parent ? out.byId.get(n.parent) : undefined
    if (!p) {
      continue
    }
    const a = center(p)
    const b = center(n)
    const rate = n.kind === 'gately' ? v.rate : nodeRate(n)
    if (n.kind === 'gately') {
      out.edges.push({ id: p.id + '>' + n.id, from: p.id, to: n.id, d: `M${a.x},${a.y} L${b.x},${b.y}`, rate, labelX: (a.x + b.x) / 2, labelY: (a.y + b.y) / 2 - 8 })
      continue
    }
    const rp = Math.hypot(a.x - c, a.y - c)
    const rc = Math.hypot(b.x - c, b.y - c)
    const ap = p.kind === 'gately' ? n.angle! : p.angle!
    const ac = n.angle!
    const rm = (rp + rc) / 2
    const c1x = c + rm * Math.cos(ap)
    const c1y = c + rm * Math.sin(ap)
    const c2x = c + rm * Math.cos(ac)
    const c2y = c + rm * Math.sin(ac)
    // 三次贝塞尔曲线 t=0.5 处的点作为标注位置
    const lx = (a.x + 3 * c1x + 3 * c2x + b.x) / 8
    const ly = (a.y + 3 * c1y + 3 * c2y + b.y) / 8
    out.edges.push({ id: p.id + '>' + n.id, from: p.id, to: n.id, d: `M${a.x},${a.y} C${c1x},${c1y} ${c2x},${c2y} ${b.x},${b.y}`, rate, labelX: lx, labelY: ly - 4 })
  }
}

// 桑基图参数：各列竖条的左边缘、竖条宽度、文字预留宽度与行高
export const SANKEY_COLS = [20, 190, 360, 580, 820]
export const SANKEY_BAR = 12
export const SANKEY_LABEL = 230
export const SANKEY_ROW = 46
const SANKEY_MIN = 6

// layoutSankey 按行排版（与树形相同，刷新时位置稳定），竖条高度与连线带宽度随 QPS 变化：
// 父节点的竖条按子节点 QPS 的占比切分，每段连到对应子节点的竖条。
function layoutSankey(v: TopologyView, roots: TNode[], out: Layout) {
  interface P {
    t: TNode
    depth: number
    cy: number
    span: number
    kids: P[]
  }
  let row = 0
  const place = (t: TNode, depth: number): P => {
    if (t.kids.length) {
      const kids = t.kids.map((k) => place(k, depth + 1))
      const first = kids[0]
      const last = kids[kids.length - 1]
      const top = first.cy - first.span / 2
      const bottom = last.cy + last.span / 2
      return { t, depth, cy: (first.cy + last.cy) / 2, span: bottom - top, kids }
    }
    const cy = PAD_Y + row * SANKEY_ROW + SANKEY_ROW / 2
    row++
    return { t, depth, cy, span: SANKEY_ROW, kids: [] }
  }
  const ps = roots.map((t) => place(t, 0))
  const rows = Math.max(1, row)
  const spanAll = rows * SANKEY_ROW
  const total = Math.max(0, v.rate.qps)
  // 全部流量占总高度的 70%，单个节点不超过自己所占的行
  const k = total > 0 ? (spanAll * 0.7) / total : 0
  const barH = (qps: number, span: number) => Math.min(Math.max(SANKEY_MIN, span - 10), Math.max(SANKEY_MIN, k * qps))
  const addBar = (p: P) => {
    const h = barH(nodeRate(p.t).qps, p.span)
    addNode(out, { ...strip(p.t), col: 2 + p.depth, x: SANKEY_COLS[2 + p.depth], y: p.cy - h / 2, w: SANKEY_BAR, h, leaf: !p.kids.length })
    p.kids.forEach(addBar)
  }
  ps.forEach(addBar)
  const midY = ps.length ? (ps[0].cy + ps[ps.length - 1].cy) / 2 : PAD_Y + SANKEY_ROW / 2
  const hubH = barH(total, spanAll)
  addNode(out, { id: 'internet', kind: 'internet', col: 0, x: SANKEY_COLS[0], y: midY - hubH / 2, w: SANKEY_BAR, h: hubH })
  addNode(out, { id: 'gately', kind: 'gately', col: 1, x: SANKEY_COLS[1], y: midY - hubH / 2, w: SANKEY_BAR, h: hubH, parent: 'internet' })
  out.width = SANKEY_COLS[4] + SANKEY_BAR + SANKEY_LABEL
  out.height = PAD_Y * 2 + spanAll

  // 连线带：父节点竖条按子节点 QPS 占比切分（空闲的子节点也留一小段），连到子节点整根竖条
  for (const [pid, kids] of out.children) {
    const p = out.byId.get(pid)
    if (!p) {
      continue
    }
    const nodes = kids.map((id) => out.byId.get(id)!).filter(Boolean)
    const qps = nodes.map((n) => Math.max(0, n.kind === 'gately' ? v.rate.qps : nodeRate(n).qps))
    const sum = qps.reduce((a, b) => a + b, 0)
    const eps = sum > 0 ? sum * 0.03 : 1
    const weights = qps.map((q) => q + eps)
    const wsum = weights.reduce((a, b) => a + b, 0)
    let off = p.y
    nodes.forEach((n, i) => {
      const th = (p.h * weights[i]) / wsum
      const x1 = p.x + p.w
      const x2 = n.x
      const xm = (x1 + x2) / 2
      const py0 = off
      const py1 = off + th
      off = py1
      const cy0 = n.y
      const cy1 = n.y + n.h
      const d = `M${x1},${py0} C${xm},${py0} ${xm},${cy0} ${x2},${cy0} L${x2},${cy1} C${xm},${cy1} ${xm},${py1} ${x1},${py1} Z`
      const a = (py0 + py1) / 2
      const b = (cy0 + cy1) / 2
      const flow = `M${x1},${a} C${xm},${a} ${xm},${b} ${x2},${b}`
      const rate = n.kind === 'gately' ? v.rate : nodeRate(n)
      out.edges.push({ id: p.id + '>' + n.id, from: p.id, to: n.id, d, flow, band: true, rate, labelX: xm, labelY: (a + b) / 2 - 5 })
    })
  }
}

// 旭日图参数：三层环的内外半径（监听、站点、后端），圆心 Gately 的半径
export const SUN_RINGS: [number, number][] = [
  [64, 150],
  [154, 300],
  [304, 440],
]
export const SUN_HUB = 50
// 外环每个叶子至少占的弧长（px），叶子很多时整体放大
const SUN_MIN_ARC = 10
const SUN_GROUP_GAP = 0.3

// arcPath 生成圆心 (c,c)、半径 r0~r1、角度 a0~a1 的环形扇区路径。
function arcPath(c: number, r0: number, r1: number, a0: number, a1: number): string {
  // 整圈时留一条细缝，避免起止点重合导致圆弧消失
  const end = Math.min(a1, a0 + Math.PI * 2 - 1e-4)
  const large = end - a0 > Math.PI ? 1 : 0
  const pt = (r: number, a: number) => `${(c + r * Math.cos(a)).toFixed(2)},${(c + r * Math.sin(a)).toFixed(2)}`
  return `M${pt(r1, a0)} A${r1},${r1} 0 ${large} 1 ${pt(r1, end)} L${pt(r0, end)} A${r0},${r0} 0 ${large} 0 ${pt(r0, a0)} Z`
}

// layoutSunburst 以 Gately 为圆心，监听、站点、后端依次占据三层环，叶子平分角度，父节点覆盖子节点的角度范围。
// 旭日图没有连线，层级关系由相邻的环表示；没有互联网节点。
function layoutSunburst(roots: TNode[], out: Layout) {
  const leafCount = (t: TNode): number => (t.kids.length ? t.kids.reduce((n, k) => n + leafCount(k), 0) : 1)
  const units = roots.reduce((n, t) => n + leafCount(t), 0) + Math.max(0, roots.length - 1) * SUN_GROUP_GAP
  const span = Math.PI * 2
  const outerMid = (SUN_RINGS[2][0] + SUN_RINGS[2][1]) / 2
  const k = Math.max(1, (SUN_MIN_ARC * units) / span / outerMid)
  const rings = SUN_RINGS.map(([a, b], i) => (i === 0 ? [a, b] : [a * k, b * k]) as [number, number])
  const half = Math.round(rings[2][1] + 16)
  const c = half
  const step = units ? span / units : 0
  // 从正上方开始顺时针排列
  const start = -Math.PI / 2
  let pos = 0
  const place = (t: TNode, depth: number): [number, number] => {
    let a0: number
    let a1: number
    if (t.kids.length) {
      const as = t.kids.map((kid) => place(kid, depth + 1))
      a0 = as[0][0]
      a1 = as[as.length - 1][1]
    } else {
      a0 = start + pos * step
      a1 = a0 + step
      pos++
    }
    const [r0, r1] = rings[depth]
    // 扇区之间留约 1.5px 的缝
    const pad = Math.min((a1 - a0) / 4, 0.75 / r1)
    const am = (a0 + a1) / 2
    const rm = (r0 + r1) / 2
    const arcLen = (a1 - a0) * rm
    addNode(out, {
      ...strip(t),
      col: 2 + depth,
      x: 0,
      y: 0,
      w: 0,
      h: 0,
      angle: am,
      leaf: !t.kids.length,
      shape: arcPath(c, r0, r1, a0 + pad, a1 - pad),
      label: { x: c + rm * Math.cos(am), y: c + rm * Math.sin(am), angle: am, max: r1 - r0 - 14, show: arcLen >= 13 },
    })
    return [a0, a1]
  }
  roots.forEach((t, i) => {
    if (i > 0) {
      pos += SUN_GROUP_GAP
    }
    place(t, 0)
  })
  addNode(out, { id: 'gately', kind: 'gately', col: 1, x: c - SUN_HUB, y: c - SUN_HUB, w: SUN_HUB * 2, h: SUN_HUB * 2 })
  out.width = half * 2
  out.height = half * 2
}

// 竖向树参数：卡片宽高、相邻叶子的间距、各层卡片顶部的纵坐标（互联网、Gately、监听、站点、后端）
export const VERT_CARD = 168
const VERT_SLOT = 182
const VERT_PAD = 16
export const VERT_LEVELS = [8, 118, 240, 340, 440]

// layoutVertical 按“互联网 → Gately → 监听 → 站点 → 后端”从上到下排列：每个叶子占一列，父节点位于子节点的水平中点。
// 互联网与 Gately 的文字放在圆形右侧，避免压在竖直连线上。
function layoutVertical(v: TopologyView, roots: TNode[], out: Layout) {
  let col = 0
  const colX = (c: number) => VERT_PAD + c * VERT_SLOT + VERT_SLOT / 2
  const place = (t: TNode, depth: number): number => {
    let x: number
    if (t.kids.length) {
      const xs = t.kids.map((k) => place(k, depth + 1))
      x = (xs[0] + xs[xs.length - 1]) / 2
    } else {
      x = colX(col)
      col++
    }
    addNode(out, { ...strip(t), col: 2 + depth, x: x - VERT_CARD / 2, y: VERT_LEVELS[2 + depth], w: VERT_CARD, h: NODE_H, leaf: !t.kids.length })
    return x
  }
  const xs = roots.map((t) => place(t, 0))
  const cols = Math.max(1, col)
  const midX = xs.length ? (xs[0] + xs[xs.length - 1]) / 2 : colX(0)
  addNode(out, { id: 'internet', kind: 'internet', col: 0, x: midX - COLS[0].w / 2, y: VERT_LEVELS[0], w: COLS[0].w, h: COLS[0].w })
  addNode(out, { id: 'gately', kind: 'gately', col: 1, x: midX - COLS[1].w / 2, y: VERT_LEVELS[1], w: COLS[1].w, h: COLS[1].w, parent: 'internet' })
  // 宽度至少容纳互联网、Gately 右侧的文字
  out.width = Math.max(VERT_PAD * 2 + cols * VERT_SLOT, midX + 240)
  out.height = VERT_LEVELS[4] + NODE_H + VERT_PAD

  // 连线：从上游节点底边中点连到下游节点顶边中点
  for (const n of out.nodes) {
    const p = n.parent ? out.byId.get(n.parent) : undefined
    if (!p) {
      continue
    }
    const x1 = p.x + p.w / 2
    const y1 = p.y + p.h
    const x2 = n.x + n.w / 2
    const y2 = n.y
    const dy = Math.max(16, (y2 - y1) * 0.5)
    const rate = n.kind === 'gately' ? v.rate : nodeRate(n)
    out.edges.push({ id: p.id + '>' + n.id, from: p.id, to: n.id, d: `M${x1},${y1} C${x1},${y1 + dy} ${x2},${y2 - dy} ${x2},${y2}`, rate, labelX: (x1 + x2) / 2 + 4, labelY: (y1 + y2) / 2 })
  }
}

// 矩形树图与气泡图的尺寸：每个叶子大约占的面积（px²），以及父节点顶部留给名称的高度
const NEST_LEAF_AREA = 15000
export const NEST_HEAD = [24, 22, 20]

// layoutNested 生成矩形树图（treemap）或气泡图（pack）：Gately 为最外层容器，监听、站点、后端逐层嵌套；
// 每个叶子面积相同，刷新时位置稳定，流量大小用颜色深浅表示。没有连线与互联网节点。
function layoutNested(roots: TNode[], out: Layout) {
  const root: TNode = { id: 'gately', kind: 'gately', kids: roots }
  const h = hierarchy<TNode>(root, (d) => d.kids).sum((d) => (d.kids.length ? 0 : 1))
  const leaves = Math.max(1, h.value ?? 0)
  const add = (d: HierarchyNode<TNode>, x: number, y: number, w: number, hh: number) => {
    const t = d.data
    if (t.id === 'gately') {
      addNode(out, { id: 'gately', kind: 'gately', col: 1, x, y, w, h: hh })
      return
    }
    addNode(out, { ...strip(t), col: 1 + d.depth, x, y, w, h: hh, leaf: !t.kids.length })
  }
  if (out.mode === 'treemap') {
    // 宽高比约 1.6，叶子很少时保留最小尺寸
    const area = leaves * NEST_LEAF_AREA
    const width = Math.max(900, Math.round(Math.sqrt(area * 1.6)))
    const height = Math.max(420, Math.round(area / width) + 120)
    const tm = treemap<TNode>()
      .size([width, height])
      .tile(treemapSquarify.ratio(1.4))
      // paddingOuter 会同时设置四边，必须在 paddingTop 之前调用
      .paddingOuter(4)
      .paddingTop((d) => (d.children ? NEST_HEAD[Math.min(d.depth, 2)] : 0))
      .paddingInner(3)
      .round(true)
    const r = tm(h)
    // descendants 按先父后子的顺序，子节点画在父节点之上
    for (const d of r.descendants()) {
      add(d, d.x0, d.y0, d.x1 - d.x0, d.y1 - d.y0)
    }
    out.width = width
    out.height = height
    return
  }
  const size = Math.max(640, Math.round(Math.sqrt(leaves) * 110))
  const pk = pack<TNode>()
    .size([size, size])
    .padding((d) => (d.depth === 0 ? 14 : 6))
  const r = pk(h)
  for (const d of r.descendants()) {
    add(d, d.x - d.r, d.y - d.r, d.r * 2, d.r * 2)
  }
  out.width = size
  out.height = size
}

// resolveId 返回节点在布局中实际显示的编号：节点被收起时返回代替它的汇总节点，找不到时返回空串。
export function resolveId(layout: Layout, id: string): string {
  let cur = id
  // 收起可能嵌套（后端所在的站点又被收起），沿映射找到实际显示的节点
  for (let i = 0; i < 4; i++) {
    if (layout.byId.has(cur)) {
      return cur
    }
    const next = layout.alias.get(cur)
    if (!next) {
      return ''
    }
    cur = next
  }
  return layout.byId.has(cur) ? cur : ''
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
// 站点下没有记录到具体后端时，高亮该站点的全部后端（或去向）。被收起的节点由代替它的汇总节点高亮。
export function ipPath(layout: Layout, ip: TopoIP | undefined): Set<string> {
  const out = new Set<string>()
  if (!ip) {
    return out
  }
  const ups = new Set(ip.nodes.filter((n) => n.startsWith('u:')).map((n) => resolveId(layout, n)))
  for (const raw of ip.nodes) {
    const n = resolveId(layout, raw)
    if (!n) {
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
// 汇总节点（m:）需传入它收起的子节点 more，任一子节点经过即算经过。
export function nodeCovers(ip: TopoIP, id: string, more?: string[]): boolean {
  if (id === 'internet' || id === 'gately') {
    return true
  }
  if (id.startsWith('m:')) {
    return (more ?? []).some((c) => nodeCovers(ip, c))
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

// activeAt 返回地址在节点 id 上进行中的请求数：互联网与 Gately 为总数，监听为其下各站点之和，去向节点取所属站点，
// 汇总节点为它收起的子节点之和。
export function activeAt(ip: TopoIP, id: string, more?: string[]): number {
  if (id === 'internet' || id === 'gately') {
    return ip.active
  }
  if (id.startsWith('m:')) {
    return (more ?? []).reduce((n, c) => n + activeAt(ip, c), 0)
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
  // 空值与通配地址（监听全部地址）不含任何信息，原样显示
  if (!ip || ip === '0.0.0.0' || ip === '::') {
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

// isValidIP 严格校验 IP 地址（IPv4 每段 0–255，IPv6 交给 URL 解析），用于判断筛选框输入是否已完整。
export function isValidIP(s: string): boolean {
  if (ipv4Re.test(s)) {
    return s.split('.').every((p) => Number(p) <= 255)
  }
  if (!s.includes(':') || !/^[0-9a-fA-F:.]+$/.test(s)) {
    return false
  }
  try {
    new URL(`http://[${s}]/`)
    return true
  } catch {
    return false
  }
}

// maskText 打码自由文本（如监听的错误信息 listen tcp 192.168.1.5:443: bind: ...）中出现的 IP 地址。
export function maskText(text: string): string {
  return text
    .replace(/\[([0-9a-fA-F:.]+)\]/g, (m, ip: string) => (ip.includes(':') ? '[' + maskIP(ip) + ']' : m))
    .replace(/(?<![\w.:])\d{1,3}(?:\.\d{1,3}){3}(?![\w.])/g, (ip) => maskIP(ip))
    .replace(/(?<![\w:[])(?:[0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}(?![\w:\]])/g, (ip) => (isValidIP(ip) ? maskIP(ip) : ip))
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

// maskDomain 打码域名：保留第一段与顶级域，中间替换为 ***（如 nas.home.example.com → nas.***.com），
// 只有两段时保留首字母（example.com → e***.com）；通配前缀 *. 原样保留，首段通配（api-*.example.com）按普通首段处理，
// 单段名称（如 localhost）不变。
export function maskDomain(host: string): string {
  if (!host || isIPLiteral(host)) {
    return host
  }
  const wild = host.startsWith('*.')
  const labels = (wild ? host.slice(2) : host).split('.')
  if (labels.length < 2) {
    return host
  }
  const tld = labels[labels.length - 1]
  const masked = labels.length === 2 ? labels[0].slice(0, 1) + '***.' + tld : labels[0] + '.***.' + tld
  return (wild ? '*.' : '') + masked
}

// domainInText 匹配自由文本中的域名。首段可以是 *（*.example.com），也可以是含一个 * 的首段通配（api-*.example.com），
// 站点名称默认取首个域名，漏掉首段通配会让打码后的画面仍露出完整域名。String.replace 每次从头匹配，共用同一个 /g 正则是安全的
const domainInText = /(?<![\w.*-])(?:[a-z0-9_-]*\*[a-z0-9_-]*\.)?(?:[a-z0-9_-]+\.)+[a-z][a-z0-9-]*[a-z](?![\w.-])/gi

// maskDomainText 打码自由文本（如站点名称）中出现的域名。
export function maskDomainText(text: string): string {
  return text.replace(domainInText, (d) => maskDomain(d))
}

// maskURL 打码后端地址中的 IP 主机（保留协议、端口与路径）；names 为 true 时主机名同样按域名打码。
export function maskURL(raw: string, names = false): string {
  const m = /^([a-z][a-z0-9+.-]*:\/\/)(\[[^\]]+\]|[^/:?#]+)(.*)$/i.exec(raw)
  if (!m) {
    return raw
  }
  const host = isIPLiteral(m[2]) ? maskHost(m[2]) : names ? maskDomain(m[2]) : m[2]
  return m[1] + host + m[3]
}

// 打码方式：stars 以 * 遮住地址末段与域名中段；alias 把整个值换成本页内一致的代号（如 Visitor-01、domain-02.com），
// 不保留任何网段或域名片段，不同的值编号不同，画面上仍能分辨哪些节点相同。
export type MaskStyle = 'stars' | 'alias'

// Aliases 为代号映射表：同一类别内同一原始值始终得到同一个编号，按首次出现的顺序递增。
// 映射只保存在页面内存中，刷新页面后重新编号。
export class Aliases {
  private maps = new Map<string, Map<string, string>>()

  // token 返回 prefix 类别下 raw 的代号，如 Visitor-01；编号至少两位
  token(prefix: string, raw: string): string {
    let m = this.maps.get(prefix)
    if (!m) {
      m = new Map()
      this.maps.set(prefix, m)
    }
    let v = m.get(raw)
    if (!v) {
      v = prefix + '-' + String(m.size + 1).padStart(2, '0')
      m.set(raw, v)
    }
    return v
  }
}

// 常见的二级公共后缀：这些后缀下的可注册域名多一段（如 example.com.cn）。不引入完整的公共后缀列表，覆盖常见情况即可
const SECOND_LEVEL = new Set(['com', 'net', 'org', 'gov', 'edu', 'ac', 'co', 'ne', 'or', 'go'])

// splitDomain 把域名拆成主机部分、可注册部分与公共后缀，如 a.b.example.com.cn → ['a.b', 'example', 'com.cn']。
function splitDomain(host: string): [string, string, string] | undefined {
  const labels = host.split('.')
  if (labels.length < 2) {
    return undefined
  }
  let n = 1
  if (labels.length >= 3 && labels[labels.length - 1].length === 2 && SECOND_LEVEL.has(labels[labels.length - 2])) {
    n = 2
  }
  const suffix = labels.slice(-n).join('.')
  const reg = labels[labels.length - n - 1]
  const sub = labels.slice(0, labels.length - n - 1).join('.')
  return [sub, reg, suffix]
}

// aliasIP 把 IP 换成 prefix 类别的代号；通配地址（监听全部地址）不含信息，原样显示。
export function aliasIP(a: Aliases, ip: string, prefix: string): string {
  if (!ip || ip === '0.0.0.0' || ip === '::') {
    return ip
  }
  return a.token(prefix, ip.toLowerCase())
}

// aliasHost 换掉地址中的 IP（IPv6 保留方括号），主机名不变。
export function aliasHost(a: Aliases, host: string, prefix: string): string {
  if (!isIPLiteral(host)) {
    return host
  }
  if (host.startsWith('[')) {
    return '[' + aliasIP(a, host.slice(1, -1), prefix) + ']'
  }
  return aliasIP(a, host, prefix)
}

// aliasDomain 把域名换成代号，只保留公共后缀与通配前缀：可注册部分按 domain-NN 编号，
// 其上的主机部分按 host-NN 编号（同一可注册域名下的子域名共享 domain 编号），如 nas.home.example.com → host-01.domain-01.com。
// 单段名称（如 localhost、nas）整体换成 host-NN，IP 原样返回由调用方按地址类别处理。
export function aliasDomain(a: Aliases, host: string): string {
  if (!host || isIPLiteral(host) || host === '*') {
    return host
  }
  const wild = host.startsWith('*.')
  const h = (wild ? host.slice(2) : host).toLowerCase()
  const parts = splitDomain(h)
  if (!parts) {
    return (wild ? '*.' : '') + a.token('host', h)
  }
  const [sub, reg, suffix] = parts
  const base = a.token('domain', reg + '.' + suffix) + '.' + suffix
  return (wild ? '*.' : '') + (sub ? a.token('host', h) + '.' : '') + base
}

// aliasDomainText 换掉自由文本（如站点名称）中出现的域名。
export function aliasDomainText(a: Aliases, text: string): string {
  return text.replace(domainInText, (d) => aliasDomain(a, d))
}

// aliasText 换掉自由文本（如监听错误信息）中出现的 IP 地址。
export function aliasText(a: Aliases, text: string, prefix: string): string {
  return text
    .replace(/\[([0-9a-fA-F:.]+)\]/g, (m, ip: string) => (ip.includes(':') ? '[' + aliasIP(a, ip, prefix) + ']' : m))
    .replace(/(?<![\w.:])\d{1,3}(?:\.\d{1,3}){3}(?![\w.])/g, (ip) => aliasIP(a, ip, prefix))
    .replace(/(?<![\w:[])(?:[0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}(?![\w:\]])/g, (ip) => (isValidIP(ip) ? aliasIP(a, ip, prefix) : ip))
}

// aliasURL 把后端或跳转地址的主机换成代号（保留协议、端口与路径）：IP 按 prefix 类别编号，主机名按域名编号。
export function aliasURL(a: Aliases, raw: string, prefix: string): string {
  const m = /^([a-z][a-z0-9+.-]*:\/\/)(\[[^\]]+\]|[^/:?#]+)(.*)$/i.exec(raw)
  if (!m) {
    return raw
  }
  const host = isIPLiteral(m[2]) ? aliasHost(a, m[2], prefix) : aliasDomain(a, m[2])
  return m[1] + host + m[3]
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
