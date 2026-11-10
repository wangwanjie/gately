<template>
  <!-- 导航面板链接的图标预览，与访客页面的显示规则一致：图片、文字（emoji），或名称首字配底色 -->
  <span class="panel-icon" :class="{ img: !!src }" :style="{ width: `${size}px`, height: `${size}px`, fontSize: `${Math.round(size * 0.42)}px`, background: src ? undefined : bg }">
    <img v-if="src" :src="src" alt="" referrerpolicy="no-referrer" />
    <template v-else>{{ text }}</template>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { iconKind, initial, paletteColor, safeIconSrc } from '../panels'

const props = withDefaults(defineProps<{ icon?: string; name?: string; color?: string; size?: number }>(), { icon: '', name: '', color: '', size: 32 })

// 只显示 http(s) 与 data:image 图片，其余写法按文字显示
const src = computed(() => (iconKind(props.icon) === 'image' ? safeIconSrc(props.icon) : null))
const text = computed(() => (iconKind(props.icon) === 'text' ? props.icon.trim() : initial(props.name)))
const bg = computed(() => (/^#[0-9a-f]{6}$/i.test(props.color) ? props.color : paletteColor(props.name)))
</script>

<style scoped>
.panel-icon {
  display: inline-grid;
  place-items: center;
  flex: none;
  border-radius: 28%;
  color: #fff;
  font-weight: 650;
  line-height: 1;
  overflow: hidden;
}
.panel-icon.img {
  background: var(--el-fill-color-light);
  border: 1px solid var(--g-border);
}
.panel-icon img {
  width: 72%;
  height: 72%;
  object-fit: contain;
}
</style>
