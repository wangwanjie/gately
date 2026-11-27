<template>
  <div class="g-card">
    <div class="g-card-head">
      <div class="g-card-title"><Users :size="16" />{{ t('sharedFileUsers.title') }} <span class="g-card-desc">{{ t('sharedFileUsers.cardDesc') }}</span></div>
      <div class="s-actions">
        <span class="switch-label">{{ t('sharedFileUsers.enable') }}</span>
        <el-switch v-model="enabled" :aria-label="t('sharedFileUsers.enable')" />
      </div>
    </div>
    <div class="g-card-body">
      <div class="notice">
        <div v-if="loadError" class="tags">
          <el-tag size="small" type="danger">{{ t('sharedFileUsers.loadError', { error: loadError }) }}</el-tag>
        </div>
        <div v-if="users.length" class="summary">
          <span>{{ t('sharedFileUsers.count', { count: users.length }) }}</span>
          <div class="tags">
            <el-tag v-for="u in users" :key="u.name" size="small" :type="tagType(u.name)">{{ u.name }}</el-tag>
          </div>
        </div>
        <div v-else>{{ t('sharedFileUsers.none') }}</div>
        <el-tag v-if="!enabled" size="small" type="info">{{ t('sharedFileUsers.off') }}</el-tag>
        <div v-if="enabled && shadowed.length" class="tags">
          <el-tag size="small" type="warning">{{ t('sharedFileUsers.shadowed', { names: shadowed.join(', ') }) }}</el-tag>
        </div>
        <div v-if="enabled && issues?.length" class="tags">
          <el-tag v-for="is in issues" :key="is.name" size="small" type="warning">{{ issueText(is) }}</el-tag>
        </div>
      </div>
      <router-link to="/fileusers" class="manage">
        <el-button size="small" :icon="Settings2">{{ t('sharedFileUsers.manage') }}</el-button>
      </router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Settings2, Users } from 'lucide-vue-next'
import type { FileUser } from './FileUsers.vue'
import { issueText as formatIssue, shadowedNames, type SMBIssue } from '../composables/useSharedFileUsers'

// 服务页面上的共享用户摘要：共享用户在「文件共享用户」页面统一维护，这里只显示已保存的列表并提供本服务的开关
const props = defineProps<{
  // users 为已保存的共享用户
  users: FileUser[]
  // localNames 为本服务表单中的本地用户名，同名的共享用户只按本地用户校验
  localNames: string[]
  // caseInsensitive 为 true 时按不区分大小写比较用户名（SMB）
  caseInsensitive?: boolean
  // issues 为无法用于本服务的共享用户（已保存的内容），只有 SMB 页面传入
  issues?: SMBIssue[]
  // loadError 为服务端读取到的共享用户配置未通过校验的错误
  loadError?: string
}>()
const enabled = defineModel<boolean>('enabled', { required: true })
const { t } = useI18n()

const issueText = (is: SMBIssue) => formatIssue(t, is)

const shadowed = computed(() => shadowedNames(props.users, props.localNames, props.caseInsensitive))

// tagType 让开关打开时被本地用户遮蔽或无法使用的共享用户显示为警告色
function tagType(name: string): 'warning' | 'info' | 'primary' {
  // 已停用的共享用户不能登录，与开关关闭时一样显示为灰色
  if (!enabled.value || props.users.some((u) => u.name === name && u.disabled)) {
    return 'info'
  }
  if (shadowed.value.includes(name) || props.issues?.some((i) => i.name === name)) {
    return 'warning'
  }
  return 'primary'
}
</script>

<style scoped>
.s-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.switch-label {
  font-size: 13px;
  color: var(--el-text-color-regular);
}
.notice {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
  margin-bottom: 12px;
}
.summary {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.tags .el-tag {
  max-width: 100%;
  height: auto;
  white-space: normal;
  line-height: 1.5;
  padding: 2px 8px;
}
.manage {
  text-decoration: none;
}
</style>
