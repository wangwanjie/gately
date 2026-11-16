<template>
  <!-- 表单的「高级设置」开关：常用字段默认显示，其余字段收在开关后面，减少新用户一次看到的字段数。
       count 为已改动（非默认值）的高级字段数，收起时提示，避免以为没有设置；页面打开已有条目时若有改动应默认展开 -->
  <div class="adv-toggle">
    <button type="button" class="adv-btn" :aria-expanded="open" @click="open = !open">
      <ChevronRight :size="14" class="adv-icon" :class="{ 'is-open': open }" aria-hidden="true" />
      <span>{{ label || t('common.advancedSettings') }}</span>
      <span v-if="!open && count" class="adv-count">{{ t('common.advancedChanged', { count }) }}</span>
    </button>
    <span class="adv-line" aria-hidden="true" />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ChevronRight } from 'lucide-vue-next'

defineProps<{ label?: string; count?: number }>()
const open = defineModel<boolean>({ required: true })
const { t } = useI18n()
</script>

<style scoped>
.adv-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 4px 0 14px;
}
.adv-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 32px;
  padding: 0 4px 0 0;
  border: 0;
  background: none;
  font: inherit;
  font-size: 13px;
  font-weight: 500;
  color: var(--el-color-primary);
  cursor: pointer;
  white-space: nowrap;
}
.adv-btn:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 2px;
  border-radius: 4px;
}
.adv-icon {
  transition: transform 0.15s ease;
}
.adv-icon.is-open {
  transform: rotate(90deg);
}
.adv-count {
  margin-left: 4px;
  font-weight: 400;
  color: var(--el-text-color-secondary);
}
.adv-line {
  flex: 1;
  height: 1px;
  background: var(--el-border-color-lighter);
}
@media (max-width: 767.98px) {
  .adv-btn {
    min-height: 40px;
  }
}
@media (prefers-reduced-motion: reduce) {
  .adv-icon {
    transition: none;
  }
}
</style>
