// 站点的敲门准入与登录后放行设置（与后端 webservice.Site 的 knock、knockLan、knockGrant 字段对应），
// 供站点编辑框的 SiteKnock 组件与 Web 服务页面共用。

// KnockGrant 为放行入口设置，与后端 webservice.KnockGrant 对应
export interface KnockGrant {
  rule: string
  minutes: number
  v6Prefix: number
  probeV4: string
  probeV6: string
}

export interface KnockFields {
  webAuth: boolean
  basicAuth: boolean
  domains: string[] | null
  denyClose: boolean
  knock?: string
  knockLan?: boolean
  knockGrant: KnockGrant
}

// emptyKnockGrant 返回放行入口的默认设置（未开启）
export function emptyKnockGrant(): KnockGrant {
  return { rule: '', minutes: 360, v6Prefix: 64, probeV4: '', probeV6: '' }
}

// knockPagePath 为放行入口页面的路径，与后端 knockPagePath 相同
export const knockPagePath = '/__gately_knock'
