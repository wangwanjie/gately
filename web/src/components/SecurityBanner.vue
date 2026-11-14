<template>
  <!-- 管理端安全加固提示：存在高风险时一直显示，只有“建议加固”级别可以暂缓 7 天 -->
  <div v-if="level" class="sec-banner" :class="level" role="alert">
    <component :is="level === 'risk' ? ShieldAlert : ShieldCheck" :size="18" class="sb-icon" />
    <span class="sb-text">{{ text }}</span>
    <div class="sb-actions">
      <el-button size="small" :type="level === 'risk' ? 'danger' : 'warning'" @click="$emit('fix', first)">{{ t('security.banner.fix') }}</el-button>
      <el-button v-if="level === 'warn'" size="small" text @click="$emit('snooze')">{{ t('security.banner.snooze') }}</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ShieldAlert, ShieldCheck } from 'lucide-vue-next'
import { failingItems, type SecStatus } from '../security'

const props = defineProps<{ status?: SecStatus; level: '' | 'warn' | 'risk' }>()
defineEmits<{ (e: 'fix', id: string): void; (e: 'snooze'): void }>()
const { t } = useI18n()

const items = computed(() => failingItems(props.status))
const first = computed(() => items.value[0]?.id ?? 'password')
// 最多列出 3 项，其余用“等”概括，避免窄屏上提示过长
const text = computed(() => {
  const names = items.value.slice(0, 3).map((it) => t(`security.items.${it.id}.title`))
  const joined = names.join(t('security.banner.sep')) + (items.value.length > 3 ? t('security.banner.more') : '')
  return props.level === 'risk' ? t('security.banner.risk', { items: joined }) : t('security.banner.warn', { score: props.status?.score ?? 0, items: joined })
})
</script>

<style scoped>
.sec-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  margin-bottom: 16px;
  border-radius: var(--g-radius);
  border: 1px solid var(--el-color-warning-light-5);
  background: var(--el-color-warning-light-9);
  font-size: 13px;
  line-height: 1.5;
}
.sec-banner.risk {
  border-color: var(--el-color-danger-light-5);
  background: var(--el-color-danger-light-9);
}
.sb-icon {
  flex-shrink: 0;
  color: var(--el-color-warning);
}
.risk .sb-icon {
  color: var(--el-color-danger);
}
.sb-text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
.sb-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}
@media (max-width: 767.98px) {
  .sec-banner {
    flex-wrap: wrap;
    align-items: flex-start;
  }
  .sb-text {
    flex-basis: calc(100% - 30px);
  }
  .sb-actions {
    margin-left: 28px;
  }
}
</style>
