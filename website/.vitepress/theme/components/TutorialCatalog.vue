<script setup lang="ts">
// 教程首页的卡片列表：按 level 取对应难度分组里、当前语言实际存在的教程，顺序与侧边栏一致。
import { computed } from 'vue'
import { useData, withBase } from 'vitepress'
import { Clock, PlayCircle } from 'lucide-vue-next'
import { tutorialGroups } from '../../i18n/structure'
import { data as tutorials } from '../tutorials.data'

const props = defineProps<{
  level: 'basics' | 'advanced' | 'scenarios'
  // 带视频的教程卡片上显示的标签文字
  videoLabel?: string
}>()

const { localeIndex } = useData()

const items = computed(() => {
  const prefix = localeIndex.value === 'root' ? '' : `/${localeIndex.value}`
  const group = tutorialGroups.find((g) => g.id === props.level)
  if (!group) {
    return []
  }
  const byUrl = new Map(tutorials.map((t) => [t.url, t]))
  return group.slugs
    .map((slug) => byUrl.get(`${prefix}/docs/${slug}`))
    .filter((t): t is NonNullable<typeof t> => !!t)
})
</script>

<template>
  <div class="tut-grid">
    <a v-for="(t, i) in items" :key="t.url" class="tut-card" :href="withBase(t.url)">
      <span class="tut-no">{{ String(i + 1).padStart(2, '0') }}</span>
      <span class="tut-title">{{ t.title }}</span>
      <span class="tut-desc">{{ t.description }}</span>
      <span class="tut-meta">
        <span v-if="t.time" class="tut-time"><Clock :size="14" />{{ t.time }}</span>
        <span v-if="t.video" class="tut-video"><PlayCircle :size="14" />{{ videoLabel ?? 'Video' }}</span>
      </span>
    </a>
  </div>
</template>

<style scoped>
.tut-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 14px;
  margin: 16px 0 28px;
}
.tut-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 16px 18px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 12px;
  background: var(--vp-c-bg-soft);
  color: inherit;
  text-decoration: none !important;
  transition: border-color 0.2s, transform 0.2s;
}
.tut-card:hover {
  border-color: var(--vp-c-brand-1);
  transform: translateY(-2px);
}
.tut-no {
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  color: var(--vp-c-brand-1);
}
.tut-title {
  font-weight: 600;
  font-size: 15px;
  line-height: 1.4;
  color: var(--vp-c-text-1);
}
.tut-desc {
  flex: 1;
  font-size: 13px;
  line-height: 1.6;
  color: var(--vp-c-text-2);
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.tut-meta {
  display: flex;
  gap: 12px;
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.tut-time,
.tut-video {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.tut-video {
  color: var(--vp-c-brand-1);
}
</style>
