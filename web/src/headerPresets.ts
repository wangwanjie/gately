// 站点请求头 / 响应头与 Content-Type 的常用预设和格式校验。
// 后端按 "Key: Value" 原样设置（不支持变量），Value 为空表示删除该头；见 internal/modules/webservice/site.go applyHeaders。

export type HeaderDirection = 'request' | 'response'

export interface HeaderPreset {
  key: string
  // 常用取值；空字符串表示"删除该头"
  values: string[]
  // 说明文案的键：headerEditor.desc.<id>
  id: string
}

// 发往后端的请求头
export const requestPresets: HeaderPreset[] = [
  { key: 'X-Forwarded-Proto', values: ['https', 'http'], id: 'xForwardedProto' },
  { key: 'X-Forwarded-Ssl', values: ['on'], id: 'xForwardedSsl' },
  { key: 'X-Forwarded-Prefix', values: ['/app'], id: 'xForwardedPrefix' },
  { key: 'Authorization', values: ['Bearer <token>', 'Basic <base64>'], id: 'authorization' },
  { key: 'X-Api-Key', values: ['<key>'], id: 'xApiKey' },
  { key: 'User-Agent', values: ['Gately'], id: 'userAgent' },
  { key: 'Accept-Encoding', values: ['identity'], id: 'acceptEncoding' },
  { key: 'Origin', values: [''], id: 'origin' },
  { key: 'Referer', values: [''], id: 'referer' },
  { key: 'Cookie', values: [''], id: 'cookie' },
  { key: 'Accept-Language', values: ['zh-CN', 'en-US'], id: 'acceptLanguage' },
  { key: 'Cache-Control', values: ['no-cache'], id: 'reqCacheControl' },
]

// 返回给客户端的响应头
export const responsePresets: HeaderPreset[] = [
  {
    key: 'Strict-Transport-Security',
    values: ['max-age=31536000', 'max-age=31536000; includeSubDomains', 'max-age=63072000; includeSubDomains; preload'],
    id: 'hsts',
  },
  { key: 'X-Frame-Options', values: ['SAMEORIGIN', 'DENY'], id: 'xFrameOptions' },
  { key: 'X-Content-Type-Options', values: ['nosniff'], id: 'xContentTypeOptions' },
  {
    key: 'Referrer-Policy',
    values: ['strict-origin-when-cross-origin', 'no-referrer', 'same-origin', 'no-referrer-when-downgrade'],
    id: 'referrerPolicy',
  },
  {
    key: 'Content-Security-Policy',
    values: ["default-src 'self'", "frame-ancestors 'self'", 'upgrade-insecure-requests'],
    id: 'csp',
  },
  { key: 'Permissions-Policy', values: ['camera=(), microphone=(), geolocation=()'], id: 'permissionsPolicy' },
  { key: 'Cross-Origin-Opener-Policy', values: ['same-origin', 'same-origin-allow-popups'], id: 'coop' },
  { key: 'Cross-Origin-Resource-Policy', values: ['same-origin', 'same-site', 'cross-origin'], id: 'corp' },
  {
    key: 'Cache-Control',
    values: ['no-store', 'no-cache', 'private, no-cache', 'public, max-age=3600', 'public, max-age=31536000, immutable'],
    id: 'respCacheControl',
  },
  { key: 'X-Robots-Tag', values: ['noindex, nofollow', 'noindex'], id: 'xRobotsTag' },
  { key: 'Access-Control-Allow-Origin', values: ['*', 'https://example.com'], id: 'acao' },
  { key: 'Content-Disposition', values: ['inline', 'attachment'], id: 'contentDisposition' },
  { key: 'Server', values: [''], id: 'server' },
  { key: 'X-Powered-By', values: [''], id: 'xPoweredBy' },
  { key: 'X-XSS-Protection', values: ['0'], id: 'xXssProtection' },
]

// "一键添加常用安全头"使用的组合
export const securityBundle: [string, string][] = [
  ['X-Content-Type-Options', 'nosniff'],
  ['X-Frame-Options', 'SAMEORIGIN'],
  ['Referrer-Policy', 'strict-origin-when-cross-origin'],
]

export const presetsFor = (dir: HeaderDirection) => (dir === 'request' ? requestPresets : responsePresets)

export const findPreset = (dir: HeaderDirection, key: string) => {
  const k = key.trim().toLowerCase()
  return presetsFor(dir).find((p) => p.key.toLowerCase() === k)
}

// 一条校验反馈；msg 为 headerEditor.issue.<msg> 的键，params 为文案参数
export interface Issue {
  level: 'error' | 'warning' | 'info'
  msg: string
  params?: Record<string, string | number>
}

// RFC 9110 token：头名称允许的字符
const tokenRe = /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/
// 值中不允许出现的控制字符（制表符除外）
const ctlRe = /[\x00-\x08\x0a-\x1f\x7f]/
// 由连接自动管理的头，手动设置会被忽略或破坏传输
const managedHeaders = new Set(['connection', 'keep-alive', 'proxy-connection', 'transfer-encoding', 'te', 'trailer', 'upgrade', 'content-length'])
// 开启「添加 X-Forwarded-*」时由 Gately 写入访客地址与源端口的请求头，写固定值会覆盖真实值
const clientAddrHeaders = new Set(['x-forwarded-for', 'x-real-ip', 'x-real-port', 'x-forwarded-for-port'])

export interface HeaderRow {
  key: string
  value: string
}

// 校验一行头配置；rows 为全部行，用于检查重名
export function checkHeader(dir: HeaderDirection, rows: HeaderRow[], index: number): Issue[] {
  const key = rows[index].key.trim()
  const value = rows[index].value.trim()
  const issues: Issue[] = []
  if (!key && !value) {
    return issues
  }
  if (!key) {
    issues.push({ level: 'error', msg: 'emptyKey' })
    return issues
  }
  if (!tokenRe.test(key)) {
    issues.push({ level: 'error', msg: 'badKey' })
    return issues
  }
  if (ctlRe.test(value)) {
    issues.push({ level: 'error', msg: 'badValue' })
  }
  const lower = key.toLowerCase()
  if (dir === 'request' && lower === 'host') {
    issues.push({ level: 'warning', msg: 'host' })
  } else if (managedHeaders.has(lower)) {
    issues.push({ level: 'warning', msg: 'managed' })
  } else if (dir === 'request' && clientAddrHeaders.has(lower)) {
    issues.push({ level: 'warning', msg: 'clientIp' })
  }
  if (/\$[a-z_]\w*/i.test(value)) {
    issues.push({ level: 'warning', msg: 'variable' })
  } else if (/<(?!https?:\/\/|\/)[^<>]+>/.test(value)) {
    issues.push({ level: 'warning', msg: 'placeholder' })
  }
  // 同名头按顺序依次设置，后面的覆盖前面的
  const later = rows.findIndex((r, i) => i > index && r.key.trim().toLowerCase() === lower)
  if (later >= 0) {
    issues.push({ level: 'warning', msg: 'duplicate', params: { line: later + 1 } })
  }
  if (!value && !issues.some((i) => i.level === 'error')) {
    issues.push({ level: 'info', msg: 'remove' })
  }
  return issues
}

// "Key: Value" 与行数据互转
export function parseHeaderLine(line: string): HeaderRow {
  const i = line.indexOf(':')
  if (i < 0) {
    return { key: line.trim(), value: '' }
  }
  return { key: line.slice(0, i).trim(), value: line.slice(i + 1).trim() }
}

export function formatHeaderRow(r: HeaderRow): string {
  const v = r.value.trim()
  return v ? `${r.key.trim()}: ${v}` : `${r.key.trim()}:`
}

// 常用 Content-Type
export const contentTypePresets = [
  'text/plain; charset=utf-8',
  'text/html; charset=utf-8',
  'application/json',
  'application/xml',
  'text/css; charset=utf-8',
  'text/javascript; charset=utf-8',
  'text/markdown; charset=utf-8',
  'text/csv; charset=utf-8',
  'image/svg+xml',
  'application/octet-stream',
]

const mediaTypes = new Set(['application', 'audio', 'font', 'image', 'message', 'model', 'multipart', 'text', 'video'])
const token = "[!#$%&'*+.^_`|~0-9A-Za-z-]+"
const mimeRe = new RegExp(`^(${token})/(${token})((\\s*;\\s*${token}=(${token}|"[^"]*"))*)\\s*;?$`)

// 校验 Content-Type；为空时后端使用 text/plain; charset=utf-8
export function checkContentType(raw: string): Issue[] {
  const v = raw.trim()
  if (!v) {
    return [{ level: 'info', msg: 'ctDefault' }]
  }
  if (ctlRe.test(v)) {
    return [{ level: 'error', msg: 'badValue' }]
  }
  const m = mimeRe.exec(v)
  if (!m) {
    return [{ level: 'error', msg: 'ctFormat' }]
  }
  const issues: Issue[] = []
  const type = m[1].toLowerCase()
  if (!mediaTypes.has(type)) {
    issues.push({ level: 'warning', msg: 'ctUnknownType', params: { type: m[1] } })
  }
  if (type === 'text' && !/;\s*charset=/i.test(v)) {
    issues.push({ level: 'warning', msg: 'ctCharset' })
  }
  return issues
}
