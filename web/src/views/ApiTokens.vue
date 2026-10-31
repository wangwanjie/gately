<template>
  <div>
    <PageHeader :title="t('apiTokens.title')" :desc="t('apiTokens.desc')" :icon="KeyRound">
      <router-link to="/api-docs"><el-button :icon="BookOpen">{{ t('apiTokens.apiDocs') }}</el-button></router-link>
      <el-button type="primary" :icon="Plus" @click="openCreate">{{ t('apiTokens.create') }}</el-button>
    </PageHeader>

    <el-alert type="info" :closable="false" show-icon class="block-gap">
      <template #title>
        <i18n-t keypath="apiTokens.intro" tag="span">
          <template #header><span class="mono code">Authorization: Bearer gtl_…</span></template>
        </i18n-t>
      </template>
    </el-alert>

    <div class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><ShieldCheck :size="16" />{{ t('apiTokens.listTitle') }} <span class="g-card-desc">{{ t('apiTokens.total', { count: tokens.length }) }}</span></div>
        <div class="head-actions">
          <ListTools :gl="gl" size="small" :placeholder="t('groupSort.search')" />
          <CompactToggle v-model="compact" size="small" />
        </div>
      </div>
      <div class="g-card-body">
        <!-- 桌面与移动端共用卡片列表，支持分组与排序 -->
        <GroupedList :gl="gl" :loading="loading" :compact="compact" grid :empty-text="isMobile ? t('apiTokens.emptyMobile') : t('apiTokens.emptyDesktop')" :empty-image-size="60">
          <template #item="{ it, item: tk, sorting, bind }">
            <EntityCard v-bind="bind" :off="tk.disabled || tk.expired" :compact="compact || sorting">
              <template #lead>
                <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
                <el-switch v-else :model-value="!tk.disabled" :loading="busyId === tk.id" @change="(v: string | number | boolean) => toggle(tk, Boolean(v))" />
              </template>
              <template #title>
                <span>{{ tk.name }}</span>
                <el-tooltip v-if="tk.legacy" :content="t('apiTokens.legacyTip')" placement="top">
                  <el-tag size="small" type="warning" effect="light">{{ t('apiTokens.legacy') }}</el-tag>
                </el-tooltip>
                <el-tag v-if="tk.expired" size="small" type="danger" effect="light">{{ t('apiTokens.expired') }}</el-tag>
              </template>
              <template #sub><span class="mono">{{ tk.hint }}</span></template>
              <Field :label="t('apiTokens.scopes')" wide>
                <ScopeTags :scopes="tk.scopes" :titles="domainTitles" />
              </Field>
              <Field :label="t('apiTokens.expiresAt')">
                <span v-if="!tk.expiresAt" class="muted">{{ t('apiTokens.never') }}</span>
                <span v-else :class="{ 'text-danger': tk.expired }">{{ fmtTime(tk.expiresAt) }}</span>
              </Field>
              <Field :label="t('apiTokens.lastUsed')">
                <template v-if="tk.lastUsedAt">
                  {{ fmtTime(tk.lastUsedAt) }}<span v-if="tk.lastUsedIP" class="meta mono"> · {{ tk.lastUsedIP }}</span>
                </template>
                <span v-else class="muted">{{ t('apiTokens.neverUsed') }}</span>
              </Field>
              <template v-if="tk.legacy && !sorting" #extra>
                <p class="legacy-tip">{{ t('apiTokens.legacyShort') }}</p>
              </template>
              <template v-if="sorting" #actions>
                <SortMoves :gl="gl" :it="it" />
              </template>
              <template v-else #actions>
                <el-button text size="small" :icon="Pencil" @click="openEdit(tk)">{{ t('common.edit') }}</el-button>
                <el-button text size="small" :icon="RefreshCw" @click="regenerate(tk)">{{ t('apiTokens.regenerate') }}</el-button>
                <el-button text size="small" type="danger" :icon="Trash2" @click="remove(tk)">{{ t('common.delete') }}</el-button>
              </template>
            </EntityCard>
          </template>
        </GroupedList>
      </div>
    </div>

    <!-- 新建 / 编辑对话框 -->
    <el-dialog v-model="formVisible" :title="editingId ? t('apiTokens.editTitle') : t('apiTokens.create')" width="760px" :fullscreen="isMobile" :close-on-click-modal="false" append-to-body>
      <el-form label-width="90px" :label-position="labelPosition" @submit.prevent>
        <el-form-item :label="t('common.name')" required>
          <el-input v-model="form.name" maxlength="64" :placeholder="t('apiTokens.namePlaceholder')" />
        </el-form-item>
        <GroupInput v-model="form.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('apiTokens.expiresAt')">
          <div class="exp-row">
            <el-select v-model="form.expiry" class="exp-select">
              <el-option v-for="o in expiryOptions" :key="o.value" :label="o.label" :value="o.value" />
            </el-select>
            <el-date-picker
              v-if="form.expiry === 'custom'"
              v-model="form.customDate"
              type="datetime"
              :placeholder="t('apiTokens.pickExpiry')"
              :disabled-date="disablePast"
              class="exp-date"
            />
          </div>
        </el-form-item>
        <el-form-item :label="t('apiTokens.scopes')" required>
          <div class="perm">
            <div class="perm-presets">
              <el-button size="small" @click="applyPreset('read')">{{ t('apiTokens.allRead') }}</el-button>
              <el-button size="small" @click="applyPreset('write')">{{ t('apiTokens.allWrite') }}</el-button>
              <el-button size="small" @click="applyPreset('none')">{{ t('common.clear') }}</el-button>
              <span class="perm-count">{{ t('apiTokens.granted', { granted: grantedCount, total: domains.length }) }}</span>
            </div>

            <div v-for="g in domainGroups" :key="g.title" class="perm-group">
              <div class="perm-group-title">{{ g.title }}</div>
              <div v-for="d in g.items" :key="d.id" class="perm-row">
                <div class="perm-name">
                  <span>{{ d.title }}</span>
                  <span class="perm-id mono">{{ d.id }}</span>
                  <el-tooltip v-if="d.canExec" :content="t('apiTokens.execTip')" placement="top">
                    <el-tag size="small" type="warning" effect="light">{{ t('apiTokens.execTag') }}</el-tag>
                  </el-tooltip>
                </div>
                <el-radio-group v-model="levels[d.id]" size="small" class="perm-radio">
                  <el-radio-button value="none">{{ t('common.none') }}</el-radio-button>
                  <el-radio-button v-if="!d.writeOnly" value="read">{{ t('apiTokens.readOnly') }}</el-radio-button>
                  <el-radio-button v-if="!d.readOnly" value="write">{{ d.writeOnly ? t('apiTokens.execute') : t('apiTokens.readWrite') }}</el-radio-button>
                </el-radio-group>
              </div>
            </div>
            <el-alert v-if="execGranted.length" type="warning" :closable="false" show-icon class="exec-warn">
              {{ t('apiTokens.execWarn', { modules: execGranted.join(t('apiTokens.listSep')) }) }}
            </el-alert>
            <div class="form-help">{{ t('apiTokens.permHelp') }}</div>
          </div>
        </el-form-item>
        <el-form-item v-if="editingId" :label="t('common.enable')">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="submit">{{ editingId ? t('common.save') : t('common.create') }}</el-button>
      </template>
    </el-dialog>

    <!-- 明文仅显示一次：必须点击“我已保存”才能关闭 -->
    <el-dialog
      v-model="secretVisible"
      :title="t('apiTokens.secretTitle')"
      width="620px"
      :fullscreen="isMobile"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :show-close="false"
      append-to-body
    >
      <el-alert type="warning" :closable="false" show-icon :title="t('apiTokens.secretWarn')" class="block-gap" />
      <div class="sec-label">{{ secretName }}</div>
      <div class="sec-box">
        <code ref="secretEl" class="sec-text mono" @click="selectNode(secretEl)">{{ secret }}</code>
        <el-button type="primary" :icon="Copy" @click="copy(secret, secretEl)">{{ t('common.copy') }}</el-button>
      </div>
      <div class="sec-label">{{ t('apiTokens.example') }}</div>
      <div class="sec-box sec-curl-box">
        <pre ref="curlEl" class="sec-curl mono" @click="selectNode(curlEl)">{{ curlExample }}</pre>
        <el-button :icon="Copy" @click="copy(curlExample, curlEl)">{{ t('common.copy') }}</el-button>
      </div>
      <template #footer>
        <el-button type="primary" :icon="Check" @click="closeSecret">{{ t('apiTokens.saved') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { BookOpen, Check, Copy, KeyRound, Pencil, Plus, RefreshCw, ShieldCheck, Trash2 } from 'lucide-vue-next'
import { api, type ScopeDomain, type TokenInput, type TokenSecret, type TokenView } from '../api'
import { apiOrigin } from '../basePath'
import PageHeader from '../components/PageHeader.vue'
import ScopeTags from '../components/ScopeTags.vue'
import EntityCard from '../components/EntityCard.vue'
import Field from '../components/Field.vue'
import CompactToggle from '../components/CompactToggle.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import { useCompactView } from '../composables/useCompactView'
import { useGroupedList } from '../composables/useGroupSort'
import { flatten, groupChanges } from '../groupSort'
import { useFormLayout } from '../composables/useIsMobile'
import { copyText, selectNode } from '../composables/useClipboard'

// 单个权限域的授权级别
type Level = 'none' | 'read' | 'write'

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()

const tokens = ref<TokenView[]>([])
const domains = ref<ScopeDomain[]>([])
const loading = ref(false)
const busyId = ref('')
const compact = useCompactView('apiTokens')
// 令牌列表的搜索、分组与排序：未手动排序前按创建时间从新到旧，调整后按保存的顺序
const gl = useGroupedList<TokenView>({
  storageKey: 'apiTokens.groups',
  items: () => tokens.value,
  idOf: (tk) => tk.id,
  reload: load,
  matches: (tk, q) => [tk.name, tk.group ?? '', tk.hint, ...tk.scopes].join('\n').toLowerCase().includes(q),
  save: async (next) => {
    for (const [group, keys] of groupChanges(next, (tk) => tk.id, (tk) => tk.group ?? '')) {
      await api.put('/api/tokens/group', { keys, group })
    }
    const order = flatten(next).map((tk) => tk.id)
    if (order.join('\n') !== tokens.value.map((tk) => tk.id).join('\n')) {
      await api.put('/api/tokens/order', { keys: order })
    }
  },
})

// 权限域 ID -> 中文标题，供列表摘要使用
const domainTitles = computed(() => {
  const m: Record<string, string> = {}
  for (const d of domains.value) {
    m[d.id] = d.title
  }
  return m
})

// ---------- 加载 ----------
async function load() {
  loading.value = true
  try {
    const [list, scopes] = await Promise.all([api.get<TokenView[] | null>('/api/tokens'), api.get<ScopeDomain[] | null>('/api/tokens/scopes')])
    tokens.value = list ?? []
    domains.value = scopes ?? []
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

function fmtTime(s: string): string {
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) {
    return s
  }
  return d.toLocaleString()
}

// ---------- 权限矩阵 ----------

// 功能模块与“系统”两组；builtin 的域（状态、日志、导入）归入系统
const domainGroups = computed(() => {
  const mods = domains.value.filter((d) => !d.builtin)
  const sys = domains.value.filter((d) => d.builtin)
  const groups: { title: string; items: ScopeDomain[] }[] = []
  if (mods.length) {
    groups.push({ title: t('apiTokens.groupModules'), items: mods })
  }
  if (sys.length) {
    groups.push({ title: t('apiTokens.groupSystem'), items: sys })
  }
  return groups
})

// 某个域能达到的最高级别
function maxLevel(d: ScopeDomain): Level {
  return d.readOnly ? 'read' : 'write'
}

const levels = reactive<Record<string, Level>>({})

// 已授予写权限、且写权限可以执行命令的模块标题
const execGranted = computed(() => domains.value.filter((d) => d.canExec && levels[d.id] === 'write').map((d) => d.title))

const grantedCount = computed(() => domains.value.filter((d) => (levels[d.id] ?? 'none') !== 'none').length)

// 把级别按“取较高者”合并
function higher(a: Level, b: Level): Level {
  const rank: Record<Level, number> = { none: 0, read: 1, write: 2 }
  return rank[a] >= rank[b] ? a : b
}

// applyPreset 一键设置所有权限域：只读时跳过只有写权限的域
function applyPreset(p: 'read' | 'write' | 'none') {
  for (const d of domains.value) {
    if (p === 'none') {
      levels[d.id] = 'none'
    } else if (p === 'read') {
      levels[d.id] = d.writeOnly ? 'none' : 'read'
    } else {
      levels[d.id] = maxLevel(d)
    }
  }
}

// scopesToLevels 把后端 scopes 数组还原为矩阵，能识别 `*`、`*:read`、`*:write` 通配
function scopesToLevels(scopes: string[]) {
  applyPreset('none')
  for (const raw of scopes) {
    const s = raw.trim()
    if (s === '*' || s === '*:write') {
      for (const d of domains.value) {
        levels[d.id] = higher(levels[d.id], maxLevel(d))
      }
      continue
    }
    if (s === '*:read') {
      for (const d of domains.value) {
        if (!d.writeOnly) {
          levels[d.id] = higher(levels[d.id], 'read')
        }
      }
      continue
    }
    const idx = s.lastIndexOf(':')
    const id = idx > 0 ? s.slice(0, idx) : s
    const mode = idx > 0 ? s.slice(idx + 1) : 'write'
    const d = domains.value.find((x) => x.id === id)
    if (!d) {
      continue
    }
    // 写含读；对只读域的 write 视为 read
    const lv: Level = mode === 'read' ? 'read' : maxLevel(d)
    levels[d.id] = higher(levels[d.id], lv)
  }
}

// levelsToScopes 把矩阵转换为 scopes：全部读写用 ["*"]，全部只读用 ["*:read"]，其余逐项列出
function levelsToScopes(): string[] {
  const ds = domains.value
  if (!ds.length) {
    return []
  }
  if (ds.every((d) => levels[d.id] === maxLevel(d))) {
    return ['*']
  }
  if (ds.every((d) => (d.writeOnly ? levels[d.id] === 'none' : levels[d.id] === 'read'))) {
    return ['*:read']
  }
  const out: string[] = []
  for (const d of ds) {
    const lv = levels[d.id]
    if (lv === 'read') {
      out.push(`${d.id}:read`)
    } else if (lv === 'write') {
      out.push(`${d.id}:write`)
    }
  }
  return out
}

// ---------- 新建 / 编辑 ----------
const expiryOptions = computed(() => [
  { value: 'never', label: t('apiTokens.never') },
  { value: '7', label: t('apiTokens.days', { n: 7 }) },
  { value: '30', label: t('apiTokens.days', { n: 30 }) },
  { value: '90', label: t('apiTokens.days', { n: 90 }) },
  { value: '365', label: t('apiTokens.oneYear') },
  { value: 'custom', label: t('apiTokens.customDate') },
])

const formVisible = ref(false)
const editingId = ref('')
const saving = ref(false)
const form = reactive({
  name: '',
  group: '',
  expiry: 'never',
  customDate: null as Date | null,
  enabled: true,
})

// 自定义过期时间不能早于今天
function disablePast(d: Date): boolean {
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  return d.getTime() < today.getTime()
}

function openCreate() {
  editingId.value = ''
  form.name = ''
  form.group = ''
  form.expiry = '30'
  form.customDate = null
  form.enabled = true
  // 新建时默认不授予任何权限，避免误给过大权限
  applyPreset('none')
  formVisible.value = true
}

function openEdit(tk: TokenView) {
  editingId.value = tk.id
  form.name = tk.name
  form.group = tk.group ?? ''
  form.enabled = !tk.disabled
  if (tk.expiresAt) {
    form.expiry = 'custom'
    form.customDate = new Date(tk.expiresAt)
  } else {
    form.expiry = 'never'
    form.customDate = null
  }
  scopesToLevels(tk.scopes)
  formVisible.value = true
}

// resolveExpiry 把表单的过期选择转换为 RFC3339 字符串或 null
function resolveExpiry(): string | null | undefined {
  if (form.expiry === 'never') {
    return null
  }
  if (form.expiry === 'custom') {
    if (!form.customDate) {
      return undefined
    }
    return form.customDate.toISOString()
  }
  const days = Number(form.expiry)
  return new Date(Date.now() + days * 86400 * 1000).toISOString()
}

async function submit() {
  const name = form.name.trim()
  if (!name) {
    ElMessage.warning(t('apiTokens.nameRequired'))
    return
  }
  const scopes = levelsToScopes()
  if (!scopes.length) {
    ElMessage.warning(t('apiTokens.scopeRequired'))
    return
  }
  const expiresAt = resolveExpiry()
  if (expiresAt === undefined) {
    ElMessage.warning(t('apiTokens.expiryRequired'))
    return
  }
  if (expiresAt && new Date(expiresAt).getTime() <= Date.now()) {
    ElMessage.warning(t('apiTokens.expiryFuture'))
    return
  }
  const body: TokenInput = { name, scopes, expiresAt, group: form.group.trim() }
  saving.value = true
  try {
    if (editingId.value) {
      body.disabled = !form.enabled
      await api.put<TokenView>(`/api/tokens/${encodeURIComponent(editingId.value)}`, body)
      ElMessage.success(t('common.saved'))
      formVisible.value = false
      await load()
    } else {
      const r = await api.post<TokenSecret>('/api/tokens', body)
      formVisible.value = false
      showSecret(r)
      await load()
    }
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

// ---------- 启停 / 重新生成 / 删除 ----------
async function toggle(tk: TokenView, enabled: boolean) {
  busyId.value = tk.id
  try {
    const body: TokenInput = { name: tk.name, scopes: tk.scopes, expiresAt: tk.expiresAt, disabled: !enabled }
    await api.put<TokenView>(`/api/tokens/${encodeURIComponent(tk.id)}`, body)
    ElMessage.success(enabled ? t('common.enabled') : t('apiTokens.disabledMsg'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busyId.value = ''
    await load()
  }
}

async function regenerate(tk: TokenView) {
  try {
    await ElMessageBox.confirm(t('apiTokens.regenerateConfirm', { name: tk.name }), t('apiTokens.regenerateTitle'), {
      type: 'warning',
      confirmButtonText: t('apiTokens.regenerate'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  try {
    const r = await api.post<TokenSecret>(`/api/tokens/${encodeURIComponent(tk.id)}/regenerate`)
    showSecret(r)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(tk: TokenView) {
  try {
    await ElMessageBox.confirm(t('apiTokens.deleteConfirm', { name: tk.name }), t('apiTokens.deleteTitle'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
      confirmButtonClass: 'el-button--danger',
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  try {
    await api.del(`/api/tokens/${encodeURIComponent(tk.id)}`)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// ---------- 明文展示 ----------
const secretVisible = ref(false)
const secret = ref('')
const secretName = ref('')
const secretEl = ref<HTMLElement>()
const curlEl = ref<HTMLElement>()

// curl 示例使用当前访问地址，调用 /api/status
const curlExample = computed(() => `curl -H "Authorization: Bearer ${secret.value}" ${apiOrigin}/api/status`)

function showSecret(r: TokenSecret) {
  secret.value = r.secret
  secretName.value = r.token.name
  secretVisible.value = true
}

// 关闭时清空明文，避免残留在内存中的响应式状态里
function closeSecret() {
  secretVisible.value = false
  secret.value = ''
  secretName.value = ''
}

// 复制明文：selectNode 供点击文本时全选，copyText 含降级逻辑，失败时选中对应节点
function copy(text: string, el?: HTMLElement | null) {
  copyText(text, { el })
}

onMounted(load)
</script>

<style scoped>
.head-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.exec-warn {
  margin-top: 8px;
}
.block-gap {
  margin-bottom: 16px;
}
.code {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--el-fill-color-light);
  font-size: 12px;
  word-break: break-all;
}

/* 移动端卡片 */
.legacy-tip {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--el-color-warning);
  line-height: 1.5;
}

/* 表单 */
.exp-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  width: 100%;
}
.exp-select {
  width: 160px;
}
.exp-date {
  flex: 1;
  min-width: 200px;
}
.perm {
  width: 100%;
  min-width: 0;
}
.perm-presets {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.perm-presets .el-button + .el-button {
  margin-left: 0;
}
.perm-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.perm-group {
  margin-top: 12px;
  border: 1px solid var(--g-border);
  border-radius: 8px;
  overflow: hidden;
}
.perm-group-title {
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
}
.perm-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 12px;
  border-top: 1px solid var(--g-border);
  line-height: 1.4;
}
.perm-name {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px 8px;
  min-width: 0;
  font-size: 13px;
}
.perm-id {
  font-size: 11px;
  color: var(--el-text-color-placeholder);
}
.perm-radio {
  flex-shrink: 0;
}

/* 明文对话框 */
.sec-label {
  margin: 12px 0 6px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.sec-box {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sec-text {
  flex: 1;
  min-width: 0;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  border: 1px solid var(--g-border);
  font-size: 13px;
  word-break: break-all;
  user-select: all;
}
.sec-curl-box {
  align-items: flex-start;
}
.sec-curl {
  flex: 1;
  min-width: 0;
  margin: 0;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  border: 1px solid var(--g-border);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}

@media (max-width: 767.98px) {
  .exp-select {
    width: 100%;
  }
  .perm-row {
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;
  }
  .sec-box {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
