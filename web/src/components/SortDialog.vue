<template>
  <!-- 嵌套列表的分组与排序对话框（如 DDNS 任务内的解析记录）：在本地副本上调整分组与顺序，点击保存后一次性提交 -->
  <el-dialog v-model="visible" :title="title" width="680px" :fullscreen="isMobile" append-to-body>
    <GroupedList :gl="gl" :empty-text="t('groupSort.noMatched')" class="sort-dialog-list">
      <template #item="{ it, item, bind }">
        <div v-bind="bind" class="sd-row" :class="{ 'is-off': item.off }">
          <SortCheck :gl="gl" :id="it.id" />
          <div class="sd-main">
            <div class="sd-label" :class="{ mono: mono }">{{ item.label }}</div>
            <div v-if="item.sub" class="sd-sub">{{ item.sub }}</div>
          </div>
          <SortMoves :gl="gl" :it="it" />
        </div>
      </template>
    </GroupedList>
    <template #footer>
      <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="saving" :disabled="!changed" @click="submit">{{ t('common.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import GroupedList from './GroupedList.vue'
import SortCheck from './SortCheck.vue'
import SortMoves from './SortMoves.vue'
import { useGroupedList } from '../composables/useGroupSort'
import { useIsMobile } from '../composables/useIsMobile'
import type { SortEntry } from '../groupSort'

// entries 为打开时的条目与分组；save 收到调整后的顺序（含新分组），成功后关闭对话框；mono 为标签使用等宽字体
const props = defineProps<{
  title: string
  entries: SortEntry[]
  save: (entries: SortEntry[]) => Promise<void> | void
  mono?: boolean
}>()
const visible = defineModel<boolean>({ default: false })
const { t } = useI18n()
const isMobile = useIsMobile()
const local = ref<SortEntry[]>([])
const saving = ref(false)

const gl = useGroupedList<SortEntry>({
  storageKey: 'sortDialog',
  items: () => local.value,
  idOf: (e) => e.id,
  groupOf: (e) => e.group,
  matches: () => true,
  // 对话框内只修改本地副本，保存按钮再统一提交
  save: async (next) => {
    local.value = next.flatMap((s) => s.items.map((e) => ({ ...e, group: s.name })))
  },
  reload: async () => {},
})

// layoutKey 用于判断顺序或分组是否有变化
function layoutKey(list: SortEntry[]) {
  return list.map((e) => `${e.id}\u0000${e.group}`).join('\n')
}
const changed = computed(() => layoutKey(local.value) !== layoutKey(props.entries))

// 每次打开时从最新的条目重新开始，并直接进入排序模式
watch(visible, (v) => {
  if (!v) {
    return
  }
  local.value = props.entries.map((e) => ({ ...e }))
  gl.clearSelection()
  gl.resetDrag()
  gl.sortMode.value = true
})

async function submit() {
  saving.value = true
  try {
    await props.save(local.value.map((e) => ({ ...e })))
    visible.value = false
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.sort-dialog-list {
  max-height: 60vh;
  overflow-y: auto;
  padding: 2px;
}
.sd-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  background: var(--el-bg-color);
  font-size: 13px;
}
.sd-row.is-off .sd-main {
  opacity: 0.55;
}
.sd-main {
  flex: 1;
  min-width: 0;
}
.sd-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sd-sub {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
@media (max-width: 767.98px) {
  .sort-dialog-list {
    max-height: none;
  }
}
</style>
