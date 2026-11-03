<template>
  <el-dialog :model-value="modelValue" :title="t('errorPages.managerTitle')" width="720px" append-to-body :fullscreen="isMobile" @update:model-value="emit('update:modelValue', $event)">
    <div class="form-help top-help">{{ t('errorPages.managerHelp') }}</div>
    <div v-loading="loading" class="rows" data-test="error-pages">
      <el-empty v-if="!pages.length && !loading" :image-size="48" :description="t('errorPages.empty')" />
      <div v-for="p in pages" :key="p.key" class="item">
        <div class="item-head">
          <span class="name">{{ p.name }}</span>
          <el-tag size="small" :type="p.source === 'file' ? 'warning' : 'info'">{{ p.source === 'file' ? t('errorPages.sourceFile') : t('errorPages.sourceInline') }}</el-tag>
          <el-tag v-if="p.error" size="small" type="danger">{{ t('errorPages.unavailable') }}</el-tag>
          <span class="actions">
            <el-button text size="small" :icon="Eye" @click="previewPage(p)">{{ t('errorPages.preview') }}</el-button>
            <el-button text size="small" :icon="Pencil" @click="openEditor(p)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('errorPages.deleteConfirm')" width="240" @confirm="remove(p)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2" :disabled="!!p.usedBy?.length">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </span>
        </div>
        <div v-if="p.file" class="meta mono path">{{ p.file }}</div>
        <div class="meta">
          {{ formatSize(p.size) }}<span v-if="p.updatedAt"> · {{ t('errorPages.updated', { time: new Date(p.updatedAt).toLocaleString() }) }}</span>
          <span v-if="p.usedBy?.length"> · {{ t('errorPages.usedBy', { sites: usedByText(p.usedBy) }) }}</span>
          <span v-if="p.remark"> · {{ p.remark }}</span>
        </div>
        <div v-if="p.error" class="err">{{ p.error }}</div>
      </div>
      <div>
        <el-button size="small" :icon="Plus" :disabled="pages.length >= maxPages" @click="openEditor()">{{ t('errorPages.add') }}</el-button>
      </div>
    </div>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">{{ t('common.close') }}</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="editorVisible" :title="editing.key ? t('errorPages.edit') : t('errorPages.add')" width="760px" top="5vh" append-to-body :fullscreen="isMobile" :close-on-click-modal="false">
    <el-form :model="editing" label-width="110px" :label-position="labelPosition" @submit.prevent>
      <el-form-item :label="t('common.name')" required>
        <el-input v-model="editing.name" maxlength="64" :placeholder="t('errorPages.namePlaceholder')" data-test="ep-name" />
      </el-form-item>
      <el-form-item :label="t('errorPages.source')">
        <el-radio-group v-model="editing.source">
          <el-radio-button value="inline">{{ t('errorPages.sourceInline') }}</el-radio-button>
          <el-radio-button value="file">{{ t('errorPages.sourceFile') }}</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <template v-if="editing.source === 'inline'">
        <el-form-item :label="t('errorPages.upload')">
          <div class="upload-row">
            <!-- 只在浏览器中读取文件内容，填入下方编辑框，保存时连同其他字段一起提交并在服务端检查 -->
            <input ref="fileInput" type="file" accept=".html,.htm,text/html" class="hidden-input" data-test="ep-file" @change="onFile" />
            <el-button :icon="Upload" @click="fileInput?.click()">{{ t('errorPages.chooseFile') }}</el-button>
            <span class="form-help inline">{{ t('errorPages.uploadHelp', { size: formatSize(maxCustomPageBytes) }) }}</span>
          </div>
        </el-form-item>
        <el-form-item :label="t('errorPages.html')" required>
          <el-input v-model="editing.html" type="textarea" :rows="isMobile ? 8 : 10" class="mono code" spellcheck="false" :placeholder="t('errorPages.htmlPlaceholder')" data-test="ep-html" />
          <div class="form-help size-line" :class="{ over: htmlBytes > maxCustomPageBytes }">{{ formatSize(htmlBytes) }} / {{ formatSize(maxCustomPageBytes) }}</div>
        </el-form-item>
      </template>
      <el-form-item v-else :label="t('errorPages.filePath')" required>
        <el-input v-model="editing.file" class="mono" placeholder="/data/www/errors/50x.html" data-test="ep-path" />
        <div class="form-help">{{ t('errorPages.filePathHelp') }}</div>
      </el-form-item>
      <el-form-item :label="t('errorPages.check')">
        <div class="check" data-test="ep-check">
          <div v-if="checking" class="state">{{ t('errorPages.checking') }}</div>
          <el-alert v-else-if="checkError" :title="checkError" type="error" :closable="false" show-icon />
          <template v-else-if="report">
            <el-alert :title="report.removed?.length || report.warnings?.length ? t('errorPages.checkWarn') : t('errorPages.checkOk')" :type="report.removed?.length || report.warnings?.length ? 'warning' : 'success'" :closable="false" show-icon />
            <div v-if="report.removed?.length" class="list">
              <div class="list-title">{{ t('errorPages.removed') }}</div>
              <ul>
                <li v-for="r in report.removed" :key="r">{{ r }}</li>
              </ul>
            </div>
            <div v-if="report.warnings?.length" class="list">
              <div class="list-title">{{ t('errorPages.warnings') }}</div>
              <ul>
                <li v-for="w in report.warnings" :key="w">{{ w }}</li>
              </ul>
            </div>
            <div class="form-help">{{ t('errorPages.checkSummary', { size: formatSize(report.size), title: report.pageTitle || '-', placeholders: report.placeholders?.join(' ') || '-' }) }}</div>
          </template>
          <div v-else class="state">{{ t('errorPages.checkIdle') }}</div>
          <div v-if="previewHtml" class="mini">
            <ErrorPageThumb :html="previewHtml" :title="t('errorPages.preview')" />
            <el-button size="small" :icon="Maximize2" class="mini-btn" @click="draftPreview = true">{{ t('errorPages.previewFull') }}</el-button>
          </div>
        </div>
      </el-form-item>
      <el-form-item :label="t('errorPages.placeholders')">
        <div class="chips">
          <el-tooltip v-for="p in placeholders" :key="p" :content="t(`errorPages.ph.${p.slice(1, -1)}`)" placement="top">
            <code class="chip">{{ p }}</code>
          </el-tooltip>
        </div>
        <div class="form-help">{{ t('errorPages.placeholdersHelp') }}</div>
      </el-form-item>
      <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" maxlength="200" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="saving" :disabled="checking || !!checkError || !canCheck" data-test="ep-save" @click="save">{{ t('common.save') }}</el-button>
    </template>
  </el-dialog>

  <ErrorPagePreview v-model="draftPreview" :title="editing.name || t('errorPages.preview')" :load="draftLoader" />
  <ErrorPagePreview v-model="pagePreview" :title="pagePreviewTitle" :load="pageLoader" />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Eye, Maximize2, Pencil, Plus, Trash2, Upload } from 'lucide-vue-next'
import ErrorPageThumb from './ErrorPageThumb.vue'
import ErrorPagePreview from './ErrorPagePreview.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { decodeUTF8, formatSize } from '../errorPageUtil'
import { errorPagesApi, maxCustomPageBytes, placeholders, type CheckReport, type CustomErrorPage, type ErrorPageView } from '../errorPages'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [boolean]; changed: [] }>()
const { t, locale } = useI18n()
const { isMobile, labelPosition } = useFormLayout()

// 与后端 maxErrorPages 一致
const maxPages = 32

const pages = ref<ErrorPageView[]>([])
const loading = ref(false)
const editorVisible = ref(false)
const saving = ref(false)
const editing = ref<CustomErrorPage>(empty())
const fileInput = ref<HTMLInputElement>()

const checking = ref(false)
const checkError = ref('')
const report = ref<CheckReport | null>(null)
const previewHtml = ref('')
const draftPreview = ref(false)
const pagePreview = ref(false)
const pagePreviewTitle = ref('')
const pageLoader = ref<((code: number) => Promise<string>) | null>(null)

// usedByText 把引用方列表按界面语言连接；默认站点由后端以 @default 表示
function usedByText(list: string[]) {
  const names = list.map((s) => s.replace(/@default$/, t('web.defaultSite')))
  return new Intl.ListFormat(locale.value, { style: 'narrow', type: 'unit' }).format(names)
}

function empty(): CustomErrorPage {
  return { name: '', source: 'inline', html: '', file: '', remark: '' }
}

const htmlBytes = computed(() => new TextEncoder().encode(editing.value.html ?? '').length)

// canCheck 为内容或路径已填写，可以检查与保存
const canCheck = computed(() => (editing.value.source === 'inline' ? !!editing.value.html?.trim() : !!editing.value.file?.trim()))

async function load() {
  loading.value = true
  try {
    pages.value = await errorPagesApi.list()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (v) => {
    if (v) {
      load()
    }
  },
  { immediate: true },
)

async function openEditor(p?: ErrorPageView) {
  report.value = null
  checkError.value = ''
  previewHtml.value = ''
  if (p) {
    try {
      const full = await errorPagesApi.get(p.key)
      editing.value = { key: full.key, name: full.name, source: full.source, html: full.html ?? '', file: full.file ?? '', remark: full.remark ?? '' }
    } catch (e) {
      ElMessage.error((e as Error).message)
      return
    }
  } else {
    editing.value = empty()
  }
  editorVisible.value = true
}

// onFile 在浏览器中读取选中的文件：检查扩展名、大小与 UTF-8 编码后填入编辑框
async function onFile(e: Event) {
  const input = e.target as HTMLInputElement
  const f = input.files?.[0]
  input.value = ''
  if (!f) {
    return
  }
  if (!/\.html?$/i.test(f.name)) {
    ElMessage.error(t('errorPages.badExt'))
    return
  }
  if (f.size > maxCustomPageBytes) {
    ElMessage.error(t('errorPages.tooLarge', { size: formatSize(maxCustomPageBytes) }))
    return
  }
  const text = decodeUTF8(await f.arrayBuffer())
  if (text === null) {
    ElMessage.error(t('errorPages.notUtf8'))
    return
  }
  editing.value.html = text
  if (!editing.value.name.trim()) {
    editing.value.name = f.name.replace(/\.html?$/i, '').slice(0, 64)
  }
}

// 内容或路径变化后稍等再检查，连续输入时只检查最后一次
let timer: ReturnType<typeof setTimeout> | undefined
let checkSeq = 0
watch(
  () => [editorVisible.value, editing.value.source, editing.value.html, editing.value.file] as const,
  ([visible]) => {
    clearTimeout(timer)
    // 任何变化都让进行中的检查作废，旧请求的结果不会写到新内容上，也不会提前放开保存按钮
    checkSeq++
    if (!visible) {
      checking.value = false
      return
    }
    if (!canCheck.value) {
      checking.value = false
      report.value = null
      checkError.value = ''
      previewHtml.value = ''
      return
    }
    checking.value = true
    timer = setTimeout(runCheck, 500)
  },
)

async function runCheck() {
  const my = ++checkSeq
  const e = editing.value
  try {
    const r = await errorPagesApi.check({ source: e.source, html: e.source === 'inline' ? e.html : undefined, file: e.source === 'file' ? e.file : undefined, code: 502 })
    if (my === checkSeq) {
      report.value = r.report
      previewHtml.value = r.preview
      checkError.value = ''
    }
  } catch (err) {
    if (my === checkSeq) {
      report.value = null
      previewHtml.value = ''
      checkError.value = (err as Error).message
    }
  } finally {
    if (my === checkSeq) {
      checking.value = false
    }
  }
}

// draftLoader 按状态码预览编辑中的页面（未保存）
const draftLoader = (code: number) => {
  const e = editing.value
  return errorPagesApi.check({ source: e.source, html: e.source === 'inline' ? e.html : undefined, file: e.source === 'file' ? e.file : undefined, code }).then((r) => r.preview)
}

function previewPage(p: ErrorPageView) {
  pagePreviewTitle.value = p.name
  pageLoader.value = (code: number) => errorPagesApi.preview(p.key, code).then((r) => r.html)
  pagePreview.value = true
}

async function save() {
  const e = editing.value
  if (!e.name.trim()) {
    ElMessage.error(t('errorPages.nameRequired'))
    return
  }
  saving.value = true
  try {
    const body: CustomErrorPage = { name: e.name, source: e.source, remark: e.remark, html: e.source === 'inline' ? e.html : '', file: e.source === 'file' ? e.file : '' }
    const res = e.key ? await errorPagesApi.update(e.key, body) : await errorPagesApi.create(body)
    editorVisible.value = false
    const removed = res.report.removed?.length ?? 0
    ElMessage.success(removed ? t('errorPages.savedCleaned', { n: removed }) : t('common.saved'))
    await load()
    emit('changed')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    saving.value = false
  }
}

async function remove(p: ErrorPageView) {
  try {
    await errorPagesApi.remove(p.key)
    await load()
    emit('changed')
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
</script>

<style scoped>
.top-help {
  margin: 0 0 10px;
}
.rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 60px;
}
.item {
  border: 1px solid var(--g-border);
  border-radius: 8px;
  padding: 8px 10px;
}
.item-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.name {
  font-weight: 600;
  word-break: break-all;
}
.actions {
  margin-left: auto;
}
.meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
.path {
  word-break: break-all;
}
.err {
  font-size: 12px;
  color: var(--el-color-danger);
  word-break: break-all;
}
.upload-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 12px;
}
.hidden-input {
  display: none;
}
.form-help.inline {
  margin: 0;
}
.code :deep(textarea) {
  font-size: 12px;
  line-height: 1.5;
}
.size-line.over {
  color: var(--el-color-danger);
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.chip {
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--el-fill-color);
  font-size: 12px;
  cursor: default;
}
.check {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.state {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.list {
  font-size: 13px;
}
.list-title {
  font-weight: 600;
}
.list ul {
  margin: 2px 0 0;
  padding-left: 20px;
  word-break: break-all;
}
.mini {
  position: relative;
  max-width: 360px;
  border: 1px solid var(--el-border-color);
  border-radius: 6px;
  overflow: hidden;
}
.mini-btn {
  position: absolute;
  right: 6px;
  bottom: 6px;
}
</style>
