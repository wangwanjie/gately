// 导航面板（Web → 导航面板）的数据结构与纯函数：链接分组、排序、从 Web 服务站点生成链接、图标写法判断。
// 与界面无关，便于单元测试（tests/panels.test.mjs）。

// PanelAlt 是链接的其他访问方式（如 IPv6、frp、STUN、Cloudflare 隧道），访客页面显示为卡片上的小链接
export interface PanelAlt {
  name: string
  url: string
}

export interface PanelLink {
  name: string
  group: string
  description: string
  // url 为外网地址；lanUrl 为内网地址，留空时内网访客也用 url。两者都可用 {host}、{STUN_规则名_PORT} 等变量
  url: string
  lanUrl: string
  alts: PanelAlt[]
  // icon 为内置图标（builtin:编号）、http(s) 图片地址、data:image 内联图片或不超过 8 个字符的文字，留空显示名称首字
  icon: string
  color: string
  sameTab: boolean
  hide: boolean
}

export interface Panel {
  key?: string
  name: string
  enable: boolean
  title: string
  subtitle: string
  icon: string
  theme: '' | 'light' | 'dark'
  // skin 为皮肤（色系 ID，与管理界面的主题配色相同），空串为默认色系
  skin: string
  lanCidrs: string[]
  links: PanelLink[]
  remark: string
}

// PanelView 为列表接口返回的面板，带引用它的站点（监听名 / 站点名，默认站点为 “监听名 / @default”）
export interface PanelView extends Panel {
  usedBy?: string[]
}

// 与后端一致的上限
export const MAX_LINKS = 500
export const MAX_ICON_BYTES = 96 * 1024
export const MAX_TEXT_ICON = 8
export const MAX_ALTS = 8
export const BUILTIN_PREFIX = 'builtin:'

export function emptyPanel(): Panel {
  return { name: '', enable: true, title: '', subtitle: '', icon: '', theme: '', skin: '', lanCidrs: [], links: [], remark: '' }
}

export function emptyLink(group = ''): PanelLink {
  return { name: '', group, description: '', url: '', lanUrl: '', alts: [], icon: '', color: '', sameTab: false, hide: false }
}

// toPanel 去掉列表接口附带的只读字段（usedBy、key），得到可提交的面板；后端拒绝未知字段
export function toPanel(v: PanelView | Panel): Panel {
  return {
    name: v.name,
    enable: v.enable,
    title: v.title ?? '',
    subtitle: v.subtitle ?? '',
    icon: v.icon ?? '',
    theme: v.theme ?? '',
    skin: v.skin ?? '',
    lanCidrs: [...(v.lanCidrs ?? [])],
    links: (v.links ?? []).map((l) => ({ ...emptyLink(), ...l, alts: (l.alts ?? []).map((a) => ({ name: a.name, url: a.url })) })),
    remark: v.remark ?? '',
  }
}

export interface LinkGroup {
  name: string
  // 链接在面板 links 中的下标，按列表顺序
  items: number[]
}

// groupLinks 按分组首次出现的顺序把链接分组，与访客页面的显示顺序一致
export function groupLinks(links: readonly PanelLink[]): LinkGroup[] {
  const out: LinkGroup[] = []
  const at = new Map<string, number>()
  links.forEach((l, i) => {
    const g = l.group.trim()
    let idx = at.get(g)
    if (idx === undefined) {
      idx = out.length
      at.set(g, idx)
      out.push({ name: g, items: [] })
    }
    out[idx].items.push(i)
  })
  return out
}

// flatten 按分组顺序重新排列链接，使同组链接相邻（页面按分组显示，列表顺序与之保持一致）
function flatten(links: PanelLink[], groups: LinkGroup[]): PanelLink[] {
  return groups.flatMap((g) => g.items.map((i) => links[i]))
}

// moveLink 把第 index 个链接在所在分组内上移（delta=-1）或下移（delta=1），越界时不变。返回新数组
export function moveLink(links: PanelLink[], index: number, delta: -1 | 1): PanelLink[] {
  const groups = groupLinks(links)
  const g = groups.find((x) => x.items.includes(index))
  if (!g) {
    return links
  }
  const pos = g.items.indexOf(index)
  const to = pos + delta
  if (to < 0 || to >= g.items.length) {
    return links
  }
  ;[g.items[pos], g.items[to]] = [g.items[to], g.items[pos]]
  return flatten(links, groups)
}

// moveGroup 把名为 name 的分组整体上移或下移一位，越界时不变。返回新数组
export function moveGroup(links: PanelLink[], name: string, delta: -1 | 1): PanelLink[] {
  const groups = groupLinks(links)
  const pos = groups.findIndex((g) => g.name === name)
  const to = pos + delta
  if (pos < 0 || to < 0 || to >= groups.length) {
    return links
  }
  ;[groups[pos], groups[to]] = [groups[to], groups[pos]]
  return flatten(links, groups)
}

// insertLink 把新链接放到同组最后一个链接之后（没有同组链接时放在最后），返回新数组与新链接的下标
export function insertLink(links: PanelLink[], link: PanelLink): { links: PanelLink[]; index: number } {
  const g = link.group.trim()
  let last = -1
  links.forEach((l, i) => {
    if (l.group.trim() === g) {
      last = i
    }
  })
  const at = last < 0 ? links.length : last + 1
  const out = [...links.slice(0, at), link, ...links.slice(at)]
  return { links: out, index: at }
}

// ---- 从 Web 服务站点生成链接 ----

export interface ImportSite {
  // 监听key/站点key，用于勾选
  id: string
  listener: string
  name: string
  group: string
  domain: string
  url: string
  // type 为站点类型，导航面板站点自身不列出
  type: string
}

export interface ImportListener {
  key?: string
  port: number
  name: string
  enable: boolean
  sites: { key?: string; name: string; group?: string; enable: boolean; type: string; domains: string[] | null }[]
}

// importableSites 列出能生成具体地址的站点（取第一个能拼出地址的域名），跳过断开连接与导航面板站点；
// linkFor 按监听的访问协议与端口拼出域名的地址，域名是通配等无法直接访问时返回 null（见 siteUrl.ts 的 siteLink）
export function importableSites<L extends ImportListener>(listeners: readonly L[], linkFor: (l: L, domain: string) => string | null): ImportSite[] {
  const out: ImportSite[] = []
  for (const l of listeners) {
    for (const s of l.sites ?? []) {
      if (s.type === 'close' || s.type === 'panel') {
        continue
      }
      for (const d of s.domains ?? []) {
        const url = linkFor(l, d)
        if (url) {
          out.push({ id: `${l.key ?? ''}/${s.key ?? ''}`, listener: l.name || `:${l.port}`, name: s.name || d, group: s.group ?? '', domain: d, url, type: s.type })
          break
        }
      }
    }
  }
  return out
}

// linksFromSites 把勾选的站点转换成链接，已有相同地址的站点跳过
export function linksFromSites(sites: readonly ImportSite[], existing: readonly PanelLink[]): PanelLink[] {
  const seen = new Set(existing.map((l) => l.url))
  const out: PanelLink[] = []
  for (const s of sites) {
    if (seen.has(s.url)) {
      continue
    }
    seen.add(s.url)
    out.push({ ...emptyLink(s.group), name: s.name, url: s.url })
  }
  return out
}

// ---- 图标 ----

export type IconKind = 'none' | 'builtin' | 'image' | 'text'

export function iconKind(icon: string): IconKind {
  const v = icon.trim()
  if (!v) {
    return 'none'
  }
  if (v.startsWith(BUILTIN_PREFIX)) {
    return 'builtin'
  }
  if (/^data:image\//i.test(v) || /^https?:\/\//i.test(v)) {
    return 'image'
  }
  return 'text'
}

// safeIconSrc 返回可放进 <img src> 的图标地址：只接受 http(s) 与 data:image，其余返回 null
export function safeIconSrc(icon: string): string | null {
  const v = icon.trim()
  if (/^data:image\/(png|jpeg|gif|webp|svg\+xml|x-icon|vnd\.microsoft\.icon);base64,[A-Za-z0-9+/]+={0,2}$/.test(v)) {
    return v
  }
  try {
    const u = new URL(v)
    if ((u.protocol === 'http:' || u.protocol === 'https:') && u.hostname && !u.username && !u.password) {
      return u.href
    }
  } catch {
    // 不是地址
  }
  return null
}

// initial 返回名称的首个字母或汉字（大写），与访客页面的默认图标一致
export function initial(name: string): string {
  const m = /[\p{L}\p{N}]/u.exec(name.trim())
  if (m) {
    return m[0].toUpperCase()
  }
  const first = [...name.trim()][0]
  return first ?? '?'
}

const palette = ['#0e7c66', '#2563eb', '#7c3aed', '#db2777', '#ea580c', '#b45309', '#15803d', '#0e7490', '#4f46e5', '#dc2626']

// paletteColor 与后端相同的 FNV-1a 取色，未指定颜色时预览与访客页面一致
export function paletteColor(name: string): string {
  let h = 2166136261
  for (const b of new TextEncoder().encode(name)) {
    h ^= b
    h = Math.imul(h, 16777619) >>> 0
  }
  return palette[h % palette.length]
}

// textIconOk 判断文字图标是否在长度限制内（码点不超过 8 个、UTF-8 不超过 32 字节）且不含尖括号
export function textIconOk(icon: string): boolean {
  const v = icon.trim()
  return [...v].length <= MAX_TEXT_ICON && new TextEncoder().encode(v).length <= 32 && !/[<>\p{Cc}]/u.test(v)
}

// hasVars 判断地址中是否用了变量（如 {host}、{STUN_web_PORT}），列表中按原样显示并提示
export function hasVars(url: string): boolean {
  return /\{[^{}]+\}/.test(url)
}

// builtinId 返回内置图标写法中的编号，不是内置图标时为空
export function builtinId(icon: string): string {
  const v = icon.trim()
  return v.startsWith(BUILTIN_PREFIX) ? v.slice(BUILTIN_PREFIX.length) : ''
}

// lightColor 与后端 webpage.LightColor 相同：底色相对亮度高于 0.66 时内置图标用深色图形
export function lightColor(hex: string): boolean {
  if (!/^#[0-9a-f]{6}$/i.test(hex)) {
    return false
  }
  const ch = (i: number) => parseInt(hex.slice(i, i + 2), 16) / 255
  return 0.2126 * ch(1) + 0.7152 * ch(3) + 0.0722 * ch(5) > 0.66
}

// BuiltinIcon 为 GET /api/webservice/panels/icons 返回的内置图标：src 为白色图形，srcDark 为深色图形
export interface BuiltinIcon {
  id: string
  name: string
  cat: 'gately' | 'apps' | 'system' | 'generic'
  color: string
  src: string
  srcDark: string
}

// filterIcons 按分类与关键词（名称或编号）筛选内置图标
export function filterIcons(list: readonly BuiltinIcon[], cat: string, q: string): BuiltinIcon[] {
  const k = q.trim().toLowerCase()
  return list.filter((i) => (!cat || i.cat === cat) && (!k || i.name.toLowerCase().includes(k) || i.id.includes(k)))
}
