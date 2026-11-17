// 侧边栏导航与模块元信息：Layout 与总览页共用，保证图标与名称一致。
import { computed, ref, type Component } from 'vue'
import { t } from './i18n'
import { firstTab, hubOf, hubTabs } from './navRoutes'
import {
  Activity,
  ArrowLeftRight,
  Bird,
  BookOpen,
  Bell,
  Cable,
  CalendarClock,
  ClipboardList,
  Cloud,
  Container,
  FolderOpen,
  FolderSync,
  FolderTree,
  Gauge,
  HardDrive,
  Globe,
  History,
  Info,
  KeyRound,
  LayoutDashboard,
  Network,
  Power,
  Radar,
  ScrollText,
  Server,
  Settings,
  ShieldBan,
  ShieldCheck,
  ShieldHalf,
  Split,
  SquareTerminal,
  UserCog,
  UsersRound,
  Waypoints,
  Workflow,
  Footprints,
  LayoutGrid,
} from 'lucide-vue-next'

export interface NavItem {
  path: string
  // 文案键名（nav.json），渲染处用 t() 翻译
  titleKey: string
  icon: Component
  // 对应后端模块 ID（/api/status 中的 modules[].id）
  moduleId?: string
  // 不属于单个模块、由多个模块共用的页面：其中任一模块开启时显示
  anyModule?: string[]
  // 总览页卡片上的一句话说明
  descKey?: string
  // 只有管理员可用的页面（接口全部需要管理员权限），只读账号不显示也不能进入
  adminOnly?: boolean
  // 作为标签显示在哪个汇总页里（汇总页路径，见 navRoutes.ts）；标签页不单独出现在侧边栏，
  // 路径与汇总页相同的标签（如 Web 服务的「站点」）兼作侧边栏入口
  hub?: string
  // 汇总页标签上的短名称键名，默认用 titleKey
  tabKey?: string
  // 汇总页的侧边栏入口：本身没有内容，打开时落到第一个可见标签；全部标签都不可见时隐藏
  hubRoot?: boolean
  // 不在侧边栏显示的页面（从设置页、用户菜单或全局搜索进入）
  hidden?: boolean
}

export interface NavGroup {
  // 分组标题键名，空串表示无标题
  titleKey: string
  items: NavItem[]
}

// navGroups 列出全部页面（含汇总页里的标签与不在侧边栏显示的页面），按侧边栏分组与顺序排列；
// 设置页的功能模块列表、日志来源等也按这里的分组与顺序展示
export const navGroups: NavGroup[] = [
  {
    titleKey: '',
    items: [{ path: '/overview', titleKey: 'nav.items.overview.title', icon: LayoutDashboard }],
  },
  {
    titleKey: 'nav.groups.network',
    items: [
      { path: '/portforward', titleKey: 'nav.items.portforward.title', icon: ArrowLeftRight, moduleId: 'portforward', descKey: 'nav.items.portforward.desc' },
      { path: '/protoroute', titleKey: 'nav.items.protoroute.title', icon: Split, moduleId: 'protoroute', descKey: 'nav.items.protoroute.desc' },
      { path: '/ddns', titleKey: 'nav.items.ddns.title', icon: Globe, moduleId: 'ddns', descKey: 'nav.items.ddns.desc' },
      { path: '/stun', titleKey: 'nav.items.stun.title', icon: Radar, moduleId: 'stun', descKey: 'nav.items.stun.desc' },
    ],
  },
  {
    titleKey: 'nav.groups.security',
    items: [
      { path: '/guard', titleKey: 'nav.items.guard.title', icon: ShieldBan, moduleId: 'guard', descKey: 'nav.items.guard.desc' },
      { path: '/knock', titleKey: 'nav.items.knock.title', icon: KeyRound, moduleId: 'knock', descKey: 'nav.items.knock.desc' },
      { path: '/waf', titleKey: 'nav.items.waf.title', icon: ShieldHalf, moduleId: 'waf', descKey: 'nav.items.waf.desc' },
    ],
  },
  {
    titleKey: 'nav.groups.web',
    items: [
      { path: '/web', titleKey: 'nav.items.web.title', icon: Server, moduleId: 'webservice', descKey: 'nav.items.web.desc', hub: '/web', tabKey: 'nav.tabs.webSites' },
      { path: '/web/topology', titleKey: 'nav.items.webtopology.title', icon: Workflow, anyModule: ['webservice'], hub: '/web' },
      { path: '/web/visitors', titleKey: 'nav.items.webvisitors.title', icon: Footprints, anyModule: ['webservice'], hub: '/web' },
      { path: '/web/panels', titleKey: 'nav.items.webpanels.title', icon: LayoutGrid, anyModule: ['webservice'], hub: '/web' },
      { path: '/ssl', titleKey: 'nav.items.ssl.title', icon: ShieldCheck, moduleId: 'ssl', descKey: 'nav.items.ssl.desc' },
      { path: '/webusers', titleKey: 'nav.items.webusers.title', icon: UserCog, anyModule: ['webservice'] },
    ],
  },
  {
    titleKey: 'nav.groups.tunnel',
    items: [
      { path: '/mesh', titleKey: 'nav.items.mesh.title', icon: Network, hubRoot: true },
      { path: '/mesh/tailscale', titleKey: 'nav.items.tailscale.title', icon: Network, moduleId: 'tailscale', descKey: 'nav.items.tailscale.desc', hub: '/mesh' },
      { path: '/mesh/netbird', titleKey: 'nav.items.netbird.title', icon: Bird, moduleId: 'netbird', descKey: 'nav.items.netbird.desc', hub: '/mesh' },
      { path: '/mesh/easytier', titleKey: 'nav.items.easytier.title', icon: Cable, moduleId: 'easytier', descKey: 'nav.items.easytier.desc', hub: '/mesh' },
      { path: '/frp', titleKey: 'nav.items.frp.title', icon: Waypoints, moduleId: 'frp', descKey: 'nav.items.frp.desc' },
      { path: '/cloudflared', titleKey: 'nav.items.cloudflared.title', icon: Cloud, moduleId: 'cloudflared', descKey: 'nav.items.cloudflared.desc' },
    ],
  },
  {
    titleKey: 'nav.groups.storage',
    items: [
      { path: '/files', titleKey: 'nav.items.files.title', icon: FolderSync, hubRoot: true },
      { path: '/files/webdav', titleKey: 'nav.items.webdav.title', icon: FolderSync, moduleId: 'webdav', descKey: 'nav.items.webdav.desc', hub: '/files', tabKey: 'nav.tabs.webdav' },
      { path: '/files/ftp', titleKey: 'nav.items.ftp.title', icon: Server, moduleId: 'ftp', descKey: 'nav.items.ftp.desc', hub: '/files', tabKey: 'nav.tabs.ftp' },
      { path: '/files/smb', titleKey: 'nav.items.smb.title', icon: FolderTree, moduleId: 'smb', descKey: 'nav.items.smb.desc', hub: '/files', tabKey: 'nav.tabs.smb' },
      { path: '/files/users', titleKey: 'nav.items.fileusers.title', icon: UsersRound, anyModule: ['webdav', 'ftp', 'smb'], hub: '/files', tabKey: 'nav.tabs.fileusers' },
      { path: '/storage', titleKey: 'nav.items.storage.title', icon: HardDrive, moduleId: 'storage', descKey: 'nav.items.storage.desc' },
      { path: '/filebrowser', titleKey: 'nav.items.filebrowser.title', icon: FolderOpen, moduleId: 'filebrowser', descKey: 'nav.items.filebrowser.desc' },
    ],
  },
  {
    titleKey: 'nav.groups.tools',
    items: [
      { path: '/wol', titleKey: 'nav.items.wol.title', icon: Power, moduleId: 'wol', descKey: 'nav.items.wol.desc' },
      { path: '/terminal', titleKey: 'nav.items.terminal.title', icon: SquareTerminal, moduleId: 'webterminal', descKey: 'nav.items.terminal.desc' },
      { path: '/nettools', titleKey: 'nav.items.nettools.title', icon: Activity, moduleId: 'nettools', descKey: 'nav.items.nettools.desc' },
      { path: '/docker', titleKey: 'nav.items.docker.title', icon: Container, moduleId: 'docker', descKey: 'nav.items.docker.desc' },
      { path: '/cron', titleKey: 'nav.items.cron.title', icon: CalendarClock, moduleId: 'cron', descKey: 'nav.items.cron.desc' },
    ],
  },
  {
    titleKey: 'nav.groups.system',
    items: [
      { path: '/notify', titleKey: 'nav.items.notify.title', icon: Bell, moduleId: 'notify', descKey: 'nav.items.notify.desc' },
      { path: '/perf', titleKey: 'nav.items.perf.title', icon: Gauge, moduleId: 'perf', descKey: 'nav.items.perf.desc' },
      { path: '/logs', titleKey: 'nav.items.logsHub.title', icon: ScrollText, hubRoot: true },
      { path: '/logs/system', titleKey: 'nav.items.logs.title', icon: ScrollText, hub: '/logs' },
      { path: '/logs/audit', titleKey: 'nav.items.audit.title', icon: ClipboardList, adminOnly: true, hub: '/logs' },
      { path: '/logs/history', titleKey: 'nav.items.history.title', icon: History, adminOnly: true, hub: '/logs' },
      { path: '/settings', titleKey: 'nav.items.settings.title', icon: Settings, adminOnly: true },
      { path: '/api-tokens', titleKey: 'nav.items.apiTokens.title', icon: KeyRound, adminOnly: true, hidden: true },
      { path: '/api-docs', titleKey: 'nav.items.apiDocs.title', icon: BookOpen, hidden: true },
      { path: '/about', titleKey: 'nav.items.about.title', icon: Info },
    ],
  },
]

export const navItems: NavItem[] = navGroups.flatMap((g) => g.items)

// 本次启动中已关闭的功能模块 ID（来自 /api/session，重启后才会变化），对应的导航与页面全部隐藏
export const disabledModules = ref<string[]>([])

// 定制构建中没有编译进来的功能模块 ID（来自 /api/session），与已关闭的模块一样隐藏
export const absentModules = ref<string[]>([])

// 当前登录身份的角色（来自 /api/session）：'' 表示尚未读取，'admin' 或 'readonly'
export const sessionRole = ref('')

// isReadonly 为当前身份是否为只读账号；角色未读取时按管理员处理，避免误隐藏页面
export const isReadonly = computed(() => sessionRole.value === 'readonly')

// moduleBuilt 判断模块是否编译进来（不论是否开启）。
export function moduleBuilt(id: string): boolean {
  return !absentModules.value.includes(id)
}

// moduleEnabled 判断模块是否编译进来且已开启；不属于任何模块的页面视为开启。
export function moduleEnabled(id?: string): boolean {
  return !id || (!disabledModules.value.includes(id) && !absentModules.value.includes(id))
}

// hubTabItems 返回汇总页的全部标签（不论是否可见），顺序与 navRoutes.ts 的 hubTabs 一致。
export function hubTabItems(hub: string): NavItem[] {
  return (hubTabs[hub] ?? []).map((p) => navItems.find((i) => i.path === p)).filter((i): i is NavItem => !!i)
}

// itemEnabled 判断页面是否可用：所属模块开启，多个模块共用的页面只要其中一个开启；只读账号不显示管理员页面；
// 汇总页入口只要有一个标签可用就显示。
export function itemEnabled(i: NavItem): boolean {
  if (i.adminOnly && isReadonly.value) {
    return false
  }
  if (i.hubRoot) {
    return hubTabItems(i.path).some(itemEnabled)
  }
  if (i.anyModule) {
    return i.anyModule.some((id) => moduleEnabled(id))
  }
  return moduleEnabled(i.moduleId)
}

// inSidebar 判断页面是否作为侧边栏入口：汇总页里的标签与隐藏页面不单独显示，路径与汇总页相同的标签除外
function inSidebar(i: NavItem): boolean {
  if (i.hidden) {
    return false
  }
  return !i.hub || i.hub === i.path
}

// visibleNavGroups 为侧边栏显示的分组：只含侧边栏入口，去掉已关闭模块后的空分组一并隐藏。
export const visibleNavGroups = computed(() =>
  navGroups.map((g) => ({ ...g, items: g.items.filter((i) => inSidebar(i) && itemEnabled(i)) })).filter((g) => g.items.length > 0),
)

// pathEnabled 判断路由是否可以进入：所属模块开启，且只读账号不能进入管理员页面。
export function pathEnabled(path: string): boolean {
  const item = navByPath(path)
  return !item || itemEnabled(item)
}

// fallbackPath 返回不能进入 path 时的去处：汇总页里还有可见标签时落到第一个可见标签，否则回到总览
export function fallbackPath(path: string): string {
  const hub = hubOf(path)
  return (hub && firstTab(hub, pathEnabled)) || '/overview'
}

// navByModule 按模块 ID 查找导航项。
export function navByModule(id: string): NavItem | undefined {
  return navItems.find((i) => i.moduleId === id)
}

// groupOfItem 返回页面所在的导航分组
function groupOfItem(item: NavItem): NavGroup | undefined {
  return navGroups.find((g) => g.items.includes(item))
}

// groupByPath 返回路由所在的导航分组名称（已翻译）。
export function groupByPath(path: string): string {
  const item = navByPath(path)
  const g = item && groupOfItem(item)
  return g && g.titleKey ? t(g.titleKey) : ''
}

// navByPath 按路由路径查找导航项：先精确匹配，再取最长的前缀匹配（如 /web/topology 不会落到 /web）。
export function navByPath(path: string): NavItem | undefined {
  const exact = navItems.find((i) => i.path === path)
  if (exact) {
    return exact
  }
  let best: NavItem | undefined
  for (const i of navItems) {
    if (path.startsWith(i.path + '/') && (!best || i.path.length > best.path.length)) {
      best = i
    }
  }
  return best
}
