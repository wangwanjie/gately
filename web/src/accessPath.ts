// 访问路径（配置变更防护的第二层）：保存端口转发、协议分流、Web 服务与全局设置前，先请服务端判断修改是否影响
// 当前访问管理界面经过的转发（见后端 internal/app/accesspath.go），有影响时弹窗说明并让用户决定是否继续。
import { reactive } from 'vue'
import { t } from './i18n'

export interface PathHop {
  kind: string
  rule?: string
  name?: string
  route?: string
  site?: string
  siteName?: string
  listen?: string
  addr?: string
  https?: boolean
  missing?: boolean
}

export interface AccessPath {
  hops: PathHop[]
}

export interface PathRisk {
  kind: string
  hop: number
  module?: string
  fields?: string[]
}

export interface AccessCheck {
  path: AccessPath
  risks: PathRisk[]
}

// hopLabel 返回一跳在界面上的说明
export function hopLabel(h: PathHop): string {
  const name = h.name || h.rule || ''
  const missing = h.missing ? t('accessPath.missing') : ''
  switch (h.kind) {
    case 'client':
      return t('accessPath.hop.client', { addr: h.addr ?? '' })
    case 'local':
      return t('accessPath.hop.local', { addr: h.addr ?? '' })
    case 'gateway':
      return t('accessPath.hop.gateway')
    case 'portforward':
      return t('accessPath.hop.portforward', { name }) + missing
    case 'protoroute':
      return t('accessPath.hop.protoroute', { name }) + (h.route ? t('accessPath.hop.route', { route: h.route }) : '') + missing
    case 'webservice':
      return t('accessPath.hop.webservice', { name }) + t('accessPath.hop.site', { site: h.siteName || t('accessPath.defaultSite') }) + missing
    case 'admin':
      return t(h.https ? 'accessPath.hop.adminHttps' : 'accessPath.hop.admin', { listen: h.listen ?? '' })
  }
  return h.kind
}

const moduleKeys: Record<string, string> = {
  portforward: 'nav.items.portforward.title',
  protoroute: 'nav.items.protoroute.title',
  webservice: 'nav.items.web.title',
}

// riskText 返回一项风险的说明
export function riskText(r: PathRisk, path: AccessPath): string {
  const hop = r.hop >= 0 && r.hop < path.hops.length ? hopLabel(path.hops[r.hop]) : ''
  switch (r.kind) {
    case 'edit':
      return t('accessPath.risk.edit', { hop })
    case 'delete':
      return t('accessPath.risk.delete', { hop })
    case 'disable':
      return t('accessPath.risk.disable', { hop })
    case 'moduleOff':
      return t('accessPath.risk.moduleOff', { module: r.module && moduleKeys[r.module] ? t(moduleKeys[r.module]) : (r.module ?? ''), hop })
    case 'siteMove':
      return t('accessPath.risk.siteMove', { hop })
    case 'adminListen':
      return t('accessPath.risk.adminListen')
    case 'lanOnly':
      return t('accessPath.risk.lanOnly')
    case 'forceHTTPS':
      return t('accessPath.risk.forceHTTPS')
  }
  return r.kind
}

// RiskAnswer 为提醒弹窗的选择：取消、仍要保存（立即生效），或开始批量修改后保存（暂缓生效，稍后与其他修改一起生效）
export type RiskAnswer = 'cancel' | 'save' | 'hold'

// riskPrompt 为保存前的提醒弹窗状态，由 AccessRiskDialog 显示
export const riskPrompt = reactive({
  visible: false,
  check: null as AccessCheck | null,
  resolve: null as ((answer: RiskAnswer) => void) | null,
})

// askRisk 显示提醒并等待用户选择
export function askRisk(check: AccessCheck): Promise<RiskAnswer> {
  // 同时只有一个提醒：前一个未答复的按取消处理
  riskPrompt.resolve?.('cancel')
  return new Promise((resolve) => {
    riskPrompt.check = check
    riskPrompt.resolve = resolve
    riskPrompt.visible = true
  })
}

export function answerRisk(answer: RiskAnswer) {
  const r = riskPrompt.resolve
  riskPrompt.resolve = null
  riskPrompt.visible = false
  r?.(answer)
}

// checkAccess 请服务端判断写请求是否影响当前访问路径；检查本身失败时不拦保存（第一层的确认与回滚仍然生效）
export async function checkAccess(method: string, url: string, body: unknown): Promise<AccessCheck | null> {
  try {
    const resp = await fetch('/api/system/access-path/check', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ method, path: url, body: body === undefined ? null : body }),
    })
    if (!resp.ok) {
      return null
    }
    return (await resp.json()) as AccessCheck
  } catch {
    return null
  }
}
