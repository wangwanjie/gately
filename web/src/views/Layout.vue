<template>
  <div class="layout" :class="{ collapsed }">
    <!-- 键盘与读屏用户跳过侧边栏的导航项，直接进入正文；平时隐藏，获得焦点时显示 -->
    <a href="#main-content" class="skip-link" @click.prevent="skipToContent">{{ t('layout.skipToContent') }}</a>
    <aside v-if="!isMobile" class="aside">
      <SideNav :collapsed="collapsed" @search="paletteOpen = true" />
      <button class="collapse-btn" :title="collapsed ? t('layout.expandSidebar') : t('layout.collapseSidebar')" @click="toggleCollapse">
        <PanelLeftOpen v-if="collapsed" :size="18" />
        <PanelLeftClose v-else :size="18" />
      </button>
    </aside>
    <el-drawer v-else v-model="drawer" direction="ltr" size="260px" :with-header="false" class="nav-drawer">
      <SideNav collapsible @navigate="drawer = false" @search="openPaletteFromDrawer" />
    </el-drawer>

    <div class="main">
      <!-- 在线演示实例的提示条，非演示实例不显示 -->
      <DemoBanner />
      <header class="header">
        <div class="header-left">
          <button v-if="isMobile" class="icon-btn" :aria-label="t('layout.openMenu')" @click="drawer = true">
            <Menu :size="20" />
          </button>
          <!-- 手机上页头滚走后也要知道在哪一页：顶栏显示当前页面名 -->
          <template v-if="isMobile">
            <AppLogo :size="26" />
            <span class="header-title">{{ route.meta.titleKey ? t(route.meta.titleKey as string) : 'Gately' }}</span>
          </template>
          <template v-else>
            <span v-if="group" class="crumb">{{ group }}</span>
            <ChevronRight v-if="group" :size="14" class="crumb-sep" />
            <span class="header-title">{{ t(route.meta.titleKey as string) }}</span>
          </template>
        </div>
        <div class="header-right">
          <!-- 手机上侧边栏收在抽屉里，顶栏放全局搜索入口 -->
          <button v-if="isMobile" class="icon-btn" :aria-label="t('palette.open')" aria-haspopup="dialog" data-test="header-search" @click="paletteOpen = true">
            <Search :size="20" />
          </button>
          <LocaleSwitcher :remember="role === 'admin'" />
          <ThemePicker />
          <el-dropdown trigger="click" popper-class="user-popper" @command="onCommand" @visible-change="lockScroll">
            <button class="user-btn">
              <span class="avatar">{{ (username || 'A').slice(0, 1).toUpperCase() }}</span>
              <span v-if="!isMobile" class="user-name">{{ username }}</span>
              <!-- 手机顶栏放不下文字标签，只读身份改用图标表示，留出位置显示页面名 -->
              <template v-if="role === 'readonly'">
                <Eye v-if="isMobile" :size="16" class="muted" :aria-label="t('layout.readonlyBadge')" role="img" data-test="readonly-icon" />
                <el-tag v-else size="small" type="info" effect="light">{{ t('layout.readonlyBadge') }}</el-tag>
              </template>
              <ChevronDown v-if="!isMobile" :size="14" class="muted" />
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <!-- 修改密码与系统设置只对管理员开放 -->
                <el-dropdown-item v-if="role === 'admin'" command="password" :icon="KeyRound">{{ t('layout.changePassword') }}</el-dropdown-item>
                <el-dropdown-item v-if="role === 'admin'" command="settings" :icon="Settings">{{ t('layout.systemSettings') }}</el-dropdown-item>
                <el-dropdown-item command="docs" :icon="BookOpen">{{ t('common.docs') }}</el-dropdown-item>
                <!-- API 文档不在侧边栏显示，只读账号进不了设置页，从这里打开 -->
                <el-dropdown-item command="apidocs" :icon="Braces">{{ t('nav.items.apiDocs.title') }}</el-dropdown-item>
                <el-dropdown-item command="logout" :icon="LogOut" divided>{{ t('layout.logout') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>
      <main id="main-content" ref="mainEl" class="content" tabindex="-1">
        <!-- 顶部提醒区：高度写入 --g-banners-h，供一屏布局的页面（如系统日志）扣除 -->
        <div ref="bannersEl" class="top-banners">
          <!-- 管理端安全评分不理想时提醒加固：高风险一直显示，建议加固级别可暂缓 7 天。默认密码与弱密码由强制修改对话框处理，不再重复提示 -->
          <SecurityBanner v-if="role === 'admin' && !passwordIssue" :status="secStatus" :level="bannerKind" @fix="goFix" @snooze="snoozeBanner" />
          <!-- 自动检查发现新版本或当前版本受安全问题影响时提示 -->
          <UpdateBanner :is-admin="role === 'admin'" />
        </div>
        <!-- pageKey 只在命令面板跳到当前页面并带新参数时变化，让只在挂载时读取参数的页面重新挂载 -->
        <router-view v-slot="{ Component }">
          <component :is="Component" :key="pageKey" />
        </router-view>
      </main>
    </div>
    <!-- 全局搜索（命令面板） -->
    <CommandPalette v-model="paletteOpen" />
    <!-- 首次使用引导的接口只对管理员开放，读取到角色后才挂载 -->
    <Onboarding v-if="sessionRole === 'admin' && !passwordIssue" />
    <!-- 默认密码或强度不足的密码：必须先修改，对话框无法关闭 -->
    <ForcePasswordDialog v-if="role === 'admin' && passwordIssue" :issue="passwordIssue" :username="username" @done="afterPasswordFixed" />
    <!-- 保存可能切断访问路径的配置后，确认不了时倒计时提示，超时服务端自动回滚 -->
    <ConfigConfirmDialog v-if="role === 'admin'" />
    <!-- 保存前提醒：修改会影响当前访问管理界面经过的转发 -->
    <AccessRiskDialog v-if="role === 'admin'" />
  </div>
</template>

<script setup lang="ts">
import Onboarding from '../components/Onboarding.vue'
import DemoBanner from '../components/DemoBanner.vue'
import UpdateBanner from '../components/UpdateBanner.vue'
import SecurityBanner from '../components/SecurityBanner.vue'
import ForcePasswordDialog from '../components/ForcePasswordDialog.vue'
import ConfigConfirmDialog from '../components/ConfigConfirmDialog.vue'
import AccessRiskDialog from '../components/AccessRiskDialog.vue'
import { loadConfirmStatus } from '../configConfirm'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { BookOpen, Braces, ChevronDown, ChevronRight, Eye, KeyRound, LogOut, Menu, PanelLeftClose, PanelLeftOpen, Search, Settings } from 'lucide-vue-next'
import { docsUrl } from '../docsUrl'
import { api } from '../api'
import { bannerLevel, fixTarget, SEC_SNOOZE_KEY, SEC_SNOOZE_MS, snoozeKey } from '../security'
import { loadSecurity, securityStatus as secStatus } from '../composables/useSecurity'
import SideNav from '../components/SideNav.vue'
import CommandPalette from '../components/CommandPalette.vue'
import { isPaletteShortcut, pageKey, paletteOpen } from '../composables/usePalette'
import AppLogo from '../components/AppLogo.vue'
import ThemePicker from '../components/ThemePicker.vue'
import LocaleSwitcher from '../components/LocaleSwitcher.vue'
import { absentModules, disabledModules, fallbackPath, groupByPath, pathEnabled, sessionRole } from '../nav'
import { useIsMobile } from '../composables/useIsMobile'
import { useScrollLock } from '../composables/useScrollLock'

const route = useRoute()
const { t, locale } = useI18n()
const router = useRouter()
const isMobile = useIsMobile()
const username = ref('')
const role = ref('admin')
// 主账号密码的问题：default 为仍在使用默认密码，weak 为强度不足；有问题时弹出无法关闭的修改对话框
const passwordIssue = ref<'' | 'default' | 'weak'>('')
// 加固提示的暂缓记录与当前时间（响应式，点击“暂缓”后立即隐藏）
const secSnooze = ref(localStorage.getItem(SEC_SNOOZE_KEY) ?? '')
const secNow = ref(Date.now())
const bannerKind = computed(() => bannerLevel(secStatus.value, { snooze: secSnooze.value, now: secNow.value }))

// 暂缓 7 天：出现新的待加固问题时记录失效，会重新提示
function snoozeBanner() {
  if (!secStatus.value) {
    return
  }
  // 当前时间与到期时间用同一个时刻计算，否则到期时间会因略晚于页面载入时刻而超出暂缓上限，被当作被篡改的记录
  const now = Date.now()
  const v = JSON.stringify({ key: snoozeKey(secStatus.value), until: now + SEC_SNOOZE_MS })
  localStorage.setItem(SEC_SNOOZE_KEY, v)
  secNow.value = now
  secSnooze.value = v
}

// 跳到设置页对应的位置去处理
function goFix(id: string) {
  const t = fixTarget(id)
  router.push({ path: '/settings', query: { tab: t.tab, focus: t.anchor } })
}
const drawer = ref(false)
const collapsed = ref(localStorage.getItem('gately-collapsed') === '1')
const group = computed(() => groupByPath(route.path))
// 用户菜单与语言、皮肤弹出层一致：打开期间页面不随滚轮滚动
const lockScroll = useScrollLock('.user-popper')

function toggleCollapse() {
  collapsed.value = !collapsed.value
  localStorage.setItem('gately-collapsed', collapsed.value ? '1' : '0')
}

// 切换页面时关闭移动端抽屉
watch(
  () => route.path,
  () => {
    drawer.value = false
  },
)

async function onCommand(cmd: string) {
  if (cmd === 'logout') {
    // 退出前二次确认，避免误触
    try {
      await ElMessageBox.confirm(t('layout.logoutConfirm'), t('layout.logoutTitle'), {
        type: 'warning',
        confirmButtonText: t('layout.logoutOk'),
        cancelButtonText: t('common.cancel'),
      })
    } catch {
      return
    }
    await api.post('/api/logout').catch(() => undefined)
    router.replace({ name: 'login' })
  } else if (cmd === 'password') {
    router.push({ path: '/settings', query: { tab: 'account' } })
  } else if (cmd === 'settings') {
    router.push('/settings')
  } else if (cmd === 'apidocs') {
    router.push('/api-docs')
  } else if (cmd === 'docs') {
    // 官网文档：当前页面有对应的模块文档时打开它，否则打开文档首页
    window.open(docsUrl(locale.value, '/docs/' + (pageDoc() || 'guide/quick-start')), '_blank', 'noopener')
  }
}

// 从手机抽屉里的搜索框打开命令面板：先收起抽屉，避免两层遮罩叠在一起
function openPaletteFromDrawer() {
  drawer.value = false
  paletteOpen.value = true
}

// Ctrl/⌘+K 打开或关闭命令面板；焦点在 Web 终端里时只响应 ⌘K（Ctrl+K 留给 Shell）
function onGlobalKeydown(e: KeyboardEvent) {
  const inTerminal = e.target instanceof Element && !!e.target.closest('.xterm')
  if (!isPaletteShortcut(e, inTerminal)) {
    return
  }
  e.preventDefault()
  paletteOpen.value = !paletteOpen.value
}
onMounted(() => window.addEventListener('keydown', onGlobalKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onGlobalKeydown))

// skipToContent 把焦点移到正文（hash 路由下不能用锚点跳转）
const mainEl = ref<HTMLElement>()
function skipToContent() {
  mainEl.value?.focus()
  mainEl.value?.scrollIntoView()
}

// pageDoc 返回当前页面对应的文档路径（读取页头上的文档链接），没有时为空
function pageDoc(): string {
  const a = document.querySelector<HTMLAnchorElement>('.page-head-doc')
  const m = a?.href.match(/\/docs\/(.+)$/)
  return m ? m[1] : ''
}

// 顶部提醒区（默认密码、版本更新）高度变化时写入 CSS 变量；margin 由 flow-root 包含在内
const bannersEl = ref<HTMLElement>()
let bannersObserver: ResizeObserver | undefined
onMounted(() => {
  if (!bannersEl.value || typeof ResizeObserver === 'undefined') {
    return
  }
  bannersObserver = new ResizeObserver(() => {
    document.documentElement.style.setProperty('--g-banners-h', `${bannersEl.value?.offsetHeight ?? 0}px`)
  })
  bannersObserver.observe(bannersEl.value)
})
onBeforeUnmount(() => {
  bannersObserver?.disconnect()
  document.documentElement.style.removeProperty('--g-banners-h')
})

// 读取会话信息；强制修改密码完成后也会再调用，刷新密码状态
async function loadSession() {
  const s = await api.get<{ username: string; role?: string; disabledModules?: string[]; absentModules?: string[]; passwordIssue?: '' | 'default' | 'weak' }>('/api/session')
  username.value = s.username
  role.value = s.role ?? 'admin'
  sessionRole.value = role.value
  passwordIssue.value = s.passwordIssue ?? ''
  disabledModules.value = s.disabledModules ?? []
  absentModules.value = s.absentModules ?? []
  return s
}

// 密码已改好并重新登录：刷新会话与安全评分，随后引导提示（如首次引导）照常出现
async function afterPasswordFixed() {
  await loadSession()
  loadSecurity()
}

onMounted(async () => {
  await loadSession()
  if (role.value === 'admin') {
    loadSecurity()
    // 待确认的配置变更：重启后自动确认、提示别处留下的变更或刚发生的回滚
    loadConfirmStatus()
    // 第三方登录、飞牛桌面入口等整页跳转进入时后端拿不到界面语言，这里上报一次；语言没变时后端不写文件
    api.put('/api/account/ui-language', { language: locale.value }).catch(() => undefined)
  }
  // 直接通过链接打开了已关闭模块的页面时，加载模块状态后再换到汇总页里可见的标签或总览
  if (!pathEnabled(route.path)) {
    router.replace(fallbackPath(route.path))
  }
})
</script>

<style scoped>
.layout {
  --aside-w: 228px;
  display: flex;
  min-height: 100%;
}
.layout.collapsed {
  --aside-w: 68px;
}
.aside {
  position: fixed;
  inset: 0 auto 0 0;
  width: var(--aside-w);
  border-right: 1px solid var(--g-border);
  background: var(--g-sidebar-bg);
  transition: width 0.2s ease;
  z-index: 20;
  display: flex;
  flex-direction: column;
}
.aside > :first-child {
  flex: 1;
  min-height: 0;
}
.collapse-btn {
  height: 44px;
  border: none;
  border-top: 1px solid var(--g-border);
  background: transparent;
  color: var(--el-text-color-secondary);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}
.collapse-btn:hover {
  color: var(--el-color-primary);
}
.main {
  flex: 1;
  min-width: 0;
  margin-left: var(--aside-w);
  transition: margin-left 0.2s ease;
  display: flex;
  flex-direction: column;
}
.header {
  position: sticky;
  top: 0;
  z-index: 10;
  height: var(--g-header-h);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 20px;
  border-bottom: 1px solid var(--g-border);
  background: var(--g-header-bg);
  backdrop-filter: saturate(180%) blur(10px);
  -webkit-backdrop-filter: saturate(180%) blur(10px);
}
.header-left,
.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
/* 右侧按钮不压缩，空间不足时由左侧页面名省略显示 */
.header-right {
  flex-shrink: 0;
}
.crumb {
  font-size: 14px;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}
.crumb-sep {
  color: var(--el-text-color-placeholder);
  flex-shrink: 0;
}
.header-title {
  font-weight: 600;
  font-size: 15px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.icon-btn {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  border: none;
  background: transparent;
  color: var(--el-text-color-regular);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}
.icon-btn:hover {
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
}
.user-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 40px;
  padding: 0 8px 0 4px;
  border-radius: 18px;
  border: 1px solid var(--g-border);
  background: var(--el-bg-color);
  color: var(--el-text-color-primary);
  cursor: pointer;
  font-size: 13px;
}
.user-btn:hover {
  border-color: var(--el-border-color);
}
.avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--el-color-primary);
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-weight: 600;
  font-size: 13px;
}
.user-name {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.skip-link {
  position: fixed;
  top: 8px;
  left: 8px;
  z-index: 3000;
  padding: 8px 14px;
  border-radius: 6px;
  font-size: 14px;
  color: #fff;
  background: var(--el-color-primary);
  text-decoration: none;
  transform: translateY(-200%);
}
.skip-link:focus {
  transform: none;
  outline: 2px solid var(--el-color-primary-dark-2);
  outline-offset: 2px;
}
.content:focus {
  outline: none;
}
.content {
  flex: 1;
  width: 100%;
  max-width: 1440px;
  margin: 0 auto;
  padding: var(--g-content-pt) var(--g-content-px) var(--g-content-pb);
  min-width: 0;
}
.top-banners {
  display: flow-root;
}
@media (max-width: 767.98px) {
  .main {
    margin-left: 0;
  }
  .header {
    padding: 0 8px 0 4px;
  }
}
</style>

<style>
.nav-drawer .el-drawer__body {
  padding: 0;
}
</style>
