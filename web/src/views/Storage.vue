<template>
  <div>
    <PageHeader :title="t('storage.title')" :desc="t('storage.desc')" :icon="HardDrive">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('groupSort.search')" />
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('storage.addMount') }}</el-button>
    </PageHeader>

    <GroupedList :gl="gl" :compact="compact" grid :empty-text="t('storage.empty')">
      <template #item="{ it, item: m, sorting, bind }">
        <EntityCard v-bind="bind" :class="{ active: m.key === current?.key }" :compact="compact || sorting">
          <template #lead>
            <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
            <div v-else class="mt-icon"><component :is="m.type === 'local' ? Folder : m.type === 'smb' ? Server : Cloud" :size="18" /></div>
          </template>
          <template #title>{{ m.name }}</template>
          <template v-if="m.remark" #sub>{{ m.remark }}</template>
          <template #status>
            <StatusBadge v-if="!m.enable" type="off" :text="t('common.disabled')" />
            <StatusBadge v-else-if="m.readOnly" type="info" :text="t('storage.readOnly')" />
            <StatusBadge v-else type="running" :text="t('storage.readWrite')" />
          </template>
          <Field :label="m.type === 'local' ? t('storage.localDir') : m.type === 'smb' ? 'SMB' : 'WebDAV'" mono>{{ m.type === 'local' ? m.path : m.url }}</Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-switch :model-value="m.enable" size="small" @change="(v: boolean) => toggle(m, v)" />
            <el-button text size="small" :icon="Pencil" @click="openEditor(m)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('storage.confirmRemove')" @confirm="remove(m)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('storage.remove') }}</el-button>
              </template>
            </el-popconfirm>
            <el-button type="primary" size="small" :icon="FolderOpen" :disabled="!m.enable" @click="browse(m)">{{ t('storage.browse') }}</el-button>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <div v-if="current" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><FolderOpen :size="16" />{{ current.name }}</div>
        <div class="toolbar">
          <el-button size="small" :icon="RefreshCw" @click="refresh">{{ t('common.refresh') }}</el-button>
          <template v-if="!current.readOnly">
            <el-button size="small" :icon="FolderPlus" @click="newFolder">{{ t('storage.newFolder') }}</el-button>
            <el-button size="small" type="primary" :icon="Upload" :loading="uploading" @click="pickFiles">{{ t('common.upload') }}</el-button>
          </template>
          <input ref="fileInput" type="file" multiple hidden @change="onPicked" />
        </div>
      </div>
      <div class="g-card-body">
        <el-breadcrumb separator="/" class="crumbs">
          <el-breadcrumb-item v-for="(c, i) in crumbs" :key="i">
            <a class="crumb" @click="go(c.path)">{{ c.name }}</a>
          </el-breadcrumb-item>
        </el-breadcrumb>
        <el-alert v-if="listError" type="error" :title="listError" :closable="false" show-icon />
        <el-table v-else v-loading="loading" :data="entries" :empty-text="t('storage.emptyDir')" size="small">
          <el-table-column :label="t('common.name')" min-width="220">
            <template #default="{ row }">
              <a class="name" @click="open(row)">
                <component :is="row.isDir ? Folder : File" :size="15" class="ico" />{{ row.name }}
              </a>
            </template>
          </el-table-column>
          <el-table-column :label="t('storage.size')" width="110" align="right">
            <template #default="{ row }">{{ row.isDir ? '—' : fmtSize(row.size) }}</template>
          </el-table-column>
          <el-table-column :label="t('storage.modTime')" width="170" class-name="hide-sm">
            <template #default="{ row }">{{ fmtTime(row.modTime) }}</template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="150" align="right">
            <template #default="{ row }">
              <el-button v-if="!row.isDir" text size="small" :icon="Download" :aria-label="t('common.download')" @click="download(row)" />
              <template v-if="!current.readOnly">
                <el-button text size="small" :icon="Pencil" :aria-label="t('storage.rename')" @click="rename(row)" />
                <el-popconfirm :title="row.isDir ? t('storage.confirmDeleteDir', { name: row.name }) : t('storage.confirmDeleteFile', { name: row.name })" @confirm="del(row)">
                  <template #reference>
                    <el-button text size="small" type="danger" :icon="Trash2" :aria-label="t('common.delete')" />
                  </template>
                </el-popconfirm>
              </template>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <el-dialog v-model="editorVisible" :title="editing.key ? t('storage.editMount') : t('storage.addMount')" width="540px" :fullscreen="isMobile">
      <el-form :model="editing" label-width="110px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('storage.type')">
          <el-radio-group v-model="editing.type">
            <el-radio-button value="local">{{ t('storage.localDir') }}</el-radio-button>
            <el-radio-button value="webdav">{{ t('storage.remoteWebdav') }}</el-radio-button>
            <el-radio-button value="smb">{{ t('storage.remoteSmb') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="editing.type === 'local'" :label="t('storage.dir')">
          <el-input v-model="editing.path" class="mono" placeholder="/data/share" />
        </el-form-item>
        <template v-else>
          <el-form-item :label="t('storage.url')">
            <el-input v-model="editing.url" class="mono" :placeholder="editing.type === 'smb' ? 'smb://192.168.1.10/share' : 'https://nas.example.com:5006/dav'" />
            <div v-if="editing.type === 'smb'" class="form-help">{{ t('storage.smbUrlHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('storage.username')"><el-input v-model="editing.username" /></el-form-item>
          <el-form-item :label="t('storage.password')"><SecretInput v-model="editing.password" :reveal="secretOf(editing.key && `/api/storage/mounts/${editing.key}/secrets`, 'password')" /></el-form-item>
          <el-form-item v-if="editing.type === 'smb'" :label="t('storage.domain')">
            <el-input v-model="editing.domain" placeholder="WORKGROUP" />
            <div class="form-help">{{ t('storage.domainHelp') }}</div>
          </el-form-item>
          <el-form-item v-else :label="t('storage.skipVerify')">
            <el-switch v-model="editing.skipVerify" />
            <span class="help">{{ t('storage.skipVerifyHelp') }}</span>
          </el-form-item>
        </template>
        <el-form-item :label="t('storage.readOnlyLabel')"><el-switch v-model="editing.readOnly" /></el-form-item>
        <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Cloud, Download, File, Folder, FolderOpen, FolderPlus, HardDrive, Pencil, Plus, RefreshCw, Server, Trash2, Upload } from 'lucide-vue-next'
import { api } from '../api'
import SecretInput from '../components/SecretInput.vue'
import { secretOf } from '../secrets'
import PageHeader from '../components/PageHeader.vue'
import EntityCard from '../components/EntityCard.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import { useGroupedList } from '../composables/useGroupSort'
import CompactToggle from '../components/CompactToggle.vue'
import { useCompactView } from '../composables/useCompactView'
import Field from '../components/Field.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { useFormLayout } from '../composables/useIsMobile'

interface Mount {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  type: 'local' | 'webdav' | 'smb'
  remark: string
  enable: boolean
  readOnly: boolean
  path: string
  url: string
  username: string
  password: string
  skipVerify: boolean
  // SMB 认证所用的域或工作组，仅 smb 类型使用
  domain?: string
}
interface Entry {
  name: string
  isDir: boolean
  size: number
  modTime: string
}

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
const mounts = ref<Mount[]>([])
const compact = useCompactView('storage')
// 挂载列表的搜索、分组与排序
const gl = useGroupedList<Mount>({
  storageKey: 'storage.groups',
  items: () => mounts.value,
  base: '/api/storage/mounts',
  reload: load,
  matches: (m, q) => [m.name, m.group ?? '', m.remark, m.path, m.url].join('\n').toLowerCase().includes(q),
})
const editorVisible = ref(false)
const editing = ref<Mount>(empty())

const current = ref<Mount>()
const cwd = ref('/')
const entries = ref<Entry[]>([])
const loading = ref(false)
const listError = ref('')
const uploading = ref(false)
const fileInput = ref<HTMLInputElement>()

function empty(): Mount {
  return { name: '', type: 'local', remark: '', enable: true, readOnly: false, path: '', url: '', username: '', password: '', skipVerify: false, domain: '' }
}

const crumbs = computed(() => {
  const out = [{ name: t('storage.root'), path: '/' }]
  let acc = ''
  for (const seg of cwd.value.split('/').filter(Boolean)) {
    acc += '/' + seg
    out.push({ name: seg, path: acc })
  }
  return out
})

function join(dir: string, name: string): string {
  return (dir === '/' ? '' : dir) + '/' + name
}

function fmtSize(n: number): string {
  if (n < 1024) {
    return `${n} B`
  }
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 ? 0 : 1)} ${units[i]}`
}

function fmtTime(s: string): string {
  const d = new Date(s)
  return isNaN(d.getTime()) || d.getFullYear() < 1980 ? '—' : d.toLocaleString()
}

async function load() {
  mounts.value = await api.get('/api/storage/mounts')
  if (current.value) {
    current.value = mounts.value.find((m) => m.key === current.value?.key)
  }
}

function openEditor(m?: Mount) {
  editing.value = m ? JSON.parse(JSON.stringify(m)) : empty()
  editorVisible.value = true
}

async function save() {
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/storage/mounts/${key}`, body)
    } else {
      await api.post('/api/storage/mounts', body)
    }
    editorVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function toggle(m: Mount, enable: boolean) {
  try {
    await api.put(`/api/storage/mounts/${m.key}/enable`, { enable })
    if (!enable && current.value?.key === m.key) {
      current.value = undefined
    }
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(m: Mount) {
  try {
    await api.del(`/api/storage/mounts/${m.key}`)
    if (current.value?.key === m.key) {
      current.value = undefined
    }
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

function browse(m: Mount) {
  current.value = m
  go('/')
}

async function go(p: string) {
  if (!current.value) {
    return
  }
  loading.value = true
  listError.value = ''
  try {
    const r = await api.get<{ path: string; entries: Entry[] | null }>(`/api/storage/files/${current.value.key}/list?path=${encodeURIComponent(p)}`)
    cwd.value = r.path
    entries.value = r.entries ?? []
  } catch (e) {
    listError.value = (e as Error).message
    entries.value = []
  } finally {
    loading.value = false
  }
}

const refresh = () => go(cwd.value)

function open(e: Entry) {
  if (e.isDir) {
    go(join(cwd.value, e.name))
  } else {
    download(e)
  }
}

function download(e: Entry) {
  // 浏览器携带会话 Cookie 直接下载，不经过内存
  const a = document.createElement('a')
  a.href = `/api/storage/files/${current.value?.key}/download?path=${encodeURIComponent(join(cwd.value, e.name))}`
  a.download = e.name
  a.click()
}

function pickFiles() {
  fileInput.value?.click()
}

async function onPicked() {
  const input = fileInput.value
  if (!input?.files?.length || !current.value) {
    return
  }
  uploading.value = true
  try {
    // 逐个文件上传，并携带大小供远程 WebDAV 使用
    for (const f of Array.from(input.files)) {
      const fd = new FormData()
      fd.append('file', f, f.name)
      await api.post(`/api/storage/files/${current.value.key}/upload?path=${encodeURIComponent(cwd.value)}&size=${f.size}`, fd)
    }
    ElMessage.success(t('storage.uploadDone'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    input.value = ''
    uploading.value = false
    await refresh()
  }
}

async function newFolder() {
  try {
    const { value } = await ElMessageBox.prompt(t('storage.folderName'), t('storage.newFolder'), { inputPattern: /^[^/\\]+$/, inputErrorMessage: t('storage.nameInvalid') })
    await api.post(`/api/storage/files/${current.value?.key}/mkdir`, { path: join(cwd.value, value) })
    await refresh()
  } catch (e) {
    if (e !== 'cancel') {
      ElMessage.error((e as Error).message)
    }
  }
}

async function rename(e: Entry) {
  try {
    const { value } = await ElMessageBox.prompt(t('storage.newName'), t('storage.rename'), { inputValue: e.name, inputPattern: /^[^/\\]+$/, inputErrorMessage: t('storage.nameInvalid') })
    await api.post(`/api/storage/files/${current.value?.key}/rename`, { from: join(cwd.value, e.name), to: join(cwd.value, value) })
    await refresh()
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error((err as Error).message)
    }
  }
}

async function del(e: Entry) {
  try {
    await api.del(`/api/storage/files/${current.value?.key}?path=${encodeURIComponent(join(cwd.value, e.name))}`)
    await refresh()
  } catch (err) {
    ElMessage.error((err as Error).message)
  }
}

onMounted(load)
</script>

<style scoped>
/* 空状态占满整行网格，才能在页面中水平居中 */
.toolbar {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.crumbs {
  margin-bottom: 12px;
}
.crumb,
.name {
  cursor: pointer;
}
.name {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.name:hover,
.crumb:hover {
  color: var(--el-color-primary);
}
.ico {
  flex: none;
  color: var(--el-text-color-secondary);
}
.mt-icon {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: var(--el-fill-color);
  color: var(--el-color-primary);
}
.active {
  outline: 2px solid var(--el-color-primary);
  outline-offset: -2px;
}
@media (max-width: 767.98px) {
  :deep(.hide-sm) {
    display: none;
  }
}
</style>
