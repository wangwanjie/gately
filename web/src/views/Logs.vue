<template>
  <div class="logs-page">
    <PageHeader :title="t('logs.title')" :desc="t('logs.desc')" :icon="ScrollText" />
    <!-- 来源较多，单独占一行并允许横向滚动，避免被页头的 flex 布局撑出页面 -->
    <div class="sources-row">
      <el-segmented v-model="source" :options="sources" class="sources" />
    </div>
    <div class="g-card logs-card">
      <div class="g-card-body">
        <!-- 切换来源时重建组件，重新从头拉取对应日志 -->
        <LogViewer :key="url" :url="url" :clearable="source !== 'system'" fill />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ScrollText } from 'lucide-vue-next'
import PageHeader from '../components/PageHeader.vue'
import LogViewer from '../components/LogViewer.vue'
import { moduleEnabled, navItems } from '../nav'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

// 来源：系统日志 + 每个后端模块的日志
// 导航项标题以键名（titleKey）给出，渲染时再翻译
function navTitle(i: { titleKey: string }): string {
  return t(i.titleKey)
}
const sources = computed(() => [{ label: t('logs.system'), value: 'system' }, ...navItems.filter((i) => i.moduleId && moduleEnabled(i.moduleId)).map((i) => ({ label: navTitle(i), value: i.moduleId as string }))])
const initial = String(route.query.source ?? 'system')
const source = ref(sources.value.some((s) => s.value === initial) ? initial : 'system')
const url = computed(() => (source.value === 'system' ? '/api/logs' : `/api/${source.value}/logs`))

// 来源写入地址栏，刷新或分享链接时保持选择
watch(source, (v) => router.replace({ query: v === 'system' ? {} : { source: v } }))
</script>

<style scoped>
/* 页面恰好占满一屏，日志在日志区内部滚动。原先按桌面端写死日志区高度，
   移动端筛选栏折成多行后页面多出一截可滚动，滚到底时来源标签半截压在顶栏下面。
   顶部的默认密码、版本更新提醒占用的高度（--g-banners-h，由 Layout 写入）也要扣除。 */
.logs-page {
  display: flex;
  flex-direction: column;
  height: calc(100dvh - var(--g-header-h) - var(--g-content-py) - var(--g-banners-h, 0px));
  min-height: 420px;
}
.logs-card {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.logs-card > .g-card-body {
  flex: 1 1 auto;
  min-height: 0;
}
.sources-row {
  flex-shrink: 0;
  max-width: 100%;
  overflow-x: auto;
  margin-bottom: 14px;
  padding-bottom: 2px;
}
.sources {
  width: max-content;
}
@media (max-width: 767.98px) {
  .g-card-body {
    padding: 12px;
  }
}
</style>
