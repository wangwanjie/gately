<template>
  <div class="layout" :class="{ collapsed }">
    <aside v-if="!isMobile" class="aside">
      <SideNav :collapsed="collapsed" />
      <button class="collapse-btn" :title="collapsed ? t('layout.expandSidebar') : t('layout.collapseSidebar')" @click="toggleCollapse">
        <PanelLeftOpen v-if="collapsed" :size="18" />
        <PanelLeftClose v-else :size="18" />
      </button>
    </aside>
    <el-drawer v-else v-model="drawer" direction="ltr" size="260px" :with-header="false" class="nav-drawer">
      <SideNav @navigate="drawer = false" />
    </el-drawer>

    <div class="main">
      <header class="header">
        <div class="header-left">
          <button v-if="isMobile" class="icon-btn" :aria-label="t('layout.openMenu')" @click="drawer = true">
            <Menu :size="20" />
          </button>
          <template v-if="isMobile">
            <AppLogo :size="26" />
            <span class="header-title">Gately</span>
          </template>
          <template v-else>
            <span v-if="group" class="crumb">{{ group }}</span>
            <ChevronRight v-if="group" :size="14" class="crumb-sep" />
            <span class="header-title">{{ t(route.meta.titleKey as string) }}</span>
          </template>
        </div>
        <div class="header-right">
          <LocaleSwitcher :remember="role === 'admin'" />
          <ThemePicker />
          <el-dropdown trigger="click" popper-class="user-popper" @command="onCommand" @visible-change="lockScroll">
            <button class="user-btn">
              <span class="avatar">{{ (username || 'A').slice(0, 1).toUpperCase() }}</span>
              <span v-if="!isMobile" class="user-name">{{ username }}</span>
              <el-tag v-if="role === 'readonly'" size="small" type="info" effect="light">{{ t('layout.readonlyBadge') }}</el-tag>
              <ChevronDown :size="14" class="muted" />
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <!-- 修改密码与系统设置只对管理员开放 -->
                <el-dropdown-item v-if="role === 'admin'" command="password" :icon="KeyRound">{{ t('layout.changePassword') }}</el-dropdown-item>
                <el-dropdown-item v-if="role === 'admin'" command="settings" :icon="Settings">{{ t('layout.systemSettings') }}</el-dropdown-item>
                <el-dropdown-item command="logout" :icon="LogOut" :divided="role === 'admin'">{{ t('layout.logout') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>
      <main class="content">
        <!-- 顶部提醒区：高度写入 --g-banners-h，供一屏布局的页面（如系统日志）扣除 -->
        <div ref="bannersEl" class="top-banners">
          <!-- 管理端安全评分不理想时提醒加固：高风险一直显示，建议加固级别可暂缓 7 天。默认密码与弱密码由强制修改对话框处理，不再重复提示 -->
          <SecurityBanner v-if="role === 'admin' && !passwordIssue" :status="secStatus" :level="bannerKind" @fix="goFix" @snooze="snoozeBanner" />
          <!-- 自动检查发现新版本或当前版本受安全问题影响时提示 -->
          <UpdateBanner :is-admin="role === 'admin'" />
        </div>
        <router-view />
      </main>
    </div>
    <!-- 首次使用引导的接口只对管理员开放，读取到角色后才挂载 -->
    <Onboarding v-if="sessionRole === 'admin' && !passwordIssue" />
    <!-- 默认密码或强度不足的密码：必须先修改，对话框无法关闭 -->
    <ForcePasswordDialog v-if="role === 'admin' && passwordIssue" :issue="passwordIssue" :username="username" @done="afterPasswordFixed" />
  </div>
</template>

<script setup lang="ts">
import Onboarding from '../components/Onboarding.vue'
import UpdateBanner from '../components/UpdateBanner.vue'
import SecurityBanner from '../components/SecurityBanner.vue'
import ForcePasswordDialog from '../components/ForcePasswordDialog.vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { ChevronDown, ChevronRight, KeyRound, LogOut, Menu, PanelLeftClose, PanelLeftOpen, Settings } from 'lucide-vue-next'
import { api } from '../api'
import { bannerLevel, fixTarget, SEC_SNOOZE_KEY, SEC_SNOOZE_MS, snoozeKey } from '../security'
import { loadSecurity, securityStatus as secStatus } from '../composables/useSecurity'
import SideNav from '../components/SideNav.vue'
import AppLogo from '../components/AppLogo.vue'
import ThemePicker from '../components/ThemePicker.vue'
import LocaleSwitcher from '../components/LocaleSwitcher.vue'
import { absentModules, disabledModules, groupByPath, pathEnabled, sessionRole } from '../nav'
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
  }
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
    // 第三方登录、飞牛桌面入口等整页跳转进入时后端拿不到界面语言，这里上报一次；语言没变时后端不写文件
    api.put('/api/account/ui-language', { language: locale.value }).catch(() => undefined)
  }
  // 直接通过链接打开了已关闭模块的页面时，加载模块状态后再跳回总览
  if (!pathEnabled(route.path)) {
    router.replace('/overview')
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
  width: 36px;
  height: 36px;
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
  height: 36px;
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
