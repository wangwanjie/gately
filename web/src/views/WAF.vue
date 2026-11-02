<template>
  <div>
    <PageHeader :title="t('waf.title')" :desc="t('waf.desc')" :icon="ShieldHalf">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('groupSort.search')" />
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('waf.addInstance') }}</el-button>
    </PageHeader>

    <GroupedList :gl="gl" :compact="compact" :empty-text="t('waf.empty')">
      <template #item="{ it, item: w, sorting, bind }">
        <EntityCard v-bind="bind" :off="!w.enable" :compact="compact || sorting">
          <template #lead>
            <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
            <el-switch v-else :model-value="w.enable" @change="(v: string | number | boolean) => toggle(w, Boolean(v))" />
          </template>
          <template #title>
            <span>{{ w.name }}</span>
            <el-tag :type="w.mode === 'block' ? 'danger' : 'warning'" size="small" effect="plain">{{ w.mode === 'block' ? t('waf.modeBlockShort') : t('waf.modeDetectShort') }}</el-tag>
          </template>
          <template #sub><span class="mono">{{ w.key }}</span><span v-if="w.remark"> · {{ w.remark }}</span></template>
          <template #status>
            <StatusBadge v-bind="instStatus(w)" />
          </template>
          <Field :label="t('waf.ruleSets')" wide>
            <div v-for="(r, i) in w.ruleSets ?? []" :key="i" class="ruleset-line" :class="{ off: !r.enable }">
              <span class="dot" />
              {{ r.type === 'crs' ? 'OWASP CRS' : t('waf.custom') }}{{ r.remark ? t('waf.remarkSuffix', { remark: r.remark }) : '' }}
              <span v-if="r.type === 'crs'" class="meta">{{ r.param ? t('waf.ruleFiles', { count: r.param.split('\n').filter(Boolean).length }) : t('waf.allRules') }}</span>
            </div>
            <span v-if="!(w.ruleSets ?? []).length" class="muted">{{ t('common.none') }}</span>
          </Field>
          <Field :label="t('waf.safeline')">
            <span v-if="w.safeline?.enable" class="mono">{{ w.safeline.addr }}</span>
            <span v-else class="muted">{{ t('waf.safelineOff') }}</span>
          </Field>
          <Field :label="t('waf.thresholdParanoia')">{{ t('waf.thresholdValue', { inbound: w.inboundThreshold, outbound: w.outboundThreshold, level: w.paranoia }) }}</Field>
          <Field :label="t('waf.bodyCheck')">{{ bodySummary(w) }}</Field>
          <Field :label="t('waf.blocked')"><span :class="{ 'text-danger': w.blocked > 0 }">{{ w.blocked }}</span></Field>
          <Field :label="t('waf.matched')">{{ w.matched }}</Field>
          <template v-if="!sorting && w.enable && (w.error || (w.safeline?.enable && w.safelineError))" #extra>
            <div v-if="w.error" class="err-line">{{ w.error }}</div>
            <div v-else class="err-line">{{ t('waf.safelineErrorLine', { error: w.safelineError, policy: w.safeline.failClosed ? t('waf.safelineFailClosed') : t('waf.safelineFailOpen') }) }}</div>
          </template>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="Pencil" @click="openEditor(w)">{{ t('common.edit') }}</el-button>
            <el-button text size="small" :icon="CopyPlus" @click="openEditor(w, true)">{{ t('common.duplicate') }}</el-button>
            <el-popconfirm :title="t('waf.deleteConfirm')" width="240" @confirm="remove(w)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <LogCard url="/api/waf/logs" :title="t('waf.logTitle')" clearable />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('waf.editInstance') : t('waf.addInstance')" width="760px" top="5vh" :fullscreen="isMobile">
      <CopyHint :from="copyFrom" />
      <el-form :model="editing" label-width="120px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('waf.mode')">
          <el-radio-group v-model="editing.mode">
            <el-radio-button value="block">{{ t('waf.modeBlock') }}</el-radio-button>
            <el-radio-button value="detect">{{ t('waf.modeDetect') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="t('waf.paranoia')">
          <el-slider v-model="editing.paranoia" :min="1" :max="4" show-stops class="slider" />
          <span class="help">{{ t('waf.paranoiaHelp') }}</span>
        </el-form-item>
        <el-form-item :label="t('waf.inbound')"><el-input-number v-model="editing.inboundThreshold" :min="1" :max="100" /></el-form-item>
        <el-form-item :label="t('waf.outbound')"><el-input-number v-model="editing.outboundThreshold" :min="1" :max="100" /></el-form-item>
        <el-divider content-position="left">{{ t('waf.bodyCheck') }}</el-divider>
        <el-form-item :label="t('waf.bodyLimit')">
          <el-input-number v-model="editing.bodyLimitMB" :min="0" :max="1024" />
          <span class="unit">MB</span>
          <span class="help">{{ t('waf.bodyLimitHelp') }}</span>
        </el-form-item>
        <el-form-item :label="t('waf.bodyLimitAction')">
          <el-radio-group v-model="editing.bodyLimitAction">
            <el-radio-button value="reject">{{ t('waf.bodyLimitReject') }}</el-radio-button>
            <el-radio-button value="partial">{{ t('waf.bodyLimitPartial') }}</el-radio-button>
          </el-radio-group>
          <div class="help block">{{ editing.bodyLimitAction === 'partial' ? t('waf.bodyLimitPartialHelp') : t('waf.bodyLimitRejectHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('waf.webdav')">
          <el-switch v-model="editing.webdav" />
          <div class="help block">{{ t('waf.webdavHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('waf.skipBodyPaths')">
          <el-input v-model="editing.skipBodyPaths" type="textarea" :rows="3" class="mono" :placeholder="'/api/upload\n/remote.php/dav/'" />
          <div class="help block">{{ t('waf.skipBodyPathsHelp') }}</div>
        </el-form-item>
        <el-divider content-position="left">{{ t('waf.safelineSection') }}</el-divider>
        <el-form-item :label="t('waf.safelineEnable')">
          <el-switch v-model="editing.safeline.enable" />
          <div class="help block">{{ t('waf.safelineHelp') }}</div>
        </el-form-item>
        <template v-if="editing.safeline.enable">
          <el-form-item :label="t('waf.safelineAddr')">
            <div class="sl-addr">
              <el-input v-model="editing.safeline.addr" class="mono" placeholder="172.22.222.5:8000" />
              <el-button :loading="testing" @click="testSafeLine">{{ t('waf.safelineTest') }}</el-button>
            </div>
            <div class="help block">{{ t('waf.safelineAddrHelp') }}</div>
          </el-form-item>
          <el-form-item :label="t('waf.safelineTimeout')">
            <el-input-number v-model="editing.safeline.timeoutMs" :min="10" :max="30000" :step="100" />
            <span class="unit">ms</span>
          </el-form-item>
          <el-form-item :label="t('waf.safelineBodyLimit')">
            <el-input-number v-model="editing.safeline.bodyLimitKB" :min="0" :max="65536" :step="256" />
            <span class="unit">KB</span>
            <span class="help">{{ t('waf.safelineBodyLimitHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('waf.safelineOnError')">
            <el-radio-group v-model="editing.safeline.failClosed">
              <el-radio-button :value="false">{{ t('waf.safelineFailOpen') }}</el-radio-button>
              <el-radio-button :value="true">{{ t('waf.safelineFailClosed') }}</el-radio-button>
            </el-radio-group>
            <div class="help block">{{ editing.safeline.failClosed ? t('waf.safelineFailClosedHelp') : t('waf.safelineFailOpenHelp') }}</div>
          </el-form-item>
        </template>
        <el-divider content-position="left">{{ t('waf.ruleSets') }}</el-divider>
        <div v-if="editing.safeline.enable" class="help block rs-note">{{ t('waf.ruleSetsWithSafeline') }}</div>
        <div v-for="(r, i) in editing.ruleSets" :key="i" class="ruleset">
          <div class="ruleset-head">
            <el-switch v-model="r.enable" size="small" />
            <el-radio-group v-model="r.type" size="small">
              <el-radio-button value="crs">OWASP CRS</el-radio-button>
              <el-radio-button value="custom">{{ t('waf.customSecLang') }}</el-radio-button>
            </el-radio-group>
            <el-input v-model="r.remark" size="small" :placeholder="t('common.remark')" class="rs-remark" />
            <span class="rs-ops">
              <el-button text size="small" :icon="ChevronUp" :disabled="i === 0" :aria-label="t('waf.moveUp')" :title="t('waf.moveUp')" @click="moveRuleSet(i, -1)" />
              <el-button text size="small" :icon="ChevronDown" :disabled="i === editing.ruleSets.length - 1" :aria-label="t('waf.moveDown')" :title="t('waf.moveDown')" @click="moveRuleSet(i, 1)" />
              <el-button text type="danger" size="small" :icon="Trash2" :aria-label="t('waf.deleteRuleSet')" @click="editing.ruleSets.splice(i, 1)" />
            </span>
          </div>
          <el-select
            v-if="r.type === 'crs'"
            :model-value="crsSelected(r)"
            multiple
            filterable
            collapse-tags
            collapse-tags-tooltip
            :placeholder="t('waf.crsPlaceholder')"
            style="width: 100%"
            @update:model-value="(v: string[]) => (r.param = v.join('\n'))"
          >
            <el-option v-for="f in crsFiles" :key="f" :label="f" :value="f" />
          </el-select>
          <el-input v-else v-model="r.param" type="textarea" :rows="4" class="mono" placeholder='SecRule REQUEST_URI "@beginsWith /admin" "id:10001,phase:1,deny,status:403"' />
          <div v-if="r.type === 'custom'" class="help block">{{ t('waf.customHelp') }}</div>
        </div>
        <el-button size="small" :icon="Plus" @click="editing.ruleSets.push({ enable: true, remark: '', type: 'crs', param: '' })">{{ t('waf.addRuleSet') }}</el-button>
        <el-form-item :label="t('common.remark')" class="remark"><el-input v-model="editing.remark" /></el-form-item>
        <el-form-item :label="t('common.enable')"><el-switch v-model="editing.enable" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { ChevronDown, ChevronUp, CopyPlus, Pencil, Plus, ShieldHalf, Trash2 } from 'lucide-vue-next'
import { api } from '../api'
import PageHeader from '../components/PageHeader.vue'
import CompactToggle from '../components/CompactToggle.vue'
import EntityCard from '../components/EntityCard.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import { useGroupedList } from '../composables/useGroupSort'
import { useCompactView } from '../composables/useCompactView'
import Field from '../components/Field.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import CopyHint from '../components/CopyHint.vue'
import { createItem, uniqueName, type CopySource } from '../copyDraft'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'

interface RuleSet {
  enable: boolean
  remark: string
  type: string
  param: string
}

interface Instance {
  key?: string
  name: string
  enable: boolean
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  mode: string
  paranoia: number
  inboundThreshold: number
  outboundThreshold: number
  ruleSets: RuleSet[]
  bodyLimitMB: number
  bodyLimitAction: string
  skipBodyPaths: string
  webdav: boolean
  safeline: SafeLine
  remark: string
}

// SafeLine 是接入雷池检测引擎的设置
interface SafeLine {
  enable: boolean
  addr: string
  timeoutMs: number
  bodyLimitKB: number
  failClosed: boolean
}

interface InstanceView extends Instance {
  key: string
  error?: string
  blocked: number
  matched: number
  // safelineError 为最近一次雷池检测失败的原因，正常时为空
  safelineError?: string
  safelineErrors: number
}

const { t } = useI18n()
const compact = useCompactView('waf')
const { isMobile, labelPosition } = useFormLayout()
const instances = ref<InstanceView[]>([])
// 列表的搜索、分组与排序；顺序与分组的修改不会重新加载规则
const gl = useGroupedList<InstanceView>({
  storageKey: 'waf.groups',
  items: () => instances.value,
  base: '/api/waf/instances',
  reload: load,
  matches: (w, q) => [w.name, w.group ?? '', w.remark, w.key].join('\n').toLowerCase().includes(q),
})
const crsFiles = ref<string[]>([])
const editorVisible = ref(false)
const saving = ref(false)
const testing = ref(false)
const editing = ref<Instance>(empty())
// 复制条目时的模板，普通新增与编辑时为 null
const copyFrom = ref<CopySource | null>(null)
// 定时刷新，标签页在后台时暂停
const polling = usePolling(load, 5000)

function empty(): Instance {
  return { name: '', enable: true, mode: 'block', paranoia: 1, inboundThreshold: 5, outboundThreshold: 4, ruleSets: [{ enable: true, remark: t('waf.defaultRuleSetRemark'), type: 'crs', param: '' }], bodyLimitMB: 0, bodyLimitAction: 'reject', skipBodyPaths: '', webdav: false, safeline: emptySafeLine(), remark: '' }
}

function emptySafeLine(): SafeLine {
  return { enable: false, addr: '', timeoutMs: 1000, bodyLimitKB: 1024, failClosed: false }
}

// bodySummary 概括请求体检查设置：上限、超限处理方式与跳过的路径数
function bodySummary(w: InstanceView) {
  const limit = w.bodyLimitMB ? `${w.bodyLimitMB} MB` : t('waf.bodyLimitDefault')
  const action = w.bodyLimitAction === 'partial' ? t('waf.bodyLimitPartial') : t('waf.bodyLimitReject')
  const skipped = (w.skipBodyPaths ?? '').split('\n').filter(Boolean).length
  return t('waf.bodySummary', { limit, action }) + (skipped ? t('waf.bodySummarySkip', { count: skipped }) : '') + (w.webdav ? t('waf.bodySummaryWebdav') : '')
}

// instStatus 计算实例展示状态：停用的实例只显示“已停用”
function instStatus(w: InstanceView): { type: StatusType; text: string; title?: string } {
  if (!w.enable) {
    return { type: 'off', text: t('waf.statusOff') }
  }
  if (w.error) {
    return { type: 'error', text: t('waf.statusCompileFailed'), title: w.error }
  }
  if (w.safeline?.enable && w.safelineError) {
    return { type: 'warn', text: t('waf.statusSafelineDown'), title: w.safelineError }
  }
  return { type: 'running', text: w.mode === 'block' ? t('waf.statusProtecting') : t('waf.statusDetecting') }
}

function crsSelected(r: RuleSet) {
  return r.param.split('\n').map((s) => s.trim()).filter(Boolean)
}

// 规则组按列表顺序加载，同一阶段内排在前面的先执行，交换相邻两项即可调整顺序
function moveRuleSet(i: number, d: number) {
  const list = editing.value.ruleSets
  const j = i + d
  if (j < 0 || j >= list.length) {
    return
  }
  ;[list[i], list[j]] = [list[j], list[i]]
}

async function load() {
  instances.value = await api.get('/api/waf/instances')
}

// openEditor 打开编辑框：copy 为 true 时以 row 为模板新建副本
function openEditor(row?: InstanceView, copy = false) {
  if (row) {
    const { error: _e, blocked: _b, matched: _m, safelineError: _se, safelineErrors: _sn, ...inst } = row
    editing.value = JSON.parse(JSON.stringify(inst))
    editing.value.ruleSets = editing.value.ruleSets ?? []
    editing.value.bodyLimitAction = editing.value.bodyLimitAction || 'reject'
    editing.value.skipBodyPaths = editing.value.skipBodyPaths ?? ''
    editing.value.bodyLimitMB = editing.value.bodyLimitMB ?? 0
    editing.value.safeline = { ...emptySafeLine(), ...(editing.value.safeline ?? {}) }
  } else {
    editing.value = empty()
  }
  copyFrom.value = null
  if (row && copy) {
    copyFrom.value = { key: row.key ?? '', name: row.name }
    editing.value.key = ''
    editing.value.name = uniqueName(row.name, instances.value.map((x) => x.name))
  }
  editorVisible.value = true
}

async function save() {
  saving.value = true
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/waf/instances/${key}`, body)
    } else {
      await createItem('/api/waf/instances', body, copyFrom.value)
    }
    editorVisible.value = false
    ElMessage.success(t('waf.savedApplied'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

// testSafeLine 测试雷池检测引擎：确认连接，并送检一个示例攻击确认检测引擎能识别
async function testSafeLine() {
  testing.value = true
  try {
    const r = await api.post<{ latencyMs: number; blocked: boolean; eventId?: string }>('/api/waf/safeline/test', editing.value.safeline)
    if (r.blocked) {
      ElMessage.success(t('waf.safelineTestOk', { ms: r.latencyMs.toFixed(1), id: r.eventId || '-' }))
    } else {
      ElMessage.warning(t('waf.safelineTestNotBlocked', { ms: r.latencyMs.toFixed(1) }))
    }
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    testing.value = false
  }
}

async function toggle(row: InstanceView, enable: boolean) {
  try {
    await api.put(`/api/waf/instances/${row.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(row: InstanceView) {
  try {
    await api.del(`/api/waf/instances/${row.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(async () => {
  crsFiles.value = await api.get('/api/waf/crs')
  await load()
  polling.start()
})
</script>

<style scoped>
.ruleset-line {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.ruleset-line .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--el-color-success);
}
.ruleset-line.off {
  color: var(--el-text-color-secondary);
}
.ruleset-line.off .dot {
  background: var(--el-text-color-placeholder);
}
.err-line {
  font-size: 12px;
  color: var(--el-color-danger);
  word-break: break-all;
}
.slider {
  width: 240px;
  margin-right: 8px;
}
.ruleset {
  border: 1px solid var(--g-border);
  border-radius: 8px;
  padding: 10px;
  margin-bottom: 10px;
}
.ruleset-head {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 10px;
  align-items: center;
  margin-bottom: 8px;
}
.rs-remark {
  width: 200px;
}
.rs-ops {
  display: inline-flex;
  align-items: center;
}
.rs-ops .el-button + .el-button {
  margin-left: 0;
}
.remark {
  margin-top: 16px;
}
.sl-addr {
  display: flex;
  gap: 8px;
  width: 100%;
}
.sl-addr .el-input {
  flex: 1;
}
.rs-note {
  margin-bottom: 10px;
}
.unit {
  margin: 0 10px 0 6px;
  color: var(--el-text-color-secondary);
}
@media (max-width: 767.98px) {
  .slider {
    width: 100%;
    margin: 0 8px;
  }
  .rs-remark {
    width: auto;
    flex: 1;
    min-width: 120px;
  }
}
</style>
