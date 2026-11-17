<template>
  <!-- 侧边栏底部的「添加更多功能」入口：有关闭的功能模块时显示，指向「设置 → 功能模块」 -->
  <div v-if="show" class="add-features" :class="{ collapsed }">
    <el-tooltip :content="t('onboarding.addFeatures.title')" placement="right" :disabled="!collapsed" :show-after="200">
      <router-link :to="{ path: '/settings', query: { tab: 'modules' } }" class="af-link" data-testid="add-features" @click="$emit('navigate')">
        <CirclePlus :size="18" :stroke-width="1.9" class="af-icon" />
        <span v-if="!collapsed" class="af-text">{{ t('onboarding.addFeatures.title') }}</span>
      </router-link>
    </el-tooltip>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { CirclePlus } from 'lucide-vue-next'
import { disabledModules, sessionRole } from '../nav'

defineProps<{ collapsed?: boolean }>()
defineEmits<{ navigate: [] }>()

const { t } = useI18n()
// 只读账号不能修改模块开关；全部开启时没有可添加的功能
const show = computed(() => sessionRole.value === 'admin' && disabledModules.value.length > 0)
</script>

<style scoped>
.add-features {
  flex-shrink: 0;
  padding: 8px 10px 12px;
  border-top: 1px solid var(--g-border);
}
.af-link {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 38px;
  padding: 0 10px;
  border-radius: 8px;
  border: 1px dashed var(--el-border-color);
  color: var(--el-text-color-regular);
  text-decoration: none;
  font-size: 14px;
  transition:
    background 0.15s,
    color 0.15s;
}
.collapsed .af-link {
  justify-content: center;
  padding: 0;
}
.af-link:hover {
  border-color: var(--el-color-primary-light-5);
  background: var(--el-fill-color-light);
  color: var(--el-color-primary);
}
.af-icon {
  flex-shrink: 0;
}
.af-text {
  white-space: nowrap;
}
</style>
