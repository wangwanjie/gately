// DDNS 批量添加解析记录的解析函数，与界面无关，便于单元测试（tests/ddnsBatch.test.mjs）。
//
// 每行一条：域名 [类型] [记录值]
//   - 域名可以是完整域名（nas.example.com），也可以是不带点的主机记录（nas，需填写默认主域名）
//   - 类型省略时按对话框勾选的类型各生成一条；记录值省略时 A/AAAA 用 {ipv4Addr}/{ipv6Addr}
//   - 空行与 # 开头的行忽略

export const BATCH_TYPES = ['A', 'AAAA', 'CNAME', 'TXT'] as const

// BatchError 为无效行的原因，界面按 ddns.batchErr.<原因> 显示
export type BatchError = 'domain' | 'type' | 'value' | 'zone' | 'address'

export interface BatchRecord {
  sub: string
  zone: string
  type: string
  value: string
}

export interface BatchOptions {
  // zone 为默认主域名：不带点的主机记录挂在它下面，完整域名优先按它拆分
  zone: string
  // types 为未写类型的行要生成的记录类型
  types: string[]
  // knownZones 为已知主域名（任务内已有记录、同服务商其他任务的记录），用来拆分完整域名
  knownZones: string[]
  // existing 为已有记录，用于跳过重复项；line 为批量记录要使用的线路
  existing: { sub: string; zone: string; type: string; line?: string }[]
  line: string
}

export interface BatchResult {
  records: BatchRecord[]
  // duplicates 为与已有记录或前面的行重复而跳过的行（行号从 1 开始）
  duplicates: { line: number; text: string }[]
  errors: { line: number; text: string; reason: BatchError }[]
}

// 与后端 internal/modules/ddns/config.go 的 reZone、reSub 保持一致
const reZone = /^([a-z0-9-]+\.)+[a-z0-9-]+$/
const reSub = /^(@|\*|[a-z0-9_*]([a-z0-9_.*-]*[a-z0-9_])?)$/
const reIPv4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/

// 常见的二级公共后缀：没有已知主域名可匹配时，用来猜测完整域名的主域名
const multiSuffixes = new Set([
  'com.cn', 'net.cn', 'org.cn', 'gov.cn', 'edu.cn', 'ac.cn',
  'com.hk', 'org.hk', 'net.hk', 'com.tw', 'org.tw', 'net.tw', 'com.mo',
  'co.uk', 'org.uk', 'ac.uk', 'co.jp', 'ne.jp', 'or.jp', 'co.kr', 'or.kr',
  'com.au', 'net.au', 'org.au', 'co.nz', 'com.sg', 'com.my', 'com.br', 'com.mx',
  'co.in', 'co.za', 'com.tr', 'com.ru', 'eu.org',
])

function normZone(z: string) {
  return z.trim().toLowerCase().replace(/^\.+|\.+$/g, '')
}

// normDomain 去掉误粘贴的协议、路径、端口与末尾的点
function normDomain(s: string) {
  let d = s.trim().toLowerCase().replace(/^[a-z][a-z0-9+.-]*:\/\//, '')
  d = d.replace(/[/?#].*$/, '').replace(/:\d+$/, '')
  return d.replace(/\.+$/, '')
}

// guessZone 按公共后缀猜主域名：example.com、example.com.cn
function guessZone(domain: string) {
  const labels = domain.split('.')
  if (labels.length < 2) {
    return ''
  }
  const last2 = labels.slice(-2).join('.')
  if (labels.length >= 3 && multiSuffixes.has(last2)) {
    return labels.slice(-3).join('.')
  }
  return last2
}

// splitDomain 把一行的域名拆成主机记录与主域名，拆不出时返回 null
export function splitDomain(domain: string, zone: string, knownZones: string[]): { sub: string; zone: string } | null {
  const z = normZone(zone)
  // 不带点的写法视为主机记录，挂在默认主域名下
  if (!domain.includes('.')) {
    return z ? { sub: domain, zone: z } : null
  }
  // 依次尝试默认主域名与已知主域名，取最长的匹配（a.example.com.cn 优先匹配 example.com.cn）
  const candidates = [z, ...knownZones.map(normZone)].filter(Boolean).sort((a, b) => b.length - a.length)
  for (const c of candidates) {
    if (domain === c) {
      return { sub: '@', zone: c }
    }
    if (domain.endsWith('.' + c)) {
      return { sub: domain.slice(0, -c.length - 1), zone: c }
    }
  }
  const g = guessZone(domain)
  if (!g) {
    return null
  }
  return { sub: domain === g ? '@' : domain.slice(0, -g.length - 1), zone: g }
}

function recordId(r: { sub: string; zone: string; type: string; line?: string }) {
  // 与后端一致先去掉两端空白：已有记录中手动录入的 " www " 与批量输入的 www 视为同一条
  const sub = (r.sub.trim() || '@').toLowerCase()
  return `${r.type.toUpperCase()} ${sub}.${normZone(r.zone)} ${(r.line ?? '').trim()}`
}

function defaultValue(type: string) {
  if (type === 'A') {
    return '{ipv4Addr}'
  }
  if (type === 'AAAA') {
    return '{ipv6Addr}'
  }
  return ''
}

// checkValue 检查 A/AAAA 的固定地址；含变量的值交给后端在同步时替换
function checkValue(type: string, value: string): BatchError | null {
  if (!value) {
    return 'value'
  }
  if (value.includes('{')) {
    return null
  }
  if (type === 'A' && !reIPv4.test(value)) {
    return 'address'
  }
  if (type === 'AAAA' && !isIPv6(value)) {
    return 'address'
  }
  return null
}

// isIPv6 严格校验 IPv6 地址（含 ::ffff:1.2.3.4 这类内嵌 IPv4 的写法，不接受 %zone），与后端 netip.ParseAddr 一致
export function isIPv6(value: string) {
  if (!value.includes(':') || !/^[0-9a-f:.]+$/i.test(value)) {
    return false
  }
  try {
    new URL(`http://[${value}]/`)
    return true
  } catch {
    return false
  }
}

// parseBatch 解析批量输入，返回可添加的记录、重复跳过的行与无效行
export function parseBatch(text: string, opts: BatchOptions): BatchResult {
  const res: BatchResult = { records: [], duplicates: [], errors: [] }
  const seen = new Set(opts.existing.map(recordId))
  const lines = text.split(/\r?\n/)
  lines.forEach((raw, i) => {
    const s = raw.trim()
    if (!s || s.startsWith('#')) {
      return
    }
    const lineNo = i + 1
    const fail = (reason: BatchError) => {
      res.errors.push({ line: lineNo, text: s, reason })
    }
    // 第一个空白前为域名，其后依次为类型与记录值（记录值可含空格，例如 TXT）
    const m = /^(\S+)(?:\s+(\S+))?(?:\s+(.+))?$/.exec(s)
    if (!m) {
      fail('domain')
      return
    }
    const domain = normDomain(m[1])
    const parts = splitDomain(domain, opts.zone, opts.knownZones)
    if (!parts) {
      fail(domain.includes('.') ? 'domain' : 'zone')
      return
    }
    if (!reZone.test(parts.zone) || !reSub.test(parts.sub)) {
      fail('domain')
      return
    }
    let types = opts.types
    let value = (m[3] ?? '').trim()
    if (m[2]) {
      const ty = m[2].toUpperCase()
      if ((BATCH_TYPES as readonly string[]).includes(ty)) {
        types = [ty]
      } else if (!m[3] && reIPv4.test(m[2])) {
        // 省略类型直接写地址：按地址推断为 A 或 AAAA
        types = ['A']
        value = m[2]
      } else if (!m[3] && m[2].includes(':')) {
        types = ['AAAA']
        value = m[2]
      } else {
        fail('type')
        return
      }
    }
    const built: BatchRecord[] = []
    for (const type of types) {
      const v = value || defaultValue(type)
      const err = checkValue(type, v)
      if (err) {
        fail(err)
        return
      }
      built.push({ sub: parts.sub, zone: parts.zone, type, value: v })
    }
    if (!built.length) {
      fail('type')
      return
    }
    let added = false
    for (const r of built) {
      const id = recordId({ ...r, line: opts.line })
      if (seen.has(id)) {
        continue
      }
      seen.add(id)
      res.records.push(r)
      added = true
    }
    if (!added) {
      res.duplicates.push({ line: lineNo, text: s })
    }
  })
  return res
}
