<template>
  <!-- 列表项卡片：桌面与移动端共用一套模板，字段区按宽度自动换列 -->
  <div class="entity" :class="{ 'is-off': off, 'is-compact': compact }">
    <div class="entity-head">
      <button
        v-if="collapsible"
        type="button"
        class="entity-toggle"
        :class="{ 'is-collapsed': collapsed }"
        :aria-expanded="!collapsed"
        :aria-label="collapsed ? t('common.expand') : t('common.collapse')"
        :title="collapsed ? t('common.expand') : t('common.collapse')"
        @click="emit('update:collapsed', !collapsed)"
      >
        <ChevronDown :size="16" />
      </button>
      <div v-if="$slots.lead" class="entity-lead"><slot name="lead" /></div>
      <div class="entity-titles">
        <div class="entity-title"><slot name="title" /></div>
        <div v-if="$slots.sub" class="entity-sub"><slot name="sub" /></div>
      </div>
      <div v-if="$slots.status" class="entity-status"><slot name="status" /></div>
    </div>
    <div v-if="$slots.default" class="entity-body"><slot /></div>
    <div v-if="$slots.extra && !(collapsible && collapsed)" class="entity-extra"><slot name="extra" /></div>
    <div v-if="$slots.actions || $slots.meta || (collapsible && collapsed && collapsedHint)" class="entity-foot">
      <div class="entity-meta">
        <!-- 收起后在底部提示被隐藏的条目数，点击即可展开 -->
        <span v-if="collapsible && collapsed && collapsedHint" class="entity-collapsed-hint" @click="emit('update:collapsed', false)">{{ collapsedHint }}</span>
        <slot name="meta" />
      </div>
      <div class="entity-actions"><slot name="actions" /></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ChevronDown } from 'lucide-vue-next'

// off 为 true 时整体置灰，表示该项已停用；compact 为 true 时压缩成单行（仅桌面宽度生效）
// collapsible 为 true 时标题左侧显示展开/收起按钮，收起后隐藏 extra 区（组内列表），collapsedHint 为收起后底部的提示文字
defineProps<{ off?: boolean; compact?: boolean; collapsible?: boolean; collapsed?: boolean; collapsedHint?: string }>()
const emit = defineEmits<{ 'update:collapsed': [value: boolean] }>()
const { t } = useI18n()
</script>

<style scoped>
.entity {
  background: var(--el-bg-color);
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  transition: border-color 0.2s;
  min-width: 0;
  /* 网格中同一行的卡片等高：纵向排列，操作栏贴底，各卡片的按钮才能上下对齐 */
  display: flex;
  flex-direction: column;
}
.entity:hover {
  border-color: var(--el-border-color);
}
.entity-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px 0;
}
.entity-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  width: 24px;
  height: 24px;
  margin-right: -4px;
  padding: 0;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--el-text-color-secondary);
  cursor: pointer;
  transition: background-color 0.2s, color 0.2s;
}
.entity-toggle:hover {
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
}
.entity-toggle svg {
  transition: transform 0.2s;
}
.entity-toggle.is-collapsed svg {
  transform: rotate(-90deg);
}
.entity-collapsed-hint {
  cursor: pointer;
}
.entity-collapsed-hint:hover {
  color: var(--el-color-primary);
}
.entity-lead {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}
.entity-titles {
  flex: 1;
  min-width: 0;
}
.entity-title {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 8px;
  font-weight: 600;
  font-size: 15px;
  line-height: 1.4;
  word-break: break-all;
}
.entity-sub {
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-all;
}
.entity-status {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  max-width: 45%;
}
.entity-body {
  display: grid;
  /* 卡片只有 400px 左右宽时也排成两列，一行放得下的字段不换行 */
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 150px), 1fr));
  gap: 12px 16px;
  padding: 14px 16px;
}
.entity-extra {
  padding: 0 16px 14px;
}
.entity-head + .entity-extra {
  padding-top: 14px;
}
.entity-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 6px 12px;
  padding: 4px 8px 4px 16px;
  border-top: 1px solid var(--g-border);
  min-height: 36px;
}
.entity-foot {
  margin-top: auto;
}
/* 没有字段区与扩展区时，标题与操作栏之间保留间距 */
.entity-head:has(+ .entity-foot) {
  padding-bottom: 14px;
}
.entity-meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  min-width: 0;
}
.entity-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px;
  margin-left: auto;
}
.entity-actions :deep(.el-button + .el-button) {
  margin-left: 0;
}

.is-off .entity-title {
  color: var(--el-text-color-secondary);
}
.is-off .entity-body,
.is-off .entity-extra {
  opacity: 0.6;
}
/* 紧凑视图：标题、状态、字段、操作排成一行，字段不显示标签，超长内容省略 */
@media (min-width: 768px) {
  .is-compact {
    display: flex;
    flex-direction: row;
    align-items: center;
    flex-wrap: wrap;
    column-gap: 14px;
    padding: 0 8px 0 14px;
    min-height: 40px;
  }
  .is-compact .entity-head {
    /* 名称、标签、状态同占一列，随卡片宽度加宽，避免名称被挤成省略号 */
    flex: 0 0 clamp(340px, 40%, 480px);
    min-width: 0;
    padding: 0;
    gap: 10px;
  }
  .is-compact .entity-title {
    flex-wrap: nowrap;
    font-size: 14px;
  }
  .is-compact .entity-title :deep(> span:first-child) {
    min-width: 8em;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .is-compact .entity-title :deep(> *:not(:first-child)) {
    flex-shrink: 0;
    flex-wrap: nowrap;
    white-space: nowrap;
  }
  .is-compact .entity-sub {
    display: none;
  }
  .is-compact .entity-body {
    flex: 1 1 0;
    min-width: 0;
    display: flex;
    gap: 14px;
    padding: 0;
  }
  .is-compact .entity-body :deep(.field) {
    flex: 1 1 0;
    min-width: 0;
  }
  .is-compact .entity-body :deep(.field.wide) {
    flex: 2 1 0;
  }
  .is-compact .entity-body :deep(.field-label) {
    display: none;
  }
  .is-compact .entity-body :deep(.field-value) {
    font-size: 12px;
    line-height: 1.4;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  /* 字段内的多行内容（端口、更新状态、次要说明）并入同一行 */
  .is-compact .entity-body :deep(.field-value > div) {
    display: inline;
    margin: 0 0 0 6px;
  }
  .is-compact .entity-body :deep(.field-value > div:first-child) {
    margin-left: 0;
  }
  .is-compact .entity-extra {
    flex: 1 0 100%;
    order: 9;
    padding: 0 0 6px;
  }
  .is-compact .entity-head + .entity-extra {
    padding-top: 0;
  }
  .is-compact .entity-foot {
    margin-top: 0;
    flex: 0 0 auto;
    padding: 0;
    min-height: 0;
    border-top: none;
    flex-wrap: nowrap;
  }
  .is-compact .entity-head:has(+ .entity-foot) {
    padding-bottom: 0;
  }
  .is-compact .entity-meta {
    display: none;
  }

}
@media (max-width: 767.98px) {
  .entity-head {
    padding: 12px 12px 0;
    gap: 10px;
    align-items: flex-start;
  }
  .entity-lead {
    padding-top: 2px;
  }
  .entity-body {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    padding: 12px;
    gap: 10px 12px;
  }
  .entity-extra {
    padding: 0 12px 12px;
  }
  .entity-foot {
    padding: 6px 8px 6px 12px;
  }
}
</style>

<!-- 插槽里的开关不带本组件的 scoped 标记，这几条规则不能写在 scoped 样式里 -->
<style>
/* 操作栏以开关开头时，开关固定在左侧，按钮换行后与第一个按钮的左边对齐，而不是退回到开关下面 */
.entity-actions:has(> .el-switch:first-child) {
  position: relative;
  padding-left: 52px;
}
.entity-actions > .el-switch:first-child {
  position: absolute;
  left: 8px;
  top: 2px;
}
/* 紧凑视图单行排列，开关回到正常流 */
@media (min-width: 768px) {
  .is-compact .entity-actions:has(> .el-switch:first-child) {
    position: static;
    padding-left: 0;
  }
  .is-compact .entity-actions > .el-switch:first-child {
    position: static;
  }
}
</style>
