<template>
  <!-- 排序模式下条目的勾选框：勾选多项后可一起拖动或批量移到分组 -->
  <el-checkbox :model-value="gl.isSelected(id)" :aria-label="t('groupSort.selectItem')" @click.stop @change="(v: string | number | boolean) => gl.setSelected([id], Boolean(v))" />
</template>

<script setup lang="ts" generic="T">
import { useI18n } from 'vue-i18n'
import type { GroupedListState } from '../composables/useGroupSort'

defineProps<{ gl: GroupedListState<T>; id: string }>()
const { t } = useI18n()
</script>
