<template>
  <!-- 从 Sun-Panel 导入：上传 1.x 的 .sun-panel.json 或 2.x 的 .zip，后端转换为链接（不保存），确认后交给面板编辑框 -->
  <el-dialog
    v-model="visible"
    :title="t('sunpanelImport.title')"
    width="720px"
    top="6vh"
    :fullscreen="isMobile"
    :close-on-click-modal="false"
    append-to-body
    class="sp-dialog"
    data-test="sp-dialog"
    @closed="reset"
  >
    <template v-if="!result">
      <div class="form-help sp-intro">{{ t('sunpanelImport.intro') }}</div>
      <ul class="sp-steps">
        <li><b>1.x</b><span>{{ t('sunpanelImport.stepV1') }}</span></li>
        <li><b>2.x</b><span>{{ t('sunpanelImport.stepV2') }}</span></li>
      </ul>
      <label
        class="sp-drop"
        :class="{ over: dragOver, picked: !!file }"
        data-test="sp-drop"
        @dragover.prevent="dragOver = true"
        @dragleave.prevent="dragOver = false"
        @drop.prevent="onDrop"
      >
        <input ref="input" type="file" accept=".json,.zip,application/json,application/zip" hidden data-test="sp-file" @change="onPick" />
        <FileArchive v-if="file" :size="28" class="sp-drop-icon" />
        <Upload v-else :size="28" class="sp-drop-icon" />
        <span v-if="file" class="sp-file">
          <span class="ellipsis">{{ file.name }}</span>
          <span class="muted">{{ formatSize(file.size) }}</span>
        </span>
        <span v-else class="sp-drop-text">{{ t('sunpanelImport.drop') }}</span>
        <span class="muted sp-drop-hint">{{ file ? t('sunpanelImport.change') : t('sunpanelImport.dropHint') }}</span>
      </label>
      <div class="sp-base">
        <div class="sp-label">{{ t('sunpanelImport.baseUrl') }}</div>
        <el-input v-model="baseUrl" clearable placeholder="http://192.168.1.10:3002" data-test="sp-base" @keyup.enter="parse" />
        <div class="form-help">{{ t('sunpanelImport.baseUrlHelp') }}</div>
      </div>
    </template>

    <template v-else>
      <div class="sp-summary" data-test="sp-summary">
        <div class="sp-stat">
          <span class="sp-num">{{ links.length }}</span>
          <span class="muted">{{ t('sunpanelImport.statLinks') }}</span>
        </div>
        <div class="sp-stat">
          <span class="sp-num">{{ result?.groups }}</span>
          <span class="muted">{{ t('sunpanelImport.statGroups') }}</span>
        </div>
        <div class="sp-stat" :class="{ warn: skipped > 0 }">
          <span class="sp-num">{{ skipped }}</span>
          <span class="muted">{{ t('sunpanelImport.statSkipped') }}</span>
        </div>
        <div class="sp-source muted">
          <el-tag size="small" effect="plain">Sun-Panel {{ result?.format === 'v1' ? '1.x' : '2.x' }}</el-tag>
          <span v-if="result?.appVersion" class="mono">{{ result?.appVersion }}</span>
        </div>
      </div>

      <div class="sp-icons muted" data-test="sp-icons">
        <span class="sp-icons-label">{{ t('sunpanelImport.icons') }}</span>
        <span v-for="k in iconKinds" :key="k" class="sp-icon-stat">{{ t(`sunpanelImport.icon_${k}`, { n: icons[k] }) }}</span>
      </div>
      <el-alert v-if="icons.iconify" type="info" :closable="false" show-icon class="sp-alert" :title="t('sunpanelImport.iconifyNote', { n: icons.iconify })" />
      <el-alert v-if="missingIcons && !baseUrl.trim()" type="warning" :closable="false" show-icon class="sp-alert" data-test="sp-missing" :title="t('sunpanelImport.missingNote', { n: missingIcons })" />

      <el-collapse v-if="notes.length" v-model="notesOpen" class="sp-notes" data-test="sp-notes">
        <el-collapse-item name="notes">
          <template #title>
            <span class="sp-notes-title"><Info :size="14" />{{ t('sunpanelImport.notes', { n: notes.length }) }}</span>
          </template>
          <div v-for="(n, i) in notes" :key="i" class="sp-note" :class="noteLevel(n.kind)">
            <span class="sp-note-dot" />
            <span class="sp-note-text">
              <b v-if="n.name">{{ n.name }}</b>
              <span v-if="n.group" class="muted"> · {{ n.group }}</span>
              <span class="sp-note-msg" :class="{ lead: !n.name && !n.group }">{{ noteText(n) }}</span>
              <span v-if="n.value" class="mono muted sp-note-value">{{ n.value }}</span>
            </span>
          </div>
        </el-collapse-item>
      </el-collapse>

      <div v-if="mode === 'append' && duplicates" class="sp-dup">
        <el-checkbox v-model="skipDuplicates" data-test="sp-skip-dup">{{ t('sunpanelImport.skipDuplicates', { n: duplicates }) }}</el-checkbox>
      </div>

      <div class="sp-list" data-test="sp-list">
        <div v-for="g in groups" :key="g.name" class="sp-group">
          <div class="sp-group-head">
            <FolderClosed :size="13" />
            <span class="ellipsis">{{ g.name || t('panels.ungrouped') }}</span>
            <span class="sp-count">{{ g.items.length }}</span>
          </div>
          <div class="sp-cards">
            <div v-for="i in g.items" :key="i" class="sp-card" :class="{ dup: skipDuplicates && dupIndexes.has(i) }" :title="links[i].url" data-test="sp-card">
              <PanelIcon :icon="links[i].icon" :name="links[i].name" :color="links[i].color" :size="28" />
              <div class="sp-card-main">
                <div class="sp-card-name">
                  <span class="ellipsis">{{ links[i].name }}</span>
                  <el-tag v-if="skipDuplicates && dupIndexes.has(i)" size="small" type="info" data-test="sp-dup-tag">{{ t('sunpanelImport.exists') }}</el-tag>
                  <el-tag v-if="links[i].size === 'small'" size="small" type="info">{{ t('panels.sizeSmall') }}</el-tag>
                  <el-tag v-if="links[i].alts.length" size="small">{{ t('panels.altCount', { n: links[i].alts.length }) }}</el-tag>
                </div>
                <div class="mono muted ellipsis sp-card-url">{{ links[i].url }}</div>
              </div>
            </div>
          </div>
        </div>
        <div v-if="!links.length" class="links-empty">{{ t('sunpanelImport.noLinks') }}</div>
      </div>
    </template>

    <template #footer>
      <div class="sp-footer">
        <el-button v-if="result" text :icon="ArrowLeft" data-test="sp-back" @click="result = null">{{ t('sunpanelImport.back') }}</el-button>
        <span class="flex1" />
        <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
        <el-button v-if="!result" type="primary" :loading="busy" :disabled="!file" data-test="sp-parse" @click="parse">{{ t('sunpanelImport.parse') }}</el-button>
        <el-button v-else type="primary" :disabled="!importCount" data-test="sp-ok" @click="confirm">
          {{ mode === 'append' ? t('sunpanelImport.append', { n: importCount }) : t('sunpanelImport.create', { n: importCount }) }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { ArrowLeft, FileArchive, FolderClosed, Info, Upload } from 'lucide-vue-next'
import PanelIcon from './PanelIcon.vue'
import { api } from '../api'
import { useFormLayout } from '../composables/useIsMobile'
import { groupLinks, MAX_LINKS, type PanelLink } from '../panels'

// 对应 POST /api/webservice/panels/sunpanel 的返回结构
interface IconStats {
  inline: number
  remote: number
  builtin: number
  iconify: number
  text: number
  fallback: number
}
interface Note {
  kind: string
  group?: string
  name?: string
  value?: string
}
export interface SunPanelResult {
  format: 'v1' | 'v2'
  appVersion: string
  groups: number
  total: number
  links: PanelLink[]
  icons: IconStats
  notes: Note[]
}

// 上传大小上限，与后端 maxSunPanelUpload 一致
const MAX_FILE = 16 << 20
// 有数量才显示的图标统计项，按常见程度排列
const ICON_KINDS: (keyof IconStats)[] = ['inline', 'builtin', 'remote', 'iconify', 'text', 'fallback']
// 被跳过的卡片的说明种类，其余为改动说明
const SKIP_KINDS = new Set(['badUrl', 'noUrl', 'tooManyLinks'])
const INFO_KINDS = new Set(['iframe', 'truncated', 'noName', 'emptyGroup'])

const props = defineProps<{
  // new：生成新面板；append：并入正在编辑的面板
  mode: 'new' | 'append'
  // 正在编辑的面板已有的链接，用于统计重复地址
  existing?: readonly PanelLink[]
}>()
const emit = defineEmits<{ done: [links: PanelLink[], skipDuplicates: boolean] }>()
const { t } = useI18n()
const { isMobile } = useFormLayout()

const visible = defineModel<boolean>({ default: false })
const input = ref<HTMLInputElement>()
const file = ref<File | null>(null)
const baseUrl = ref('')
const busy = ref(false)
const dragOver = ref(false)
const result = ref<SunPanelResult | null>(null)
const notesOpen = ref<string[]>([])
const skipDuplicates = ref(true)

const iconKinds = computed(() => ICON_KINDS.filter((k) => (result.value?.icons[k] ?? 0) > 0))
const links = computed(() => result.value?.links ?? [])
const notes = computed(() => result.value?.notes ?? [])
const icons = computed<IconStats>(() => result.value?.icons ?? { inline: 0, remote: 0, builtin: 0, iconify: 0, text: 0, fallback: 0 })
const groups = computed(() => groupLinks(links.value))
const skipped = computed(() => (result.value ? result.value.total - result.value.links.length : 0))
const missingIcons = computed(() => result.value?.notes.filter((n) => n.kind === 'iconMissing').length ?? 0)
// 与已有链接地址相同（或在导入内容中重复）的链接下标，只在并入已有面板时统计
const dupIndexes = computed(() => {
  const out = new Set<number>()
  if (props.mode !== 'append') {
    return out
  }
  const seen = new Set((props.existing ?? []).map((l) => l.url))
  links.value.forEach((l, i) => {
    if (seen.has(l.url)) {
      out.add(i)
    } else {
      seen.add(l.url)
    }
  })
  return out
})
const duplicates = computed(() => dupIndexes.value.size)
// 实际会加入的数量：去掉跳过的重复项，再按面板的链接上限截断
const importCount = computed(() => {
  if (!result.value) {
    return 0
  }
  const n = result.value.links.length - (skipDuplicates.value ? duplicates.value : 0)
  const room = MAX_LINKS - (props.mode === 'append' ? (props.existing?.length ?? 0) : 0)
  return Math.max(0, Math.min(n, room))
})

function reset() {
  file.value = null
  result.value = null
  busy.value = false
  dragOver.value = false
  notesOpen.value = []
  skipDuplicates.value = true
  if (input.value) {
    input.value.value = ''
  }
}

function formatSize(n: number) {
  return n < 1024 ? `${n} B` : n < 1 << 20 ? `${(n / 1024).toFixed(1)} KB` : `${(n / (1 << 20)).toFixed(1)} MB`
}

function choose(f: File | undefined | null) {
  if (!f) {
    return
  }
  if (f.size > MAX_FILE) {
    ElMessage.error(t('sunpanelImport.tooLarge', { size: '16 MB' }))
    return
  }
  file.value = f
}

function onPick(e: Event) {
  choose((e.target as HTMLInputElement).files?.[0])
}

function onDrop(e: DragEvent) {
  dragOver.value = false
  choose(e.dataTransfer?.files?.[0])
}

async function parse() {
  if (!file.value || busy.value) {
    return
  }
  busy.value = true
  try {
    const form = new FormData()
    form.append('file', file.value)
    form.append('baseUrl', baseUrl.value.trim())
    const r = await api.post<SunPanelResult>('/api/webservice/panels/sunpanel', form)
    result.value = { ...r, links: r.links ?? [], notes: r.notes ?? [] }
    // 有被跳过的卡片时默认展开说明
    notesOpen.value = result.value.notes.some((n) => SKIP_KINDS.has(n.kind)) ? ['notes'] : []
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

function noteLevel(kind: string) {
  return SKIP_KINDS.has(kind) ? 'skip' : INFO_KINDS.has(kind) ? 'info' : 'warn'
}

function noteText(n: Note) {
  const key = `sunpanelImport.note_${n.kind}`
  const text = t(key, { n: n.value ?? '' })
  return text === key ? n.kind : text
}

function confirm() {
  if (!result.value) {
    return
  }
  emit('done', result.value.links, skipDuplicates.value)
  visible.value = false
}
</script>

<style scoped>
.sp-intro {
  margin: 0 0 8px;
}
.sp-steps {
  margin: 0 0 14px;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 6px;
  font-size: 13px;
}
.sp-steps li {
  display: flex;
  gap: 8px;
  align-items: baseline;
}
.sp-steps b {
  flex: none;
  min-width: 30px;
  padding: 1px 6px;
  border-radius: 6px;
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  font-size: 12px;
  text-align: center;
}
.sp-drop {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 150px;
  padding: 18px;
  border: 1.5px dashed var(--g-border, var(--el-border-color));
  border-radius: 12px;
  background: var(--el-fill-color-lighter);
  cursor: pointer;
  text-align: center;
  transition: border-color 0.15s, background-color 0.15s;
}
.sp-drop:hover,
.sp-drop.over {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.sp-drop.picked {
  border-style: solid;
}
.sp-drop-icon {
  color: var(--el-color-primary);
}
.sp-drop-text {
  font-weight: 600;
}
.sp-drop-hint {
  font-size: 12px;
}
.sp-file {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  max-width: 100%;
  font-weight: 600;
}
.sp-file .muted {
  font-weight: 400;
  font-size: 12px;
}
.sp-base {
  margin-top: 14px;
}
.sp-label {
  margin-bottom: 6px;
  font-size: 13px;
  font-weight: 600;
}
.sp-summary {
  display: flex;
  align-items: stretch;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.sp-stat {
  display: flex;
  flex-direction: column;
  min-width: 84px;
  padding: 8px 12px;
  border-radius: 10px;
  background: var(--el-fill-color-light);
  font-size: 12px;
}
.sp-stat.warn .sp-num {
  color: var(--el-color-warning);
}
.sp-num {
  font-size: 20px;
  font-weight: 700;
  line-height: 1.2;
}
.sp-source {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: auto;
  font-size: 12px;
}
.sp-icons {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-bottom: 10px;
  font-size: 12px;
}
.sp-icons-label {
  font-weight: 600;
}
.sp-alert {
  margin-bottom: 8px;
}
.sp-notes {
  margin-bottom: 10px;
  border-top: none;
}
.sp-notes :deep(.el-collapse-item__header) {
  height: 36px;
}
.sp-notes-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
}
.sp-note {
  display: flex;
  gap: 8px;
  align-items: baseline;
  padding: 3px 0;
  font-size: 12px;
  line-height: 1.6;
}
.sp-note-dot {
  flex: none;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--el-color-warning);
  transform: translateY(-1px);
}
.sp-note.skip .sp-note-dot {
  background: var(--el-color-danger);
}
.sp-note.info .sp-note-dot {
  background: var(--el-color-info);
}
.sp-note-text {
  min-width: 0;
  overflow-wrap: anywhere;
}
.sp-note-msg {
  margin-left: 8px;
}
.sp-note-msg.lead {
  margin-left: 0;
}
.sp-note-value {
  margin-left: 6px;
}
.sp-dup {
  margin-bottom: 8px;
}
.sp-list {
  max-height: min(46vh, 420px);
  overflow: auto;
  padding-right: 2px;
}
.sp-group + .sp-group {
  margin-top: 10px;
}
.sp-group-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
}
.sp-count {
  padding: 0 6px;
  border-radius: 8px;
  background: var(--el-fill-color);
  font-weight: 500;
}
.sp-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 6px;
}
.sp-card {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding: 6px 8px;
  border: 1px solid var(--g-border, var(--el-border-color-lighter));
  border-radius: 8px;
}
.sp-card.dup {
  opacity: 0.5;
  border-style: dashed;
}
.sp-card-main {
  min-width: 0;
  flex: 1;
}
.sp-card-name {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  font-weight: 600;
}
.sp-card-url {
  font-size: 11px;
}
.sp-footer {
  display: flex;
  align-items: center;
  gap: 8px;
}
.flex1 {
  flex: 1;
}
.ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.links-empty {
  padding: 18px 0;
  text-align: center;
  color: var(--el-text-color-secondary);
}
@media (max-width: 640px) {
  .sp-source {
    margin-left: 0;
    width: 100%;
  }
  .sp-stat {
    flex: 1;
    min-width: 0;
  }
  .sp-cards {
    grid-template-columns: 1fr;
  }
  .sp-list {
    max-height: none;
  }
  .sp-footer {
    flex-wrap: wrap;
    /* 按钮放不下一行时，换到下一行的主按钮靠右，与对话框其他按钮对齐 */
    justify-content: flex-end;
    row-gap: 10px;
  }
}
</style>
