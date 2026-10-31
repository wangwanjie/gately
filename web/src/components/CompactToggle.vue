<template>
  <!-- 卡片 / 紧凑 视图切换；窄屏下紧凑视图不生效，因此隐藏 -->
  <el-radio-group v-if="!isMobile" v-model="model" class="compact-toggle" :class="{ 'is-small': size === 'small' }" :size="size">
    <el-tooltip :content="t('common.viewCard')" placement="top">
      <el-radio-button :value="false"><LayoutGrid :size="15" /></el-radio-button>
    </el-tooltip>
    <el-tooltip :content="t('common.viewCompact')" placement="top">
      <el-radio-button :value="true"><Rows3 :size="15" /></el-radio-button>
    </el-tooltip>
  </el-radio-group>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { LayoutGrid, Rows3 } from 'lucide-vue-next'
import { useIsMobile } from '../composables/useIsMobile'

// size 为 small 时用于卡片标题栏
defineProps<{ size?: 'default' | 'small' }>()
const { t } = useI18n()
const isMobile = useIsMobile()
// true 表示紧凑视图
const model = defineModel<boolean>({ default: false })
</script>

<style scoped>
.compact-toggle :deep(.el-radio-button__inner) {
  display: inline-flex;
  align-items: center;
  padding: 8px 11px;
}
.compact-toggle.is-small :deep(.el-radio-button__inner) {
  padding: 5px 9px;
}
</style>
