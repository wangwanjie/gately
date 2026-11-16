// HTTPS 站点证书状态的纯函数（不依赖界面，便于单元测试）。数据来自监听列表接口的 siteCerts。

// SiteCert 为站点某个域名匹配到的证书：name 为空表示没有覆盖该域名的证书；notAfter 为到期时间（ISO 字符串）
export interface SiteCert {
  domain: string
  name?: string
  notAfter?: string
}

// SiteCertState 为站点整体的证书状态：
// missing 为有域名没有证书（访问这些域名时 TLS 握手失败）；expired 为匹配到的证书已过期；
// expiring 为最早到期的证书剩余不足 warnDays 天；ok 为全部覆盖且未临近到期。
export interface SiteCertState {
  kind: 'ok' | 'expiring' | 'expired' | 'missing'
  // days 为最早到期证书的剩余天数（向下取整，已过期时为负数）；没有可用到期时间时为 null
  days: number | null
  // missing 为没有证书的域名
  missing: string[]
  // names 为匹配到的证书名称（去重）
  names: string[]
}

const DAY = 86400000

// siteCertState 汇总站点各域名的证书匹配结果；certs 为空（非 HTTPS 或没有可检查的域名）时返回 null
export function siteCertState(certs: SiteCert[] | undefined, now: number, warnDays = 14): SiteCertState | null {
  if (!certs?.length) {
    return null
  }
  const missing = certs.filter((c) => !c.name).map((c) => c.domain)
  const names = [...new Set(certs.filter((c) => c.name).map((c) => c.name as string))]
  let earliest: number | null = null
  for (const c of certs) {
    if (c.name && c.notAfter) {
      const t = Date.parse(c.notAfter)
      if (!Number.isNaN(t) && (earliest === null || t < earliest)) {
        earliest = t
      }
    }
  }
  const days = earliest === null ? null : Math.floor((earliest - now) / DAY)
  let kind: SiteCertState['kind'] = 'ok'
  if (missing.length) {
    kind = 'missing'
  } else if (earliest !== null && earliest <= now) {
    kind = 'expired'
  } else if (days !== null && days < warnDays) {
    kind = 'expiring'
  }
  return { kind, days, missing, names }
}

// certRequestDomains 返回申请证书时预填的域名：缺证书的域名，通配写法保留原样（*.example.com 需要通配符证书）
export function certRequestDomains(st: SiteCertState): string[] {
  return st.missing.filter((d) => d && d !== '*')
}
