<template>
  <!-- 列表工具：搜索框与“排序 / 完成排序”按钮，放在页头或卡片标题栏中。排序模式下不筛选，搜索框禁用 -->
  <el-input
    v-if="placeholder && gl.total.value"
    v-model="gl.filterText.value"
    class="list-filter"
    :class="{ 'is-small': size === 'small' }"
    :size="size"
    clearable
    :disabled="gl.sortMode.value"
    :prefix-icon="Search"
    :placeholder="placeholder"
  />
  <el-button v-if="gl.total.value > 1" :size="size" :type="gl.sortMode.value ? 'primary' : 'default'" :icon="ArrowUpDown" @click="gl.toggleSortMode()">
    {{ gl.sortMode.value ? t('groupSort.sortDone') : t('groupSort.sort') }}
  </el-button>
</template>

<script setup lang="ts" generic="T">
import { useI18n } from 'vue-i18n'
import { ArrowUpDown, Search } from 'lucide-vue-next'
import type { GroupedListState } from '../composables/useGroupSort'

// placeholder 为搜索框提示，不提供时不显示搜索框
defineProps<{ gl: GroupedListState<T>; placeholder?: string; size?: 'default' | 'small' }>()
const { t } = useI18n()
</script>

<style scoped>
.list-filter {
  width: 220px;
}
.list-filter.is-small {
  width: 180px;
}
@media (max-width: 767.98px) {
  .list-filter {
    width: 160px;
  }
}
</style>
