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
        <span :title="t('common.enable')">{{ t('common.enable') }}</span><span>{{ t('fileUsers.username') }}</span><span>{{ t('fileUsers.password') }}</span><span>{{ t('common.remark') }}</span><span>{{ t('fileUsers.directory') }}</span><span :title="t('fileUsers.readOnly')">{{ t('fileUsers.readOnly') }}</span><span />
      </div>
      <div v-for="(u, i) in users" :key="i" class="u-item" :class="{ 'is-off': u.disabled }">
        <div class="u-row">
          <div class="u-cell u-on">
            <label class="u-label">{{ t('common.enable') }}</label>
            <!-- disabled 为 true 表示停用，开关显示的是「启用」，取反存取 -->
            <el-switch :model-value="!u.disabled" :aria-label="t('common.enable')" @update:model-value="(v: string | number | boolean) => setEnabled(u, !!v)" />
          </div>
          <div class="u-cell u-name"><label class="u-label">{{ t('fileUsers.username') }}</label><el-input v-model="u.name" /></div>
          <div class="u-cell u-pass"><label class="u-label">{{ t('fileUsers.password') }}</label><SecretInput v-model="u.password" :reveal="revealOf(u)" /></div>
          <div class="u-cell u-remark"><label class="u-label">{{ t('common.remark') }}</label><el-input v-model="u.remark" :placeholder="t('fileUsers.remarkPlaceholder')" maxlength="200" /></div>
          <div class="u-cell u-root">
            <label class="u-label">{{ t('fileUsers.directory') }}</label>
            <template v-if="!u.mounts?.length">
              <el-input v-model="u.root" placeholder="/data/share" class="mono">
                <template #suffix><StorageMountPicker @pick="(m) => (u.root = m.path)" /></template>
              </el-input>
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
            <div class="u-cell m-path"><label class="u-label">{{ t('fileUsers.directory') }}</label><el-input v-model="mt.path" placeholder="/data/share" class="mono">
                <template #suffix><StorageMountPicker @pick="(m) => pickMount(mt, m)" /></template>
              </el-input></div>
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
import StorageMountPicker, { type LocalMount } from './StorageMountPicker.vue'
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
  // disabled 为 true 时暂停该用户，不能登录；没有该字段即为启用
  disabled?: boolean
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

// setEnabled 设置用户是否启用：启用时删除 disabled 字段，与旧配置（没有该字段）保持一致，避免产生无意义的修改
function setEnabled(u: FileUser, on: boolean) {
  if (on) {
    delete u.disabled
  } else {
    u.disabled = true
  }
}

// pickMount 用选中的存储挂载点填写多目录中的一项：名称为空时顺便用挂载点名称作为显示名称
function pickMount(mt: FileMount, m: LocalMount) {
  mt.path = m.path
  if (!mt.name.trim()) {
    mt.name = m.name
  }
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
  grid-template-columns: 64px minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr) minmax(0, 2fr) 60px 40px;
  gap: 10px;
  align-items: center;
}
.u-head {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  padding-bottom: 6px;
}
/* 列宽固定，较长的译文（如德语）截断显示，完整文字见悬停提示 */
.u-head > span {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.u-item {
  padding: 8px 0;
  border-top: 1px solid var(--g-border);
}
/* 停用的用户除开关外整体淡化 */
.u-item.is-off .u-row > .u-cell:not(.u-on),
.u-item.is-off .m-list {
  opacity: 0.55;
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
  .m-row {
    grid-template-columns: 1fr 1fr;
  }
  /* 手机上：用户名 | 密码；备注；目录；只读 | 启用 | 删除 */
  .u-row {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
  }
  .u-name {
    order: 1;
  }
  .u-pass {
    order: 2;
    grid-column: 2 / -1;
  }
  .u-remark {
    order: 3;
  }
  .u-root {
    order: 4;
  }
  .u-ro {
    order: 5;
  }
  .u-on {
    order: 6;
  }
  .u-del {
    order: 7;
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
  .u-ro,
  .u-on {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .u-ro .u-label,
  .u-on .u-label {
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
