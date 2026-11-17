// 官网多语言的公共结构：语言清单与侧边栏骨架（各语言只提供标题文案）。

export interface SiteLocale {
  // 语言代码，同时是 i18n/<code>.json 的文件名
  code: string
  // URL 前缀目录；简体中文是站点根，其余语言放在同名子目录
  dir: string
  // 浏览器语言前缀匹配（小写），按序命中
  match: string[]
}

export const SOURCE_CODE = 'zh-CN'
export const FALLBACK_CODE = 'en'

export const siteLocales: SiteLocale[] = [
  { code: 'zh-CN', dir: '', match: ['zh-cn', 'zh-sg', 'zh-hans', 'zh'] },
  { code: 'en', dir: 'en', match: ['en'] },
  { code: 'zh-TW', dir: 'zh-TW', match: ['zh-tw', 'zh-hk', 'zh-mo', 'zh-hant'] },
  { code: 'ja', dir: 'ja', match: ['ja'] },
  { code: 'ko', dir: 'ko', match: ['ko'] },
  { code: 'es', dir: 'es', match: ['es'] },
  { code: 'fr', dir: 'fr', match: ['fr'] },
  { code: 'de', dir: 'de', match: ['de'] },
  { code: 'ru', dir: 'ru', match: ['ru'] },
  { code: 'pt-BR', dir: 'pt-BR', match: ['pt'] }
]

export interface SidebarGroup {
  id: string
  slugs: string[]
  // 仅列出该语言下实际存在页面的条目，全部缺失时整组不显示；
  // 教程目前只有部分语言提供，其余语言不会出现指向不存在页面的链接
  onlyExisting?: boolean
}

export const sidebarGroups: SidebarGroup[] = [
  {
    id: 'start',
    slugs: [
      'guide/introduction',
      'guide/quick-start',
      'guide/install',
      'guide/configuration',
      'guide/lists',
      'guide/cli-mcp',
      'guide/migration',
      'guide/benchmarks',
      'guide/faq'
    ]
  },
  {
    id: 'modules',
    slugs: [
      'modules/index',
      'modules/ddns',
      'modules/stun',
      'modules/portforward',
      'modules/protoroute',
      'modules/ssl',
      'modules/web',
      'modules/oidc',
      'modules/waf',
      'modules/guard',
      'modules/knock',
      'modules/tunnels',
      'modules/docker',
      'modules/terminal',
      'modules/nettools',
      'modules/notify',
      'modules/perf',
      'modules/backup',
      'modules/tools'
    ]
  },
  { id: 'security', slugs: ['guide/security', 'guide/audit', 'guide/hardening', 'guide/verify'] },
  {
    id: 'reference',
    slugs: [
      'reference/api',
      'reference/api-reference',
      'reference/docker-api',
      'reference/webterminal-api',
      'reference/diagnostics',
      'reference/monitoring',
      'guide/contributing'
    ]
  }
]

// 教程（顶栏「教程」）的侧边栏骨架：按难度分三组，从入门到实战场景。
// 只列出该语言实际存在的页面；某语言没有 docs/tutorials/index.md 时不显示「教程」入口。
export const tutorialGroups: SidebarGroup[] = [
  {
    id: 'basics',
    onlyExisting: true,
    slugs: [
      'tutorials/getting-started',
      'tutorials/first-site',
      'tutorials/https-certificate',
      'tutorials/ddns-basics',
      'tutorials/port-forward-basics'
    ]
  },
  {
    id: 'advanced',
    onlyExisting: true,
    slugs: [
      'tutorials/site-access-control',
      'tutorials/guard-waf',
      'tutorials/notify-alerts',
      'tutorials/history-backup',
      'tutorials/navigation-panel',
      'tutorials/stun-traversal',
      'tutorials/frp-tunnel',
      'tutorials/mesh-vpn',
      'tutorials/port-knocking',
      'tutorials/docker-terminal',
      'tutorials/oidc-sso',
      'tutorials/api-automation'
    ]
  },
  {
    id: 'scenarios',
    onlyExisting: true,
    slugs: [
      'tutorials/home-nas-end-to-end',
      'tutorials/remote-access-without-public-ip',
      'tutorials/one-port-443',
      'tutorials/https-and-login-for-apps',
      'tutorials/wildcard-cert-ddns',
      'tutorials/vps-front-door',
      'tutorials/migrate-from-nginx',
      'tutorials/replace-haproxy-nginx-fail2ban'
    ]
  }
]
