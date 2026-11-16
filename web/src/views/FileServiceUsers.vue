<template>
  <div>
    <PageHeader :title="t('sharedFileUsers.page.title')" doc="modules/tools#shared-users" :desc="t('sharedFileUsers.page.desc')" :icon="UsersRound">
      <el-button type="primary" :icon="Save" :loading="saving" :disabled="!shared.loaded.value" @click="save">{{ t('common.save') }}</el-button>
    </PageHeader>

    <el-alert v-if="shared.loadError.value" type="error" :title="t('sharedFileUsers.loadError', { error: shared.loadError.value })" :closable="false" show-icon class="block-gap" />

    <div v-if="shared.loaded.value" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Server :size="16" />{{ t('sharedFileUsers.services.title') }} <span class="g-card-desc">{{ t('sharedFileUsers.services.desc') }}</span></div>
      </div>
      <div class="g-card-body">
        <div v-for="s in shared.services.value" :key="s.id" class="svc-row">
          <div class="svc-main">
            <router-link :to="'/' + s.id" class="svc-name">{{ serviceTitle(s.id) }}</router-link>
            <el-tag v-if="!s.enable" size="small" type="info">{{ t('sharedFileUsers.services.disabled') }}</el-tag>
            <el-tag v-if="s.sharedUsers" size="small" type="success">{{ t('sharedFileUsers.services.on') }}</el-tag>
            <el-tag v-else size="small" type="info">{{ t('sharedFileUsers.services.off') }}</el-tag>
          </div>
          <div v-if="shadowedOf(s).length || s.issues?.length" class="tags">
            <el-tag v-if="shadowedOf(s).length" size="small" type="warning">{{ t('sharedFileUsers.shadowed', { names: shadowedOf(s).join(', ') }) }}</el-tag>
            <el-tag v-for="is in s.issues ?? []" :key="is.name" size="small" type="warning">{{ issueText(t, is) }}</el-tag>
          </div>
        </div>
        <el-empty v-if="!shared.services.value.length" :description="t('sharedFileUsers.services.none')" :image-size="60" />
      </div>
    </div>

    <div v-if="shared.loaded.value" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><KeyRound :size="16" />{{ t('sharedFileUsers.selfService.title') }}</div>
        <el-switch v-model="shared.selfService.value" />
      </div>
      <div class="g-card-body">
        <div class="notice no-gap">{{ t('sharedFileUsers.selfService.help', { path: '/__gately_auth/password' }) }}</div>
      </div>
    </div>

    <FileUsers v-if="shared.loaded.value" v-model="shared.users.value" :title="t('sharedFileUsers.title')" :desc="t('sharedFileUsers.usersDesc')" secrets-url="/api/fileusers/secrets">
      <template #notice>
        <div class="notice">{{ t('sharedFileUsers.help') }}</div>
      </template>
    </FileUsers>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { KeyRound, Save, Server, UsersRound } from 'lucide-vue-next'
import PageHeader from '../components/PageHeader.vue'
import FileUsers from '../components/FileUsers.vue'
import { issueText, shadowedNames, useSharedFileUsers, type FileService } from '../composables/useSharedFileUsers'
import { navByModule } from '../nav'

// 文件共享用户页面：WebDAV、FTP、SMB 共用的用户在这里统一维护，各服务是否允许共享用户登录在各服务页面设置
const { t } = useI18n()
const shared = useSharedFileUsers()
const saving = ref(false)

function serviceTitle(id: string): string {
  const nav = navByModule(id)
  return nav ? t(nav.titleKey) : id
}

// shadowedOf 按表单中正在编辑的共享用户计算，在该服务中被同名本地用户遮蔽的用户
function shadowedOf(s: FileService): string[] {
  if (!s.sharedUsers) {
    return []
  }
  return shadowedNames(shared.users.value, s.localUsers, s.caseInsensitive)
}

async function save() {
  saving.value = true
  try {
    await shared.save()
    ElMessage.success(t('common.saved'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

onMounted(() => shared.load())
</script>

<style scoped>
.block-gap {
  margin-bottom: 16px;
}
.svc-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 0;
  border-top: 1px solid var(--g-border);
}
.svc-row:first-child {
  border-top: none;
  padding-top: 0;
}
.svc-row:last-of-type {
  padding-bottom: 0;
}
.svc-main {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.svc-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--el-color-primary);
  text-decoration: none;
  margin-right: 4px;
}
.svc-name:hover {
  text-decoration: underline;
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
.notice {
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
  margin-bottom: 10px;
}
.notice.no-gap {
  margin-bottom: 0;
}
</style>
