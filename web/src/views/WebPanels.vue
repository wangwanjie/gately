<template>
  <!-- 导航面板：面板库的管理页面。Web 服务中类型为“导航面板”的站点引用这里的面板，在站点根路径输出汇总页面 -->
  <div class="panels-page">
    <PageHeader :title="t('panels.title')" :desc="t('panels.desc')" :icon="LayoutGrid">
      <el-button :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      <el-button v-if="!isReadonly && !unsupported" type="primary" :icon="Plus" data-test="panel-add" @click="openEditor()">{{ t('panels.add') }}</el-button>
    </PageHeader>

    <el-alert v-if="unsupported" type="error" :closable="false" show-icon :title="t('panels.unsupported')" class="unsupported" data-test="panels-unsupported" />
    <div v-else-if="!loading && !panels.length" class="g-card empty-card">
      <el-empty :description="t('panels.empty')">
        <div class="form-help empty-help">{{ t('panels.emptyHelp') }}</div>
        <el-button v-if="!isReadonly" type="primary" :icon="Plus" @click="openEditor()">{{ t('panels.add') }}</el-button>
      </el-empty>
    </div>

    <div v-else class="panel-grid">
      <EntityCard v-for="p in panels" :key="p.key" :off="!p.enable" data-test="panel-card">
        <template #lead>
          <PanelIcon v-if="p.icon" :icon="p.icon" :name="p.title || p.name" :size="36" />
          <span v-else class="grid-icon"><LayoutGrid :size="18" /></span>
        </template>
        <template #title>{{ p.name }}</template>
        <template v-if="p.title || p.subtitle" #sub>{{ [p.title, p.subtitle].filter(Boolean).join(' · ') }}</template>
        <template #status>
          <el-switch v-if="!isReadonly" :model-value="p.enable" size="small" :loading="toggling[p.key ?? '']" @change="(v: string | number | boolean) => toggle(p, !!v)" />
          <StatusBadge v-else :type="p.enable ? 'running' : 'off'" :text="p.enable ? t('common.enabled') : t('common.disabled')" />
        </template>
        <Field :label="t('panels.links')">
          {{ t('panels.linkCount', { n: visibleCount(p), groups: groupLinks(p.links ?? []).length }) }}
          <span v-if="hiddenCount(p)" class="muted">{{ t('panels.hiddenCount', { n: hiddenCount(p) }) }}</span>
        </Field>
        <Field :label="t('panels.usedBy')">
          <template v-if="usage(p).length">
            <div v-for="u in usage(p)" :key="u.label" class="usage">
              <UrlLink v-if="u.url" :href="u.url" />
              <span v-else>{{ u.label }}</span>
              <span v-if="u.url" class="muted usage-label">{{ u.label }}</span>
            </div>
          </template>
          <span v-else class="muted">{{ t('panels.unused') }}</span>
        </Field>
        <template #actions>
          <template v-if="!isReadonly">
            <el-button text size="small" :icon="Pencil" @click="openEditor(p)">{{ t('common.edit') }}</el-button>
            <el-button text size="small" :icon="CopyPlus" @click="openEditor(p, true)">{{ t('common.duplicate') }}</el-button>
            <el-popconfirm :title="t('panels.confirmDelete')" @confirm="remove(p)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2" :disabled="!!p.usedBy?.length" :title="p.usedBy?.length ? t('panels.deleteUsed') : ''">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
          <el-button v-else text size="small" :icon="Eye" @click="openEditor(p)">{{ t('panels.view') }}</el-button>
        </template>
      </EntityCard>
    </div>

    <!-- 编辑面板 -->
    <el-dialog
      v-model="editorVisible"
      :title="editingKey ? t('panels.edit') : t('panels.add')"
      width="960px"
      top="5vh"
      :fullscreen="isMobile"
      :close-on-click-modal="false"
      class="panel-dialog"
      data-test="panel-dialog"
    >
      <CopyHint :from="copyFrom" />
      <el-tabs v-model="tab">
        <el-tab-pane :label="t('panels.tabLinks', { n: editing.links.length })" name="links">
          <div class="links-toolbar">
            <el-button v-if="!isReadonly" type="primary" :icon="Plus" data-test="link-add" @click="openLink(-1)">{{ t('panels.addLink') }}</el-button>
            <el-button v-if="!isReadonly" :icon="Import" data-test="link-import" @click="openImport">{{ t('panels.importSites') }}</el-button>
            <span class="muted count">{{ editing.links.length }} / {{ MAX_LINKS }}</span>
          </div>
          <div v-if="!editing.links.length" class="links-empty">{{ t('panels.noLinks') }}</div>
          <div v-for="(g, gi) in linkGroups" :key="g.name" class="link-group" data-test="link-group">
            <div class="group-head">
              <FolderClosed :size="14" />
              <span class="group-name">{{ g.name || t('panels.ungrouped') }}</span>
              <span class="group-count">{{ g.items.length }}</span>
              <span class="flex1" />
              <template v-if="!isReadonly && linkGroups.length > 1">
                <el-button text size="small" :icon="ArrowUp" :disabled="gi === 0" :aria-label="t('panels.moveUp')" :title="t('panels.groupUp')" @click="shiftGroup(g.name, -1)" />
                <el-button text size="small" :icon="ArrowDown" :disabled="gi === linkGroups.length - 1" :aria-label="t('panels.moveDown')" :title="t('panels.groupDown')" @click="shiftGroup(g.name, 1)" />
              </template>
            </div>
            <div v-for="(i, pos) in g.items" :key="rowKey(i)" class="link-row" :class="{ hidden: editing.links[i].hide }" data-test="link-row">
              <PanelIcon :icon="editing.links[i].icon" :name="editing.links[i].name" :color="editing.links[i].color" :size="32" />
              <div class="link-main" @click="!isReadonly && openLink(i)">
                <div class="link-name">
                  <span class="ellipsis">{{ editing.links[i].name }}</span>
                  <el-tag v-if="editing.links[i].hide" size="small" type="info">{{ t('panels.hiddenTag') }}</el-tag>
                  <el-tag v-if="editing.links[i].sameTab" size="small" type="info">{{ t('panels.sameTabTag') }}</el-tag>
                  <el-tag v-if="editing.links[i].alts?.length" size="small" :title="editing.links[i].alts.map((a) => a.name).join(' / ')">{{ t('panels.altCount', { n: editing.links[i].alts.length }) }}</el-tag>
                </div>
                <div class="link-url mono ellipsis" :title="editing.links[i].url">{{ editing.links[i].url }}</div>
                <div v-if="editing.links[i].lanUrl" class="link-url mono ellipsis lan" :title="editing.links[i].lanUrl">
                  <span class="lan-tag">{{ t('panels.lanShort') }}</span>{{ editing.links[i].lanUrl }}
                </div>
              </div>
              <div v-if="!isReadonly" class="link-actions">
                <el-button text size="small" :icon="ArrowUp" :disabled="pos === 0" :aria-label="t('panels.moveUp')" :title="t('panels.moveUp')" @click="shiftLink(i, -1)" />
                <el-button text size="small" :icon="ArrowDown" :disabled="pos === g.items.length - 1" :aria-label="t('panels.moveDown')" :title="t('panels.moveDown')" @click="shiftLink(i, 1)" />
                <el-button text size="small" :icon="Pencil" :aria-label="t('common.edit')" :title="t('common.edit')" data-test="link-edit" @click="openLink(i)" />
                <el-button text size="small" type="danger" :icon="Trash2" :aria-label="t('common.delete')" :title="t('common.delete')" data-test="link-delete" @click="removeLink(i)" />
              </div>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('panels.tabPage')" name="page">
          <el-form :model="editing" label-width="130px" :label-position="labelPosition" :disabled="isReadonly">
            <el-form-item :label="t('panels.name')" required>
              <el-input v-model="editing.name" maxlength="64" data-test="panel-name" />
              <div class="form-help">{{ t('panels.nameHelp') }}</div>
            </el-form-item>
            <el-form-item :label="t('panels.pageTitle')">
              <el-input v-model="editing.title" maxlength="100" :placeholder="editing.name" />
            </el-form-item>
            <el-form-item :label="t('panels.subtitle')"><el-input v-model="editing.subtitle" maxlength="200" /></el-form-item>
            <el-form-item :label="t('panels.icon')">
              <PanelIconInput v-model="editing.icon" :name="editing.title || editing.name" />
            </el-form-item>
            <el-form-item :label="t('panels.theme')">
              <el-radio-group v-model="editing.theme">
                <el-radio-button value="">{{ t('panels.themeAuto') }}</el-radio-button>
                <el-radio-button value="light">{{ t('panels.themeLight') }}</el-radio-button>
                <el-radio-button value="dark">{{ t('panels.themeDark') }}</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <el-form-item :label="t('panels.lanCidrs')">
              <el-input v-model="lanText" type="textarea" :rows="2" class="mono" placeholder="100.64.0.0/10" />
              <div class="form-help">{{ t('panels.lanCidrsHelp') }}</div>
            </el-form-item>
            <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>
            <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane :label="t('panels.tabPreview')" name="preview">
          <div class="preview-bar">
            <el-radio-group v-model="previewLAN" size="small" @change="renderPreview">
              <el-radio-button :value="false">{{ t('panels.previewWan') }}</el-radio-button>
              <el-radio-button :value="true">{{ t('panels.previewLan') }}</el-radio-button>
            </el-radio-group>
            <el-button size="small" :icon="RefreshCw" :loading="previewing" @click="renderPreview">{{ t('common.refresh') }}</el-button>
            <span class="form-help preview-help">{{ t('panels.previewHelp') }}</span>
          </div>
          <el-alert v-if="previewError" type="error" :closable="false" show-icon :title="previewError" />
          <!-- 预览页面不含脚本，sandbox 也不允许脚本；允许点开链接（新标签页打开，不带管理界面的地址） -->
          <iframe v-else class="preview-frame" sandbox="allow-popups allow-popups-to-escape-sandbox" referrerpolicy="no-referrer" :srcdoc="previewHtml" :title="t('panels.tabPreview')" data-test="panel-preview" />
        </el-tab-pane>
      </el-tabs>
      <template #footer>
        <el-button @click="editorVisible = false">{{ isReadonly ? t('common.close') : t('common.cancel') }}</el-button>
        <el-button v-if="!isReadonly" type="primary" :loading="saving" data-test="panel-save" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 编辑链接 -->
    <el-dialog v-model="linkVisible" :title="linkIndex >= 0 ? t('panels.editLink') : t('panels.addLink')" width="620px" :fullscreen="isMobile" append-to-body data-test="link-dialog">
      <el-form :model="link" label-width="110px" :label-position="labelPosition">
        <el-form-item :label="t('panels.linkName')" required><el-input v-model="link.name" maxlength="64" data-test="link-name" /></el-form-item>
        <GroupInput v-model="link.group" :groups="groupNames" />
        <el-form-item :label="t('panels.url')" required>
          <el-input v-model="link.url" class="mono" placeholder="https://nas.example.com" data-test="link-url" />
        </el-form-item>
        <el-form-item :label="t('panels.lanUrl')">
          <el-input v-model="link.lanUrl" class="mono" placeholder="http://192.168.1.2:5000" data-test="link-lan-url" />
          <div class="form-help">{{ t('panels.lanUrlHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('panels.alts')">
          <div class="alts">
            <div v-for="(a, ai) in link.alts" :key="ai" class="alt-row" data-test="alt-row">
              <el-input v-model="a.name" maxlength="20" class="alt-name" :placeholder="t('panels.altNamePlaceholder')" data-test="alt-name" />
              <el-input v-model="a.url" class="mono alt-url" :placeholder="altUrlExample" data-test="alt-url" />
              <el-button text type="danger" :icon="Trash2" :aria-label="t('common.delete')" @click="link.alts.splice(ai, 1)" />
            </div>
            <el-button size="small" :icon="Plus" :disabled="link.alts.length >= MAX_ALTS" data-test="alt-add" @click="link.alts.push({ name: '', url: '' })">{{ t('panels.addAlt') }}</el-button>
            <div class="form-help">{{ t('panels.altsHelp') }}</div>
            <div class="form-help">{{ t('panels.varsHelp') }}</div>
          </div>
        </el-form-item>
        <el-form-item :label="t('panels.description')"><el-input v-model="link.description" maxlength="200" /></el-form-item>
        <el-form-item :label="t('panels.icon')">
          <PanelIconInput v-model="link.icon" :name="link.name" :color="link.color" />
        </el-form-item>
        <el-form-item v-if="iconKind(link.icon) !== 'image'" :label="t('panels.color')">
          <el-color-picker v-model="colorModel" :predefine="palette" />
          <span class="form-help inline-help">{{ link.color ? link.color : iconKind(link.icon) === 'builtin' ? t('panels.colorBuiltin') : t('panels.colorAuto') }}</span>
        </el-form-item>
        <el-form-item :label="t('panels.options')">
          <el-checkbox v-model="link.sameTab">{{ t('panels.sameTab') }}</el-checkbox>
          <el-checkbox v-model="link.hide">{{ t('panels.hide') }}</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="linkVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" data-test="link-ok" @click="saveLink">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>

    <!-- 从 Web 服务站点添加 -->
    <el-dialog v-model="importVisible" :title="t('panels.importSites')" width="640px" :fullscreen="isMobile" append-to-body data-test="import-dialog">
      <el-input v-model="importQuery" clearable :prefix-icon="Search" :placeholder="t('panels.importSearch')" class="import-search" />
      <div v-if="!importList.length" class="links-empty">{{ t('panels.importEmpty') }}</div>
      <div class="import-list">
        <label v-for="s in importList" :key="s.id" class="import-row" :class="{ added: existingUrls.has(s.url) }" data-test="import-row">
          <el-checkbox :model-value="existingUrls.has(s.url) || importSel.has(s.id)" :disabled="existingUrls.has(s.url)" @change="(v: string | number | boolean) => toggleImport(s.id, !!v)" />
          <div class="import-main">
            <div class="ellipsis">
              <b>{{ s.name }}</b>
              <span v-if="s.group" class="muted"> · {{ s.group }}</span>
            </div>
            <div class="mono muted ellipsis">{{ s.url }}</div>
          </div>
          <span class="muted import-listener">{{ existingUrls.has(s.url) ? t('panels.importAdded') : s.listener }}</span>
        </label>
      </div>
      <template #footer>
        <el-button @click="importVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :disabled="!importSel.size" data-test="import-ok" @click="doImport">{{ t('panels.importN', { n: importSel.size }) }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ArrowDown, ArrowUp, CopyPlus, Eye, FolderClosed, Import, LayoutGrid, Pencil, Plus, RefreshCw, Search, Trash2 } from 'lucide-vue-next'
import PageHeader from '../components/PageHeader.vue'
import EntityCard from '../components/EntityCard.vue'
import Field from '../components/Field.vue'
import StatusBadge from '../components/StatusBadge.vue'
import UrlLink from '../components/UrlLink.vue'
import CopyHint from '../components/CopyHint.vue'
import GroupInput from '../components/GroupInput.vue'
import PanelIcon from '../components/PanelIcon.vue'
import PanelIconInput from '../components/PanelIconInput.vue'
import { ApiError, api } from '../api'
import { isReadonly } from '../nav'
import { useFormLayout } from '../composables/useIsMobile'
import { splitLinesOrCommas, useListText } from '../composables/useListText'
import { createItem, uniqueName, type CopySource } from '../copyDraft'
import { forwardedEntry, siteLink, type ForwardSource } from '../siteUrl'
import { loadForwardSources } from '../forwardSources'
import {
  MAX_ALTS,
  MAX_LINKS,
  emptyLink,
  emptyPanel,
  groupLinks,
  iconKind,
  importableSites,
  insertLink,
  linksFromSites,
  moveGroup,
  moveLink,
  toPanel,
  type ImportListener,
  type Panel,
  type PanelLink,
  type PanelView,
} from '../panels'

// 监听只取生成访问地址与列出站点需要的字段
interface ListenerLite extends ImportListener {
  tls: boolean
  accessScheme?: string
  accessPort?: number
  sites: (ImportListener['sites'][number] & { panel?: string })[]
  default: { enable: boolean; type: string; panel?: string }
}

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()

const panels = ref<PanelView[]>([])
const listeners = ref<ListenerLite[]>([])
const forwards = ref<ForwardSource[]>([])
const loading = ref(true)
// unsupported 为后端没有面板接口（连接的是不含此功能的旧版本，接口返回 404）
const unsupported = ref(false)
const toggling = reactive<Record<string, boolean>>({})

async function load() {
  loading.value = true
  try {
    panels.value = await api.get<PanelView[]>('/api/webservice/panels')
    unsupported.value = false
    try {
      listeners.value = await api.get<ListenerLite[]>('/api/webservice/listeners')
    } catch {
      listeners.value = []
    }
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      unsupported.value = true
      return
    }
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

function visibleCount(p: Panel) {
  return (p.links ?? []).filter((l) => !l.hide).length
}
function hiddenCount(p: Panel) {
  return (p.links ?? []).length - visibleCount(p)
}

// linkFor 按监听的访问协议与端口拼出域名的地址；未手动填写访问端口时，经协议分流或端口转发进入的监听用转发的入口端口
function linkFor(l: ListenerLite, domain: string): string | null {
  const entry = l.accessPort ? null : forwardedEntry(l, domain, forwards.value)
  return siteLink(domain, entry ? { ...l, accessPort: entry.port } : l)?.href ?? null
}

// usage 列出引用面板的站点与可打开的地址；默认站点没有具体域名，只显示名称
function usage(p: PanelView): { label: string; url: string | null }[] {
  const out: { label: string; url: string | null }[] = []
  for (const l of listeners.value) {
    const ln = l.name || `:${l.port}`
    for (const s of l.sites ?? []) {
      if (s.type === 'panel' && s.panel === p.key) {
        const d = (s.domains ?? []).find((x) => linkFor(l, x))
        out.push({ label: `${ln} / ${s.name || (s.domains ?? [])[0] || ''}`, url: d ? linkFor(l, d) : null })
      }
    }
    if (l.default?.type === 'panel' && l.default.panel === p.key) {
      out.push({ label: `${ln} / ${t('web.defaultSite')}`, url: null })
    }
  }
  // 监听列表读取失败时退回接口给出的引用方
  if (!out.length && p.usedBy?.length) {
    return p.usedBy.map((u) => ({ label: u.replace('@default', t('web.defaultSite')), url: null }))
  }
  return out
}

async function toggle(p: PanelView, enable: boolean) {
  const k = p.key ?? ''
  toggling[k] = true
  try {
    await api.put(`/api/webservice/panels/${k}/enable`, { enable })
    p.enable = enable
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    toggling[k] = false
  }
}

async function remove(p: PanelView) {
  try {
    await api.del(`/api/webservice/panels/${p.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// ---- 编辑面板 ----

const editorVisible = ref(false)
const editing = ref<Panel>(emptyPanel())
const editingKey = ref('')
const copyFrom = ref<CopySource | null>(null)
const tab = ref('links')
const saving = ref(false)
const lanText = useListText(() => editing.value.lanCidrs, (v) => (editing.value.lanCidrs = v), { split: splitLinesOrCommas })

// rowIds 为链接行的稳定 id（链接没有 key，移动、删除后用对象身份找回原来的 id，避免整列表重绘）
const rowIds = new WeakMap<PanelLink, number>()
let nextRowId = 1
function rowKey(i: number) {
  const l = editing.value.links[i]
  let id = rowIds.get(l)
  if (id === undefined) {
    id = nextRowId++
    rowIds.set(l, id)
  }
  return id
}

const linkGroups = computed(() => groupLinks(editing.value.links))
const groupNames = computed(() => linkGroups.value.map((g) => g.name).filter(Boolean))

function openEditor(p?: PanelView, copy = false) {
  editing.value = p ? toPanel(JSON.parse(JSON.stringify(p))) : emptyPanel()
  editingKey.value = p && !copy ? (p.key ?? '') : ''
  copyFrom.value = null
  if (p && copy) {
    copyFrom.value = { key: p.key ?? '', name: p.name }
    editing.value.name = uniqueName(p.name, panels.value.map((x) => x.name))
  }
  // 新建时先填写名称，已有面板直接编辑链接
  tab.value = p ? 'links' : 'page'
  previewHtml.value = ''
  previewError.value = ''
  editorVisible.value = true
}

async function save() {
  if (!editing.value.name.trim()) {
    tab.value = 'page'
    ElMessage.error(t('panels.nameRequired'))
    return
  }
  saving.value = true
  try {
    const body = toPanel(editing.value)
    if (editingKey.value) {
      await api.put(`/api/webservice/panels/${editingKey.value}`, body)
    } else {
      await createItem('/api/webservice/panels', body, copyFrom.value)
    }
    editorVisible.value = false
    ElMessage.success(t('common.saved'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

function shiftLink(i: number, delta: -1 | 1) {
  editing.value.links = moveLink(editing.value.links, i, delta)
}
function shiftGroup(name: string, delta: -1 | 1) {
  editing.value.links = moveGroup(editing.value.links, name, delta)
}

async function removeLink(i: number) {
  const l = editing.value.links[i]
  try {
    await ElMessageBox.confirm(t('panels.confirmDeleteLink', { name: l.name }), { type: 'warning', confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel') })
  } catch {
    return
  }
  editing.value.links = editing.value.links.filter((_, j) => j !== i)
}

// ---- 编辑链接 ----

const linkVisible = ref(false)
// altUrlExample 为其他访问方式地址的示例（写在模板属性里时方括号会干扰模板解析）
const altUrlExample = 'https://[2001:db8::2]:8443'
const link = ref<PanelLink>(emptyLink())
const linkIndex = ref(-1)
const palette = ['#0e7c66', '#2563eb', '#7c3aed', '#db2777', '#ea580c', '#b45309', '#15803d', '#0e7490', '#4f46e5', '#dc2626']
// el-color-picker 清空时给 null，这里换成空字符串（按名称自动挑选颜色）
const colorModel = computed<string | null>({
  get: () => link.value.color || null,
  set: (v) => {
    link.value.color = (v ?? '').toLowerCase()
  },
})

function openLink(i: number) {
  if (i < 0 && editing.value.links.length >= MAX_LINKS) {
    ElMessage.error(t('panels.tooManyLinks', { n: MAX_LINKS }))
    return
  }
  linkIndex.value = i
  // 新链接默认放进最后一个分组
  const lastGroup = linkGroups.value.at(-1)?.name ?? ''
  link.value = i >= 0 ? { ...editing.value.links[i], alts: (editing.value.links[i].alts ?? []).map((a) => ({ ...a })) } : emptyLink(lastGroup)
  linkVisible.value = true
}

// looksLikeUrl 粗略检查地址带协议，具体规则（允许的协议、变量）由后端在保存时校验
function looksLikeUrl(v: string) {
  return /^[a-z][a-z0-9+.-]*:/i.test(v.trim())
}

function saveLink() {
  const l = link.value
  l.name = l.name.trim()
  l.url = l.url.trim()
  l.lanUrl = l.lanUrl.trim()
  l.group = l.group.trim()
  if (!l.name) {
    ElMessage.error(t('panels.linkNameRequired'))
    return
  }
  // 其他访问方式：去掉名称与地址都空的行，其余必须两项都填
  l.alts = l.alts.map((a) => ({ name: a.name.trim(), url: a.url.trim() })).filter((a) => a.name || a.url)
  if (l.alts.some((a) => !a.name)) {
    ElMessage.error(t('panels.altNameRequired'))
    return
  }
  if (!looksLikeUrl(l.url) || (l.lanUrl && !looksLikeUrl(l.lanUrl)) || l.alts.some((a) => !looksLikeUrl(a.url))) {
    ElMessage.error(t('panels.urlInvalid'))
    return
  }
  const links = editing.value.links
  if (linkIndex.value >= 0) {
    const old = links[linkIndex.value]
    if (old.group.trim() === l.group) {
      links[linkIndex.value] = { ...l }
    } else {
      // 换了分组：移到新分组的末尾
      const rest = links.filter((_, j) => j !== linkIndex.value)
      editing.value.links = insertLink(rest, { ...l }).links
    }
  } else {
    editing.value.links = insertLink(links, { ...l }).links
  }
  linkVisible.value = false
}

// ---- 从站点添加 ----

const importVisible = ref(false)
const importQuery = ref('')
const importSel = reactive(new Set<string>())
const allImport = computed(() => importableSites(listeners.value, linkFor))
const importList = computed(() => {
  const q = importQuery.value.trim().toLowerCase()
  return q ? allImport.value.filter((s) => [s.name, s.group, s.url, s.listener].join(' ').toLowerCase().includes(q)) : allImport.value
})
const existingUrls = computed(() => new Set(editing.value.links.map((l) => l.url)))

async function openImport() {
  importSel.clear()
  importQuery.value = ''
  importVisible.value = true
  // 站点可能在打开本页后才添加，打开时重新读取
  try {
    listeners.value = await api.get<ListenerLite[]>('/api/webservice/listeners')
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

function toggleImport(id: string, on: boolean) {
  if (on) {
    importSel.add(id)
  } else {
    importSel.delete(id)
  }
}

function doImport() {
  const picked = allImport.value.filter((s) => importSel.has(s.id))
  const add = linksFromSites(picked, editing.value.links)
  if (editing.value.links.length + add.length > MAX_LINKS) {
    ElMessage.error(t('panels.tooManyLinks', { n: MAX_LINKS }))
    return
  }
  let links = editing.value.links
  for (const l of add) {
    links = insertLink(links, l).links
  }
  editing.value.links = links
  importVisible.value = false
  ElMessage.success(t('panels.imported', { n: add.length }))
}

// ---- 预览 ----

const previewHtml = ref('')
const previewError = ref('')
const previewLAN = ref(false)
const previewing = ref(false)

async function renderPreview() {
  previewing.value = true
  try {
    const r = await api.post<{ html: string }>('/api/webservice/panels/preview', { panel: toPanel(editing.value), lan: previewLAN.value })
    previewHtml.value = r.html
    previewError.value = ''
  } catch (e) {
    previewError.value = (e as Error).message
  } finally {
    previewing.value = false
  }
}

watch(tab, (v) => {
  if (v === 'preview') {
    renderPreview()
  }
})

onMounted(async () => {
  await load()
  forwards.value = await loadForwardSources()
})
</script>

<style scoped>
.panel-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 14px;
}
.empty-card {
  padding: 12px 0;
}
.empty-help {
  max-width: 460px;
  margin: -8px auto 16px;
  text-align: center;
}
.grid-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}
.muted {
  color: var(--el-text-color-secondary);
}
.usage {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
  min-width: 0;
}
.usage-label {
  font-size: 12px;
}
.links-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.links-toolbar .el-button + .el-button {
  margin-left: 0;
}
.count {
  margin-left: auto;
  font-size: 12px;
}
.links-empty {
  padding: 36px 0;
  text-align: center;
  color: var(--el-text-color-secondary);
}
.link-group {
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  margin-bottom: 12px;
  overflow: hidden;
}
.group-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px 6px 12px;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-regular);
  font-size: 13px;
  font-weight: 600;
}
.group-count {
  min-width: 20px;
  padding: 0 6px;
  border-radius: 999px;
  background: var(--el-bg-color);
  color: var(--el-text-color-secondary);
  font-size: 11px;
  font-weight: 500;
  text-align: center;
}
.flex1 {
  flex: 1;
}
.group-head .el-button + .el-button,
.link-actions .el-button + .el-button {
  margin-left: 0;
}
.link-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 8px 8px 12px;
  border-top: 1px solid var(--g-border);
  min-width: 0;
}
.link-row.hidden {
  opacity: 0.55;
}
.link-main {
  flex: 1;
  min-width: 0;
  cursor: pointer;
}
.link-name {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  min-width: 0;
}
.link-url {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.link-url.lan {
  color: var(--el-text-color-placeholder);
}
.lan-tag {
  display: inline-block;
  margin-right: 6px;
  padding: 0 4px;
  border-radius: 4px;
  background: var(--el-fill-color);
  font-family: var(--el-font-family);
  font-size: 11px;
}
.link-actions {
  display: flex;
  flex: none;
}
.ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.preview-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.preview-help {
  margin: 0;
}
.preview-frame {
  display: block;
  width: 100%;
  height: 560px;
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
  background: #fff;
}
.unsupported {
  margin-bottom: 14px;
}
.alts {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
}
.alts > .el-button {
  align-self: flex-start;
}
.alt-row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.alt-name {
  width: 120px;
  flex: none;
}
.alt-url {
  flex: 1;
  min-width: 0;
}
.inline-help {
  margin: 0 0 0 10px;
}
.import-search {
  margin-bottom: 10px;
}
.import-list {
  max-height: 55vh;
  overflow: auto;
}
.import-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 4px;
  border-bottom: 1px solid var(--g-border);
  cursor: pointer;
  min-width: 0;
}
.import-row.added {
  cursor: default;
  opacity: 0.6;
}
.import-main {
  flex: 1;
  min-width: 0;
  font-size: 13px;
}
.import-listener {
  flex: none;
  font-size: 12px;
}
@media (max-width: 768px) {
  .panel-grid {
    grid-template-columns: 1fr;
  }
  .link-row {
    gap: 10px;
    padding: 8px 4px 8px 10px;
  }
  /* 移动端只保留上下移动与删除按钮，点链接本身进入编辑 */
  .link-actions [data-test='link-edit'] {
    display: none;
  }
  /* 移动端：名称与删除按钮一行、地址占满第二行，各组之间用分隔线区分 */
  .alt-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 6px;
    padding-bottom: 8px;
    border-bottom: 1px dashed var(--g-border);
  }
  .alt-name {
    width: auto;
  }
  .alt-url {
    grid-column: 1 / -1;
    grid-row: 2;
  }
  .preview-frame {
    height: calc(100vh - 260px);
    min-height: 360px;
  }
  .import-listener {
    max-width: 30%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}
</style>
