<template>
  <!-- 目录与 Gately 配置目录关系的提示：位于其中会被拒绝；包含它时配置目录会被自动隐藏（交给外部程序的目录不行） -->
  <div v-if="level" class="cfgdir-hint" :class="level" data-test="config-dir-hint">
    <component :is="level === 'error' ? CircleAlert : Info" :size="14" class="icon" />
    <span>{{ text }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { CircleAlert, Info } from 'lucide-vue-next'
import { configDirRelation, type ConfigDirRelation } from '../configDir'
import { useConfigDir } from '../composables/useConfigDir'

const props = defineProps<{
  // 要检查的目录（多个时按最严重的一个提示）
  paths: string[]
  // whole 为 true 表示目录整个交给外部程序（File Browser），无法隐藏其中的配置目录，包含它同样不行
  whole?: boolean
}>()

const { t } = useI18n()
const info = useConfigDir()

const relation = computed<ConfigDirRelation>(() => {
  const c = info.value
  if (!c?.dir) {
    return ''
  }
  let worst: ConfigDirRelation = ''
  for (const p of props.paths) {
    const r = configDirRelation(p ?? '', c.dir, c.fold)
    if (r === 'inside') {
      return r
    }
    if (r === 'contains') {
      worst = r
    }
  }
  return worst
})

const level = computed(() => {
  if (relation.value === 'inside' || (relation.value === 'contains' && props.whole)) {
    return 'error'
  }
  return relation.value === 'contains' ? 'info' : ''
})

const text = computed(() => {
  const dir = info.value?.dir ?? ''
  if (relation.value === 'inside') {
    return t('common.configDirInside', { dir })
  }
  return props.whole ? t('common.configDirWhole', { dir }) : t('common.configDirContains', { dir })
})
</script>

<style scoped>
.cfgdir-hint {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.5;
  width: 100%;
}
.cfgdir-hint .icon {
  flex: none;
  margin-top: 2px;
}
.cfgdir-hint.info {
  color: var(--el-text-color-secondary);
}
.cfgdir-hint.error {
  color: var(--el-color-danger);
}
</style>
