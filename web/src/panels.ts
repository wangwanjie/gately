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
  // size 为卡片尺寸：空为大卡片（图标加标题、描述），small 为小卡片（只显示图标）
  size: '' | 'small'
  // hideName、hideDesc 只对大卡片生效：隐藏标题、隐藏描述
  hideName: boolean
  hideDesc: boolean
  // authOnly 为 true 时只对已登录的访客显示（未登录时页面中没有该链接）
  authOnly: boolean
}

// PanelEngine 是页面搜索框可选的搜索引擎，url 用 {q} 代表关键词
export interface PanelEngine {
  name: string
  url: string
}

export interface Panel {
  key?: string
  name: string
  enable: boolean
  title: string
  subtitle: string
  icon: string
  // tabTitle 为浏览器标签页标题，favicon 为标签页图标（写法同 icon），description 为页面描述（meta description）；为空时分别用页面标题、页面图标
  tabTitle: string
  favicon: string
  description: string
  // footer 为页脚文字（空为 Gately 标志与名称），footerUrl 为文字的链接，hideFooter 为不显示页脚
  footer: string
  footerUrl: string
  hideFooter: boolean
  theme: '' | 'light' | 'dark'
  // skin 为皮肤（色系 ID，与管理界面的主题配色相同），空串为默认色系
  skin: string
  // engines 为搜索引擎；defaultEngine 为页面默认的搜索范围：0 为本站（过滤本页链接），n 为 engines[n-1]
  engines: PanelEngine[]
  defaultEngine: number
  // background 为背景图片：http(s) 地址或上传的 data:image，空串为默认背景；bgDim 为盖在图片上的底色遮罩浓度（0-90）
  background: string
  bgDim: number
  bgBlur: number
  // cardStyle 为链接样式：空为卡片，app 为桌面 App 图标（大图标在上、名称在下，不显示描述）。
  // 以下布局数值为 0 时用默认值：iconSize 图标边长，columns 宽屏每行最多的链接数，mobileColumns 手机每行的链接数，gap 间距，maxWidth 内容最大宽度
  cardStyle: '' | 'app'
  iconSize: number
  columns: number
  mobileColumns: number
  gap: number
  maxWidth: number
  // groupCount 为在分组标题旁显示链接数；altBadge 为在有多个访问地址的链接图标右上角显示地址数，都默认关闭
  groupCount: boolean
  altBadge: boolean
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
export const MAX_ENGINES = 12
export const MAX_BG_BYTES = 1 << 20
export const MAX_BG_DIM = 90
export const MAX_BG_BLUR = 20
// 上传背景图时选图后的默认遮罩浓度：文字在大多数图片上仍清晰
export const DEFAULT_BG_DIM = 45
// 布局的取值范围，与后端一致；0 为默认
export const LAYOUT_LIMITS = {
  iconSize: { min: 24, max: 128, step: 4 },
  columns: { min: 1, max: 12, step: 1 },
  mobileColumns: { min: 1, max: 8, step: 1 },
  gap: { min: 4, max: 48, step: 2 },
  maxWidth: { min: 640, max: 3840, step: 40 },
} as const
export type LayoutField = keyof typeof LAYOUT_LIMITS
// layoutDefaults 返回各样式下布局的默认值（未设置时页面使用），用作输入框的占位提示；0 表示按宽度自动排列
export function layoutDefaults(style: Panel['cardStyle']): Record<LayoutField, number> {
  return style === 'app'
    ? { iconSize: 64, columns: 0, mobileColumns: 0, gap: 18, maxWidth: 1200 }
    : { iconSize: 40, columns: 0, mobileColumns: 2, gap: 12, maxWidth: 1200 }
}
// normalizeLayoutValue 把输入框的值转为保存的值：清空为 0（默认），其余取整并限制在范围内
export function normalizeLayoutValue(field: LayoutField, v: number | null | undefined): number {
  if (v === null || v === undefined || !Number.isFinite(v) || v <= 0) {
    return 0
  }
  const { min, max } = LAYOUT_LIMITS[field]
  return Math.min(max, Math.max(min, Math.round(v)))
}

// ENGINE_PRESETS 为常用搜索引擎，名称用各自的通用叫法，不随界面语言变化
export const ENGINE_PRESETS: readonly PanelEngine[] = [
  { name: 'Google', url: 'https://www.google.com/search?q={q}' },
  { name: 'Bing', url: 'https://www.bing.com/search?q={q}' },
  { name: '百度', url: 'https://www.baidu.com/s?wd={q}' },
  { name: 'DuckDuckGo', url: 'https://duckduckgo.com/?q={q}' },
  { name: 'GitHub', url: 'https://github.com/search?q={q}' },
  { name: 'YouTube', url: 'https://www.youtube.com/results?search_query={q}' },
  { name: '知乎', url: 'https://www.zhihu.com/search?type=content&q={q}' },
  { name: '小红书', url: 'https://www.xiaohongshu.com/search_result?keyword={q}' },
  { name: '哔哩哔哩', url: 'https://search.bilibili.com/all?keyword={q}' },
  { name: 'Wikipedia', url: 'https://en.wikipedia.org/w/index.php?search={q}' },
]

// engineUrlProblem 粗略检查搜索引擎地址，返回问题类别（空串为通过）；完整规则由后端在保存时校验
export function engineUrlProblem(url: string): '' | 'scheme' | 'q' {
  const v = url.trim()
  if (!/^https?:\/\/[^/?#\s{}]+/i.test(v)) {
    return 'scheme'
  }
  return v.includes('{q}') ? '' : 'q'
}

// addEngine 追加一个搜索引擎，名称（不区分大小写）已存在或已达上限时返回原数组
export function addEngine(engines: PanelEngine[], e: PanelEngine): PanelEngine[] {
  if (engines.length >= MAX_ENGINES || engines.some((x) => x.name.trim().toLowerCase() === e.name.trim().toLowerCase())) {
    return engines
  }
  return [...engines, { ...e }]
}

// removeEngine 删除第 i 个引擎并让默认选项仍指向同一个引擎：默认的被删除时回到本站（0）
export function removeEngine(engines: PanelEngine[], def: number, i: number): { engines: PanelEngine[]; def: number } {
  if (i < 0 || i >= engines.length) {
    return { engines, def }
  }
  const next = engines.filter((_, j) => j !== i)
  const picked = def - 1
  return { engines: next, def: picked === i ? 0 : picked > i ? def - 1 : def }
}

// moveEngine 把第 i 个引擎上移（delta=-1）或下移（delta=1），默认选项跟随其指向的引擎；越界时不变
export function moveEngine(engines: PanelEngine[], def: number, i: number, delta: -1 | 1): { engines: PanelEngine[]; def: number } {
  const to = i + delta
  if (i < 0 || i >= engines.length || to < 0 || to >= engines.length) {
    return { engines, def }
  }
  const next = [...engines]
  ;[next[i], next[to]] = [next[to], next[i]]
  const picked = def - 1
  return { engines: next, def: picked === i ? to + 1 : picked === to ? i + 1 : def }
}

export function emptyPanel(): Panel {
  return { name: '', enable: true, title: '', subtitle: '', icon: '', tabTitle: '', favicon: '', description: '', footer: '', footerUrl: '', hideFooter: false, theme: '', skin: '', engines: [], defaultEngine: 0, background: '', bgDim: 0, bgBlur: 0, cardStyle: '', iconSize: 0, columns: 0, mobileColumns: 0, gap: 0, maxWidth: 0, groupCount: false, altBadge: false, lanCidrs: [], links: [], remark: '' }
}

export function emptyLink(group = ''): PanelLink {
  return { name: '', group, description: '', url: '', lanUrl: '', alts: [], icon: '', color: '', sameTab: false, hide: false, size: '', hideName: false, hideDesc: false, authOnly: false }
}

// toPanel 去掉列表接口附带的只读字段（usedBy、key），得到可提交的面板；后端拒绝未知字段
export function toPanel(v: PanelView | Panel): Panel {
  return {
    name: v.name,
    enable: v.enable,
    title: v.title ?? '',
    subtitle: v.subtitle ?? '',
    icon: v.icon ?? '',
    tabTitle: v.tabTitle ?? '',
    favicon: v.favicon ?? '',
    description: v.description ?? '',
    footer: v.footer ?? '',
    footerUrl: v.footerUrl ?? '',
    hideFooter: v.hideFooter ?? false,
    theme: v.theme ?? '',
    skin: v.skin ?? '',
    engines: (v.engines ?? []).map((e) => ({ name: e.name, url: e.url })),
    defaultEngine: v.defaultEngine ?? 0,
    background: v.background ?? '',
    bgDim: v.bgDim ?? 0,
    bgBlur: v.bgBlur ?? 0,
    cardStyle: v.cardStyle === 'app' ? 'app' : '',
    iconSize: v.iconSize ?? 0,
    columns: v.columns ?? 0,
    mobileColumns: v.mobileColumns ?? 0,
    gap: v.gap ?? 0,
    maxWidth: v.maxWidth ?? 0,
    groupCount: v.groupCount ?? false,
    altBadge: v.altBadge ?? false,
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

// ---- 从 Sun-Panel 导入 ----

// mergeLinks 把导入的链接并入已有链接：同名分组的链接接在该组末尾，新分组排在最后；skipDuplicates 时跳过地址已存在的链接
// （包括导入内容中重复的地址）。超过 MAX_LINKS 的部分不加入，overflow 为未加入的数量
export function mergeLinks(existing: readonly PanelLink[], incoming: readonly PanelLink[], skipDuplicates: boolean): { links: PanelLink[]; added: number; duplicates: number; overflow: number } {
  const seen = new Set(existing.map((l) => l.url))
  let links = [...existing]
  let added = 0
  let duplicates = 0
  let overflow = 0
  for (const l of incoming) {
    if (skipDuplicates && seen.has(l.url)) {
      duplicates++
      continue
    }
    if (links.length >= MAX_LINKS) {
      overflow++
      continue
    }
    seen.add(l.url)
    links = insertLink(links, { ...emptyLink(), ...l, alts: [...(l.alts ?? [])] }).links
    added++
  }
  return { links, added, duplicates, overflow }
}

// freeName 返回未被占用的名称：base 未被占用时原样返回，否则依次加上 2、3……
export function freeName(base: string, taken: Iterable<string>): string {
  const used = new Set([...taken].map((n) => n.trim()))
  if (!used.has(base)) {
    return base
  }
  for (let n = 2; ; n++) {
    if (!used.has(`${base} ${n}`)) {
      return `${base} ${n}`
    }
  }
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
