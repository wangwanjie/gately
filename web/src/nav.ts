// 侧边栏导航与模块元信息：Layout 与总览页共用，保证图标与名称一致。
import { computed, ref, type Component } from 'vue'
import { t } from './i18n'
import {
  Activity,
  ArrowLeftRight,
  BookOpen,
  Bell,
  CalendarClock,
  ClipboardList,
  Cloud,
  Container,
  FolderOpen,
  FolderSync,
  FolderTree,
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
}

export interface NavGroup {
  // 分组标题键名，空串表示无标题
  titleKey: string
  items: NavItem[]
}

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
      { path: '/guard', titleKey: 'nav.items.guard.title', icon: ShieldBan, moduleId: 'guard', descKey: 'nav.items.guard.desc' },
    ],
  },
  {
    titleKey: 'nav.groups.tunnel',
    items: [
      { path: '/tailscale', titleKey: 'nav.items.tailscale.title', icon: Network, moduleId: 'tailscale', descKey: 'nav.items.tailscale.desc' },
      { path: '/frp', titleKey: 'nav.items.frp.title', icon: Waypoints, moduleId: 'frp', descKey: 'nav.items.frp.desc' },
      { path: '/cloudflared', titleKey: 'nav.items.cloudflared.title', icon: Cloud, moduleId: 'cloudflared', descKey: 'nav.items.cloudflared.desc' },
      { path: '/stun', titleKey: 'nav.items.stun.title', icon: Radar, moduleId: 'stun', descKey: 'nav.items.stun.desc' },
    ],
  },
  {
    titleKey: 'nav.groups.web',
    items: [
      { path: '/web', titleKey: 'nav.items.web.title', icon: Server, moduleId: 'webservice', descKey: 'nav.items.web.desc' },
      { path: '/waf', titleKey: 'nav.items.waf.title', icon: ShieldHalf, moduleId: 'waf', descKey: 'nav.items.waf.desc' },
      { path: '/ssl', titleKey: 'nav.items.ssl.title', icon: ShieldCheck, moduleId: 'ssl', descKey: 'nav.items.ssl.desc' },
      { path: '/webusers', titleKey: 'nav.items.webusers.title', icon: UserCog, anyModule: ['webservice'] },
    ],
  },
  {
    titleKey: 'nav.groups.storage',
    items: [
      { path: '/storage', titleKey: 'nav.items.storage.title', icon: HardDrive, moduleId: 'storage', descKey: 'nav.items.storage.desc' },
      { path: '/webdav', titleKey: 'nav.items.webdav.title', icon: FolderSync, moduleId: 'webdav', descKey: 'nav.items.webdav.desc' },
      { path: '/ftp', titleKey: 'nav.items.ftp.title', icon: Server, moduleId: 'ftp', descKey: 'nav.items.ftp.desc' },
      { path: '/smb', titleKey: 'nav.items.smb.title', icon: FolderTree, moduleId: 'smb', descKey: 'nav.items.smb.desc' },
      { path: '/filebrowser', titleKey: 'nav.items.filebrowser.title', icon: FolderOpen, moduleId: 'filebrowser', descKey: 'nav.items.filebrowser.desc' },
      { path: '/fileusers', titleKey: 'nav.items.fileusers.title', icon: UsersRound, anyModule: ['webdav', 'ftp', 'smb'] },
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
      { path: '/logs', titleKey: 'nav.items.logs.title', icon: ScrollText },
      { path: '/audit', titleKey: 'nav.items.audit.title', icon: ClipboardList },
      { path: '/history', titleKey: 'nav.items.history.title', icon: History },
      { path: '/api-tokens', titleKey: 'nav.items.apiTokens.title', icon: KeyRound },
      { path: '/api-docs', titleKey: 'nav.items.apiDocs.title', icon: BookOpen },
      { path: '/settings', titleKey: 'nav.items.settings.title', icon: Settings },
      { path: '/about', titleKey: 'nav.items.about.title', icon: Info },
    ],
  },
]

export const navItems: NavItem[] = navGroups.flatMap((g) => g.items)

// 本次启动中已关闭的功能模块 ID（来自 /api/session，重启后才会变化），对应的导航与页面全部隐藏
export const disabledModules = ref<string[]>([])

// moduleEnabled 判断模块是否开启；不属于任何模块的页面视为开启。
export function moduleEnabled(id?: string): boolean {
  return !id || !disabledModules.value.includes(id)
}

// itemEnabled 判断导航项是否显示：所属模块开启，多个模块共用的页面只要其中一个开启。
function itemEnabled(i: NavItem): boolean {
  if (i.anyModule) {
    return i.anyModule.some((id) => moduleEnabled(id))
  }
  return moduleEnabled(i.moduleId)
}

// visibleNavGroups 为去掉已关闭模块后的导航分组，空分组一并隐藏。
export const visibleNavGroups = computed(() =>
  navGroups.map((g) => ({ ...g, items: g.items.filter(itemEnabled) })).filter((g) => g.items.length > 0),
)

// pathEnabled 判断路由所属模块是否开启。
export function pathEnabled(path: string): boolean {
  const item = navByPath(path)
  return !item || itemEnabled(item)
}

// navByModule 按模块 ID 查找导航项。
export function navByModule(id: string): NavItem | undefined {
  return navItems.find((i) => i.moduleId === id)
}

// groupByPath 返回路由所在的导航分组名称（已翻译）。
export function groupByPath(path: string): string {
  const g = navGroups.find((g) => g.items.some((i) => path === i.path || path.startsWith(i.path + '/')))
  return g && g.titleKey ? t(g.titleKey) : ''
}

// navByPath 按路由路径查找导航项。
export function navByPath(path: string): NavItem | undefined {
  return navItems.find((i) => path === i.path || path.startsWith(i.path + '/'))
}
