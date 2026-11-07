<template>
  <!-- 条数不超过最小可选每页条数时不显示分页条 -->
  <div v-if="visible" class="list-pager" :class="{ mobile: isMobile }">
    <el-pagination
      v-model:current-page="pager.page.value"
      v-model:page-size="pager.size.value"
      :total="pager.total.value"
      :page-sizes="[...pager.sizes]"
      :layout="narrow ? 'prev, pager, next, sizes' : 'total, sizes, prev, pager, next, jumper'"
      :pager-count="narrow ? 5 : 7"
      :size="narrow ? 'small' : 'default'"
      background
      @current-change="onPageChange"
    />
    <div v-if="narrow" class="mobile-total">{{ t('common.pageRange', range) }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { needsPager } from '../paging'
import type { Pager } from '../composables/usePager'
import { useIsMobile } from '../composables/useIsMobile'

const props = defineProps<{
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  pager: Pager<any>
  // anchor 是翻页后要滚回顶部的列表或其上方的工具栏
  anchor?: HTMLElement | { $el?: HTMLElement } | null
  // compact 用于对话框、侧边面板等较窄的区域，桌面端也使用移动端的紧凑布局
  compact?: boolean
}>()

const { t } = useI18n()
const isMobile = useIsMobile()

const narrow = computed(() => isMobile.value || props.compact)
const visible = computed(() => needsPager(props.pager.total.value, props.pager.sizes))

// range 是当前页显示的条目范围，紧凑布局放不下「共 N 条」时单独一行显示
const range = computed(() => {
  const total = props.pager.total.value
  const from = total ? (props.pager.page.value - 1) * props.pager.size.value + 1 : 0
  const to = Math.min(total, props.pager.page.value * props.pager.size.value)
  return { from, to, total }
})

// onPageChange 翻页后把列表滚回顶部：从页底点「下一页」时，新一页从第一行开始看
async function onPageChange() {
  await nextTick()
  const a = props.anchor
  const el = a instanceof HTMLElement ? a : a?.$el
  if (!(el instanceof HTMLElement)) {
    return
  }
  // 列表自身可滚动（如终端 SFTP 面板）时先回到顶部
  if (el.scrollHeight > el.clientHeight) {
    el.scrollTop = 0
  }
  // 顶部已滚出可视区域时滚回来；吸顶区域的高度由 html 的 scroll-padding-top 让出
  const stickyTop = parseFloat(getComputedStyle(document.documentElement).scrollPaddingTop) || 0
  if (el.getBoundingClientRect().top < stickyTop) {
    el.scrollIntoView({ block: 'start' })
  }
}
</script>

<style scoped>
.list-pager {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
  margin-top: 12px;
}
.list-pager :deep(.el-pagination) {
  flex-wrap: wrap;
  row-gap: 8px;
  justify-content: flex-end;
}
.list-pager.mobile {
  align-items: center;
}
.list-pager.mobile :deep(.el-pagination) {
  justify-content: center;
}
.mobile-total {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
