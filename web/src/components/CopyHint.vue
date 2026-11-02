<template>
  <!-- 复制条目时在编辑框顶部提示模板来源；secrets 为 true 时说明未修改的密钥沿用模板 -->
  <el-alert v-if="from" type="info" :closable="false" show-icon class="copy-hint">
    <template #title>
      {{ t('common.copyHint', { name: from.name || '-' }) }}<template v-if="secrets"> {{ t('common.copyHintSecrets') }}</template>
    </template>
  </el-alert>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { CopySource } from '../copyDraft'

defineProps<{ from?: CopySource | null; secrets?: boolean }>()
const { t } = useI18n()
</script>

<style scoped>
.copy-hint {
  margin-bottom: 16px;
}
.copy-hint :deep(.el-alert__title) {
  word-break: break-word;
}
</style>
