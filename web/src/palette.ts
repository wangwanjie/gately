// 全局搜索（命令面板）的纯函数：条目生成与匹配排序，不依赖 Vue，便于单元测试。

// 条目类别：页面（含汇总页标签、设置页标签）与各模块的配置数据
export type EntryKind = 'page' | 'site' | 'forward' | 'ddns' | 'cert'

export interface PaletteEntry {
  // 列表里的唯一标识
  id: string
  kind: EntryKind
  // 主文字（页面名、域名、规则名）
  title: string
  // 副文字（所在分组、监听、目标地址等），可为空
  sub: string
  // 跳转目标：路由路径与查询参数
  path: string
  query?: Record<string, string>
  // 额外参与匹配、但不显示的文字（如路由路径、英文 ID）
  keywords?: string
}

// normalize 统一大小写与空白，便于不区分大小写匹配
export function normalize(s: string): string {
  return s.toLowerCase().replace(/\s+/g, ' ').trim()
}

// score 计算条目与查询的匹配分数，越大越靠前；不匹配时返回 0。
// 查询按空格拆成多个词，每个词都要出现在标题、副文字或关键词中；标题开头匹配优先，其次标题内匹配，再次副文字与关键词。
export function score(e: PaletteEntry, query: string): number {
  const words = normalize(query).split(' ').filter(Boolean)
  if (words.length === 0) {
    return 1
  }
  const title = normalize(e.title)
  const rest = normalize(`${e.sub} ${e.keywords ?? ''}`)
  let total = 0
  for (const w of words) {
    if (title.startsWith(w)) {
      total += 30
    } else if (title.includes(w)) {
      total += 20
    } else if (rest.includes(w)) {
      total += 5
    } else {
      return 0
    }
  }
  // 标题越短越接近完全匹配
  return total + Math.max(0, 10 - Math.floor(title.length / 8))
}

// search 返回匹配的条目：页面排在数据之前，同类按分数降序；每类最多 perKind 条
export function search(entries: PaletteEntry[], query: string, perKind = 8): PaletteEntry[] {
  const order: EntryKind[] = ['page', 'site', 'forward', 'ddns', 'cert']
  const scored = entries.map((e) => ({ e, s: score(e, query) })).filter((x) => x.s > 0)
  const out: PaletteEntry[] = []
  for (const kind of order) {
    const list = scored.filter((x) => x.e.kind === kind)
    // 空查询保持原顺序（页面按侧边栏顺序），有查询时按分数排序；sort 是稳定排序，同分保持原顺序
    if (normalize(query)) {
      list.sort((a, b) => b.s - a.s)
    }
    out.push(...list.slice(0, perKind).map((x) => x.e))
  }
  return out
}

// ---------- 各模块列表接口的数据转换为条目 ----------

interface SiteLike {
  key?: string
  name?: string
  domains?: string[] | null
}

interface ListenerLike {
  key: string
  name?: string
  port?: number
  sites?: SiteLike[] | null
}

// siteEntries 每个站点一条：标题为第一个域名（没有域名时用站点名），其余域名参与匹配；
// 跳转到 Web 服务页并打开该站点的编辑框（Web 服务页已支持 editListener / editSite 参数）
export function siteEntries(listeners: ListenerLike[] | null | undefined): PaletteEntry[] {
  const out: PaletteEntry[] = []
  for (const l of listeners ?? []) {
    for (const s of l.sites ?? []) {
      if (!s.key) {
        continue
      }
      const domains = (s.domains ?? []).filter(Boolean)
      const title = domains[0] || s.name || s.key
      const lname = l.name ? `${l.name} :${l.port ?? ''}` : `:${l.port ?? ''}`
      out.push({
        id: `site:${l.key}/${s.key}`,
        kind: 'site',
        title,
        sub: [s.name && s.name !== title ? s.name : '', lname].filter(Boolean).join(' · '),
        path: '/web',
        query: { editListener: l.key, editSite: s.key },
        keywords: domains.slice(1).join(' '),
      })
    }
  }
  return out
}

interface ForwardLike {
  key?: string
  name?: string
  protocols?: string[] | null
  listenPorts?: string
  targetAddresses?: string[] | null
  targetPorts?: string
}

// forwardEntries 每条端口转发规则一条，按规则名、端口与目标地址匹配
export function forwardEntries(rules: ForwardLike[] | null | undefined): PaletteEntry[] {
  return (rules ?? [])
    .filter((r) => r.key)
    .map((r) => {
      const proto = (r.protocols ?? []).join('/').toUpperCase()
      const target = (r.targetAddresses ?? []).join(', ')
      return {
        id: `forward:${r.key}`,
        kind: 'forward' as const,
        title: r.name || `${proto} ${r.listenPorts ?? ''}`.trim(),
        sub: `${proto} ${r.listenPorts ?? ''} → ${target}${r.targetPorts ? ':' + r.targetPorts : ''}`.trim(),
        path: '/portforward',
      }
    })
}

interface DDNSRecordLike {
  key?: string
  zone?: string
  sub?: string
  type?: string
  remark?: string
}

interface DDNSTaskLike {
  key: string
  name?: string
  records?: DDNSRecordLike[] | null
}

// recordName 拼出解析记录的完整域名：主机记录为空或 @ 时就是主域名
export function recordName(r: DDNSRecordLike): string {
  const sub = (r.sub ?? '').trim()
  const zone = (r.zone ?? '').trim()
  if (!sub || sub === '@') {
    return zone
  }
  return zone ? `${sub}.${zone}` : sub
}

// ddnsEntries 每条解析记录一条，副文字为所属任务与记录类型
export function ddnsEntries(tasks: DDNSTaskLike[] | null | undefined): PaletteEntry[] {
  const out: PaletteEntry[] = []
  for (const task of tasks ?? []) {
    ;(task.records ?? []).forEach((r, i) => {
      const name = recordName(r)
      if (!name) {
        return
      }
      out.push({
        id: `ddns:${task.key}/${r.key ?? i}`,
        kind: 'ddns',
        title: name,
        sub: [task.name, r.type, r.remark].filter(Boolean).join(' · '),
        path: '/ddns',
      })
    })
  }
  return out
}

interface CertLike {
  key: string
  remark?: string
  acme?: { domains?: string[] | null } | null
  info?: { domains?: string[] | null } | null
}

// certEntries 每张证书一条：域名取 ACME 配置，上传的证书取证书内容；跳到证书页并按第一个域名筛选
export function certEntries(certs: CertLike[] | null | undefined): PaletteEntry[] {
  return (certs ?? [])
    .map((c) => {
      const domains = [...new Set([...(c.acme?.domains ?? []), ...(c.info?.domains ?? [])].filter(Boolean))]
      const title = domains[0] || c.remark || c.key
      return {
        id: `cert:${c.key}`,
        kind: 'cert' as const,
        title,
        sub: [c.remark && c.remark !== title ? c.remark : '', domains.length > 1 ? `+${domains.length - 1}` : ''].filter(Boolean).join(' · '),
        path: '/ssl',
        query: domains[0] ? { q: domains[0] } : undefined,
        keywords: domains.slice(1).join(' '),
      }
    })
}

// moveIndex 计算上下键移动后的选中项，首尾循环；没有条目时为 -1
export function moveIndex(cur: number, delta: number, len: number): number {
  if (len <= 0) {
    return -1
  }
  if (cur < 0) {
    return delta > 0 ? 0 : len - 1
  }
  return (((cur + delta) % len) + len) % len
}
