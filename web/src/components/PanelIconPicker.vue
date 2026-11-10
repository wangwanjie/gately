<template>
  <!-- 内置图标选择：按分类与关键词筛选，点选后返回 builtin:编号 -->
  <el-dialog v-model="visible" :title="t('panels.iconPickTitle')" width="720px" :fullscreen="isMobile" append-to-body data-test="icon-picker">
    <div class="picker-bar">
      <el-input v-model="query" clearable :prefix-icon="Search" :placeholder="t('panels.iconSearch')" class="picker-search" data-test="icon-search" />
      <el-radio-group v-model="cat" size="small" class="picker-cats">
        <el-radio-button value="">{{ t('panels.catAll') }}</el-radio-button>
        <el-radio-button v-for="c in cats" :key="c" :value="c">{{ t(`panels.cat_${c}`) }}</el-radio-button>
      </el-radio-group>
    </div>
    <div v-if="!shown.length" class="picker-empty">{{ t('panels.iconNone') }}</div>
    <div class="picker-grid">
      <button v-for="i in shown" :key="i.id" type="button" class="picker-item" :class="{ active: modelValue === BUILTIN_PREFIX + i.id }" :title="i.name" data-test="icon-item" @click="pick(i)">
        <PanelIcon :icon="BUILTIN_PREFIX + i.id" :name="i.name" :size="40" />
        <span class="picker-name">{{ i.name }}</span>
      </button>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Search } from 'lucide-vue-next'
import PanelIcon from './PanelIcon.vue'
import { BUILTIN_PREFIX, filterIcons, type BuiltinIcon } from '../panels'
import { usePanelIcons } from '../composables/usePanelIcons'
import { useFormLayout } from '../composables/useIsMobile'

defineProps<{ modelValue?: string }>()
const emit = defineEmits<{ select: [icon: string] }>()
const visible = defineModel<boolean>('visible', { default: false })
const { t } = useI18n()
const { isMobile } = useFormLayout()
const { icons } = usePanelIcons()
const cats = ['gately', 'apps', 'system', 'generic'] as const
const cat = ref('')
const query = ref('')
const shown = computed(() => filterIcons(icons.value, cat.value, query.value))

function pick(i: BuiltinIcon) {
  emit('select', BUILTIN_PREFIX + i.id)
  visible.value = false
}
</script>

<style scoped>
.picker-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.picker-search {
  flex: 1 1 100%;
}
.picker-cats {
  flex-wrap: wrap;
}
.picker-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(92px, 1fr));
  gap: 6px;
  max-height: 60vh;
  overflow: auto;
}
.picker-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding: 10px 4px 8px;
  border: 1px solid transparent;
  border-radius: 10px;
  background: none;
  color: var(--el-text-color-regular);
  cursor: pointer;
}
.picker-item:hover {
  background: var(--el-fill-color-light);
}
.picker-item.active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.picker-name {
  max-width: 100%;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.picker-empty {
  padding: 30px 0;
  text-align: center;
  color: var(--el-text-color-secondary);
}
@media (max-width: 768px) {
  .picker-grid {
    grid-template-columns: repeat(auto-fill, minmax(76px, 1fr));
    max-height: none;
  }
}
</style>
