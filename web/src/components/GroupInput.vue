<template>
  <!-- 编辑对话框中的“分组”表单项：可直接输入新分组，或从已有分组中选择；留空为未分组 -->
  <el-form-item :label="t('groupSort.group')">
    <el-autocomplete
      v-model="model"
      class="group-input"
      clearable
      :maxlength="MAX_GROUP_LEN"
      :fetch-suggestions="(q: string, cb: (v: { value: string }[]) => void) => cb(groupSuggestions(groups, q))"
      :placeholder="t('groupSort.groupPlaceholder')"
    />
  </el-form-item>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { MAX_GROUP_LEN, groupSuggestions } from '../groupSort'

// groups 为已有分组名称，用于输入提示
defineProps<{ groups: string[] }>()
const model = defineModel<string>({ default: '' })
const { t } = useI18n()
</script>

<style scoped>
.group-input {
  width: 260px;
  max-width: 100%;
}
</style>
