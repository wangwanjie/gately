<template>
  <!-- 导航面板链接的图标预览，与访客页面的显示规则一致：内置图标（底色方块上的图形）、图片、文字（emoji），或名称首字配底色 -->
  <span class="panel-icon" :class="{ img: !!src && !builtin }" :style="{ width: `${size}px`, height: `${size}px`, fontSize: `${Math.round(size * 0.42)}px`, background: src && !builtin ? undefined : bg }">
    <img v-if="src" :src="src" alt="" referrerpolicy="no-referrer" :class="{ glyph: !!builtin }" />
    <template v-else>{{ text }}</template>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { builtinId, iconKind, initial, lightColor, paletteColor, safeIconSrc } from '../panels'
import { usePanelIcons } from '../composables/usePanelIcons'

const props = withDefaults(defineProps<{ icon?: string; name?: string; color?: string; size?: number }>(), { icon: '', name: '', color: '', size: 32 })
const { byId } = usePanelIcons()

const validColor = computed(() => (/^#[0-9a-f]{6}$/i.test(props.color) ? props.color : ''))
const builtin = computed(() => byId.value.get(builtinId(props.icon)) ?? null)
// 底色：自定义颜色优先，内置图标用图标自带的颜色，其余按名称挑选
const bg = computed(() => validColor.value || builtin.value?.color || paletteColor(props.name))
const src = computed(() => {
  if (builtin.value) {
    return lightColor(bg.value) ? builtin.value.srcDark : builtin.value.src
  }
  // 只显示 http(s) 与 data:image 图片，其余写法按文字显示
  return iconKind(props.icon) === 'image' ? safeIconSrc(props.icon) : null
})
const text = computed(() => (iconKind(props.icon) === 'text' ? props.icon.trim() : initial(props.name)))
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
.panel-icon img.glyph {
  width: 56%;
  height: 56%;
}
</style>
