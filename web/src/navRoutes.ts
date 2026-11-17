// 导航收敛后的路由表（纯数据，不依赖 Vue，便于单元测试）。
// 多个相关页面合并为一个带标签的汇总页（Web 服务、组网、文件共享、日志），原页面组件原样作为标签内容；
// 合并前的旧地址全部重定向到新位置，外部书签、文档链接与页面内跳转都不会失效。

// hubTabs 为每个汇总页的标签路径，顺序即标签顺序；第一个可见标签是汇总页的默认落点
export const hubTabs: Record<string, string[]> = {
  '/web': ['/web', '/web/topology', '/web/visitors', '/web/panels'],
  '/mesh': ['/mesh/tailscale', '/mesh/netbird', '/mesh/easytier'],
  '/files': ['/files/webdav', '/files/ftp', '/files/smb', '/files/users'],
  '/logs': ['/logs/system', '/logs/audit', '/logs/history'],
}

// legacyRoutes 为旧地址到新地址的映射；查询参数与锚点由路由器原样带上（如 /logs?source=ddns）
export const legacyRoutes: Record<string, string> = {
  '/webtopology': '/web/topology',
  '/webvisitors': '/web/visitors',
  '/webpanels': '/web/panels',
  '/tailscale': '/mesh/tailscale',
  '/netbird': '/mesh/netbird',
  '/easytier': '/mesh/easytier',
  '/webdav': '/files/webdav',
  '/ftp': '/files/ftp',
  '/smb': '/files/smb',
  '/fileusers': '/files/users',
  '/audit': '/logs/audit',
  '/history': '/logs/history',
}

// hubOf 返回路径所在的汇总页（汇总页自身或它的某个标签），不属于任何汇总页时为空串
export function hubOf(path: string): string {
  for (const [hub, tabs] of Object.entries(hubTabs)) {
    if (path === hub || tabs.includes(path)) {
      return hub
    }
  }
  return ''
}

// firstTab 返回汇总页里第一个可进入的标签；全部不可进入时为空串
export function firstTab(hub: string, enabled: (path: string) => boolean): string {
  return (hubTabs[hub] ?? []).find((p) => enabled(p)) ?? ''
}

// resolveLegacy 把旧地址换成新地址；不是旧地址时原样返回
export function resolveLegacy(path: string): string {
  return legacyRoutes[path] ?? path
}
