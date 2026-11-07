<template>
  <el-drawer
    :model-value="modelValue"
    direction="rtl"
    :size="isMobile ? '100%' : '680px'"
    :with-header="false"
    append-to-body
    class="sftp-drawer"
    destroy-on-close
    @update:model-value="(v: boolean) => $emit('update:modelValue', v)"
    @open="onOpen"
  >
    <div
      class="sftp"
      @dragenter.prevent="onDragEnter"
      @dragover.prevent
      @dragleave.prevent="onDragLeave"
      @drop.prevent="onDrop"
    >
      <!-- 顶部：标题与工具 -->
      <div class="s-head">
        <el-button class="icon" text :icon="isMobile ? ArrowLeft : X" :aria-label="t('common.close')" @click="$emit('update:modelValue', false)" />
        <div class="s-title"><FolderOpen :size="16" /><span class="ellipsis">{{ t('sftp.title', { name: conn?.name ?? '' }) }}</span></div>
        <div class="s-tools">
          <el-button class="icon" text :icon="RefreshCw" :loading="loading" :aria-label="t('common.refresh')" @click="load(path)" />
          <el-dropdown trigger="click" @command="onNew">
            <el-button class="icon" text :icon="FilePlus" :aria-label="t('sftp.newItem')" />
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="mkdir" :icon="FolderPlus">{{ t('sftp.newFolder') }}</el-dropdown-item>
                <el-dropdown-item command="touch" :icon="FilePlus">{{ t('sftp.newFile') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown trigger="click" @command="onUploadPick">
            <el-button class="icon" text :icon="Upload" :aria-label="t('common.upload')" />
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="files" :icon="Upload">{{ t('sftp.uploadFiles') }}</el-dropdown-item>
                <el-dropdown-item v-if="!isMobile" command="dir" :icon="FolderUp">{{ t('sftp.uploadFolder') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown trigger="click" @command="onQuick">
            <el-button class="icon" text :icon="Star" :aria-label="t('sftp.quickAccess')" />
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="d in conn?.quickAccessDirs ?? []" :key="d" :command="'go:' + d">
                  <span class="mono">{{ d }}</span>
                </el-dropdown-item>
                <el-dropdown-item v-if="!conn?.quickAccessDirs?.length" disabled>{{ t('sftp.noQuick') }}</el-dropdown-item>
                <el-dropdown-item command="add" divided :icon="Plus" :disabled="!path">{{ t('sftp.addCurrent') }}</el-dropdown-item>
                <el-dropdown-item command="manage" :icon="Settings2">{{ t('sftp.manageDirs') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown trigger="click" @command="onSort">
            <el-button class="icon" text :icon="ArrowUpDown" :aria-label="t('sftp.sort')" />
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="o in sortOptions" :key="o.key" :command="o.key" :class="{ 'is-current': sortKey === o.key }">
                  {{ t(`sftp.sortBy.${o.key}`) }}<span v-if="sortKey === o.key">　{{ sortAsc ? '↑' : '↓' }}</span>
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button v-if="isMobile" class="icon" text :type="selectMode ? 'primary' : 'default'" :icon="ListChecks" :aria-label="t('sftp.multiSelect')" @click="toggleSelectMode" />
        </div>
      </div>

      <!-- 路径栏 -->
      <div class="crumbs">
        <el-button class="icon" text :icon="CornerLeftUp" :disabled="!parent" :aria-label="t('sftp.parentDir')" @click="load(parent)" />
        <div ref="crumbBox" class="crumb-scroll">
          <template v-for="(c, i) in crumbs" :key="c.path">
            <ChevronRight v-if="i > 1" :size="12" class="crumb-sep" />
            <button class="crumb" :class="{ cur: i === crumbs.length - 1 }" @click="load(c.path)">{{ c.name }}</button>
          </template>
        </div>
      </div>

      <el-alert v-if="error" type="error" :title="error" :closable="false" show-icon class="s-err">
        <el-button size="small" @click="load(path)">{{ t('sftp.retry') }}</el-button>
      </el-alert>

      <!-- 桌面端表头 -->
      <div v-if="!isMobile && sorted.length" class="list-head">
        <el-checkbox :model-value="allSelected" :indeterminate="selected.size > 0 && !allSelected" @change="toggleAll" />
        <span class="c-name">{{ t('common.name') }}</span>
        <span class="c-size">{{ t('sftp.col.size') }}</span>
        <span class="c-time">{{ t('sftp.col.modTime') }}</span>
        <span class="c-mode">{{ t('sftp.col.mode') }}</span>
        <span class="c-op" />
      </div>

      <!-- 目录条目多时只渲染当前页 -->
      <div ref="listEl" class="list" v-loading="loading">
        <el-empty v-if="!loading && !sorted.length && !error" :description="t('sftp.emptyDir')" :image-size="50" />
        <div
          v-for="e in filePager.rows.value"
          :key="e.path"
          class="row"
          :class="{ sel: selected.has(e.path) }"
          @click="onRowClick(e)"
          @dblclick="onRowDblClick(e)"
          @contextmenu.prevent="isMobile ? openSheet(e) : undefined"
          @pointerdown="pressStart(e, $event)"
          @pointermove="pressMove($event)"
          @pointerup="pressEnd"
          @pointercancel="pressEnd"
          @pointerleave="pressEnd"
        >
          <el-checkbox v-if="!isMobile || selectMode" :model-value="selected.has(e.path)" class="chk" @click.stop @change="toggle(e)" />
          <div class="ico" :class="{ dir: e.isDir }">
            <Folder v-if="e.isDir" :size="18" />
            <Link2 v-else-if="e.isLink" :size="18" />
            <FileText v-else :size="18" />
          </div>
          <div class="r-main">
            <div class="r-name"><span class="ellipsis">{{ e.name }}</span><Link2 v-if="e.isLink && e.isDir" :size="12" class="lnk" /></div>
            <div v-if="isMobile" class="r-meta">
              <span v-if="!e.isDir">{{ formatBytes(e.size) }}</span><span>{{ fmtTime(e.modTime) }}</span><span class="mono">{{ modeText(e.mode) }}</span>
            </div>
          </div>
          <template v-if="!isMobile">
            <span class="c-size">{{ e.isDir ? '-' : formatBytes(e.size) }}</span>
            <span class="c-time">{{ fmtTime(e.modTime) }}</span>
            <span class="c-mode mono">{{ modeText(e.mode) }}</span>
          </template>
          <div class="c-op" @click.stop>
            <el-button v-if="isMobile" class="icon" text :icon="Ellipsis" :aria-label="t('sftp.actionsLabel')" @click="openSheet(e)" />
            <el-dropdown v-else trigger="click" @command="(c: string) => runAction(c, e)">
              <el-button text :icon="Ellipsis" size="small" :aria-label="t('sftp.actionsLabel')" />
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item v-for="a in actionsFor(e)" :key="a.cmd" :command="a.cmd" :icon="a.icon" :divided="a.danger">{{ a.label }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
      </div>

      <ListPager :pager="filePager" :anchor="listEl" compact class="s-pager" />

      <!-- 上传进度 -->
      <div v-if="uploads.length" class="uploads">
        <div class="up-head">
          <span>{{ t('sftp.uploadProgress', { done: uploads.filter((u) => u.status === 'done').length, total: uploads.length }) }}</span>
          <el-button v-if="!uploading" size="small" text @click="uploads = []">{{ t('common.clear') }}</el-button>
        </div>
        <div class="up-list">
          <div v-for="u in uploads" :key="u.id" class="up-item">
            <span class="up-name ellipsis">{{ u.name }}</span>
            <el-progress :percentage="u.percent" :status="u.status === 'error' ? 'exception' : u.status === 'done' ? 'success' : undefined" :stroke-width="6" class="up-bar" />
            <span v-if="u.msg" class="up-err">{{ u.msg }}</span>
          </div>
        </div>
      </div>

      <!-- 底部操作栏 -->
      <div v-if="selected.size || clipboard.length" class="bar">
        <span v-if="selected.size" class="bar-count">{{ t('sftp.selectedCount', { count: selected.size }) }}</span>
        <span v-else class="bar-count">{{ t('sftp.clipboardCount', { count: clipboard.length }) }}</span>
        <div class="bar-ops">
          <el-button v-if="selected.size" :icon="Copy" @click="copySelected">{{ t('common.copy') }}</el-button>
          <el-button v-if="clipboard.length" :icon="ClipboardPaste" type="primary" plain @click="paste">{{ t('sftp.paste') }}</el-button>
          <el-button v-if="selected.size === 1 && selectedEntries[0] && !selectedEntries[0].isDir" :icon="Download" @click="download(selectedEntries[0])">{{ t('common.download') }}</el-button>
          <el-button v-if="selected.size" :icon="Archive" @click="openCompress(selectedEntries)">{{ t('sftp.compress') }}</el-button>
          <el-button v-if="selected.size" :icon="Trash2" type="danger" plain @click="removeEntries(selectedEntries)">{{ t('common.delete') }}</el-button>
          <el-button v-if="selected.size" @click="clearSelection">{{ t('common.cancel') }}</el-button>
          <el-button v-else-if="clipboard.length" @click="clipboard = []">{{ t('common.clear') }}</el-button>
        </div>
      </div>

      <div v-if="dragging" class="drop-mask"><Upload :size="36" /><span>{{ t('sftp.dropHint', { path }) }}</span></div>
    </div>

    <input ref="fileInput" type="file" multiple hidden @change="onPicked" />
    <input ref="dirInput" type="file" multiple hidden webkitdirectory @change="onPicked" />

    <!-- 移动端操作面板 -->
    <el-drawer v-model="sheetVisible" direction="btt" size="auto" :with-header="false" append-to-body class="sheet">
      <div class="sheet-title ellipsis">{{ sheetEntry?.name }}</div>
      <button
        v-for="a in sheetEntry ? actionsFor(sheetEntry) : []"
        :key="a.cmd"
        class="sheet-item"
        :class="{ danger: a.danger }"
        @click="onSheet(a.cmd)"
      >
        <component :is="a.icon" :size="18" />{{ a.label }}
      </button>
    </el-drawer>

    <!-- 压缩 -->
    <el-dialog v-model="compressVisible" :title="t('sftp.compressDialog.title')" width="440px" :fullscreen="isMobile" class="term-dialog" append-to-body>
      <el-form label-width="80px" :label-position="isMobile ? 'top' : 'right'">
        <el-form-item :label="t('sftp.compressDialog.fileName')"><el-input v-model="compressName" /></el-form-item>
        <el-form-item :label="t('sftp.compressDialog.format')">
          <el-radio-group v-model="compressFormat" @change="onFormatChange">
            <el-radio-button value="zip">zip</el-radio-button>
            <el-radio-button value="tar.gz">tar.gz</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <div class="form-help">{{ t('sftp.compressDialog.hint', { count: compressTargets.length, path }) }}</div>
      </el-form>
      <template #footer>
        <el-button @click="compressVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="busy" @click="doCompress">{{ t('sftp.compress') }}</el-button>
      </template>
    </el-dialog>

    <!-- 权限 -->
    <el-dialog v-model="chmodVisible" :title="t('sftp.chmodDialog.title')" width="400px" :fullscreen="isMobile" class="term-dialog" append-to-body>
      <el-form label-width="80px" :label-position="isMobile ? 'top' : 'right'">
        <el-form-item :label="t('sftp.chmodDialog.target')"><span class="mono ellipsis">{{ chmodEntry?.path }}</span></el-form-item>
        <el-form-item :label="t('sftp.chmodDialog.mode')">
          <el-input v-model="chmodMode" maxlength="4" class="mono" :placeholder="t('sftp.chmodDialog.placeholder')" />
          <div class="perm-grid">
            <div v-for="(g, gi) in ['owner', 'group', 'other']" :key="g" class="perm-row">
              <span class="perm-name">{{ t(`sftp.chmodDialog.${g}`) }}</span>
              <el-checkbox v-for="(b, bi) in ['read', 'write', 'exec']" :key="b" :model-value="permBit(gi, bi)" @change="(v: unknown) => setPermBit(gi, bi, !!v)">{{ t(`sftp.chmodDialog.${b}`) }}</el-checkbox>
            </div>
          </div>
        </el-form-item>
        <el-form-item v-if="chmodEntry?.isDir" :label="t('sftp.chmodDialog.recursive')"><el-switch v-model="chmodRecursive" /><span class="help">{{ t('sftp.chmodDialog.recursiveHelp') }}</span></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="chmodVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="busy" @click="doChmod">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>

    <!-- 压缩包预览 -->
    <el-dialog v-model="previewVisible" :title="t('sftp.previewDialog.title', { name: previewName })" width="560px" :fullscreen="isMobile" class="term-dialog" append-to-body>
      <el-skeleton v-if="previewLoading" :rows="5" animated />
      <div v-else class="pv-list">
        <div v-for="it in previewItems" :key="it.name" class="pv-row">
          <component :is="it.isDir ? Folder : FileText" :size="14" />
          <span class="pv-name mono">{{ it.name }}</span>
          <span class="meta">{{ it.isDir ? '' : formatBytes(it.size) }}</span>
        </div>
        <el-empty v-if="!previewItems.length" :description="t('sftp.previewDialog.empty')" :image-size="50" />
      </div>
    </el-dialog>

    <SftpEditor v-model="editorVisible" :conn-key="conn?.key ?? ''" :path="editorPath" @saved="load(path)" />
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import type { Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Archive, ArrowLeft, ArrowUpDown, ChevronRight, ClipboardPaste, Copy, CornerLeftUp, Download, Ellipsis, FilePlus, FileText, Folder, FolderOpen, FolderPlus, FolderUp, KeyRound,
  Link2, ListChecks, Pencil, Plus, RefreshCw, Settings2, Star, Trash2, Upload, X, FileArchive, FileEdit,
} from 'lucide-vue-next'
import { api, formatBytes } from '../../api'
import { useIsMobile } from '../../composables/useIsMobile'
import { usePager } from '../../composables/usePager'
import ListPager from '../ListPager.vue'
import SftpEditor from './SftpEditor.vue'
import {
  baseName, breadcrumbs, dirName, downloadUrl, isArchive, joinPath, listDir, modeOctal, modeText, post, remove, sortEntries, uploadForm,
} from './sftpApi'
import { ensureAccess, isCancel } from './security'
import { fmtTime, type Conn, type SftpEntry } from './types'

interface Action {
  cmd: string
  label: string
  icon: Component
  danger?: boolean
}
interface UpItem {
  id: number
  name: string
  percent: number
  status: 'wait' | 'run' | 'done' | 'error'
  msg?: string
}

const props = defineProps<{ modelValue: boolean; conn: Conn | null }>()
const emit = defineEmits<{ 'update:modelValue': [v: boolean]; quick: []; 'quick-saved': [] }>()

const { t } = useI18n()
const isMobile = useIsMobile()
const path = ref('')
const parent = ref('')
const entries = ref<SftpEntry[]>([])
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const sortKey = ref<'name' | 'size' | 'modTime'>('name')
const sortAsc = ref(true)
const selected = ref<Set<string>>(new Set())
const selectMode = ref(false)
const clipboard = ref<string[]>([])
const crumbBox = ref<HTMLElement>()
const fileInput = ref<HTMLInputElement>()
const dirInput = ref<HTMLInputElement>()
const dragging = ref(false)
let dragDepth = 0

// 排序项只存键名，显示文案在模板中翻译
const sortOptions = [{ key: 'name' }, { key: 'size' }, { key: 'modTime' }] as const

const key = computed(() => props.conn?.key ?? '')
const crumbs = computed(() => breadcrumbs(path.value || '/'))
const sorted = computed(() => sortEntries(entries.value, sortKey.value, sortAsc.value))
const selectedEntries = computed(() => entries.value.filter((e) => selected.value.has(e.path)))
// 分页后只渲染当前页：进入其他目录或改变排序时回到第 1 页，操作后刷新保持当前页
const filePager = usePager(() => sorted.value, { storageKey: 'sftp.files', resetOn: [path, sortKey, sortAsc] })
const listEl = ref<HTMLElement | null>(null)
// 表头复选框只作用于当前页，避免选中看不见的条目后被一并删除；跨页已选的数量显示在底部操作栏
const allSelected = computed(() => filePager.rows.value.length > 0 && filePager.rows.value.every((e) => selected.value.has(e.path)))

// ---- 目录加载 ----
function lastPathKey(): string {
  return `gately-sftp-last-${key.value}`
}

async function load(p: string) {
  if (!key.value) {
    return
  }
  loading.value = true
  error.value = ''
  try {
    const r = await listDir(key.value, p)
    path.value = r.path
    parent.value = r.parent && r.parent !== r.path ? r.parent : ''
    entries.value = r.entries ?? []
    selected.value = new Set()
    sessionStorage.setItem(lastPathKey(), r.path)
    nextTick(() => {
      if (crumbBox.value) {
        crumbBox.value.scrollLeft = crumbBox.value.scrollWidth
      }
    })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

function onOpen() {
  selectMode.value = false
  uploads.value = []
  path.value = ''
  entries.value = []
  load(sessionStorage.getItem(lastPathKey()) ?? '')
}

// guard 统一处理操作的忙碌状态、错误提示与操作后刷新。
async function guard(fn: () => Promise<void>, okMsg = '', refresh = true): Promise<boolean> {
  busy.value = true
  try {
    await fn()
    if (okMsg) {
      ElMessage.success(okMsg)
    }
    if (refresh) {
      await load(path.value)
    }
    return true
  } catch (e) {
    if (e !== 'cancel' && e !== 'close' && !isCancel(e)) {
      ElMessage.error((e as Error).message)
    }
    return false
  } finally {
    busy.value = false
  }
}

// ---- 选择 ----
function toggle(e: SftpEntry) {
  const s = new Set(selected.value)
  if (s.has(e.path)) {
    s.delete(e.path)
  } else {
    s.add(e.path)
  }
  selected.value = s
  if (isMobile.value && s.size === 0) {
    selectMode.value = false
  }
}

function toggleAll(v: unknown) {
  const s = new Set(selected.value)
  for (const e of filePager.rows.value) {
    if (v) {
      s.add(e.path)
    } else {
      s.delete(e.path)
    }
  }
  selected.value = s
}

function toggleSelectMode() {
  selectMode.value = !selectMode.value
  if (!selectMode.value) {
    selected.value = new Set()
  }
}

function clearSelection() {
  selected.value = new Set()
  selectMode.value = false
}

// ---- 行点击与长按 ----
let pressTimer: ReturnType<typeof setTimeout> | undefined
let pressX = 0
let pressY = 0
let longFired = false

function pressStart(e: SftpEntry, ev: PointerEvent) {
  longFired = false
  if (!isMobile.value || ev.pointerType === 'mouse') {
    return
  }
  pressX = ev.clientX
  pressY = ev.clientY
  pressTimer = setTimeout(() => {
    longFired = true
    selectMode.value = true
    const s = new Set(selected.value)
    s.add(e.path)
    selected.value = s
    navigator.vibrate?.(15)
  }, 450)
}

function pressMove(ev: PointerEvent) {
  if (pressTimer && Math.hypot(ev.clientX - pressX, ev.clientY - pressY) > 8) {
    pressEnd()
  }
}

function pressEnd() {
  if (pressTimer) {
    clearTimeout(pressTimer)
    pressTimer = undefined
  }
}

function onRowClick(e: SftpEntry) {
  if (longFired) {
    longFired = false
    return
  }
  if (isMobile.value) {
    if (selectMode.value) {
      toggle(e)
    } else if (e.isDir) {
      load(e.path)
    } else {
      openSheet(e)
    }
    return
  }
  if (e.isDir) {
    load(e.path)
  }
}

function onRowDblClick(e: SftpEntry) {
  if (!isMobile.value && !e.isDir) {
    openEditor(e)
  }
}

// ---- 行操作 ----
function actionsFor(e: SftpEntry): Action[] {
  const a: Action[] = []
  if (e.isDir) {
    a.push({ cmd: 'open', label: t('sftp.action.open'), icon: FolderOpen })
  } else {
    a.push({ cmd: 'edit', label: t('common.edit'), icon: FileEdit }, { cmd: 'download', label: t('common.download'), icon: Download })
  }
  a.push(
    { cmd: 'rename', label: t('sftp.action.rename'), icon: Pencil },
    { cmd: 'copy', label: t('common.copy'), icon: Copy },
    { cmd: 'chmod', label: t('sftp.action.chmod'), icon: KeyRound },
    { cmd: 'compress', label: t('sftp.compress'), icon: Archive },
  )
  if (!e.isDir && isArchive(e.name)) {
    a.push({ cmd: 'decompress', label: t('sftp.action.decompress'), icon: FileArchive }, { cmd: 'preview', label: t('sftp.action.previewArchive'), icon: FileText })
  }
  a.push({ cmd: 'delete', label: t('common.delete'), icon: Trash2, danger: true })
  return a
}

const sheetVisible = ref(false)
const sheetEntry = ref<SftpEntry>()

function openSheet(e: SftpEntry) {
  sheetEntry.value = e
  sheetVisible.value = true
}

function onSheet(cmd: string) {
  sheetVisible.value = false
  if (sheetEntry.value) {
    runAction(cmd, sheetEntry.value)
  }
}

function runAction(cmd: string, e: SftpEntry) {
  switch (cmd) {
    case 'open':
      load(e.path)
      break
    case 'edit':
      openEditor(e)
      break
    case 'download':
      download(e)
      break
    case 'rename':
      rename(e)
      break
    case 'copy':
      clipboard.value = [e.path]
      ElMessage.success(t('sftp.msg.copied', { name: e.name }))
      break
    case 'chmod':
      openChmod(e)
      break
    case 'compress':
      openCompress([e])
      break
    case 'decompress':
      decompress(e)
      break
    case 'preview':
      previewArchive(e)
      break
    case 'delete':
      removeEntries([e])
      break
  }
}

// ---- 新建 / 重命名 / 删除 / 复制 ----
async function onNew(cmd: string) {
  const isDir = cmd === 'mkdir'
  try {
    const { value } = await ElMessageBox.prompt('', isDir ? t('sftp.newFolder') : t('sftp.newFile'), {
      inputPlaceholder: t('sftp.msg.namePlaceholder'),
      inputValidator: (v) => (!!v && !v.includes('/')) || t('sftp.msg.nameInvalid'),
    })
    await guard(() => post(key.value, cmd, { path: joinPath(path.value, value) }).then(() => undefined), t('sftp.msg.created'))
  } catch {
    // 取消输入
  }
}

async function rename(e: SftpEntry) {
  try {
    const { value } = await ElMessageBox.prompt('', t('sftp.action.rename'), {
      inputValue: e.name,
      inputValidator: (v) => (!!v && !v.includes('/')) || t('sftp.msg.nameInvalid'),
    })
    if (value === e.name) {
      return
    }
    await guard(() => post(key.value, 'rename', { from: e.path, to: joinPath(dirName(e.path), value) }).then(() => undefined), t('sftp.msg.renamed'))
  } catch {
    // 取消输入
  }
}

async function removeEntries(list: SftpEntry[]) {
  if (!list.length) {
    return
  }
  try {
    const names = list.slice(0, 5).map((e) => e.name).join(t('sftp.msg.nameSeparator'))
    const message = list.length > 5 ? t('sftp.msg.deleteConfirmMany', { names, count: list.length }) : t('sftp.msg.deleteConfirmFew', { names })
    await ElMessageBox.confirm(message, t('sftp.msg.deleteTitle'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
    })
  } catch {
    return
  }
  await guard(() => remove(key.value, list.map((e) => e.path)).then(() => undefined), t('common.deleted'))
}

function copySelected() {
  clipboard.value = selectedEntries.value.map((e) => e.path)
  ElMessage.success(t('sftp.msg.copiedCount', { count: clipboard.value.length }))
  clearSelection()
}

async function paste() {
  if (!clipboard.value.length) {
    return
  }
  const ok = await guard(() => post(key.value, 'copy', { from: clipboard.value, to: path.value }).then(() => undefined), t('sftp.msg.pasted'))
  if (ok) {
    clipboard.value = []
  }
}

// ---- 下载与编辑 ----
async function download(e: SftpEntry) {
  if (e.isDir) {
    ElMessage.info(t('sftp.msg.dirNeedCompress'))
    return
  }
  try {
    // 下载链接走普通 GET，先确认协议与二次验证已通过
    await ensureAccess()
  } catch {
    return
  }
  const a = document.createElement('a')
  a.href = downloadUrl(key.value, e.path)
  a.download = e.name
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

const editorVisible = ref(false)
const editorPath = ref('')

function openEditor(e: SftpEntry) {
  editorPath.value = e.path
  editorVisible.value = true
}

// ---- 压缩 / 解压 / 预览 ----
const compressVisible = ref(false)
const compressTargets = ref<SftpEntry[]>([])
const compressName = ref('')
const compressFormat = ref<'zip' | 'tar.gz'>('zip')

function openCompress(list: SftpEntry[]) {
  compressTargets.value = list
  compressFormat.value = 'zip'
  const base = list.length === 1 ? list[0].name : baseName(path.value) || 'archive'
  compressName.value = `${base}.zip`
  compressVisible.value = true
}

function onFormatChange() {
  compressName.value = compressName.value.replace(/\.(zip|tar\.gz)$/i, '') + '.' + compressFormat.value
}

async function doCompress() {
  if (!compressName.value.trim() || compressName.value.includes('/')) {
    ElMessage.warning(t('sftp.msg.invalidFileName'))
    return
  }
  const ok = await guard(
    () =>
      post(key.value, 'compress', {
        paths: compressTargets.value.map((e) => e.path),
        dest: joinPath(path.value, compressName.value.trim()),
        format: compressFormat.value,
      }).then(() => undefined),
    t('sftp.msg.compressed'),
  )
  if (ok) {
    compressVisible.value = false
    clearSelection()
  }
}

async function decompress(e: SftpEntry) {
  try {
    const { value } = await ElMessageBox.prompt(t('sftp.msg.decompressTo'), t('sftp.msg.decompressTitle', { name: e.name }), { inputValue: path.value })
    await guard(() => post(key.value, 'decompress', { path: e.path, dest: value }).then(() => undefined), t('sftp.msg.decompressed'))
  } catch {
    // 取消输入
  }
}

const previewVisible = ref(false)
const previewLoading = ref(false)
const previewName = ref('')
const previewItems = ref<{ name: string; size: number; isDir: boolean }[]>([])

async function previewArchive(e: SftpEntry) {
  previewName.value = e.name
  previewItems.value = []
  previewVisible.value = true
  previewLoading.value = true
  try {
    previewItems.value = (await api.get<{ name: string; size: number; isDir: boolean }[] | null>(`/api/webterminal/sftp/${encodeURIComponent(key.value)}/preview-archive?path=${encodeURIComponent(e.path)}`)) ?? []
  } catch (err) {
    ElMessage.error((err as Error).message)
    previewVisible.value = false
  } finally {
    previewLoading.value = false
  }
}

// ---- 权限 ----
const chmodVisible = ref(false)
const chmodEntry = ref<SftpEntry>()
const chmodMode = ref('755')
const chmodRecursive = ref(false)

function openChmod(e: SftpEntry) {
  chmodEntry.value = e
  chmodMode.value = modeOctal(e.mode) || (e.isDir ? '755' : '644')
  chmodRecursive.value = false
  chmodVisible.value = true
}

// permBit 读取权限输入框中第 g 组（所有者/用户组/其他）第 b 位（读/写/执行）是否置位。
function permBit(g: number, b: number): boolean {
  const bits = /^[0-7]{3,4}$/.test(chmodMode.value) ? parseInt(chmodMode.value, 8) : 0
  return (bits & (1 << (8 - (g * 3 + b)))) !== 0
}

function setPermBit(g: number, b: number, v: boolean) {
  let bits = /^[0-7]{3,4}$/.test(chmodMode.value) ? parseInt(chmodMode.value, 8) & 0o777 : 0
  const mask = 1 << (8 - (g * 3 + b))
  bits = v ? bits | mask : bits & ~mask
  chmodMode.value = bits.toString(8).padStart(3, '0')
}

async function doChmod() {
  if (!/^[0-7]{3,4}$/.test(chmodMode.value) || !chmodEntry.value) {
    ElMessage.warning(t('sftp.msg.invalidMode'))
    return
  }
  const e = chmodEntry.value
  const ok = await guard(
    () => post(key.value, 'chmod', { path: e.path, mode: chmodMode.value, recursive: chmodRecursive.value }).then(() => undefined),
    t('sftp.msg.modeChanged'),
  )
  if (ok) {
    chmodVisible.value = false
  }
}

// ---- 快速访问 ----
async function onQuick(cmd: string) {
  if (cmd.startsWith('go:')) {
    load(cmd.slice(3))
  } else if (cmd === 'manage') {
    emit('quick')
  } else if (cmd === 'add' && props.conn && path.value) {
    const dirs = [...(props.conn.quickAccessDirs ?? [])]
    if (dirs.includes(path.value)) {
      ElMessage.info(t('sftp.msg.quickExists'))
      return
    }
    dirs.push(path.value)
    await guard(() => api.put(`/api/webterminal/connections/${encodeURIComponent(key.value)}/quickaccess`, { dirs }).then(() => undefined), t('sftp.msg.added'), false)
    emit('quick-saved')
  }
}

function onSort(k: string) {
  if (sortKey.value === k) {
    sortAsc.value = !sortAsc.value
  } else {
    sortKey.value = k as typeof sortKey.value
    sortAsc.value = true
  }
}

// ---- 上传 ----
interface UpFile {
  file: File
  rel: string
}
const uploads = ref<UpItem[]>([])
const uploading = computed(() => uploads.value.some((u) => u.status === 'wait' || u.status === 'run'))
let upSeq = 0

function onUploadPick(cmd: string) {
  if (cmd === 'dir') {
    dirInput.value?.click()
  } else {
    fileInput.value?.click()
  }
}

function onPicked(ev: Event) {
  const input = ev.target as HTMLInputElement
  const list: UpFile[] = Array.from(input.files ?? []).map((f) => ({ file: f, rel: f.webkitRelativePath || f.name }))
  input.value = ''
  uploadFiles(list)
}

async function uploadFiles(list: UpFile[]) {
  if (!list.length) {
    return
  }
  const target = path.value
  const items = list.map((f) => {
    const it: UpItem = { id: ++upSeq, name: f.rel, percent: 0, status: 'wait' }
    uploads.value.push(it)
    return it
  })
  // 顺序上传，每个文件独立显示进度
  for (let i = 0; i < list.length; i++) {
    const it = uploads.value.find((u) => u.id === items[i].id)
    if (!it) {
      continue
    }
    it.status = 'run'
    try {
      const fd = new FormData()
      fd.append('path', target)
      fd.append('files', list[i].file, list[i].file.name)
      if (list[i].rel !== list[i].file.name) {
        fd.append('relativePaths', list[i].rel)
      }
      await uploadForm(key.value, fd, (p) => {
        it.percent = p
      })
      it.percent = 100
      it.status = 'done'
    } catch (e) {
      it.status = 'error'
      it.msg = (e as Error).message
    }
  }
  if (path.value === target) {
    await load(target)
  }
}

function onDragEnter(e: DragEvent) {
  if (e.dataTransfer?.types.includes('Files')) {
    dragDepth++
    dragging.value = true
  }
}

function onDragLeave() {
  dragDepth = Math.max(0, dragDepth - 1)
  if (dragDepth === 0) {
    dragging.value = false
  }
}

async function onDrop(e: DragEvent) {
  dragDepth = 0
  dragging.value = false
  const dt = e.dataTransfer
  if (!dt) {
    return
  }
  // 拖放的数据在异步等待后会失效，需要同步取出目录项
  const entriesList = Array.from(dt.items ?? [])
    .map((i) => (i.webkitGetAsEntry ? i.webkitGetAsEntry() : null))
    .filter((x): x is FileSystemEntry => !!x)
  const plain = Array.from(dt.files ?? [])
  const out: UpFile[] = []
  const walk = async (entry: FileSystemEntry, prefix: string) => {
    if (entry.isFile) {
      const f = await new Promise<File>((res, rej) => (entry as FileSystemFileEntry).file(res, rej))
      out.push({ file: f, rel: prefix + f.name })
    } else if (entry.isDirectory) {
      const reader = (entry as FileSystemDirectoryEntry).createReader()
      const all: FileSystemEntry[] = []
      for (;;) {
        const batch = await new Promise<FileSystemEntry[]>((res, rej) => reader.readEntries(res, rej))
        if (!batch.length) {
          break
        }
        all.push(...batch)
      }
      for (const c of all) {
        await walk(c, prefix + entry.name + '/')
      }
    }
  }
  try {
    if (entriesList.length) {
      for (const en of entriesList) {
        await walk(en, '')
      }
    } else {
      for (const f of plain) {
        out.push({ file: f, rel: f.name })
      }
    }
  } catch (err) {
    ElMessage.error((err as Error).message)
    return
  }
  uploadFiles(out)
}
</script>

<style scoped>
.sftp {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.s-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding-bottom: 8px;
}
.s-title {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
}
.s-tools {
  display: flex;
  align-items: center;
}
.s-tools :deep(.el-button + .el-button),
.s-tools :deep(.el-dropdown + .el-dropdown),
.s-tools :deep(.el-dropdown + .el-button) {
  margin-left: 0;
}
.icon {
  padding: 8px;
  min-width: 32px;
}
.crumbs {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 0 8px;
  border-bottom: 1px solid var(--g-border);
}
.crumb-scroll {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  overflow-x: auto;
  white-space: nowrap;
  scrollbar-width: none;
}
.crumb-scroll::-webkit-scrollbar {
  display: none;
}
.crumb {
  flex: 0 0 auto;
  padding: 4px 6px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--el-text-color-regular);
  font-size: 13px;
  cursor: pointer;
}
.crumb:hover {
  background: var(--el-fill-color);
}
.crumb.cur {
  color: var(--el-color-primary);
  font-weight: 600;
}
.crumb-sep {
  flex: 0 0 auto;
  color: var(--el-text-color-placeholder);
}
.s-pager {
  flex-shrink: 0;
  margin-top: 8px;
}
.s-err {
  margin: 8px 0;
}
.list-head,
.row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.list-head {
  padding: 6px 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  border-bottom: 1px solid var(--g-border);
}
.list-head .c-name {
  flex: 1;
  padding-left: 30px;
}
.c-size {
  width: 80px;
  text-align: right;
  flex-shrink: 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.c-time {
  width: 140px;
  flex-shrink: 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.c-mode {
  width: 76px;
  flex-shrink: 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.c-op {
  width: 36px;
  flex-shrink: 0;
  display: flex;
  justify-content: center;
}
.list {
  flex: 1;
  min-height: 80px;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}
.row {
  min-height: 40px;
  padding: 4px 8px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  user-select: none;
  -webkit-user-select: none;
  -webkit-touch-callout: none;
}
.row:hover {
  background: var(--el-fill-color-lighter);
}
.row.sel {
  background: var(--el-color-primary-light-9);
}
.chk {
  flex-shrink: 0;
}
.ico {
  width: 28px;
  height: 28px;
  border-radius: 7px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
}
.ico.dir {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.r-main {
  flex: 1;
  min-width: 0;
}
.r-name {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 14px;
  cursor: default;
}
.lnk {
  color: var(--el-text-color-placeholder);
}
.r-meta {
  display: flex;
  gap: 10px;
  margin-top: 1px;
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
.uploads {
  margin-top: 8px;
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius-sm);
  padding: 6px 10px;
  max-height: 150px;
  display: flex;
  flex-direction: column;
}
.up-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.up-list {
  overflow-y: auto;
}
.up-item {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 3px 0;
  font-size: 12px;
}
.up-name {
  width: 36%;
}
.up-bar {
  flex: 1;
  min-width: 100px;
}
.up-err {
  width: 100%;
  color: var(--el-color-danger);
}
.bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
  padding: 8px 4px calc(4px + env(safe-area-inset-bottom));
  border-top: 1px solid var(--g-border);
}
.bar-count {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.bar-ops {
  flex: 1;
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
}
.bar-ops .el-button + .el-button {
  margin-left: 0;
}
.drop-mask {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  border: 2px dashed var(--el-color-primary);
  border-radius: var(--g-radius);
  background: color-mix(in srgb, var(--el-bg-color) 88%, var(--el-color-primary));
  color: var(--el-color-primary);
  pointer-events: none;
}
.sheet-title {
  padding: 4px 4px 10px;
  font-weight: 600;
  border-bottom: 1px solid var(--g-border);
}
.sheet-item {
  width: 100%;
  min-height: 48px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 6px;
  border: none;
  border-bottom: 1px solid var(--el-border-color-lighter);
  background: transparent;
  color: var(--el-text-color-primary);
  font-size: 15px;
  text-align: left;
  cursor: pointer;
}
.sheet-item:active {
  background: var(--el-fill-color-light);
}
.sheet-item.danger {
  color: var(--el-color-danger);
}
.perm-grid {
  width: 100%;
  margin-top: 8px;
}
.perm-row {
  display: flex;
  align-items: center;
  gap: 4px;
}
.perm-name {
  width: 56px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.pv-list {
  max-height: 60vh;
  overflow: auto;
}
.pv-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.pv-name {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  word-break: break-all;
}
.is-current {
  color: var(--el-color-primary);
}
@media (max-width: 767.98px) {
  .row {
    min-height: 56px;
    padding: 6px 4px;
  }
  .icon {
    min-width: 40px;
    height: 40px;
    padding: 0;
  }
  .r-name {
    font-size: 15px;
  }
  .bar-ops :deep(.el-button) {
    min-height: 40px;
  }
}
</style>

<style>
.sftp-drawer .el-drawer__body {
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.sftp-drawer .el-drawer__body > .sftp {
  flex: 1;
}
@media (max-width: 767.98px) {
  .sftp-drawer .el-drawer__body {
    padding: calc(8px + env(safe-area-inset-top)) 8px 8px;
  }
}
</style>
