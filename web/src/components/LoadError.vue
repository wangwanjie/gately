<template>
  <!-- 列表加载失败：bar 为 true 时是一行不打扰的提示（轮询偶发失败、仍显示上次加载的数据），否则是占位的错误块（加载失败 + 原因 + 重试），
       用来代替“暂无数据”的空状态，避免接口出错时误以为条目都没了 -->
  <div v-if="bar" class="load-error-bar" role="status">
    <TriangleAlert :size="14" class="icon" aria-hidden="true" />
    <span class="text">{{ t('common.refreshFailed', { error }) }}</span>
    <el-button link type="primary" size="small" @click="emit('retry')">{{ t('common.retry') }}</el-button>
  </div>
  <div v-else class="load-error" role="alert">
    <CloudOff :size="compact ? 28 : 40" class="icon" aria-hidden="true" />
    <div class="title">{{ t('common.loadFailed') }}</div>
    <div class="reason">{{ error }}</div>
    <el-button :icon="RotateCw" size="small" @click="emit('retry')">{{ t('common.retry') }}</el-button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { CloudOff, RotateCw, TriangleAlert } from 'lucide-vue-next'

// error 为失败原因；compact 为小尺寸（放在卡片或表格内时）
defineProps<{ error: string; bar?: boolean; compact?: boolean }>()
const emit = defineEmits<{ retry: [] }>()
const { t } = useI18n()
</script>

<style scoped>
.load-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 28px 16px;
  text-align: center;
}
.load-error .icon {
  color: var(--g-text-danger, var(--el-color-danger));
}
.load-error .title {
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.load-error .reason {
  max-width: 560px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
  overflow-wrap: anywhere;
}
.load-error .el-button {
  margin-top: 6px;
}
.load-error-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  padding: 6px 10px;
  border-radius: var(--g-radius-sm, 4px);
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-regular);
  background: var(--el-color-warning-light-9);
}
.load-error-bar .icon {
  flex-shrink: 0;
  color: var(--g-text-warning, var(--el-color-warning));
}
.load-error-bar .text {
  min-width: 0;
  overflow-wrap: anywhere;
}
</style>
