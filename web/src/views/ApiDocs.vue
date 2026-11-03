<template>
  <div>
    <PageHeader :title="t('apiDocs.title')" :desc="t('apiDocs.desc')" :icon="BookOpen">
      <!-- 令牌管理只对管理员开放 -->
      <router-link v-if="!isReadonly" to="/api-tokens"><el-button :icon="KeyRound">{{ t('apiDocs.tokens') }}</el-button></router-link>
      <el-button :icon="Download" :disabled="!doc" @click="download">{{ t('apiDocs.downloadSpec') }}</el-button>
    </PageHeader>

    <el-alert v-if="error" type="error" :title="error" :closable="false" show-icon class="block-gap">
      <el-button size="small" @click="load">{{ t('apiDocs.retry') }}</el-button>
    </el-alert>

    <div v-loading="loading" class="docs-wrap">
      <template v-if="doc">
        <!-- 简介 / Base URL / 认证 -->
        <div class="g-card">
          <div class="g-card-body intro">
            <!-- 内容已在 renderText 中转义，v-html 仅注入受控的 code / strong / br 标签 -->
            <div class="rich" v-html="renderText(doc.info?.description ?? '')" />
            <div class="intro-row">
              <span class="intro-label">Base URL</span>
              <code class="mono code-pill" @click="selectNode($event.currentTarget as HTMLElement)">{{ origin }}</code>
              <el-button size="small" text :icon="Copy" @click="copyText(origin)">{{ t('common.copy') }}</el-button>
            </div>
            <div class="intro-row">
              <span class="intro-label">{{ t('apiDocs.auth') }}</span>
              <span class="intro-text">
                <i18n-t keypath="apiDocs.authDesc" tag="span">
                  <template #header><code class="mono code-pill">Authorization: Bearer gtl_…</code></template>
                  <template #link><router-link v-if="!isReadonly" to="/api-tokens">{{ t('apiDocs.tokensPage') }}</router-link><span v-else>{{ t('apiDocs.tokensPage') }}</span></template>
                </i18n-t>
              </span>
            </div>
          </div>
        </div>

        <!-- 移动端：顶部下拉导航 -->
        <div v-if="isMobile" class="m-nav">
          <el-input v-model="keyword" :prefix-icon="Search" :placeholder="t('apiDocs.searchPlaceholder')" clearable />
          <el-select :model-value="activeTag" :placeholder="t('apiDocs.jumpToGroup')" @change="goTag">
            <el-option v-for="g in visibleGroups" :key="g.tag" :label="t('apiDocs.groupLabel', { tag: g.tag, count: g.ops.length })" :value="g.tag" />
          </el-select>
        </div>

        <div class="docs-body">
          <!-- 桌面端：左侧分组导航 -->
          <aside v-if="!isMobile" class="docs-nav">
            <el-input v-model="keyword" :prefix-icon="Search" :placeholder="t('apiDocs.searchPlaceholder')" clearable />
            <div class="nav-list">
              <button v-for="g in visibleGroups" :key="g.tag" type="button" class="nav-item" :class="{ active: activeTag === g.tag }" @click="goTag(g.tag)">
                <span class="nav-text">{{ g.tag }}</span>
                <span class="nav-count">{{ g.ops.length }}</span>
              </button>
              <div v-if="!visibleGroups.length" class="nav-empty">{{ t('apiDocs.noMatch') }}</div>
            </div>
          </aside>

          <!-- 接口列表 -->
          <div class="docs-main">
            <el-empty v-if="!visibleGroups.length" :description="t('apiDocs.noMatchEmpty')" :image-size="60" />
            <section v-for="g in visibleGroups" :id="anchorId(g.tag)" :key="g.tag" class="tag-section">
              <h3 class="tag-title">{{ g.tag }}<span class="tag-count">{{ g.ops.length }}</span></h3>
              <div v-for="op in g.ops" :key="op.key" class="op" :class="{ open: isOpen(op) }">
                <div class="op-head" role="button" tabindex="0" @click="toggleOpen(op)" @keydown.enter="toggleOpen(op)" @keydown.space.prevent="toggleOpen(op)">
                  <span class="method" :class="`m-${op.method}`">{{ op.method.toUpperCase() }}</span>
                  <span class="op-path mono"><template v-for="(seg, i) in splitPath(op.path)" :key="i"><span v-if="seg.param" class="p-param">{{ seg.text }}</span><template v-else>{{ seg.text }}</template></template></span>
                  <span class="op-summary">{{ op.summary }}</span>
                  <span class="op-scope">
                    <el-tag v-if="op.scope === '!denied'" size="small" type="warning" effect="light">{{ t('apiDocs.sessionOnly') }}</el-tag>
                    <el-tag v-else-if="op.scope === ''" size="small" type="info" effect="plain">{{ t('apiDocs.anyToken') }}</el-tag>
                    <el-tag v-else size="small" type="primary" effect="light" class="mono">{{ op.scope }}</el-tag>
                  </span>
                  <ChevronDown :size="16" class="op-arrow" />
                </div>

                <div v-if="isOpen(op)" class="op-detail">
                  <div v-if="op.description" class="rich op-desc" v-html="renderText(op.description)" />

                  <div v-if="op.params.length" class="sub">
                    <div class="sub-title">{{ t('apiDocs.params') }}</div>
                    <div class="tbl-wrap">
                      <table class="doc-table">
                        <thead><tr><th>{{ t('common.name') }}</th><th>{{ t('apiDocs.location') }}</th><th>{{ t('apiDocs.type') }}</th><th>{{ t('apiDocs.required') }}</th><th>{{ t('apiDocs.description') }}</th></tr></thead>
                        <tbody>
                          <tr v-for="p in op.params" :key="p.in + p.name">
                            <td class="mono">{{ p.name }}</td>
                            <td>{{ p.in === 'path' ? t('apiDocs.inPath') : p.in === 'query' ? t('apiDocs.inQuery') : p.in }}</td>
                            <td class="mono">{{ typeLabel(p.schema) }}</td>
                            <td>{{ p.required ? t('common.yes') : t('common.no') }}</td>
                            <td>{{ p.description ?? '' }}</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>

                  <div v-if="op.bodySchema" class="sub">
                    <div class="sub-title">{{ t('apiDocs.requestBody') }} <span class="muted">application/json · {{ typeLabel(op.bodySchema) }}</span></div>
                    <div v-if="op.fields.length" class="tbl-wrap">
                      <table class="doc-table">
                        <thead><tr><th>{{ t('apiDocs.field') }}</th><th>{{ t('apiDocs.type') }}</th><th>{{ t('apiDocs.required') }}</th><th>{{ t('apiDocs.description') }}</th></tr></thead>
                        <tbody>
                          <tr v-for="f in op.fields" :key="f.name">
                            <td class="mono"><span :style="{ paddingLeft: f.depth * 14 + 'px' }">{{ f.name }}</span></td>
                            <td class="mono">{{ f.type }}</td>
                            <td>{{ f.required ? t('common.yes') : '' }}</td>
                            <td>{{ f.desc }}</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                    <div v-else class="muted note">{{ t('apiDocs.bodyNoFields') }}</div>
                  </div>

                  <div class="sub">
                    <div class="sub-title">
                      {{ t('apiDocs.example') }}
                      <el-button v-if="op.tokenAllowed" size="small" text :icon="Copy" @click="copyText(curlOf(op))">{{ t('common.copy') }}</el-button>
                    </div>
                    <pre v-if="op.tokenAllowed" class="curl mono">{{ curlOf(op) }}</pre>
                    <el-alert v-else type="warning" :closable="false" show-icon :title="t('apiDocs.sessionOnlyWarn')" />
                  </div>
                </div>
              </div>
            </section>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { BookOpen, ChevronDown, Copy, Download, KeyRound, Search } from 'lucide-vue-next'
import { api } from '../api'
import { isReadonly } from '../nav'
import { apiOrigin } from '../basePath'
import PageHeader from '../components/PageHeader.vue'
import { useIsMobile } from '../composables/useIsMobile'
import { copyText, selectNode } from '../composables/useClipboard'

// ---------- OpenAPI 文档的最小类型 ----------
interface Schema {
  $ref?: string
  type?: string
  format?: string
  description?: string
  properties?: Record<string, Schema>
  required?: string[]
  items?: Schema
  additionalProperties?: Schema | boolean
  enum?: unknown[]
}
interface Param {
  name: string
  in: string
  required?: boolean
  description?: string
  schema?: Schema
}
interface Operation {
  tags?: string[]
  summary?: string
  description?: string
  parameters?: Param[]
  requestBody?: { content?: Record<string, { schema?: Schema }> }
  'x-required-scope'?: string
  'x-token-allowed'?: boolean
}
interface OpenAPIDoc {
  info?: { title?: string; description?: string; version?: string }
  tags?: { name: string }[]
  paths: Record<string, Record<string, Operation>>
  components?: { schemas?: Record<string, Schema> }
}

// 字段表的一行
interface FieldRow {
  name: string
  type: string
  required: boolean
  desc: string
  depth: number
}
// 展开后的接口
interface Op {
  key: string
  method: string
  path: string
  tag: string
  summary: string
  description: string
  scope: string
  tokenAllowed: boolean
  params: Param[]
  bodySchema?: Schema
  fields: FieldRow[]
}
interface Group {
  tag: string
  ops: Op[]
}

const { t } = useI18n()
const isMobile = useIsMobile()
// 经统一网关访问时接口地址带路径前缀
const origin = apiOrigin
const doc = ref<OpenAPIDoc>()
const loading = ref(false)
const error = ref('')
const keyword = ref('')
const activeTag = ref('')
// 已展开的接口 key 集合
const openKeys = ref<Set<string>>(new Set())

// ---------- 文本渲染 ----------

// escapeHTML 转义 HTML 特殊字符，防止文档内容注入标签
function escapeHTML(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;')
}

// renderText 轻量渲染：先整体转义，再把 `行内代码`、**加粗** 与换行转换为受控标签
function renderText(s: string): string {
  return escapeHTML(s)
    .replace(/`([^`\n]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
    .replace(/\n\n+/g, '<br><br>')
    .replace(/\n/g, '<br>')
}

// splitPath 把路径按 {参数} 切段，便于高亮参数
function splitPath(p: string): { text: string; param: boolean }[] {
  return p
    .split(/(\{[^}]+\})/)
    .filter((seg) => seg !== '')
    .map((seg) => ({ text: seg, param: seg.startsWith('{') && seg.endsWith('}') }))
}

function anchorId(tag: string): string {
  return 'tag-' + tag
}

// ---------- Schema 处理 ----------

// resolve 解析 $ref（仅支持 #/components/schemas/ 引用），最多跟随 8 层避免循环
function resolve(s?: Schema): Schema {
  let cur: Schema = s ?? {}
  for (let i = 0; i < 8 && cur.$ref; i++) {
    const name = cur.$ref.replace('#/components/schemas/', '')
    const next = doc.value?.components?.schemas?.[name]
    if (!next) {
      return {}
    }
    cur = next
  }
  return cur
}

function refName(s?: Schema): string {
  return s?.$ref ? s.$ref.replace('#/components/schemas/', '') : ''
}

// typeLabel 生成类型文本：引用显示名称，数组显示 array<T>，映射显示 map<string, T>
function typeLabel(s?: Schema): string {
  if (!s) {
    return 'any'
  }
  if (s.$ref) {
    const r = resolve(s)
    // 引用到数组/映射时展开成其结构，其余显示引用名
    if (r.type === 'array' || (r.type === 'object' && r.additionalProperties && !r.properties)) {
      return typeLabel(r)
    }
    return refName(s)
  }
  if (s.type === 'array') {
    return `array<${typeLabel(s.items)}>`
  }
  if (s.type === 'object' && s.additionalProperties && typeof s.additionalProperties === 'object') {
    return `map<string, ${typeLabel(s.additionalProperties)}>`
  }
  if (s.type === 'object' && !s.properties) {
    return 'object'
  }
  if (s.type) {
    return s.format ? `${s.type}(${s.format})` : s.type
  }
  return s.properties ? 'object' : 'any'
}

// childObject 返回字段可继续展开的对象 schema 以及名称后缀（数组 []，映射 .*）
function childObject(p: Schema): { schema: Schema; suffix: string } | undefined {
  const r = resolve(p)
  if (r.properties) {
    return { schema: r, suffix: '' }
  }
  if (r.type === 'array' && r.items) {
    const it = resolve(r.items)
    if (it.properties) {
      return { schema: it, suffix: '[]' }
    }
  }
  if (r.additionalProperties && typeof r.additionalProperties === 'object') {
    const v = resolve(r.additionalProperties)
    if (v.properties) {
      return { schema: v, suffix: '.*' }
    }
  }
  return undefined
}

// fieldRows 展开对象字段，最多展开 2 层（depth 0 与 1）
function fieldRows(obj: Schema, depth: number, prefix: string): FieldRow[] {
  const rows: FieldRow[] = []
  const props = obj.properties ?? {}
  for (const name of Object.keys(props).sort()) {
    const p = props[name]
    rows.push({
      name: prefix + name,
      type: typeLabel(p),
      required: obj.required?.includes(name) ?? false,
      desc: p.description ?? resolve(p).description ?? '',
      depth,
    })
    if (depth < 1) {
      const child = childObject(p)
      if (child) {
        rows.push(...fieldRows(child.schema, depth + 1, `${prefix}${name}${child.suffix}.`))
      }
    }
  }
  return rows
}

// bodyFields 计算请求体字段表；顶层为数组时展开其元素
function bodyFields(s?: Schema): FieldRow[] {
  if (!s) {
    return []
  }
  const r = resolve(s)
  if (r.properties) {
    return fieldRows(r, 0, '')
  }
  if (r.type === 'array' && r.items) {
    const it = resolve(r.items)
    if (it.properties) {
      return fieldRows(it, 0, '[].')
    }
  }
  return []
}

// example 按 schema 生成占位 JSON：字符串 "string"、数字 0、布尔 false、数组一个元素，嵌套对象最多 2 层
function example(s: Schema | undefined, depth: number): unknown {
  const r = resolve(s)
  if (r.properties) {
    if (depth >= 2) {
      return {}
    }
    const out: Record<string, unknown> = {}
    for (const k of Object.keys(r.properties)) {
      out[k] = example(r.properties[k], depth + 1)
    }
    return out
  }
  switch (r.type) {
    case 'string':
      return r.enum && r.enum.length ? r.enum[0] : 'string'
    case 'integer':
    case 'number':
      return 0
    case 'boolean':
      return false
    case 'array':
      // 数组深度也受限，避免无限嵌套
      return depth >= 3 ? [] : [example(r.items, depth + 1)]
    case 'object':
      if (r.additionalProperties && typeof r.additionalProperties === 'object' && depth < 2) {
        return { key: example(r.additionalProperties, depth + 1) }
      }
      return {}
    default:
      return null
  }
}

// ---------- 接口数据整理 ----------
const methodOrder = ['get', 'post', 'put', 'patch', 'delete']

const groups = computed<Group[]>(() => {
  const d = doc.value
  if (!d) {
    return []
  }
  const byTag = new Map<string, Op[]>()
  // tags 顺序即展示顺序
  for (const tg of d.tags ?? []) {
    byTag.set(tg.name, [])
  }
  for (const path of Object.keys(d.paths ?? {}).sort()) {
    const item = d.paths[path]
    const methods = Object.keys(item)
      .filter((m) => methodOrder.includes(m))
      .sort((a, b) => methodOrder.indexOf(a) - methodOrder.indexOf(b))
    for (const m of methods) {
      const o = item[m]
      const tag = o.tags?.[0] ?? t('apiDocs.other')
      const bodySchema = o.requestBody?.content?.['application/json']?.schema
      const op: Op = {
        key: `${m} ${path}`,
        method: m,
        path,
        tag,
        summary: o.summary ?? '',
        description: o.description ?? '',
        scope: o['x-required-scope'] ?? '',
        // 缺省视为允许令牌；!denied 一律视为仅会话
        tokenAllowed: o['x-token-allowed'] !== false && o['x-required-scope'] !== '!denied',
        params: o.parameters ?? [],
        bodySchema,
        fields: bodyFields(bodySchema),
      }
      if (!byTag.has(tag)) {
        byTag.set(tag, [])
      }
      byTag.get(tag)!.push(op)
    }
  }
  return [...byTag.entries()].filter(([, ops]) => ops.length > 0).map(([tag, ops]) => ({ tag, ops }))
})

// 按关键字过滤路径 / 摘要 / 分组名
const visibleGroups = computed<Group[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) {
    return groups.value
  }
  return groups.value
    .map((g) => ({
      tag: g.tag,
      ops: g.ops.filter((o) => o.path.toLowerCase().includes(kw) || o.summary.toLowerCase().includes(kw) || g.tag.toLowerCase().includes(kw)),
    }))
    .filter((g) => g.ops.length > 0)
})

// ---------- curl 示例 ----------
function curlOf(op: Op): string {
  const lines: string[] = []
  const method = op.method.toUpperCase()
  lines.push(`curl${method === 'GET' ? '' : ' -X ' + method} '${origin}${op.path}'`)
  lines.push(`  -H 'Authorization: Bearer gtl_xxxx'`)
  if (op.bodySchema) {
    lines.push(`  -H 'Content-Type: application/json'`)
    lines.push(`  -d '${JSON.stringify(example(op.bodySchema, 0), null, 2)}'`)
  }
  return lines.join(' \\\n')
}

// ---------- 交互 ----------
function isOpen(op: Op): boolean {
  return openKeys.value.has(op.key)
}

function toggleOpen(op: Op) {
  const next = new Set(openKeys.value)
  if (next.has(op.key)) {
    next.delete(op.key)
  } else {
    next.add(op.key)
  }
  openKeys.value = next
}

// goTag 滚动到分组位置
function goTag(tag: string) {
  activeTag.value = tag
  document.getElementById(anchorId(tag))?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// download 把文档以 Blob 形式保存为 openapi.json
function download() {
  if (!doc.value) {
    return
  }
  const blob = new Blob([JSON.stringify(doc.value, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'openapi.json'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    doc.value = await api.get<OpenAPIDoc>('/api/openapi.json')
    activeTag.value = groups.value[0]?.tag ?? ''
  } catch (e) {
    error.value = (e as Error).message
    ElMessage.error(error.value)
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.block-gap {
  margin-bottom: 16px;
}
.docs-wrap {
  min-height: 120px;
}
.intro {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.intro-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 13px;
}
.intro-label {
  width: 64px;
  flex-shrink: 0;
  color: var(--el-text-color-secondary);
}
.intro-text {
  min-width: 0;
  line-height: 1.8;
}
.code-pill,
.rich :deep(code) {
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--el-fill-color-light);
  font-family: var(--g-mono);
  font-size: 12px;
  word-break: break-all;
}
.code-pill {
  cursor: pointer;
}
.rich {
  font-size: 13px;
  line-height: 1.8;
  color: var(--el-text-color-regular);
  word-break: break-word;
}

.m-nav {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 16px;
}
.docs-body {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  margin-top: 16px;
}
.docs-nav {
  /* 吸在顶栏与页面头部下方，原先 top: 0 会被顶栏挡住 */
  position: sticky;
  top: calc(var(--g-sticky-top) + 12px);
  width: 220px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: calc(100vh - var(--g-sticky-top) - 24px);
}
.nav-list {
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px;
  background: var(--el-bg-color);
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius);
}
.nav-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  padding: 7px 10px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--el-text-color-regular);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.nav-item:hover {
  background: var(--el-fill-color-light);
}
.nav-item.active {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  font-weight: 600;
}
.nav-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.nav-count {
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
.nav-empty {
  padding: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  text-align: center;
}
.docs-main {
  flex: 1;
  min-width: 0;
}
.tag-section {
  scroll-margin-top: 12px;
}
.tag-section + .tag-section {
  margin-top: 20px;
}
.tag-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 8px;
  font-size: 15px;
  font-weight: 600;
}
.tag-count {
  padding: 0 7px;
  border-radius: 999px;
  background: var(--el-fill-color);
  font-size: 11px;
  font-weight: 400;
  color: var(--el-text-color-secondary);
  line-height: 18px;
}

/* 单个接口 */
.op {
  background: var(--el-bg-color);
  border: 1px solid var(--g-border);
  border-radius: var(--g-radius-sm);
  overflow: hidden;
}
.op + .op {
  margin-top: 8px;
}
.op-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  cursor: pointer;
  user-select: none;
}
.op-head:hover {
  background: var(--el-fill-color-lighter);
}
.op-head:focus-visible {
  outline: 2px solid var(--el-color-primary-light-5);
  outline-offset: -2px;
}
.method {
  --c: var(--el-color-info);
  flex-shrink: 0;
  min-width: 56px;
  padding: 2px 0;
  border-radius: 4px;
  text-align: center;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.3px;
  color: var(--c);
  background: color-mix(in srgb, var(--c) 14%, transparent);
}
.m-get {
  --c: var(--el-color-success);
}
.m-post {
  --c: var(--el-color-primary);
}
.m-put {
  --c: var(--el-color-warning);
}
.m-delete {
  --c: var(--el-color-danger);
}
.op-path {
  flex-shrink: 0;
  font-size: 13px;
  word-break: break-all;
}
.p-param {
  color: var(--el-color-warning);
  font-weight: 600;
}
.op-summary {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.op-scope {
  flex-shrink: 0;
}
.op-arrow {
  flex-shrink: 0;
  color: var(--el-text-color-secondary);
  transition: transform 0.15s;
}
.op.open .op-arrow {
  transform: rotate(180deg);
}
.op-detail {
  padding: 4px 14px 14px;
  border-top: 1px solid var(--g-border);
}
.op-desc {
  margin-top: 10px;
}
.sub {
  margin-top: 14px;
}
.sub-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  font-size: 13px;
  font-weight: 600;
}
.sub-title .muted {
  font-size: 12px;
  font-weight: 400;
}
.note {
  font-size: 12px;
}
.tbl-wrap {
  overflow-x: auto;
  border: 1px solid var(--g-border);
  border-radius: 6px;
}
.doc-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}
.doc-table th,
.doc-table td {
  padding: 6px 10px;
  text-align: left;
  vertical-align: top;
  border-top: 1px solid var(--g-border);
  white-space: nowrap;
}
.doc-table td:last-child {
  white-space: normal;
  min-width: 120px;
}
.doc-table thead th {
  border-top: 0;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-secondary);
  font-weight: 500;
}
.curl {
  margin: 0;
  padding: 10px 12px;
  border-radius: 6px;
  background: var(--el-fill-color-light);
  border: 1px solid var(--g-border);
  font-size: 12px;
  line-height: 1.6;
  overflow-x: auto;
  white-space: pre;
}

@media (max-width: 767.98px) {
  .docs-body {
    margin-top: 12px;
  }
  .op-head {
    flex-wrap: wrap;
    gap: 6px 8px;
  }
  .op-path {
    flex: 1;
    min-width: 0;
  }
  .op-summary {
    order: 5;
    flex-basis: 100%;
    white-space: normal;
  }
  .op-arrow {
    order: 4;
  }
  .op-scope {
    order: 6;
  }
  .intro-label {
    width: 100%;
  }
  .tag-section {
    scroll-margin-top: 8px;
  }
}
</style>
