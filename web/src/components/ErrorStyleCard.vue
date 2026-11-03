<template>
  <!-- 错误页样式卡片：缩略图 + 名称 + 预览按钮，整张卡片可点击选择（键盘回车或空格同样可选） -->
  <div
    class="card"
    :class="{ active, broken: option.broken }"
    role="radio"
    :aria-checked="active"
    :tabindex="focusable ? 0 : -1"
    :data-style="option.id || 'default'"
    @click="emit('select')"
    @keydown.enter.prevent="emit('select')"
    @keydown.space.prevent="emit('select')"
    @keydown.right.prevent="emit('move', 1)"
    @keydown.down.prevent="emit('move', 1)"
    @keydown.left.prevent="emit('move', -1)"
    @keydown.up.prevent="emit('move', -1)"
  >
    <ErrorPageThumb :html="option.html" :error="option.error" :title="option.name" />
    <div class="label">
      <span class="name" :title="option.name">{{ option.name }}</span>
      <button type="button" class="eye" :title="t('errorPages.preview')" :aria-label="t('errorPages.preview') + ' ' + option.name" @click.stop="emit('preview')" @keydown.stop>
        <Eye :size="14" />
      </button>
    </div>
    <span v-if="active" class="check" aria-hidden="true"><Check :size="12" /></span>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Check, Eye } from 'lucide-vue-next'
import ErrorPageThumb from './ErrorPageThumb.vue'

// StyleOption 是一个可选的错误页样式：id 为空表示默认样式，内置风格为其 ID，自定义页面为 custom:key
export interface StyleOption {
  id: string
  name: string
  html: string
  error: string
  // broken 为自定义页面当前不可用（如文件被删除）
  broken?: boolean
}

// focusable 为该卡片是否处于 Tab 顺序中（单选组只让选中的一项可聚焦）
defineProps<{ option: StyleOption; active: boolean; focusable: boolean }>()
const emit = defineEmits<{ select: []; preview: []; move: [delta: number] }>()
const { t } = useI18n()
</script>

<style scoped>
.card {
  position: relative;
  border: 1.5px solid var(--el-border-color);
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  background: var(--el-bg-color);
  transition:
    border-color 0.15s,
    box-shadow 0.15s;
  outline: none;
}
.card:hover {
  border-color: var(--el-color-primary-light-5);
}
.card:focus-visible {
  box-shadow: 0 0 0 3px var(--el-color-primary-light-7);
}
.card.active {
  border-color: var(--el-color-primary);
  box-shadow: 0 0 0 2px var(--el-color-primary-light-8);
}
.card.broken {
  border-style: dashed;
}
.label {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 3px 3px 3px 8px;
  border-top: 1px solid var(--el-border-color-lighter);
  font-size: 12.5px;
  line-height: 20px;
}
.name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.card.active .name {
  color: var(--el-color-primary);
  font-weight: 600;
}
.eye {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: none;
  color: var(--el-text-color-secondary);
  cursor: pointer;
}
.eye:hover {
  color: var(--el-color-primary);
  background: var(--el-fill-color);
}
.check {
  position: absolute;
  top: 6px;
  right: 6px;
  display: grid;
  place-items: center;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--el-color-primary);
  color: #fff;
}
</style>
