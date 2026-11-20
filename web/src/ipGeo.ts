// IP 归属地显示的纯函数：从各处显示的地址文本中取出要查询的地址，把查询结果拼成显示文字。
// 查询与缓存见 composables/useIpGeo.ts。

// GeoLoc 是 /api/geoip/lookup 返回的一个地址的归属地；country 为国家代码，内网与本机地址为 LAN
export interface GeoLoc {
  country: string
  region?: string
  city?: string
}

const ipLike = /^[0-9a-fA-F:.]+(\/\d{1,3})?$/

// lookupKey 从 IP、IP:端口、[IPv6]:端口、CIDR 中取出用于查询的地址（CIDR 保留前缀长度），
// 去掉 IPv6 区域标识；不像地址的文本（主机名、空串）返回空串，不发起查询。
export function lookupKey(raw: string): string {
  let s = raw.trim()
  const bracket = /^\[([^\]]+)\](?::\d+)?$/.exec(s)
  if (bracket) {
    s = bracket[1]
  } else {
    const v4port = /^(\d{1,3}(?:\.\d{1,3}){3}):\d+$/.exec(s)
    if (v4port) {
      s = v4port[1]
    }
  }
  const zone = s.indexOf('%')
  if (zone >= 0) {
    s = s.slice(0, zone)
  }
  if (!ipLike.test(s) || !/[.:]/.test(s)) {
    return ''
  }
  return s
}

// geoParts 返回归属地的各级名称：国家（按界面语言的短名称）、省（州）、城市；
// 重名的级别（例如香港的城市就叫香港）只保留一个。内网地址只有一项 lan。
export function geoParts(loc: GeoLoc | undefined, locale: string, lan: string): string[] {
  if (!loc?.country) {
    return []
  }
  if (loc.country === 'LAN') {
    return [lan]
  }
  const out: string[] = []
  for (const p of [countryName(loc.country, locale), loc.region, loc.city]) {
    if (p && !out.some((x) => x.toLowerCase() === p.toLowerCase())) {
      out.push(p)
    }
  }
  return out
}

const nameCache = new Map<string, Intl.DisplayNames | null>()

// countryName 把国家代码转为界面语言的短名称（如「香港」「US」），浏览器不支持时返回代码本身。
export function countryName(code: string, locale: string): string {
  let names = nameCache.get(locale)
  if (names === undefined) {
    try {
      names = new Intl.DisplayNames([locale], { type: 'region', style: 'short' })
    } catch {
      names = null
    }
    nameCache.set(locale, names)
  }
  try {
    return names?.of(code) ?? code
  } catch {
    return code
  }
}
