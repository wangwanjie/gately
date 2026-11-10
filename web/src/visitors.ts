// 访客审计接口（/api/webservice/visitors）的数据类型，见 internal/visitors。时间均为 Unix 秒。

export interface SiteRef {
  listener: string
  key: string
  name: string
  // unmatched 表示请求的域名没有对应的站点（由默认站点处理或直接 404）
  unmatched?: boolean
  req?: number
}

export interface SiteStat extends SiteRef {
  req: number
  denied: number
  last: number
}

// Ident 是访客使用过的身份：user 为站点认证，admin 为带管理后台登录态
export interface Ident {
  name: string
  kind: 'user' | 'admin'
  req: number
  last: number
}

export interface LastReq {
  time: number
  method: string
  host: string
  path: string
  status: number
  deny?: string
  siteName: string
}

export interface Item {
  ip: string
  // country 为国家代码，内网地址为 LAN，没有国家库时为空
  country?: string
  banned: boolean
  // noise 表示只有被拦截或访问未匹配站点的请求（扫描类）
  noise: boolean
  first: number
  last: number
  // requests、denied 为统计范围内的次数
  requests: number
  denied: number
  totalRequests: number
  sites: SiteRef[]
  siteCount: number
  identities: Ident[]
  agent?: string
  lastReq: LastReq
}

export interface VisitorSummary {
  visitors: number
  requests: number
  denied: number
  authed: number
  banned: number
  noise: number
  new: number
}

export interface DayPoint {
  // day 为本地日期 YYYY-MM-DD
  day: string
  visitors: number
  requests: number
  denied: number
}

export interface VisitorInfo {
  disable: boolean
  days: number
  max: number
  count: number
  fileBytes: number
  savedAt?: number
  error?: string
}

export interface VisitorList {
  items: Item[]
  total: number
  summary: VisitorSummary
  daily: DayPoint[]
  sites: SiteRef[]
  info: VisitorInfo
  listeners: Record<string, string>
}

export interface VisitorDetail {
  ip: string
  country?: string
  banned: boolean
  noiseOnly: boolean
  first: number
  last: number
  requests: number
  denied: number
  bytes: number
  // status 依次为 1xx~5xx 的次数，444 计入 4xx 并单独计入 closed
  status: number[]
  closed: number
  deny: { name: string; n: number }[]
  daily: DayPoint[]
  sites: SiteStat[]
  paths: { path: string; req: number; last: number; status: number }[]
  identities: Ident[]
  agents: { ua: string; req: number; last: number }[]
  lastReq: LastReq
  listeners?: Record<string, string>
}

// latestOnly 生成请求序号：每次发起请求取一个新序号，响应回来时序号已不是最新的说明之后又发起过请求，应丢弃。
// 用于条件快速切换（翻页、筛选、切换访客）时，先发出的慢请求覆盖后发出的结果。
export function latestOnly() {
  let seq = 0
  return {
    next(): number {
      seq++
      return seq
    },
    isLatest(n: number): boolean {
      return n === seq
    },
  }
}

// shrinkImpact 判断保存设置会立即删除哪些数据：保留天数调小且已有数据时为 days；
// 最多访客数调小到少于现有访客数时为 max，n 为将被删除的访客数。都不删除时返回空列表。
export function shrinkImpact(
  cur: Pick<VisitorInfo, 'days' | 'max' | 'count'> | null | undefined,
  next: { days: number; max: number },
): ({ kind: 'days'; days: number } | { kind: 'max'; max: number; n: number })[] {
  const out: ({ kind: 'days'; days: number } | { kind: 'max'; max: number; n: number })[] = []
  if (!cur || cur.count <= 0) {
    return out
  }
  if (next.days < cur.days) {
    out.push({ kind: 'days', days: next.days })
  }
  if (next.max < cur.max && cur.count > next.max) {
    out.push({ kind: 'max', max: next.max, n: cur.count - next.max })
  }
  return out
}
