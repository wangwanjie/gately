<template>
  <div class="g-card">
    <div class="g-card-head">
      <div class="g-card-title"><Users :size="16" />{{ title || t('fileUsers.title') }} <span class="g-card-desc">{{ desc || t('fileUsers.desc') }}</span></div>
      <div class="u-actions">
        <slot name="head" />
        <el-button size="small" :icon="UserPlus" @click="users.push({ name: '', password: '', root: '', readOnly: false, remark: '' })">{{ t('fileUsers.addUser') }}</el-button>
      </div>
    </div>
    <div class="g-card-body">
      <slot name="notice" />
      <div v-if="users.length" class="u-head">
        <span>{{ t('fileUsers.username') }}</span><span>{{ t('fileUsers.password') }}</span><span>{{ t('common.remark') }}</span><span>{{ t('fileUsers.directory') }}</span><span>{{ t('fileUsers.readOnly') }}</span><span />
      </div>
      <div v-for="(u, i) in users" :key="i" class="u-item">
        <div class="u-row">
          <div class="u-cell"><label class="u-label">{{ t('fileUsers.username') }}</label><el-input v-model="u.name" /></div>
          <div class="u-cell"><label class="u-label">{{ t('fileUsers.password') }}</label><SecretInput v-model="u.password" :reveal="revealOf(u)" /></div>
          <div class="u-cell u-remark"><label class="u-label">{{ t('common.remark') }}</label><el-input v-model="u.remark" :placeholder="t('fileUsers.remarkPlaceholder')" maxlength="200" /></div>
          <div class="u-cell u-root">
            <label class="u-label">{{ t('fileUsers.directory') }}</label>
            <template v-if="!u.mounts?.length">
              <el-input v-model="u.root" placeholder="/data/share" class="mono" />
              <el-button link type="primary" size="small" class="u-multi" @click="toMulti(u)">{{ t('fileUsers.mountMultiple') }}</el-button>
            </template>
            <span v-else class="u-summary">{{ t(mountSummaryKey || 'fileUsers.mountSummary', { count: u.mounts.length }) }}</span>
          </div>
          <div class="u-cell u-ro">
            <label class="u-label">{{ t('fileUsers.readOnly') }}</label>
            <el-switch v-if="!u.mounts?.length" v-model="u.readOnly" />
            <span v-else class="u-summary">{{ t('fileUsers.perDirectory') }}</span>
          </div>
          <div class="u-cell u-del"><el-button text type="danger" :icon="Trash2" :aria-label="t('fileUsers.deleteUser')" @click="users.splice(i, 1)" /></div>
        </div>
        <div v-if="u.mounts?.length" class="m-list">
          <div v-for="(mt, j) in u.mounts" :key="j" class="m-row">
            <div class="u-cell"><label class="u-label">{{ t('common.name') }}</label><el-input v-model="mt.name" :placeholder="mountNamePlaceholder || t('fileUsers.displayName')" /></div>
            <div class="u-cell m-path"><label class="u-label">{{ t('fileUsers.directory') }}</label><el-input v-model="mt.path" placeholder="/data/share" class="mono" /></div>
            <div class="u-cell u-ro"><label class="u-label">{{ t('fileUsers.readOnly') }}</label><el-switch v-model="mt.readOnly" /></div>
            <div class="u-cell u-del"><el-button text type="danger" :icon="Trash2" :aria-label="t('fileUsers.deleteDirectory')" @click="removeMount(u, j)" /></div>
          </div>
          <el-button link type="primary" size="small" :icon="FolderPlus" @click="u.mounts.push({ name: '', path: '', readOnly: false })">{{ t('fileUsers.addDirectory') }}</el-button>
        </div>
        <ConfigDirHint :paths="u.mounts?.length ? u.mounts.map((mt) => mt.path) : [u.root]" />
      </div>
      <el-empty v-if="!users.length" :description="t('fileUsers.empty')" :image-size="60" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { toRaw, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { FolderPlus, Trash2, UserPlus, Users } from 'lucide-vue-next'
import SecretInput from './SecretInput.vue'
import ConfigDirHint from './ConfigDirHint.vue'
import { fetchSecret } from '../secrets'

export interface FileMount {
  name: string
  path: string
  readOnly: boolean
}
export interface FileUser {
  name: string
  password: string
  root: string
  readOnly: boolean
  // mounts 非空时 root 与 readOnly 不再使用，根目录按名称列出各目录
  mounts?: FileMount[]
  // remark 为备注（例如账号的用途），只在管理界面展示
  remark?: string
}

const { t } = useI18n()
const users = defineModel<FileUser[]>({ required: true })
// 以下文案可由使用方覆盖：SMB 中每个目录是一个共享，说明与占位提示不同于 FTP / WebDAV
// title 与插槽供共享用户卡片使用：head 放在标题栏右侧，notice 放在用户列表上方
// secretsUrl 为查看已保存密码明文的接口（返回按用户名索引的密码表），不提供时密码框只切换显示
const props = defineProps<{ title?: string; desc?: string; mountSummaryKey?: string; mountNamePlaceholder?: string; secretsUrl?: string }>()

// savedNames 记录每个用户载入时的用户名：明文表按已保存的用户名索引，用户在界面上改名后仍按原名取回密码
const savedNames = new WeakMap<FileUser, string>()
watch(
  users,
  (list) => {
    for (const u of list) {
      if (!savedNames.has(toRaw(u))) {
        savedNames.set(toRaw(u), u.name)
      }
    }
  },
  { immediate: true },
)

// revealOf 返回取回用户已保存密码明文的函数
function revealOf(u: FileUser): (() => Promise<string | undefined>) | undefined {
  const url = props.secretsUrl
  if (!url) {
    return undefined
  }
  return () => fetchSecret(url, savedNames.get(toRaw(u)) ?? u.name)
}

// toMulti 把单目录用户转换为多目录：已填写的目录成为第一个挂载
function toMulti(u: FileUser) {
  const name = u.root.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || ''
  u.mounts = [{ name, path: u.root, readOnly: u.readOnly }]
  u.root = ''
  u.readOnly = false
}

// removeMount 删除最后一个目录时回到单目录模式，保留其目录与只读设置
function removeMount(u: FileUser, j: number) {
  const mounts = u.mounts ?? []
  if (mounts.length === 1) {
    u.root = mounts[0].path
    u.readOnly = mounts[0].readOnly
    u.mounts = []
    return
  }
  mounts.splice(j, 1)
}
</script>

<style scoped>
.u-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.u-head,
.u-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr) minmax(0, 2fr) 60px 40px;
  gap: 10px;
  align-items: center;
}
.u-head {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  padding-bottom: 6px;
}
.u-item {
  padding: 8px 0;
  border-top: 1px solid var(--g-border);
}
.u-label {
  display: none;
}
.u-root {
  display: flex;
  align-items: center;
  gap: 8px;
}
.u-multi {
  flex-shrink: 0;
}
.u-summary {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.m-list {
  margin: 8px 0 0 24px;
  padding-left: 12px;
  border-left: 2px solid var(--g-border);
}
.m-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 3fr) 60px 40px;
  gap: 10px;
  align-items: center;
  margin-bottom: 6px;
}
@media (max-width: 767.98px) {
  .u-head {
    display: none;
  }
  .u-item {
    padding: 12px;
    margin-bottom: 10px;
    border: 1px solid var(--g-border);
    border-radius: 8px;
  }
  .u-row,
  .m-row {
    grid-template-columns: 1fr 1fr;
  }
  .u-label {
    display: block;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-bottom: 2px;
  }
  .u-remark,
  .u-root {
    grid-column: 1 / -1;
    flex-wrap: wrap;
    gap: 4px 8px;
  }
  .u-root .u-label {
    width: 100%;
    margin-bottom: 0;
  }
  .m-path {
    grid-column: 1 / -1;
  }
  .u-ro {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .u-ro .u-label {
    margin: 0;
  }
  .u-del {
    display: flex;
    justify-content: flex-end;
  }
  .m-list {
    margin-left: 0;
  }
}
</style>
