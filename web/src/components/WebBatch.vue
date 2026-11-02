<template>
  <!-- Web 服务站点的批量操作：批量增删或替换域名、批量修改部分选项；启停与删除通过 enable / remove 方法直接执行。
       所有操作一次请求提交，后端全部校验通过才保存，任一站点失败都不修改 -->
  <el-dialog v-model="visible" :title="mode === 'domains' ? t('webBatch.domainsTitle') : t('webBatch.optionsTitle')" width="560px" :fullscreen="isMobile" @closed="reset">
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
          <el-select v-model="waf" class="choice">
            <el-option value="__keep" :label="t('webBatch.keep')" />
            <el-option value="" :label="t('webBatch.wafNone')" />
            <el-option v-for="w in wafInstances" :key="w.key" :value="w.key" :label="w.name || w.key" />
          </el-select>
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

export interface SiteRef {
  listener: string
  site: string
}

defineProps<{ wafInstances: { key: string; name: string }[] }>()
const emit = defineEmits<{ done: [] }>()
const { t } = useI18n()
const isMobile = useIsMobile()

const BATCH_URL = '/api/webservice/sites/batch'
const visible = ref(false)
const busy = ref(false)
const mode = ref<'domains' | 'options'>('domains')
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
    return waf.value !== '__keep' || Object.values(choice).some((c) => c !== 'keep')
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
  for (const k of Object.keys(choice)) {
    choice[k] = 'keep'
  }
}

// open 打开批量修改域名或选项的对话框，r 为选中的站点
function open(m: 'domains' | 'options', r: SiteRef[]) {
  mode.value = m
  refs.value = r
  visible.value = true
}

function summary(res: { changed: number; unchanged: number }) {
  return res.unchanged ? t('webBatch.resultSome', res) : t('webBatch.result', res)
}

async function submit() {
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

defineExpose({ open, enable, remove })
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
</style>
