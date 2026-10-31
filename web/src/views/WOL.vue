<template>
  <div>
    <PageHeader :title="t('wol.title')" :desc="t('wol.desc')" :icon="Power">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('groupSort.search')" />
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('wol.addDevice') }}</el-button>
    </PageHeader>

    <GroupedList :gl="gl" :compact="compact" grid :empty-text="t('wol.empty')">
      <template #item="{ it, item: d, sorting, bind }">
        <EntityCard v-bind="bind" :compact="compact || sorting">
          <template #lead>
            <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
            <div v-else class="dev-icon" :class="{ online: status[d.key ?? ''] === 'online' }"><Monitor :size="18" /></div>
          </template>
          <template #title>{{ d.name }}</template>
          <template v-if="d.remark" #sub>{{ d.remark }}</template>
          <template #status>
            <StatusBadge v-bind="devStatus(d)" />
          </template>
          <Field :label="t('wol.macAddress')" mono>
            <div v-for="m in d.macs" :key="m">{{ m }}</div>
          </Field>
          <Field :label="t('wol.broadcast')" mono>{{ (d.broadcastIps ?? []).length ? (d.broadcastIps ?? []).join(', ') : t('wol.auto') }}:{{ d.port }} ×{{ d.repeat }}</Field>
          <Field v-if="d.checkAddr" :label="t('wol.onlineCheck')" mono>{{ d.checkAddr }}</Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="Pencil" @click="openEditor(d)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('wol.confirmDelete')" @confirm="remove(d)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
            <el-button type="primary" size="small" :icon="Zap" :loading="waking[d.key ?? '']" class="wake" @click="wake(d)">{{ t('wol.wake') }}</el-button>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <LogCard url="/api/wol/logs" clearable />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('wol.editDevice') : t('wol.addDevice')" width="540px" :fullscreen="isMobile">
      <el-form :model="editing" label-width="110px" :label-position="labelPosition">
        <el-form-item :label="t('common.name')"><el-input v-model="editing.name" /></el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('wol.macAddress')"><el-input v-model="macsText" type="textarea" :rows="2" class="mono" :placeholder="t('wol.macPlaceholder')" /></el-form-item>
        <el-form-item :label="t('wol.broadcastAddress')"><el-input v-model="bcText" :placeholder="t('wol.broadcastPlaceholder')" /></el-form-item>
        <el-form-item :label="t('wol.port')"><el-input-number v-model="editing.port" :min="1" :max="65535" /></el-form-item>
        <el-form-item :label="t('wol.repeat')"><el-input-number v-model="editing.repeat" :min="1" :max="20" /></el-form-item>
        <el-form-item :label="t('wol.onlineCheck')">
          <el-input v-model="editing.checkAddr" :placeholder="t('wol.checkPlaceholder')" />
          <div class="form-help">{{ t('wol.checkHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Monitor, Pencil, Plus, Power, Trash2, Zap } from 'lucide-vue-next'
import { api } from '../api'
import PageHeader from '../components/PageHeader.vue'
import EntityCard from '../components/EntityCard.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import { useGroupedList } from '../composables/useGroupSort'
import CompactToggle from '../components/CompactToggle.vue'
import { useCompactView } from '../composables/useCompactView'
import Field from '../components/Field.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { useListText, splitLinesOrCommas } from '../composables/useListText'
import { usePolling } from '../composables/usePolling'

interface Device {
  key?: string
  name: string
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  macs: string[]
  broadcastIps: string[] | null
  port: number
  repeat: number
  checkAddr: string
  remark: string
  enable: boolean
}

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
const devices = ref<Device[]>([])
const compact = useCompactView('wol')
// 列表的搜索、分组与排序
const gl = useGroupedList<Device>({
  storageKey: 'wol.groups',
  items: () => devices.value,
  base: '/api/wol/devices',
  reload: load,
  matches: (d, q) => [d.name, d.group ?? '', d.remark, ...d.macs, d.checkAddr].join('\n').toLowerCase().includes(q),
})
const status = ref<Record<string, string>>({})
const waking = reactive<Record<string, boolean>>({})
const editorVisible = ref(false)
const editing = ref<Device>(empty())
// 定时刷新，标签页在后台时暂停
const polling = usePolling(load, 10000)

// 输入框保留原始文本，回车、逗号可正常输入（见 useListText）
const macsText = useListText(() => editing.value.macs, (v) => (editing.value.macs = v), { split: splitLinesOrCommas })
const bcText = useListText(() => editing.value.broadcastIps, (v) => (editing.value.broadcastIps = v), {
  split: (v) => v.split(',').map((s) => s.trim()).filter(Boolean),
  separator: ', ',
})

function empty(): Device {
  return { name: '', macs: [], broadcastIps: [], port: 9, repeat: 3, checkAddr: '', remark: '', enable: true }
}

// devStatus 计算设备在线状态；未配置探测地址时明确提示，而不是显示为等待
function devStatus(d: Device): { type: StatusType; text: string } {
  if (!d.checkAddr) {
    return { type: 'info', text: t('wol.noCheck') }
  }
  const s = status.value[d.key ?? '']
  if (s === 'online') {
    return { type: 'running', text: t('wol.online') }
  }
  if (s === 'offline') {
    return { type: 'off', text: t('wol.offline') }
  }
  return { type: 'pending', text: t('wol.probing') }
}

async function load() {
  devices.value = await api.get('/api/wol/devices')
  status.value = await api.get('/api/wol/status')
}

function openEditor(d?: Device) {
  editing.value = d ? JSON.parse(JSON.stringify(d)) : empty()
  editorVisible.value = true
}

async function save() {
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/wol/devices/${key}`, body)
    } else {
      await api.post('/api/wol/devices', body)
    }
    editorVisible.value = false
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function wake(d: Device) {
  const k = d.key ?? ''
  waking[k] = true
  try {
    const r = await api.post<{ sent: number }>(`/api/wol/devices/${k}/wake`)
    ElMessage.success(t('wol.sent', { count: r.sent }))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    waking[k] = false
  }
}

async function remove(d: Device) {
  try {
    await api.del(`/api/wol/devices/${d.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(() => {
  load()
  polling.start()
})
</script>

<style scoped>
.dev-icon {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-secondary);
}
.dev-icon.online {
  background: var(--el-color-success-light-9);
  color: var(--el-color-success);
}
.wake {
  margin-left: 6px !important;
}
</style>
