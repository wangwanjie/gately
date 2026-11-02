<template>
  <div>
    <el-divider content-position="left">{{ t('ipLists.title') }}</el-divider>
    <el-form-item :label="t('ipLists.lists')">
      <div class="rows">
        <div class="form-help top-help">{{ t('ipLists.help') }}</div>
        <el-empty v-if="!lists.length" :image-size="36" :description="t('ipLists.empty')" />
        <div v-for="l in lists" :key="l.key" class="item" :class="{ off: !l.enable }">
          <div class="item-head">
            <el-switch :model-value="l.enable" size="small" @change="(v: string | number | boolean) => toggle(l, Boolean(v))" />
            <span class="name">{{ l.name }}</span>
            <el-tag size="small" :type="statusOf(l).type">{{ statusOf(l).text }}</el-tag>
            <span class="actions">
              <el-button text size="small" :icon="RotateCw" :loading="refreshing === l.key" :disabled="!l.enable" @click="refresh(l)">{{ t('ipLists.refresh') }}</el-button>
              <el-button text size="small" :icon="Pencil" @click="openEditor(l)">{{ t('common.edit') }}</el-button>
              <el-button text size="small" :icon="CopyPlus" @click="openEditor(l, true)">{{ t('common.duplicate') }}</el-button>
              <el-popconfirm :title="t('ipLists.deleteConfirm')" width="240" @confirm="remove(l)">
                <template #reference>
                  <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
                </template>
              </el-popconfirm>
            </span>
          </div>
          <div class="meta mono url">{{ l.url }}</div>
          <div class="meta">
            {{ l.updatedAt ? t('ipLists.updated', { count: l.count, time: new Date(l.updatedAt).toLocaleString(), hours: l.refreshHours }) : t('ipLists.neverUpdated') }}
            <span v-if="l.usedBy?.length"> · {{ t('ipLists.usedBy', { sites: l.usedBy.map(usedByLabel).join('、') }) }}</span>
          </div>
          <div v-if="l.lastError" class="err">{{ l.lastError }}</div>
        </div>
        <div>
          <el-button size="small" :icon="Plus" @click="openEditor()">{{ t('ipLists.add') }}</el-button>
        </div>
      </div>
    </el-form-item>

    <el-dialog v-model="editorVisible" :title="editing.key ? t('ipLists.edit') : t('ipLists.add')" width="560px" append-to-body :fullscreen="isMobile">
      <CopyHint :from="copyFrom" />
      <el-form :model="editing" label-width="110px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')" required><el-input v-model="editing.name" :placeholder="t('ipLists.namePlaceholder')" /></el-form-item>
        <el-form-item :label="t('ipLists.url')" required>
          <el-input v-model="editing.url" class="mono" placeholder="https://metowolf.github.io/iplist/data/cncity/440000.txt" />
          <div class="form-help">{{ t('ipLists.urlHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('ipLists.proxy')">
          <ProxyInput v-model="editing.proxy" :owner="editing" />
          <div class="form-help">{{ t('ipLists.proxyHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('ipLists.refreshHours')">
          <el-input-number v-model="editing.refreshHours" :min="1" :max="720" />
          <span class="inline-help">{{ t('ipLists.hours') }}</span>
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
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
import { CopyPlus, Pencil, Plus, RotateCw, Trash2 } from 'lucide-vue-next'
import { api } from '../api'
import CopyHint from './CopyHint.vue'
import { createItem, uniqueName, type CopySource } from '../copyDraft'
import { useFormLayout } from '../composables/useIsMobile'
import { usePolling } from '../composables/usePolling'
import ProxyInput from './ProxyInput.vue'

// IPList 对应 /api/webservice/iplists 的条目：按地址订阅、定时刷新的 IP 列表
export interface IPList {
  key?: string
  name: string
  enable: boolean
  url: string
  refreshHours: number
  // proxy 为下载使用的代理：留空不指定，update 使用全局更新代理，或代理地址
  proxy: string
  remark: string
  updatedAt?: string
  count?: number
}

interface IPListView extends IPList {
  key: string
  loaded: boolean
  refreshing: boolean
  lastError?: string
  usedBy?: string[]
}

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
const emit = defineEmits<{ changed: [] }>()
const lists = ref<IPListView[]>([])
const editorVisible = ref(false)
const saving = ref(false)
const refreshing = ref('')
const editing = ref<IPList>(empty())
// 复制条目时的模板，普通新增与编辑时为 null；副本保存后由系统重新下载
const copyFrom = ref<CopySource | null>(null)
// 定时刷新，标签页在后台时暂停
const polling = usePolling(load, 5000)

// 其它模块登记的引用方形如“portforward / 规则名”，把模块 ID 换成界面语言的模块名
const moduleRefs = ['portforward', 'protoroute', 'stun']
function usedByLabel(s: string) {
  const [id, ...rest] = s.split(' / ')
  if (rest.length && moduleRefs.includes(id)) {
    return [t(`nav.items.${id}.title`), ...rest].join(' / ')
  }
  return s
}

function empty(): IPList {
  return { name: '', enable: true, url: '', refreshHours: 24, proxy: '', remark: '' }
}

// statusOf 计算列表状态：停用、下载中、已载入或下载失败
function statusOf(l: IPListView): { type: 'info' | 'success' | 'warning' | 'danger'; text: string } {
  if (!l.enable) {
    return { type: 'info', text: t('ipLists.statusOff') }
  }
  if (l.refreshing) {
    return { type: 'warning', text: t('ipLists.statusRefreshing') }
  }
  if (l.loaded) {
    return { type: l.lastError ? 'warning' : 'success', text: t('ipLists.statusLoaded', { count: l.count ?? 0 }) }
  }
  return { type: l.lastError ? 'danger' : 'warning', text: l.lastError ? t('ipLists.statusFailed') : t('ipLists.statusPending') }
}

async function load() {
  lists.value = await api.get<IPListView[]>('/api/webservice/iplists')
}

// openEditor 打开编辑框：copy 为 true 时以 l 为模板新建副本
function openEditor(l?: IPListView, copy = false) {
  copyFrom.value = null
  if (l) {
    editing.value = { key: l.key, name: l.name, enable: l.enable, url: l.url, refreshHours: l.refreshHours, proxy: l.proxy ?? '', remark: l.remark }
    if (copy) {
      copyFrom.value = { key: l.key ?? '', name: l.name }
      editing.value.key = ''
      editing.value.name = uniqueName(l.name, lists.value.map((x) => x.name))
    }
  } else {
    editing.value = empty()
  }
  editorVisible.value = true
}

async function save() {
  saving.value = true
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/webservice/iplists/${key}`, body)
    } else {
      await createItem('/api/webservice/iplists', body, copyFrom.value)
    }
    editorVisible.value = false
    ElMessage.success(t('common.saved'))
    await load()
    emit('changed')
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function toggle(l: IPListView, enable: boolean) {
  try {
    await api.put(`/api/webservice/iplists/${l.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function refresh(l: IPListView) {
  refreshing.value = l.key
  try {
    const v = await api.post<IPListView>(`/api/webservice/iplists/${l.key}/refresh`)
    ElMessage.success(t('ipLists.refreshed', { count: v?.count ?? 0 }))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    refreshing.value = ''
    await load()
  }
}

async function remove(l: IPListView) {
  try {
    await api.del(`/api/webservice/iplists/${l.key}`)
    await load()
    emit('changed')
  } catch (e) {
    // 被引用时后端列出“模块 ID / 规则名”，同样换成界面语言的模块名
    ElMessage.error((e as Error).message.replace(new RegExp(`\\b(${moduleRefs.join('|')}) / `, 'g'), (_, id: string) => `${t(`nav.items.${id}.title`)} / `))
  }
}

onMounted(() => {
  load().catch((e) => ElMessage.error((e as Error).message))
  // 新增列表后在后台下载，定时刷新以显示进度
  polling.start()
})
</script>

<style scoped>
.rows {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.top-help {
  margin: 0;
}
.item {
  border: 1px solid var(--g-border);
  border-radius: 8px;
  padding: 8px 10px;
}
.item.off {
  opacity: 0.65;
}
.item-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.name {
  font-weight: 600;
}
.actions {
  margin-left: auto;
}
.url {
  word-break: break-all;
}
.meta,
.inline-help {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
.inline-help {
  margin-left: 10px;
}
.err {
  font-size: 12px;
  color: var(--el-color-danger);
  word-break: break-all;
}
</style>
