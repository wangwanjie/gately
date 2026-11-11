import { watch } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import { i18n, t } from './i18n'
import { absentModules, disabledModules, pathEnabled, sessionRole } from './nav'
import { clearClips } from './components/terminal/clipboard'

const router = createRouter({
  // 使用 hash 路由，避免反向代理子路径部署时刷新 404
  history: createWebHashHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('./views/Login.vue'), meta: { titleKey: 'nav.login' } },
    {
      path: '/',
      component: () => import('./views/Layout.vue'),
      redirect: '/overview',
      children: [
        { path: 'overview', name: 'overview', component: () => import('./views/Overview.vue'), meta: { titleKey: 'nav.items.overview.title' } },
        { path: 'ddns', name: 'ddns', component: () => import('./views/DDNS.vue'), meta: { titleKey: 'nav.items.ddns.title' } },
        { path: 'stun', name: 'stun', component: () => import('./views/STUN.vue'), meta: { titleKey: 'nav.items.stun.title' } },
        { path: 'tailscale', name: 'tailscale', component: () => import('./views/Tailscale.vue'), meta: { titleKey: 'nav.items.tailscale.title' } },
        { path: 'netbird', name: 'netbird', component: () => import('./views/NetBird.vue'), meta: { titleKey: 'nav.items.netbird.title' } },
        { path: 'frp', name: 'frp', component: () => import('./views/Frp.vue'), meta: { titleKey: 'nav.items.frp.title' } },
        { path: 'cloudflared', name: 'cloudflared', component: () => import('./views/Cloudflared.vue'), meta: { titleKey: 'nav.items.cloudflared.title' } },
        { path: 'ssl', name: 'ssl', component: () => import('./views/SSL.vue'), meta: { titleKey: 'nav.items.ssl.title' } },
        { path: 'web', name: 'web', component: () => import('./views/Web.vue'), meta: { titleKey: 'nav.items.web.title' } },
        { path: 'webtopology', name: 'webtopology', component: () => import('./views/WebTopologyPage.vue'), meta: { titleKey: 'nav.items.webtopology.title' } },
        { path: 'webvisitors', name: 'webvisitors', component: () => import('./views/WebVisitors.vue'), meta: { titleKey: 'nav.items.webvisitors.title' } },
        { path: 'webpanels', name: 'webpanels', component: () => import('./views/WebPanels.vue'), meta: { titleKey: 'nav.items.webpanels.title' } },
        { path: 'webusers', name: 'webusers', component: () => import('./views/WebUsers.vue'), meta: { titleKey: 'nav.items.webusers.title' } },
        { path: 'portforward', name: 'portforward', component: () => import('./views/PortForward.vue'), meta: { titleKey: 'nav.items.portforward.title' } },
        { path: 'protoroute', name: 'protoroute', component: () => import('./views/ProtoRoute.vue'), meta: { titleKey: 'nav.items.protoroute.title' } },
        { path: 'waf', name: 'waf', component: () => import('./views/WAF.vue'), meta: { titleKey: 'nav.items.waf.title' } },
        { path: 'guard', name: 'guard', component: () => import('./views/Guard.vue'), meta: { titleKey: 'nav.items.guard.title' } },
        { path: 'knock', name: 'knock', component: () => import('./views/Knock.vue'), meta: { titleKey: 'nav.items.knock.title' } },
        { path: 'wol', name: 'wol', component: () => import('./views/WOL.vue'), meta: { titleKey: 'nav.items.wol.title' } },
        { path: 'terminal', name: 'terminal', component: () => import('./views/Terminal.vue'), meta: { titleKey: 'nav.items.terminal.title' } },
        { path: 'docker', name: 'docker', component: () => import('./views/Docker.vue'), meta: { titleKey: 'nav.items.docker.title' } },
        { path: 'webdav', name: 'webdav', component: () => import('./views/WebDAV.vue'), meta: { titleKey: 'nav.items.webdav.title' } },
        { path: 'storage', name: 'storage', component: () => import('./views/Storage.vue'), meta: { titleKey: 'nav.items.storage.title' } },
        { path: 'ftp', name: 'ftp', component: () => import('./views/FTP.vue'), meta: { titleKey: 'nav.items.ftp.title' } },
        { path: 'smb', name: 'smb', component: () => import('./views/SMB.vue'), meta: { titleKey: 'nav.items.smb.title' } },
        { path: 'filebrowser', name: 'filebrowser', component: () => import('./views/FileBrowser.vue'), meta: { titleKey: 'nav.items.filebrowser.title' } },
        { path: 'fileusers', name: 'fileusers', component: () => import('./views/FileServiceUsers.vue'), meta: { titleKey: 'nav.items.fileusers.title' } },
        { path: 'cron', name: 'cron', component: () => import('./views/Cron.vue'), meta: { titleKey: 'nav.items.cron.title' } },
        { path: 'nettools', name: 'nettools', component: () => import('./views/NetTools.vue'), meta: { titleKey: 'nav.items.nettools.title' } },
        { path: 'notify', name: 'notify', component: () => import('./views/Notify.vue'), meta: { titleKey: 'nav.items.notify.title' } },
        { path: 'perf', name: 'perf', component: () => import('./views/Perf.vue'), meta: { titleKey: 'nav.items.perf.title' } },
        { path: 'logs', name: 'logs', component: () => import('./views/Logs.vue'), meta: { titleKey: 'nav.items.logs.title' } },
        { path: 'audit', name: 'audit', component: () => import('./views/Audit.vue'), meta: { titleKey: 'nav.items.audit.title' } },
        { path: 'history', name: 'history', component: () => import('./views/History.vue'), meta: { titleKey: 'nav.items.history.title' } },
        { path: 'api-tokens', name: 'api-tokens', component: () => import('./views/ApiTokens.vue'), meta: { titleKey: 'nav.items.apiTokens.title' } },
        { path: 'api-docs', name: 'api-docs', component: () => import('./views/ApiDocs.vue'), meta: { titleKey: 'nav.items.apiDocs.title' } },
        { path: 'settings', name: 'settings', component: () => import('./views/Settings.vue'), meta: { titleKey: 'nav.items.settings.title' } },
        { path: 'about', name: 'about', component: () => import('./views/About.vue'), meta: { titleKey: 'nav.items.about.title' } },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/overview' },
  ],
})

// 按当前路由与界面语言刷新页面标题
function updateTitle() {
  const key = router.currentRoute.value.meta.titleKey as string | undefined
  document.title = key ? `${t(key)} - Gately` : 'Gately'
}

// 会话保存在 HttpOnly Cookie 中，前端无法直接判断是否已登录。首次进入登录页以外的页面前先请求一次
// /api/session，未登录时直接跳到登录页，避免先渲染主界面再因接口 401 跳转造成闪屏。
// 确认登录后不再重复检查（会话中途过期由接口的 401 处理）；进入登录页时清除标记，退出登录后会重新检查。
let sessionChecked = false

async function hasSession(): Promise<boolean> {
  try {
    // 直接用 fetch：api.ts 依赖本模块，这里引用它会形成循环依赖；子路径前缀由 basePath.ts 统一补上
    const resp = await fetch('/api/session', { credentials: 'same-origin' })
    // 只有明确的 401 视为未登录，网络错误等交给页面自身的接口处理
    if (resp.status === 401) {
      return false
    }
    if (resp.ok) {
      // 先记下角色与已关闭模块再放行：只读账号直接打开管理员页面（如刷新 #/settings）时，
      // 路由守卫即可跳回总览，避免页面先挂载、请求管理员接口得到 403
      const s = (await resp.json().catch(() => null)) as { role?: string; disabledModules?: string[]; absentModules?: string[] } | null
      if (s) {
        sessionRole.value = s.role === 'readonly' ? 'readonly' : 'admin'
        disabledModules.value = s.disabledModules ?? []
        absentModules.value = s.absentModules ?? []
      }
    }
    return true
  } catch {
    return true
  }
}

router.beforeEach(async (to) => {
  if (to.name === 'login') {
    sessionChecked = false
    // 退出登录后可能换另一身份登录，角色由 Layout 重新读取
    sessionRole.value = ''
    // 退出登录或会话失效：清掉 Web 终端的复制记录，避免同一标签页里下一位登录的用户看到
    clearClips()
    return
  }
  if (!sessionChecked) {
    if (!(await hasSession())) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }
    sessionChecked = true
  }
  // 已关闭模块的页面不可访问，直接回到总览（首次进入时模块状态尚未加载，由 Layout 加载后再检查一次）
  if (!pathEnabled(to.path)) {
    return '/overview'
  }
})

router.afterEach(() => {
  updateTitle()
})

// 切换语言后同步更新标题
watch(i18n.global.locale, () => {
  updateTitle()
})

export default router
