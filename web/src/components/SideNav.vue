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
    <div class="groups">
      <div v-for="g in visibleNavGroups" :key="g.titleKey" class="group">
        <div v-if="g.titleKey && !collapsed" class="group-title">{{ t(g.titleKey) }}</div>
        <div v-else-if="g.titleKey" class="group-sep" />
        <el-tooltip v-for="it in g.items" :key="it.path" :content="t(it.titleKey)" placement="right" :disabled="!collapsed" :show-after="200">
          <router-link :to="it.path" class="nav-item" :class="{ active: isActive(it.path) }" @click="$emit('navigate')">
            <component :is="it.icon" :size="18" :stroke-width="isActive(it.path) ? 2.2 : 1.9" class="nav-icon" />
            <span v-if="!collapsed" class="nav-text">{{ t(it.titleKey) }}</span>
          </router-link>
        </el-tooltip>
      </div>
    </div>
    <AddFeaturesLink :collapsed="collapsed" @navigate="$emit('navigate')" />
  </nav>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLogo from './AppLogo.vue'
import AddFeaturesLink from './AddFeaturesLink.vue'
import { visibleNavGroups } from '../nav'

defineProps<{ collapsed?: boolean }>()
defineEmits<{ navigate: [] }>()

const route = useRoute()
const { t } = useI18n()

function isActive(path: string) {
  return route.path === path || route.path.startsWith(path + '/')
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
