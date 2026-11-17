<template>
  <!-- 分组侧边导航：桌面侧栏与移动端抽屉共用 -->
  <nav class="side-nav" :class="{ collapsed }" :aria-label="t('sideNav.ariaLabel')">
    <div class="brand">
      <AppLogo :size="30" />
      <div v-if="!collapsed" class="brand-text">
        <div class="brand-name">Gately</div>
        <div class="brand-sub">{{ t('sideNav.brandSub') }}</div>
      </div>
    </div>
    <!-- 全局搜索入口：点击打开命令面板，快捷键 Ctrl/⌘+K -->
    <div class="search-row">
      <el-tooltip :content="searchLabel" placement="right" :disabled="!collapsed" :show-after="200">
        <button type="button" class="search-btn" :aria-label="searchLabel" aria-haspopup="dialog" data-test="nav-search" @click="$emit('search')">
          <Search :size="16" class="nav-icon" aria-hidden="true" />
          <template v-if="!collapsed">
            <span class="search-text">{{ t('palette.open') }}</span>
            <!-- 手机抽屉里没有实体键盘，不显示快捷键 -->
            <kbd v-if="!collapsible" class="search-kbd">{{ shortcut }}</kbd>
          </template>
        </button>
      </el-tooltip>
    </div>
    <div class="groups">
      <div v-for="g in visibleNavGroups" :key="g.titleKey" class="group">
        <!-- 手机抽屉里分组可折叠，默认只展开当前页面所在的分组 -->
        <button
          v-if="g.titleKey && !collapsed && collapsible"
          type="button"
          class="group-title group-toggle"
          :aria-expanded="isOpen(g.titleKey)"
          @click="toggleGroup(g.titleKey)"
        >
          <span>{{ t(g.titleKey) }}</span>
          <ChevronRight :size="14" class="group-chevron" :class="{ open: isOpen(g.titleKey) }" aria-hidden="true" />
        </button>
        <div v-else-if="g.titleKey && !collapsed" class="group-title">{{ t(g.titleKey) }}</div>
        <div v-else-if="g.titleKey" class="group-sep" />
        <template v-if="!collapsible || !g.titleKey || isOpen(g.titleKey)">
          <el-tooltip v-for="it in g.items" :key="it.path" :content="t(it.titleKey)" placement="right" :disabled="!collapsed" :show-after="200">
            <router-link :to="it.path" class="nav-item" :class="{ active: isActive(it.path) }" @click="$emit('navigate')">
              <component :is="it.icon" :size="18" :stroke-width="isActive(it.path) ? 2.2 : 1.9" class="nav-icon" />
              <span v-if="!collapsed" class="nav-text">{{ t(it.titleKey) }}</span>
            </router-link>
          </el-tooltip>
        </template>
      </div>
    </div>
    <AddFeaturesLink :collapsed="collapsed" @navigate="$emit('navigate')" />
  </nav>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ChevronRight, Search } from 'lucide-vue-next'
import AppLogo from './AppLogo.vue'
import AddFeaturesLink from './AddFeaturesLink.vue'
import { navByPath, navGroups, visibleNavGroups } from '../nav'

// collapsible 为分组是否可折叠（手机抽屉）
const props = defineProps<{ collapsed?: boolean; collapsible?: boolean }>()
defineEmits<{ navigate: []; search: [] }>()

const route = useRoute()
const { t } = useI18n()

// 快捷键提示按系统显示：macOS / iOS 为 ⌘K，其他为 Ctrl K
const shortcut = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent) ? '⌘K' : 'Ctrl K'
const searchLabel = computed(() => `${t('palette.open')} (${shortcut})`)

function isActive(path: string) {
  return route.path === path || route.path.startsWith(path + '/')
}

// currentGroup 返回当前页面所在分组的键名（不在任何分组时为空串）
function currentGroup(): string {
  const item = navByPath(route.path)
  return navGroups.find((g) => item && g.items.includes(item))?.titleKey ?? ''
}

// 展开的分组：默认全部展开，用户可以手动收起；切换页面时保留手动收起的状态，只确保当前页面所在分组展开
const openGroups = ref(new Set<string>(navGroups.map((g) => g.titleKey).filter(Boolean)))
watch(
  () => route.path,
  () => {
    const cur = currentGroup()
    if (cur && !openGroups.value.has(cur)) {
      const next = new Set(openGroups.value)
      next.add(cur)
      openGroups.value = next
    }
  },
)

function isOpen(key: string) {
  return !props.collapsible || openGroups.value.has(key)
}

function toggleGroup(key: string) {
  const next = new Set(openGroups.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  openGroups.value = next
}
</script>

<style scoped>
.side-nav {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--g-sidebar-bg);
}
.brand {
  height: 60px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 18px;
  flex-shrink: 0;
}
.collapsed .brand {
  justify-content: center;
  padding: 0;
}
.brand-name {
  font-weight: 700;
  font-size: 17px;
  letter-spacing: 0.2px;
  line-height: 1.2;
}
.brand-sub {
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
.search-row {
  padding: 2px 10px 6px;
  flex-shrink: 0;
}
.search-btn {
  width: 100%;
  height: 36px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px 0 10px;
  border: 1px solid var(--g-border);
  border-radius: 8px;
  background: var(--el-bg-color);
  color: var(--el-text-color-secondary);
  font-size: 13px;
  cursor: pointer;
  transition: border-color 0.15s;
}
.search-btn:hover {
  border-color: var(--el-color-primary-light-5);
  color: var(--el-text-color-regular);
}
.search-btn:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 1px;
}
.collapsed .search-btn {
  justify-content: center;
  padding: 0;
}
.search-text {
  flex: 1;
  min-width: 0;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.search-kbd {
  flex-shrink: 0;
  padding: 1px 5px;
  border: 1px solid var(--g-border);
  border-radius: 4px;
  font-family: inherit;
  font-size: 11px;
  background: var(--el-fill-color-light);
}
.groups {
  flex: 1;
  overflow-y: auto;
  padding: 4px 10px 16px;
}
.group-title {
  font-size: 11px;
  font-weight: 600;
  /* 11px 的分组标题用次要文字色，占位符色在侧栏底色上只有约 2.3:1 */
  color: var(--el-text-color-secondary);
  letter-spacing: 0.5px;
  padding: 14px 10px 6px;
}
.group-toggle {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border: none;
  background: transparent;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
  /* 手指点击区域不小于 40px */
  min-height: 40px;
  padding-top: 8px;
  padding-bottom: 8px;
  margin-top: 4px;
  border-radius: 8px;
}
.group-toggle:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: -2px;
}
.group-chevron {
  transition: transform 0.15s;
}
.group-chevron.open {
  transform: rotate(90deg);
}
.group-sep {
  height: 1px;
  margin: 10px 8px;
  background: var(--g-border);
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 38px;
  padding: 0 10px;
  margin: 2px 0;
  border-radius: 8px;
  color: var(--el-text-color-regular);
  text-decoration: none;
  font-size: 14px;
  transition:
    background 0.15s,
    color 0.15s;
}
.collapsed .nav-item {
  justify-content: center;
  padding: 0;
}
.nav-item:hover {
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
}
.nav-item.active {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  font-weight: 600;
}
.nav-icon {
  flex-shrink: 0;
}
.nav-text {
  white-space: nowrap;
}
</style>
