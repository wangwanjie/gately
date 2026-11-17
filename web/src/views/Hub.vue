<template>
  <!-- 汇总页外壳：顶部为标签栏，下方原样渲染各标签对应的原页面组件；只有一个可见标签时不显示标签栏 -->
  <div class="hub">
    <nav v-if="tabs.length > 1" class="hub-tabs" :aria-label="title">
      <router-link
        v-for="it in tabs"
        :key="it.path"
        :to="it.path"
        class="hub-tab"
        :class="{ active: route.path === it.path }"
        :aria-current="route.path === it.path ? 'page' : undefined"
        :data-tab="it.path"
      >
        {{ t(it.tabKey ?? it.titleKey) }}
      </router-link>
    </nav>
    <router-view />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { hubTabItems, itemEnabled, navByPath } from '../nav'

const route = useRoute()
const { t } = useI18n()

// hub 为当前汇总页路径（路由 meta.hub），tabs 为其中可见的标签：已关闭模块与只读账号不能进入的标签隐藏
const hub = computed(() => (route.meta.hub as string | undefined) ?? '')
const tabs = computed(() => hubTabItems(hub.value).filter(itemEnabled))
// 标签栏的可访问名称用汇总页在侧边栏里的名称
const title = computed(() => {
  const item = navByPath(hub.value)
  return item ? t(item.titleKey) : ''
})
</script>

<style scoped>
.hub-tabs {
  display: flex;
  gap: 4px;
  margin: -4px 0 14px;
  border-bottom: 1px solid var(--g-border);
  /* 标签较多或文字较长时在标签栏内部横向滚动，不撑宽页面 */
  overflow-x: auto;
  scrollbar-width: none;
}
.hub-tabs::-webkit-scrollbar {
  display: none;
}
.hub-tab {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  height: 40px;
  padding: 0 12px;
  margin-bottom: -1px;
  border-bottom: 2px solid transparent;
  color: var(--el-text-color-regular);
  text-decoration: none;
  font-size: 14px;
  white-space: nowrap;
  transition:
    color 0.15s,
    border-color 0.15s;
}
.hub-tab:hover {
  color: var(--el-color-primary);
}
.hub-tab.active {
  color: var(--el-color-primary);
  border-bottom-color: var(--el-color-primary);
  font-weight: 600;
}
.hub-tab:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: -2px;
  border-radius: 6px 6px 0 0;
}
</style>
