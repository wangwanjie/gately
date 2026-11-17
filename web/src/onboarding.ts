// 首次启动引导的纯函数（不依赖界面，便于单元测试）：使用场景到功能模块的对应、「开始使用」清单的内容与完成判断。

// SceneId 为使用场景编号，与后端 internal/app/onboarding.go 的 onboardingScenes 一致
export type SceneId = 'web' | 'forward' | 'ddns' | 'mesh' | 'files' | 'vps' | 'all'

// Scene 为一个使用场景：选择后开启 modules 中的模块（连同它们依赖的模块）
export interface Scene {
  id: SceneId
  modules: string[]
}

// SCENES 为首次启动可选的使用场景，顺序即界面上的顺序。思路与 modules.json 的 presets 相同，
// 按「想做什么」而不是按设备类型划分；all 表示全部功能（不关闭任何模块）
export const SCENES: Scene[] = [
  // 把内网服务发布成域名：反向代理 + 证书，附带动态域名、WAF 与入侵防护
  { id: 'web', modules: ['webservice', 'ssl', 'ddns', 'waf', 'guard'] },
  // 端口转发 / 打洞
  { id: 'forward', modules: ['portforward', 'stun', 'ddns', 'guard'] },
  { id: 'ddns', modules: ['ddns'] },
  // 异地组网
  { id: 'mesh', modules: ['tailscale', 'netbird', 'easytier'] },
  // 文件共享；配置备份依赖存储挂载
  { id: 'files', modules: ['webdav', 'smb', 'ftp', 'filebrowser', 'storage', 'backup'] },
  // VPS 入口：协议分流 + frp 服务端，常与反向代理同机
  { id: 'vps', modules: ['protoroute', 'frp', 'webservice', 'ssl', 'waf', 'guard', 'knock'] },
  { id: 'all', modules: [] },
]

// BASE_MODULES 为任何场景下都保留的模块：通知（清单里的「设置通知」）与网络诊断
export const BASE_MODULES = ['notify', 'nettools']

// ModuleMeta 为设置接口返回的模块信息（id 与依赖）
export interface ModuleMeta {
  id: string
  requires?: string[] | null
}

// enabledModulesFor 返回所选场景需要开启的模块（含依赖）；选择了 all 或没有选择任何场景时返回 null，表示全部开启
export function enabledModulesFor(scenes: string[], all: ModuleMeta[]): Set<string> | null {
  if (!scenes.length || scenes.includes('all')) {
    return null
  }
  const known = new Set(all.map((m) => m.id))
  const on = new Set<string>()
  const stack = [...BASE_MODULES, ...SCENES.filter((s) => scenes.includes(s.id)).flatMap((s) => s.modules)]
  // 依赖可能是多级的，逐层开启
  while (stack.length) {
    const id = stack.pop() as string
    if (on.has(id) || !known.has(id)) {
      continue
    }
    on.add(id)
    stack.push(...(all.find((m) => m.id === id)?.requires ?? []))
  }
  return on
}

// disabledModulesFor 返回保存到设置 disabledModules 的列表：所选场景用不到的模块。
// keep 为必须保持开启的模块（如设置了管理界面 HTTPS 时的证书模块）；absent 为定制构建中没有编译进来的模块，原样保留其开关
export function disabledModulesFor(scenes: string[], all: ModuleMeta[], keep: string[] = [], current: string[] = [], absent: string[] = []): string[] {
  const on = enabledModulesFor(scenes, all)
  const off = new Set(current.filter((id) => absent.includes(id)))
  if (on) {
    for (const k of keep) {
      on.add(k)
    }
    for (const m of all) {
      if (!on.has(m.id)) {
        off.add(m.id)
      }
    }
  }
  return [...off].sort()
}

// ChecklistId 为「开始使用」清单项编号
export type ChecklistId =
  | 'site'
  | 'https'
  | 'ddns'
  | 'portforward'
  | 'stun'
  | 'mesh'
  | 'fileuser'
  | 'fileservice'
  | 'protoroute'
  | 'frps'
  | 'security'
  | 'check'
  | 'import'
  | 'notify'

// ChecklistDef 描述清单项依赖的模块：modules 中任意一个开启时显示；manual 为无法从配置判断、点过即视为完成的项目
export interface ChecklistDef {
  id: ChecklistId
  modules?: string[]
  manual?: boolean
}

export const CHECKLIST: Record<ChecklistId, ChecklistDef> = {
  site: { id: 'site', modules: ['webservice'] },
  https: { id: 'https', modules: ['ssl'] },
  ddns: { id: 'ddns', modules: ['ddns'] },
  portforward: { id: 'portforward', modules: ['portforward'] },
  stun: { id: 'stun', modules: ['stun'] },
  mesh: { id: 'mesh', modules: ['tailscale', 'netbird', 'easytier'] },
  fileuser: { id: 'fileuser', modules: ['webdav', 'smb', 'ftp'] },
  fileservice: { id: 'fileservice', modules: ['webdav', 'smb', 'ftp', 'filebrowser'] },
  protoroute: { id: 'protoroute', modules: ['protoroute'] },
  frps: { id: 'frps', modules: ['frp'] },
  security: { id: 'security' },
  check: { id: 'check', manual: true },
  import: { id: 'import', manual: true },
  notify: { id: 'notify', modules: ['notify'] },
}

// 各场景的清单项（按完成顺序）
const SCENE_ITEMS: Record<SceneId, ChecklistId[]> = {
  web: ['site', 'https', 'ddns'],
  forward: ['portforward', 'stun', 'ddns'],
  ddns: ['ddns'],
  mesh: ['mesh'],
  files: ['fileuser', 'fileservice'],
  vps: ['protoroute', 'frps', 'site', 'https'],
  all: ['site', 'https', 'portforward', 'ddns'],
}

// 原有引导的四项排在清单后部
const TAIL_ITEMS: ChecklistId[] = ['security', 'check', 'import', 'notify']

// checklistFor 按所选场景生成清单（去重）；没有选择场景（跳过或升级的实例）时按全部功能生成。
// enabled 判断模块是否开启，所依赖的模块全部关闭的项目不显示
export function checklistFor(scenes: string[], enabled: (id: string) => boolean = () => true): ChecklistId[] {
  const picked = scenes.length ? scenes : ['all']
  const out: ChecklistId[] = []
  for (const s of SCENES) {
    if (!picked.includes(s.id)) {
      continue
    }
    for (const id of SCENE_ITEMS[s.id]) {
      if (!out.includes(id)) {
        out.push(id)
      }
    }
  }
  out.push(...TAIL_ITEMS)
  return out.filter((id) => {
    const mods = CHECKLIST[id].modules
    return !mods || mods.some(enabled)
  })
}

// ChecklistFacts 为判断清单项是否完成所需的事实，由界面从各模块接口汇总；未读取（模块关闭或无权限）的项目为 0 / false
export interface ChecklistFacts {
  sites: number
  httpsSites: number
  ddnsTasks: number
  portforwardRules: number
  stunRules: number
  meshInstances: number
  fileUsers: number
  fileServices: number
  protorouteRules: number
  frpServers: number
  totp: boolean
  passkeys: number
  notifyChannels: number
  // done 为手动标记完成的项目（来自后端）
  done: string[]
}

export function emptyFacts(): ChecklistFacts {
  return {
    sites: 0,
    httpsSites: 0,
    ddnsTasks: 0,
    portforwardRules: 0,
    stunRules: 0,
    meshInstances: 0,
    fileUsers: 0,
    fileServices: 0,
    protorouteRules: 0,
    frpServers: 0,
    totp: false,
    passkeys: 0,
    notifyChannels: 0,
    done: [],
  }
}

// itemDone 判断清单项是否已完成：能从配置判断的看实际配置（例如已有启用的站点即完成「发布第一个站点」），其余看是否手动标记
export function itemDone(id: ChecklistId, f: ChecklistFacts): boolean {
  switch (id) {
    case 'site':
      return f.sites > 0
    case 'https':
      return f.httpsSites > 0
    case 'ddns':
      return f.ddnsTasks > 0
    case 'portforward':
      return f.portforwardRules > 0
    case 'stun':
      return f.stunRules > 0
    case 'mesh':
      return f.meshInstances > 0
    case 'fileuser':
      return f.fileUsers > 0
    case 'fileservice':
      return f.fileServices > 0
    case 'protoroute':
      return f.protorouteRules > 0
    case 'frps':
      return f.frpServers > 0
    case 'security':
      return f.totp || f.passkeys > 0
    case 'notify':
      return f.notifyChannels > 0
    default:
      return f.done.includes(id)
  }
}

// countEnabled 统计列表中启用的条目（没有 enable 字段的视为启用）；接口返回 null 时为 0
export function countEnabled(list: { enable?: boolean }[] | null | undefined): number {
  return (list ?? []).filter((x) => x.enable !== false).length
}

// ListenerLike 为监听列表接口中判断站点完成情况所需的字段
export interface ListenerLike {
  enable: boolean
  tls: boolean
  sites?: { key?: string; enable: boolean }[] | null
  siteCerts?: Record<string, { name?: string }[]> | null
}

// countSites 统计已启用监听下启用的站点，以及其中 HTTPS 且每个域名都匹配到证书的站点
export function countSites(listeners: ListenerLike[] | null | undefined): { sites: number; https: number } {
  let sites = 0
  let https = 0
  for (const l of listeners ?? []) {
    if (!l.enable) {
      continue
    }
    for (const s of l.sites ?? []) {
      if (!s.enable) {
        continue
      }
      sites++
      const certs = s.key ? l.siteCerts?.[s.key] : undefined
      if (l.tls && certs?.length && certs.every((c) => !!c.name)) {
        https++
      }
    }
  }
  return { sites, https }
}
