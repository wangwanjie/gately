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
      'modules/ssl',
      'modules/web',
      'modules/oidc',
      'modules/waf',
      'modules/guard',
      'modules/tunnels',
      'modules/docker',
      'modules/terminal',
      'modules/nettools',
      'modules/notify',
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
