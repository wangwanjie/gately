<template>
  <!-- 分组列表：按 useGroupedList 的结果依次渲染分组标题与条目，处理排序模式下的拖动、勾选与分组操作。
       条目内容由 item 插槽提供，插槽根元素需绑定 bind（拖动事件与插入线样式）；排序模式下可用 SortCheck / SortMoves 显示勾选框与上下移按钮 -->
  <SortToolbar
    v-if="toolbar && gl.sortMode.value"
    :class="{ 'is-flush': flush }"
    :selected-count="gl.selected.value.size"
    :total="gl.total.value"
    :groups="gl.groupNames.value"
    :busy="gl.busy.value"
    @select-all="gl.selectAll()"
    @clear="gl.clearSelection()"
    @set-group="gl.setGroup"
  />
  <div v-loading="loading" v-bind="$attrs" class="grouped-list" :class="{ 'is-sorting': sorting, 'is-compact': dense, 'is-flush': flush }">
    <el-empty v-if="!loading && !gl.total.value" :description="emptyText" :image-size="emptyImageSize" />
    <el-empty v-else-if="!loading && !gl.items.value.length" :description="t('groupSort.noMatched')" :image-size="60" />
    <div v-for="b in blocks" :key="b.id" class="gl-block">
      <GroupHeader
        v-if="b.header"
        :name="b.header.name"
        :count-text="countText ? countText(b.header.count) : t('groupSort.count', { count: b.header.count })"
        :collapsed="b.header.collapsed"
        :sorting="sorting"
        :can-up="b.header.canUp"
        :can-down="b.header.canDown"
        :checked="b.header.checked"
        :indeterminate="b.header.indeterminate"
        :class="gl.dropClass(b.header.id)"
        @toggle="gl.toggleGroup(b.header.name)"
        @rename="gl.renameGroup(b.header.name)"
        @up="gl.moveGroup(b.header.index, -1)"
        @down="gl.moveGroup(b.header.index, 1)"
        @check="(v: boolean) => gl.setSelected(b.header!.ids, v)"
        @dragover.prevent="gl.dragOver(b.header.id, $event, false)"
        @dragleave="gl.dragLeave(b.header.id)"
        @drop.prevent="gl.drop()"
      />
      <div v-if="b.rows.length" class="gl-rows" :class="[layout, { 'is-grouped': !!b.header }]">
        <template v-for="it in b.rows" :key="it.id">
          <slot name="item" :it="it" :item="it.item" :sorting="sorting" :bind="bindOf(it)" />
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts" generic="T">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupHeader from './GroupHeader.vue'
import SortToolbar from './SortToolbar.vue'
import { useIsMobile } from '../composables/useIsMobile'
import type { GroupedListState } from '../composables/useGroupSort'
import type { GroupItem } from '../groupSort'

defineOptions({ inheritAttrs: false })

// gl 为 useGroupedList 的返回值；emptyImageSize 为空列表插图大小（放在卡片内时用小图）；compact 为紧凑视图（排序模式下总是紧凑）；
// grid 为 true 时卡片视图按多列网格排列，紧凑视图与排序模式下为单列；flush 为 true 时条目紧贴排列、不缩进（用于侧栏中带分隔线的行列表）；toolbar 为 false 时不显示排序工具栏（由页面自行放置）
const props = withDefaults(
  defineProps<{
    gl: GroupedListState<T>
    loading?: boolean
    emptyText?: string
    emptyImageSize?: number
    compact?: boolean
    grid?: boolean
    flush?: boolean
    toolbar?: boolean
    countText?: (count: number) => string
  }>(),
  { toolbar: true },
)

defineSlots<{
  item(props: { it: Extract<GroupItem<T>, { kind: 'item' }>; item: T; sorting: boolean; bind: Record<string, unknown> }): unknown
}>()

const { t } = useI18n()
const isMobile = useIsMobile()
const sorting = computed(() => props.gl.sortMode.value)
const dense = computed(() => props.compact || sorting.value)
const layout = computed(() => {
  if (props.flush) {
    return 'is-flush'
  }
  return props.grid && !dense.value ? 'card-grid' : ['card-list', { 'is-compact': dense.value }]
})

type Header = Extract<GroupItem<T>, { kind: 'group' }>
type Row = Extract<GroupItem<T>, { kind: 'item' }>

// blocks 把依次显示的标题与条目切成块：每个分组标题与其下的条目为一块，没有命名分组时只有一块且不显示标题
const blocks = computed(() => {
  const out: { id: string; header?: Header; rows: Row[] }[] = []
  for (const it of props.gl.items.value) {
    if (it.kind === 'group') {
      out.push({ id: it.id, header: it, rows: [] })
    } else {
      if (!out.length) {
        out.push({ id: '_', rows: [] })
      }
      out[out.length - 1].rows.push(it)
    }
  }
  return out
})

// bindOf 返回条目根元素要绑定的属性：插入线样式、是否可拖动与拖放事件；窄屏不支持拖动，改用上下移按钮
function bindOf(it: Row) {
  const gl = props.gl
  return {
    class: ['gl-item', gl.dropClass(it.id)],
    draggable: sorting.value && !isMobile.value,
    onDragstart: (e: DragEvent) => gl.dragStart(it.id, e),
    onDragover: (e: DragEvent) => {
      e.preventDefault()
      gl.dragOver(it.id, e)
    },
    onDragleave: () => gl.dragLeave(it.id),
    onDrop: (e: DragEvent) => {
      e.preventDefault()
      gl.drop()
    },
    onDragend: () => gl.resetDrag(),
  }
}
</script>

<style scoped>
.grouped-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 40px;
}
.grouped-list.is-compact {
  gap: 8px;
}
.gl-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}
.grouped-list.is-compact .gl-block {
  gap: 6px;
}
.gl-block :deep(.group-header) {
  border-radius: var(--g-radius);
}
.gl-rows.is-grouped {
  margin-left: 16px;
}
.grouped-list.is-sorting .gl-rows :deep(.gl-item[draggable='true']) {
  cursor: grab;
}
.gl-rows :deep(.gl-item.dragging) {
  opacity: 0.4;
}
.gl-rows :deep(.gl-item.drop-before) {
  box-shadow: 0 -3px 0 var(--el-color-primary);
}
.gl-rows :deep(.gl-item.drop-after) {
  box-shadow: 0 3px 0 var(--el-color-primary);
}
.sort-bar.is-flush {
  margin: 8px;
}
.grouped-list.is-flush,
.grouped-list.is-flush .gl-block {
  gap: 0;
}
.grouped-list.is-flush .gl-block :deep(.group-header) {
  border-radius: 0;
}
.gl-rows.is-flush.is-grouped {
  margin-left: 0;
}
.gl-rows.is-flush :deep(.gl-item.drop-before) {
  box-shadow: inset 0 3px 0 var(--el-color-primary);
}
.gl-rows.is-flush :deep(.gl-item.drop-after) {
  box-shadow: inset 0 -3px 0 var(--el-color-primary);
}
@media (max-width: 767.98px) {
  .gl-rows.is-grouped {
    margin-left: 8px;
  }
}
</style>
