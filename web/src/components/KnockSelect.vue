<template>
  <!-- 选择端口敲门规则：留空表示不要求敲门；引用的规则已不存在时仍显示其 key，便于发现并清除 -->
  <el-select v-model="model" clearable filterable :size="size" :placeholder="t('knockSelect.placeholder')" :no-data-text="t('knockSelect.empty')" :empty-values="[null, undefined, '']" :value-on-clear="''" class="knock-select">
    <el-option v-for="r in options" :key="r.key" :label="r.name" :value="r.key">
      <span>{{ r.name }}</span>
      <span v-if="r.missing" class="opt-warn">{{ t('knockSelect.missing') }}</span>
      <span v-else-if="!r.enable" class="opt-warn">{{ t('knockSelect.disabled') }}</span>
    </el-option>
  </el-select>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useKnockRules } from '../composables/useKnockRules'

defineProps<{ size?: 'small' | 'default' | 'large' }>()
const model = defineModel<string | undefined>({ default: '' })
const { t } = useI18n()
const rules = useKnockRules()

// options 为可选的敲门规则；当前引用的规则不在列表中（已删除或没有查看权限）时补一项
const options = computed(() => {
  const list = rules.value.map((r) => ({ ...r, missing: false }))
  if (model.value && !list.some((r) => r.key === model.value)) {
    list.push({ key: model.value, name: model.value, enable: false, missing: true })
  }
  return list
})
</script>

<style scoped>
.knock-select {
  width: 100%;
}
.opt-warn {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-color-warning);
}
</style>
