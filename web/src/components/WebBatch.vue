<template>
  <!-- Web 服务站点的批量操作：批量增删或替换域名、批量修改部分选项、移到其他监听；启停、删除与拖拽移动通过 enable / remove / move 方法直接执行。
       所有操作一次请求提交，后端全部校验通过才保存，任一站点失败都不修改 -->
  <el-dialog v-model="visible" :title="titles[mode]" width="560px" :fullscreen="isMobile" @closed="reset">
    <el-alert :title="t('webBatch.scope', { count: refs.length })" type="info" :closable="false" show-icon class="scope" />

    <template v-if="mode === 'domains'">
      <el-radio-group v-model="action" class="actions">
        <el-radio-button value="addDomains">{{ t('webBatch.add') }}</el-radio-button>
        <el-radio-button value="removeDomains">{{ t('webBatch.removeDomains') }}</el-radio-button>
        <el-radio-button value="replaceDomain">{{ t('webBatch.replace') }}</el-radio-button>
      </el-radio-group>
      <template v-if="action === 'replaceDomain'">
        <el-form label-position="top" @submit.prevent>
          <el-form-item :label="t('webBatch.from')"><el-input v-model="from" class="mono" placeholder="old.example.com" /></el-form-item>
          <el-form-item :label="t('webBatch.to')"><el-input v-model="to" class="mono" placeholder="new.example.com" /></el-form-item>
        </el-form>
        <div class="form-help">{{ t('webBatch.replaceHelp') }}</div>
      </template>
      <template v-else>
        <el-input v-model="domainText" type="textarea" :rows="6" class="mono" :placeholder="t('webBatch.domainsPlaceholder')" />
        <div class="form-help">{{ action === 'addDomains' ? t('webBatch.addHelp') : t('webBatch.removeHelp') }}</div>
      </template>
    </template>

    <template v-else-if="mode === 'move'">
      <el-form label-width="110px" label-position="left" @submit.prevent>
        <el-form-item :label="t('webBatch.moveTarget')">
          <el-select v-model="moveListener" class="choice-wide" :placeholder="t('webBatch.moveTargetPlaceholder')">
            <el-option v-for="l in listeners" :key="l.key" :value="l.key" :label="listenerLabel(l)" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('webBatch.moveGroup')">
          <el-select v-model="moveGroup" class="choice-wide" filterable allow-create default-first-option :empty-values="[null, undefined]">
            <el-option value="__keep" :label="t('webBatch.moveGroupKeep')" />
            <el-option value="" :label="t('webBatch.moveGroupNone')" />
            <el-option v-for="g in targetGroups" :key="g" :value="g" :label="g" />
          </el-select>
        </el-form-item>
      </el-form>
      <div class="form-help">{{ t('webBatch.moveHelp') }}</div>
      <el-alert v-if="moveListener && plainRisk(refs, moveListener)" :title="plainWarn(refs.length)" type="warning" :closable="false" show-icon class="warn" />
    </template>

    <template v-else>
      <div class="form-help intro">{{ t('webBatch.optionsHelp') }}</div>
      <el-form label-width="150px" label-position="left" @submit.prevent>
        <el-form-item v-for="o in switches" :key="o.key" :label="o.label">
          <el-select v-model="choice[o.key]" class="choice">
            <el-option value="keep" :label="t('webBatch.keep')" />
            <el-option value="on" :label="t('webBatch.on')" />
            <el-option value="off" :label="t('webBatch.off')" />
          </el-select>
        </el-form-item>
        <el-form-item label="WAF">
          <el-select v-model="waf" class="choice" :empty-values="[null, undefined]">
            <el-option value="__keep" :label="t('webBatch.keep')" />
            <el-option value="" :label="t('webBatch.wafNone')" />
            <el-option v-for="w in wafInstances" :key="w.key" :value="w.key" :label="w.name || w.key" />
          </el-select>
        </el-form-item>
        <!-- 错误页样式：只改样式与自定义页面，各站点的拦截后端错误设置保持不变 -->
        <el-form-item :label="t('errorPages.batchLabel')">
          <el-select v-model="errorStyle" class="choice" :empty-values="[null, undefined]" data-test="batch-error-style" @visible-change="(v: boolean) => v && loadCustomPages()">
            <el-option value="__keep" :label="t('webBatch.keep')" />
            <el-option value="" :label="t('errorPages.styles.default')" />
            <el-option v-for="id in builtinStyles" :key="id" :value="id" :label="t(`errorPages.styles.${id}`)" />
            <el-option v-for="p in customPages" :key="p.key" :value="`custom:${p.key}`" :label="p.name" />
          </el-select>
          <div class="form-help">{{ t('errorPages.batchHelp') }}</div>
        </el-form-item>
      </el-form>
    </template>

    <template #footer>
      <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="busy" :disabled="!canSubmit" @click="submit">{{ t('webBatch.apply', { count: refs.length }) }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '../api'
import { useIsMobile } from '../composables/useIsMobile'
import { builtinStyles, errorPagesApi, type ErrorPageView } from '../errorPages'
import { movePlainRisk, type MoveListener, type MoveTarget, type SiteRef } from '../siteMove'


const props = defineProps<{ wafInstances: { key: string; name: string }[]; listeners?: MoveListener[] }>()
const emit = defineEmits<{ done: [] }>()
const { t } = useI18n()
const isMobile = useIsMobile()

const BATCH_URL = '/api/webservice/sites/batch'
const visible = ref(false)
const busy = ref(false)
const mode = ref<'domains' | 'options' | 'move'>('domains')
const titles = computed(() => ({ domains: t('webBatch.domainsTitle'), options: t('webBatch.optionsTitle'), move: t('webBatch.moveTitle') }))
const refs = ref<SiteRef[]>([])
const action = ref<'addDomains' | 'removeDomains' | 'replaceDomain'>('addDomains')
const domainText = ref('')
const from = ref('')
const to = ref('')
type Choice = 'keep' | 'on' | 'off'
const switches = computed(() => [
  { key: 'compress', label: t('webBatch.compress') },
  { key: 'accessLog', label: t('webBatch.accessLog') },
  { key: 'cors', label: 'CORS' },
  { key: 'forwardedHeaders', label: t('webBatch.forwardedHeaders') },
  { key: 'denyClose', label: t('webBatch.denyClose') },
])
const choice = reactive<Record<string, Choice>>({ compress: 'keep', accessLog: 'keep', cors: 'keep', forwardedHeaders: 'keep', denyClose: 'keep' })
const waf = ref('__keep')
// errorStyle 为 __keep 时不修改；custom:key 为页面库中的自定义页面
const errorStyle = ref('__keep')
const customPages = ref<ErrorPageView[]>([])
async function loadCustomPages() {
  try {
    customPages.value = await errorPagesApi.list()
  } catch {
    customPages.value = []
  }
}
const moveListener = ref('')
// moveGroup 为 __keep 时保留各站点原有分组
const moveGroup = ref('__keep')
const listeners = computed(() => props.listeners ?? [])
const targetGroups = computed(() => listeners.value.find((l) => l.key === moveListener.value)?.groups ?? [])

function listenerLabel(l: MoveListener) {
  return `${l.name || t('web.unnamedListener')} (${l.tls ? 'https' : 'http'} :${l.port})`
}

// plainRisk 判断是否要把开启了认证的站点从 HTTPS 监听移到明文 HTTP 监听
function plainRisk(r: SiteRef[], target: string) {
  return movePlainRisk(r, target, listeners.value)
}

// plainWarn 返回移到明文监听的认证提醒：只移动一个站点时说“该站点”，多个时说“部分站点”
function plainWarn(count: number) {
  return count === 1 ? t('webBatch.movePlainWarnOne') : t('webBatch.movePlainWarn')
}

// parseDomains 按换行、逗号、分号拆分域名；行内空格不作分隔，含空格的写法交给后端报错，避免把错误写法拆成多个域名
function parseDomains(text: string) {
  return text
    .split(/[\r\n,，;；]+/)
    .map((d) => d.trim())
    .filter(Boolean)
}

const canSubmit = computed(() => {
  if (!refs.value.length) {
    return false
  }
  if (mode.value === 'options') {
    return waf.value !== '__keep' || errorStyle.value !== '__keep' || Object.values(choice).some((c) => c !== 'keep')
  }
  if (mode.value === 'move') {
    return !!moveListener.value
  }
  if (action.value === 'replaceDomain') {
    return !!from.value.trim() && !!to.value.trim()
  }
  return parseDomains(domainText.value).length > 0
})

function reset() {
  domainText.value = ''
  from.value = ''
  to.value = ''
  action.value = 'addDomains'
  waf.value = '__keep'
  errorStyle.value = '__keep'
  moveListener.value = ''
  moveGroup.value = '__keep'
  for (const k of Object.keys(choice)) {
    choice[k] = 'keep'
  }
}

// open 打开批量修改域名、选项或移动站点的对话框，r 为选中的站点
function open(m: 'domains' | 'options' | 'move', r: SiteRef[]) {
  mode.value = m
  refs.value = r
  visible.value = true
  if (m === 'options') {
    loadCustomPages()
  }
}

function summary(res: { changed: number; unchanged: number }) {
  return res.unchanged ? t('webBatch.resultSome', res) : t('webBatch.result', res)
}

async function submit() {
  if (mode.value === 'move') {
    const target: MoveTarget = { listener: moveListener.value }
    if (moveGroup.value !== '__keep') {
      target.group = moveGroup.value
    }
    // 对话框中已显示明文传输警告，这里直接提交
    if (await post(refs.value, target)) {
      visible.value = false
    }
    return
  }
  const body: Record<string, unknown> = { sites: refs.value }
  if (mode.value === 'domains') {
    body.action = action.value
    if (action.value === 'replaceDomain') {
      body.from = from.value.trim()
      body.to = to.value.trim()
    } else {
      body.domains = parseDomains(domainText.value)
    }
  } else {
    const options: Record<string, unknown> = {}
    for (const [k, c] of Object.entries(choice)) {
      if (c !== 'keep') {
        options[k] = c === 'on'
      }
    }
    if (waf.value !== '__keep') {
      options.waf = waf.value
    }
    if (errorStyle.value !== '__keep') {
      const custom = errorStyle.value.startsWith('custom:')
      options.errorPage = { style: custom ? 'custom' : errorStyle.value, page: custom ? errorStyle.value.slice(7) : '' }
    }
    body.action = 'set'
    body.options = options
  }
  busy.value = true
  try {
    const res = await api.post<{ changed: number; unchanged: number }>(BATCH_URL, body)
    visible.value = false
    ElMessage.success(summary(res))
    emit('done')
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

// enable 批量启用或停用站点，返回是否成功
async function enable(r: SiteRef[], on: boolean) {
  try {
    const res = await api.post<{ changed: number; unchanged: number }>(BATCH_URL, { sites: r, action: on ? 'enable' : 'disable' })
    ElMessage.success(summary(res))
    emit('done')
    return true
  } catch (e) {
    ElMessage.error((e as Error).message)
    return false
  }
}

// remove 确认后批量删除站点，names 为用于确认提示的站点名称；返回是否已删除
async function remove(r: SiteRef[], names: string[]) {
  const shown = names.slice(0, 5).join('、') + (names.length > 5 ? t('groupSort.batchRemoveMore', { count: names.length - 5 }) : '')
  try {
    await ElMessageBox.confirm(t('webBatch.removeConfirm', { count: r.length, names: shown }), t('groupSort.batchRemoveTitle'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return false
  }
  try {
    await api.post(BATCH_URL, { sites: r, action: 'delete' })
    ElMessage.success(t('groupSort.batchRemoved', { count: r.length }))
    emit('done')
    return true
  } catch (e) {
    ElMessage.error((e as Error).message)
    return false
  }
}

// post 提交移动请求并提示结果，返回是否成功
async function post(r: SiteRef[], target: MoveTarget) {
  busy.value = true
  try {
    const res = await api.post<{ changed: number; unchanged: number }>(BATCH_URL, { sites: r, action: 'move', move: target })
    ElMessage.success(res.changed ? t('webBatch.moved', { count: res.changed }) : t('webBatch.moveNothing'))
    emit('done')
    return true
  } catch (e) {
    ElMessage.error((e as Error).message)
    return false
  } finally {
    busy.value = false
  }
}

// move 确认后把站点移到其他监听（拖拽时使用），names 为用于确认提示的站点名称；返回是否已移动
async function move(r: SiteRef[], names: string[], target: MoveTarget) {
  const to = listeners.value.find((l) => l.key === target.listener)
  const shown = names.slice(0, 5).join('、') + (names.length > 5 ? t('groupSort.batchRemoveMore', { count: names.length - 5 }) : '')
  let msg = t('webBatch.moveConfirm', { count: r.length, names: shown, target: to ? listenerLabel(to) : target.listener })
  if (plainRisk(r, target.listener)) {
    msg += '\n\n' + plainWarn(r.length)
  }
  try {
    await ElMessageBox.confirm(msg, t('webBatch.moveTitle'), {
      type: 'warning',
      confirmButtonText: t('webBatch.moveOk'),
      cancelButtonText: t('common.cancel'),
      customStyle: { whiteSpace: 'pre-line' },
    })
  } catch {
    return false
  }
  return post(r, target)
}

defineExpose({ open, enable, remove, move })
</script>

<style scoped>
.scope,
.actions {
  margin-bottom: 12px;
}
.intro {
  margin-bottom: 12px;
}
.choice {
  width: 220px;
}
.choice-wide {
  width: 100%;
}
.warn {
  margin-top: 12px;
}
</style>
