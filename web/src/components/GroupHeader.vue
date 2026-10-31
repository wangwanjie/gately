<template>
  <!-- 分组标题行：平时点击收起或展开；命名分组可重命名；排序模式下可勾选整组、整体上移下移，也可作为拖动的放置目标（移到该分组末尾） -->
  <div class="group-header" :class="{ 'is-sorting': sorting }" @click="!sorting && emit('toggle')">
    <el-checkbox v-if="sorting" :model-value="checked" :indeterminate="indeterminate" :aria-label="t('groupSort.selectGroup')" @click.stop @change="(v: string | number | boolean) => emit('check', Boolean(v))" />
    <ChevronRight v-else :size="14" class="group-arrow" :class="{ open: !collapsed }" />
    <Folder :size="14" class="group-icon" />
    <span class="group-name">{{ name || t('groupSort.ungrouped') }}</span>
    <span class="group-count">{{ countText }}</span>
    <span v-if="name && (renamable || sorting)" class="group-ops" :class="{ 'is-hover': !sorting }" @click.stop>
      <el-button v-if="renamable" text size="small" :icon="Pencil" :aria-label="t('groupSort.rename')" :title="t('groupSort.rename')" @click="emit('rename')" />
      <template v-if="sorting">
      <el-button text size="small" :icon="ChevronUp" :disabled="!canUp" :aria-label="t('groupSort.moveGroupUp')" :title="t('groupSort.moveGroupUp')" @click="emit('up')" />
      <el-button text size="small" :icon="ChevronDown" :disabled="!canDown" :aria-label="t('groupSort.moveGroupDown')" :title="t('groupSort.moveGroupDown')" @click="emit('down')" />
      </template>
    </span>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ChevronDown, ChevronRight, ChevronUp, Folder, Pencil } from 'lucide-vue-next'

// name 为空表示“未分组”；countText 为分组内条目数的说明文字；renamable 为 false 时不显示重命名按钮（只读展示的分组）
withDefaults(
  defineProps<{
    name: string
    countText: string
    collapsed?: boolean
    sorting?: boolean
    canUp?: boolean
    canDown?: boolean
    checked?: boolean
    indeterminate?: boolean
    renamable?: boolean
  }>(),
  { renamable: true },
)
const emit = defineEmits<{ toggle: []; up: []; down: []; check: [value: boolean]; rename: [] }>()
const { t } = useI18n()
</script>

<style scoped>
.group-header {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 30px;
  padding: 3px 12px;
  font-size: 12px;
  font-weight: 600;
  background: var(--el-fill-color-light);
  cursor: pointer;
  user-select: none;
}
.group-header.is-sorting {
  cursor: default;
}
.group-header.drop-before {
  box-shadow: inset 0 0 0 2px var(--el-color-primary);
}
.group-arrow {
  transition: transform 0.15s;
}
.group-arrow.open {
  transform: rotate(90deg);
}
.group-icon {
  color: var(--el-text-color-secondary);
}
.group-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.group-count {
  flex-shrink: 0;
  font-weight: 400;
  color: var(--el-text-color-secondary);
}
.group-ops {
  display: flex;
  margin-left: auto;
}
/* 非排序模式下重命名按钮在悬停时显示；触屏设备没有悬停，始终显示 */
@media (hover: hover) {
  .group-ops.is-hover {
    opacity: 0;
    transition: opacity 0.15s;
  }
  .group-header:hover .group-ops.is-hover,
  .group-ops.is-hover:focus-within {
    opacity: 1;
  }
}
.group-ops :deep(.el-button + .el-button) {
  margin-left: 0;
}
</style>
