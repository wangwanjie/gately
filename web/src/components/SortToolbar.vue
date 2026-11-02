<template>
  <!-- 排序模式工具栏：显示操作提示、已选数量，并把选中项批量移到某个分组 -->
  <div class="sort-bar">
    <div class="sort-hint">{{ isMobile ? t('groupSort.hintMobile') : t('groupSort.hint') }}</div>
    <div class="sort-ops">
      <span class="sort-count">{{ t('groupSort.selectedCount', { count: selectedCount }) }}</span>
      <el-button link type="primary" :disabled="!total || selectedCount === total" @click="emit('selectAll')">{{ t('groupSort.selectAll') }}</el-button>
      <el-button link :disabled="!selectedCount" @click="emit('clear')">{{ t('groupSort.clearSelection') }}</el-button>
      <div class="sort-group">
        <el-autocomplete
          v-model="target"
          size="small"
          clearable
          class="sort-group-input"
          :maxlength="64"
          :fetch-suggestions="(q: string, cb: (v: { value: string }[]) => void) => cb(groupSuggestions(groups, q))"
          :placeholder="t('groupSort.targetPlaceholder')"
          @keyup.enter="apply"
        />
        <el-button size="small" type="primary" :icon="FolderInput" :disabled="!selectedCount || busy" @click="apply">
          {{ target.trim() ? t('groupSort.moveToGroup') : t('groupSort.removeFromGroup') }}
        </el-button>
      </div>
      <!-- 批量启停与删除；页面可通过默认插槽补充自己的批量操作 -->
      <div v-if="batch || $slots.default" class="sort-batch">
        <template v-if="batch?.enable">
          <el-button size="small" :icon="Power" :disabled="!selectedCount || busy" @click="emit('enable', true)">{{ t('groupSort.batchEnable') }}</el-button>
          <el-button size="small" :icon="PowerOff" :disabled="!selectedCount || busy" @click="emit('enable', false)">{{ t('groupSort.batchDisable') }}</el-button>
        </template>
        <slot />
        <el-button v-if="batch?.remove" size="small" type="danger" plain :icon="Trash2" :disabled="!selectedCount || busy" @click="emit('remove')">{{ t('groupSort.batchRemove') }}</el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { FolderInput, Power, PowerOff, Trash2 } from 'lucide-vue-next'
import { groupSuggestions } from '../groupSort'
import { useIsMobile } from '../composables/useIsMobile'

// groups 为已有分组名称，用于输入提示；busy 为保存中，期间不能重复提交
// batch 为可用的批量操作（启停、删除），缺省时不显示对应按钮
const props = defineProps<{ selectedCount: number; total: number; groups: string[]; busy?: boolean; batch?: { enable?: boolean; remove?: boolean } | null }>()
const emit = defineEmits<{ selectAll: []; clear: []; setGroup: [name: string]; enable: [enable: boolean]; remove: [] }>()
const { t } = useI18n()
const isMobile = useIsMobile()
const target = ref('')

function apply() {
  if (!props.selectedCount || props.busy) {
    return
  }
  emit('setGroup', target.value.trim())
}
</script>

<style scoped>
.sort-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px 16px;
  margin-bottom: 12px;
  padding: 8px 12px;
  border: 1px dashed var(--el-color-primary-light-5);
  border-radius: var(--g-radius);
  background: var(--el-color-primary-light-9);
  font-size: 13px;
}
.sort-hint {
  color: var(--el-text-color-regular);
}
.sort-ops {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 12px;
}
.sort-ops :deep(.el-button + .el-button) {
  margin-left: 0;
}
.sort-count {
  color: var(--el-text-color-secondary);
}
.sort-group {
  display: flex;
  align-items: center;
  gap: 6px;
}
.sort-batch {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.sort-batch :deep(.el-button + .el-button) {
  margin-left: 0;
}
.sort-group-input {
  width: 180px;
}
@media (max-width: 767.98px) {
  .sort-group {
    width: 100%;
  }
  .sort-batch {
    width: 100%;
  }
  .sort-group-input {
    flex: 1;
  }
}
</style>
