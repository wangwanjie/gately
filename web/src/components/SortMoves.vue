<template>
  <!-- 排序模式下条目的拖动把手与上移、下移按钮；窄屏不支持拖动，只显示按钮 -->
  <span class="sort-moves" @click.stop>
    <GripVertical v-if="!isMobile" :size="16" class="grip" />
    <el-button text size="small" :icon="ChevronUp" :disabled="!it.canUp || gl.busy.value" :aria-label="t('groupSort.moveUp')" :title="t('groupSort.moveUp')" @click="gl.moveItem(it.id, -1)" />
    <el-button text size="small" :icon="ChevronDown" :disabled="!it.canDown || gl.busy.value" :aria-label="t('groupSort.moveDown')" :title="t('groupSort.moveDown')" @click="gl.moveItem(it.id, 1)" />
  </span>
</template>

<script setup lang="ts" generic="T">
import { useI18n } from 'vue-i18n'
import { ChevronDown, ChevronUp, GripVertical } from 'lucide-vue-next'
import type { GroupedListState } from '../composables/useGroupSort'
import { useIsMobile } from '../composables/useIsMobile'

// it 为 GroupedList item 插槽给出的条目信息，含所在分组内能否上移、下移
defineProps<{ gl: GroupedListState<T>; it: { id: string; canUp: boolean; canDown: boolean } }>()
const { t } = useI18n()
const isMobile = useIsMobile()
</script>

<style scoped>
.sort-moves {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}
.sort-moves :deep(.el-button + .el-button) {
  margin-left: 0;
}
.grip {
  margin-right: 4px;
  color: var(--el-text-color-secondary);
  cursor: grab;
}
</style>
