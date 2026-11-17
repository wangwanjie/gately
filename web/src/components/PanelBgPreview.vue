<template>
  <!-- 背景效果的实时预览：按 1280×720 的页面排版后整体缩小，模糊半径与遮罩浓度和访客页面成比例；拖动滑块时立即更新 -->
  <div ref="box" class="bg-live" :style="boxStyle" data-test="bg-live">
    <div class="bg-live-page" :style="pageStyle">
      <div class="bg-live-bg">
        <img :src="background" alt="" referrerpolicy="no-referrer" :style="{ filter: `blur(${blur}px)` }" data-test="bg-live-img" />
        <i :style="{ opacity: dim / 100 }" data-test="bg-live-dim" />
      </div>
      <div class="bg-live-top">
        <span class="bg-live-logo" />
        <b>{{ title }}</b>
      </div>
      <div class="bg-live-grid" :class="{ app: cardStyle === 'app' }">
        <div v-for="(l, i) in sample" :key="i" class="bg-live-card">
          <PanelIcon :icon="l.icon" :name="l.name" :color="l.color" :size="cardStyle === 'app' ? 64 : 40" />
          <span>{{ l.name }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import PanelIcon from './PanelIcon.vue'
import { DEFAULT_FAMILY, resolveTheme, themeFamilies } from '../themes'
import type { Panel, PanelLink } from '../panels'

const props = defineProps<{
  background: string
  blur: number
  dim: number
  skin: string
  theme: Panel['theme']
  cardStyle: Panel['cardStyle']
  title: string
  links: readonly PanelLink[]
}>()

// 访客页面的宽度按 1280 排版；预览框按实际宽度缩放
const PAGE_W = 1280
const PAGE_H = 600
const box = ref<HTMLElement>()
const scale = ref(0.4)
let ro: ResizeObserver | undefined
onMounted(() => {
  const update = () => {
    if (box.value && box.value.clientWidth > 0) {
      scale.value = box.value.clientWidth / PAGE_W
    }
  }
  update()
  ro = new ResizeObserver(update)
  if (box.value) {
    ro.observe(box.value)
  }
})
onBeforeUnmount(() => ro?.disconnect())

// 配色取面板的皮肤与亮暗模式（跟随系统时按亮色），遮罩用页面底色，与访客页面相同
const palette = computed(() => {
  const fam = themeFamilies.find((f) => f.id === (props.skin || DEFAULT_FAMILY)) ?? themeFamilies[0]
  return resolveTheme(fam, props.theme === 'dark')
})
const boxStyle = computed(() => ({ height: `${Math.round(PAGE_H * scale.value)}px` }))
const pageStyle = computed(() => ({
  width: `${PAGE_W}px`,
  height: `${PAGE_H}px`,
  transform: `scale(${scale.value})`,
  '--lv-bg': palette.value.page,
  '--lv-card': palette.value.card,
  '--lv-text': palette.value.text,
  '--lv-brand': palette.value.primary,
}))
// 示例链接取面板中前几个可见链接，没有链接时用占位名称
const sample = computed(() => {
  const list = props.links.filter((l) => !l.hide).slice(0, props.cardStyle === 'app' ? 8 : 6)
  if (list.length) {
    return list
  }
  return ['NAS', 'Jellyfin', 'Home', 'Router'].map((name) => ({ name, icon: '', color: '' }) as PanelLink)
})
</script>

<style scoped>
.bg-live {
  position: relative;
  width: 100%;
  max-width: 560px;
  overflow: hidden;
  border: 1px solid var(--el-border-color);
  border-radius: 10px;
}
.bg-live-page {
  position: absolute;
  top: 0;
  left: 0;
  transform-origin: 0 0;
  padding: 48px 56px;
  color: var(--lv-text);
  background: var(--lv-bg);
  overflow: hidden;
}
.bg-live-bg {
  position: absolute;
  inset: 0;
  overflow: hidden;
}
/* 与访客页面相同：图片四周放大 24 像素，模糊后边缘不露底 */
.bg-live-bg img {
  position: absolute;
  inset: -24px;
  width: calc(100% + 48px);
  height: calc(100% + 48px);
  object-fit: cover;
}
.bg-live-bg i {
  position: absolute;
  inset: 0;
  background: var(--lv-bg);
}
.bg-live-top {
  position: relative;
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 36px;
  font-size: 28px;
}
.bg-live-logo {
  width: 52px;
  height: 52px;
  border-radius: 14px;
  background: var(--lv-brand);
}
.bg-live-grid {
  position: relative;
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}
.bg-live-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px 18px;
  border-radius: 14px;
  font-size: 18px;
  font-weight: 600;
  background: color-mix(in srgb, var(--lv-card) 62%, transparent);
  backdrop-filter: blur(10px);
}
.bg-live-grid.app {
  grid-template-columns: repeat(8, 1fr);
  gap: 26px 12px;
}
.bg-live-grid.app .bg-live-card {
  flex-direction: column;
  gap: 8px;
  padding: 0;
  font-size: 15px;
  font-weight: 500;
  background: transparent;
  backdrop-filter: none;
  text-shadow: 0 1px 3px var(--lv-bg);
}
.bg-live-card span {
  max-width: 100%;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
</style>
