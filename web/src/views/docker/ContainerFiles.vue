<template>
  <div class="files">
    <div class="bar">
      <div class="crumbs">
        <button type="button" class="crumb" @click="go('/')">/</button>
        <template v-for="(c, i) in crumbs" :key="c.path">
          <button type="button" class="crumb" @click="go(c.path)">{{ c.name }}</button>
          <span v-if="i < crumbs.length - 1" class="sep">/</span>
        </template>
      </div>
      <div class="ops">
        <el-button :icon="ArrowUp" :disabled="path === '/'" @click="go(parent)">{{ t('docker.container.files.up') }}</el-button>
        <el-button :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <el-button :icon="FolderPlus" @click="mkdir">{{ t('docker.container.files.newFolder') }}</el-button>
        <el-button type="primary" :icon="Upload" :loading="uploading" @click="picker?.click()">{{ t('common.upload') }}</el-button>
        <input ref="picker" type="file" multiple class="hidden" @change="onPick" />
      </div>
    </div>

    <!-- 目录条目多时只渲染当前页 -->
    <div ref="listEl" v-loading="loading" class="list">
      <div v-for="f in filePager.rows.value" :key="f.path" class="item" @click="open(f)">
        <component :is="f.dir ? Folder : File" :size="20" class="ico" :class="{ dir: f.dir }" />
        <div class="main">
          <div class="name">{{ f.name }}<span v-if="f.link" class="link mono"> → {{ f.link }}</span></div>
          <div class="sub">{{ f.dir ? '' : formatBytes(f.size) + ' · ' }}{{ formatTime(f.modified) }}<span v-if="f.mode" class="mono"> · {{ f.mode }}</span></div>
        </div>
        <div class="acts" @click.stop>
          <template v-if="!isMobile">
            <el-button text :icon="Download" :aria-label="t('common.download')" @click="downloadFile(f)" />
            <el-button text :icon="Pencil" :aria-label="t('docker.container.files.rename')" @click="rename(f)" />
            <el-button text type="danger" :icon="Trash2" :aria-label="t('common.delete')" @click="remove(f)" />
          </template>
          <el-dropdown v-else trigger="click" @command="(c: string) => onCmd(c, f)">
            <el-button text :icon="Ellipsis" class="touch" :aria-label="t('common.more')" />
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="download" :icon="Download">{{ t('common.download') }}</el-dropdown-item>
                <el-dropdown-item command="rename" :icon="Pencil">{{ t('docker.container.files.rename') }}</el-dropdown-item>
                <el-dropdown-item command="delete" :icon="Trash2" divided>{{ t('common.delete') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>
      <el-empty v-if="!loading && !entries.length" :description="t('docker.container.files.emptyDir')" :image-size="60" />
    </div>
    <ListPager :pager="filePager" :anchor="listEl" />

    <el-dialog v-model="editVisible" :title="editPath" :width="'860px'" :fullscreen="isMobile" top="4vh" append-to-body>
      <el-alert v-if="truncated" type="warning" :closable="false" show-icon class="gap" :title="t('docker.container.files.truncated')" />
      <el-input v-model="content" type="textarea" :rows="isMobile ? 18 : 24" class="editor mono" spellcheck="false" :readonly="truncated" />
      <template #footer>
        <el-button @click="editVisible = false">{{ t('common.close') }}</el-button>
        <el-button type="primary" :icon="Save" :disabled="truncated" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowUp, Download, Ellipsis, File, Folder, FolderPlus, Pencil, RefreshCw, Save, Trash2, Upload } from 'lucide-vue-next'
import { api } from '../../api'
import { useIsMobile } from '../../composables/useIsMobile'
import { usePager } from '../../composables/usePager'
import ListPager from '../../components/ListPager.vue'
import { download, formatBytes, formatTime } from './format'
import type { FileEntry } from './types'

const { t } = useI18n()

const props = defineProps<{ containerId: string }>()
const isMobile = useIsMobile()
const base = `/api/docker/containers/${props.containerId}/files`

const path = ref('/')
const entries = ref<FileEntry[]>([])
const loading = ref(false)
const uploading = ref(false)
const picker = ref<HTMLInputElement>()
const editVisible = ref(false)
const editPath = ref('')
const content = ref('')
const truncated = ref(false)
const saving = ref(false)
// 目录列表分页：进入其他目录时回到第 1 页，刷新与增删后保持当前页
const filePager = usePager(() => entries.value, { storageKey: 'docker.files', resetOn: path })
const listEl = ref<HTMLElement | null>(null)

const crumbs = computed(() => {
  const parts = path.value.split('/').filter(Boolean)
  return parts.map((name, i) => ({ name, path: '/' + parts.slice(0, i + 1).join('/') }))
})
const parent = computed(() => {
  const parts = path.value.split('/').filter(Boolean)
  parts.pop()
  return '/' + parts.join('/')
})

function join(dir: string, name: string): string {
  return dir === '/' ? `/${name}` : `${dir}/${name}`
}

async function load() {
  loading.value = true
  try {
    entries.value = (await api.get<FileEntry[]>(`${base}/list?path=${encodeURIComponent(path.value)}`)) ?? []
  } catch (e) {
    ElMessage.error((e as Error).message)
    entries.value = []
  } finally {
    loading.value = false
  }
}

function go(p: string) {
  path.value = p || '/'
  load()
}

async function open(f: FileEntry) {
  if (f.dir) {
    go(f.path)
    return
  }
  try {
    const r = await api.get<{ content: string; size: number; truncated: boolean }>(`${base}/read?path=${encodeURIComponent(f.path)}`)
    content.value = r.content
    truncated.value = r.truncated
    editPath.value = f.path
    editVisible.value = true
  } catch (e) {
    ElMessage.error(t('docker.container.files.binaryHint', { message: (e as Error).message }))
  }
}

async function save() {
  saving.value = true
  try {
    await api.post(`${base}/write`, { path: editPath.value, content: content.value })
    ElMessage.success(t('common.saved'))
    editVisible.value = false
    load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

function downloadFile(f: FileEntry) {
  download(`${base}/download?path=${encodeURIComponent(f.path)}`)
}

async function mkdir() {
  try {
    const { value } = await ElMessageBox.prompt(t('docker.container.files.folderName'), t('docker.container.files.newFolder'), { inputPattern: /^[^/]+$/, inputErrorMessage: t('docker.container.files.nameInvalid') })
    await api.post(`${base}/mkdir`, { path: join(path.value, value) })
    load()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error((e as Error).message)
    }
  }
}

async function rename(f: FileEntry) {
  try {
    const { value } = await ElMessageBox.prompt(t('docker.container.files.newName'), t('docker.container.files.rename'), { inputValue: f.name, inputPattern: /^[^/]+$/, inputErrorMessage: t('docker.container.files.nameInvalid') })
    await api.post(`${base}/rename`, { from: f.path, to: join(path.value, value) })
    load()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error((e as Error).message)
    }
  }
}

async function remove(f: FileEntry) {
  try {
    await ElMessageBox.confirm(t(f.dir ? 'docker.container.files.deleteConfirmDir' : 'docker.container.files.deleteConfirm', { path: f.path }), t('common.delete'), { type: 'warning' })
    await api.del(`${base}?path=${encodeURIComponent(f.path)}`)
    load()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error((e as Error).message)
    }
  }
}

function onCmd(cmd: string, f: FileEntry) {
  if (cmd === 'download') {
    downloadFile(f)
  } else if (cmd === 'rename') {
    rename(f)
  } else {
    remove(f)
  }
}

async function onPick(ev: Event) {
  const input = ev.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  input.value = ''
  if (!files.length) {
    return
  }
  const fd = new FormData()
  fd.append('path', path.value)
  for (const f of files) {
    fd.append('file', f)
  }
  uploading.value = true
  try {
    await api.post(`${base}/upload`, fd)
    ElMessage.success(t('docker.container.files.uploaded', { count: files.length }))
    load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    uploading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  margin-bottom: 10px;
}
.crumbs {
  flex: 1 1 240px;
  min-width: 0;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px;
  font-family: var(--g-mono);
  font-size: 13px;
}
.crumb {
  border: none;
  background: transparent;
  color: var(--el-color-primary);
  padding: 4px 4px;
  font: inherit;
  cursor: pointer;
  border-radius: 4px;
}
.crumb:hover {
  background: var(--el-fill-color-light);
}
.sep {
  color: var(--el-text-color-placeholder);
}
.ops {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.ops .el-button + .el-button {
  margin-left: 0;
}
.hidden {
  display: none;
}
.list {
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  min-height: 120px;
  max-height: 58vh;
  overflow-y: auto;
}
.item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 8px 8px 14px;
  border-bottom: 1px solid var(--g-border);
  cursor: pointer;
  min-height: 48px;
}
.item:last-child {
  border-bottom: none;
}
.item:hover {
  background: var(--el-fill-color-lighter);
}
.ico {
  flex-shrink: 0;
  color: var(--el-text-color-secondary);
}
.ico.dir {
  color: var(--el-color-warning);
}
.main {
  flex: 1;
  min-width: 0;
}
.name {
  font-size: 14px;
  word-break: break-all;
}
.link {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 1px;
}
.acts {
  flex-shrink: 0;
  display: flex;
}
.gap {
  margin-bottom: 8px;
}
.editor :deep(textarea) {
  font-family: var(--g-mono);
  font-size: 13px;
  line-height: 1.5;
  white-space: pre;
  overflow-x: auto;
}
@media (max-width: 767.98px) {
  .ops {
    width: 100%;
  }
  .ops .el-button {
    flex: 1 1 40%;
    min-height: 40px;
  }
  .list {
    max-height: calc(100dvh - 300px);
  }
  .item {
    min-height: 56px;
  }
  .touch {
    min-width: 40px;
    min-height: 40px;
  }
  .editor :deep(textarea) {
    font-size: 16px;
  }
}
</style>
