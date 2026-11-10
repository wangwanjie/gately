<template>
  <div>
    <PageHeader :title="t('wol.title')" :desc="t('wol.desc')" :icon="Power">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('groupSort.search')" />
      <!-- 巴法云语音控制：按钮上的圆点表示连接状态 -->
      <el-button :icon="Mic" @click="openBemfa">
        {{ t('wol.bemfa') }}<span v-if="bemfa.settings.enable" class="dot" :class="bemfaDot" />
      </el-button>
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('wol.addDevice') }}</el-button>
    </PageHeader>

    <GroupedList :gl="gl" :compact="compact" grid :empty-text="t('wol.empty')">
      <template #item="{ it, item: d, sorting, bind }">
        <EntityCard v-bind="bind" :compact="compact || sorting" class="wol-card">
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
          <Field v-if="d.shutdownConn" :label="t('wol.shutdownField')">{{ connName(d.shutdownConn) }}</Field>
          <Field v-if="d.bemfaTopic" :label="t('wol.bemfa')" mono>{{ d.bemfaTopic }}</Field>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-button text size="small" :icon="Pencil" @click="openEditor(d)">{{ t('common.edit') }}</el-button>
            <el-button text size="small" :icon="CopyPlus" @click="openEditor(d, true)">{{ t('common.duplicate') }}</el-button>
            <el-popconfirm :title="t('wol.confirmDelete')" @confirm="remove(d)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
            <el-popconfirm v-if="d.shutdownConn" :title="t('wol.shutdownConfirm')" @confirm="shutdown(d)">
              <template #reference>
                <el-button size="small" :icon="PowerOff" :loading="stopping[d.key ?? '']" class="wake">{{ t('wol.shutdown') }}</el-button>
              </template>
            </el-popconfirm>
            <el-button type="primary" size="small" :icon="Zap" :loading="waking[d.key ?? '']" class="wake" @click="wake(d)">{{ t('wol.wake') }}</el-button>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <LogCard url="/api/wol/logs" clearable />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('wol.editDevice') : t('wol.addDevice')" width="540px" :fullscreen="isMobile">
      <CopyHint :from="copyFrom" />
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
        <!-- 远程关机：经 Web 终端中已记录主机密钥的 SSH 连接执行关机命令 -->
        <el-form-item v-if="moduleBuilt('webterminal') || editing.shutdownConn" :label="t('wol.shutdownConn')">
          <el-select v-model="editing.shutdownConn" filterable clearable :placeholder="t('wol.shutdownConnPlaceholder')" :no-data-text="t('wol.noConnections')" class="full" @change="onConnChange">
            <el-option v-for="o in sshTargets" :key="o.key" :label="o.name" :value="o.key">
              <span>{{ o.name }}</span>
              <span class="opt-meta mono">{{ o.host }}</span>
              <span v-if="!o.trusted" class="opt-warn">{{ t('wol.untrusted') }}</span>
            </el-option>
          </el-select>
          <div v-if="editing.shutdownConn && sshTargets.find((o) => o.key === editing.shutdownConn && !o.trusted)" class="form-help warn">{{ t('wol.untrustedHelp') }}</div>
        </el-form-item>
        <el-form-item v-if="editing.shutdownConn" :label="t('wol.shutdownCommand')">
          <el-input v-model="editing.shutdownCommand" class="mono" placeholder="shutdown -h now" />
          <div class="presets">
            <span class="form-help">{{ t('wol.shutdownPresets') }}</span>
            <el-button v-for="p in presets" :key="p.label" size="small" text type="primary" @click="editing.shutdownCommand = p.cmd">{{ p.label }}</el-button>
          </div>
          <div class="form-help">{{ t('wol.shutdownHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('wol.bemfaTopic')">
          <el-input v-model="editing.bemfaTopic" class="mono" :placeholder="t('wol.bemfaTopicPlaceholder')" />
          <div class="form-help">{{ t('wol.bemfaTopicHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="editing.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="bemfaVisible" :title="t('wol.bemfaTitle')" width="540px" :fullscreen="isMobile">
      <el-alert type="info" :closable="false" class="bemfa-steps">
        <ol>
          <li>{{ t('wol.bemfaStep1') }}</li>
          <li>{{ t('wol.bemfaStep2') }}</li>
          <li>{{ t('wol.bemfaStep3') }}</li>
        </ol>
      </el-alert>
      <el-form :model="bemfaForm" label-width="110px" :label-position="labelPosition">
        <el-form-item :label="t('wol.bemfaStatus')">
          <StatusBadge v-bind="bemfaStatus()" />
          <div v-if="bemfa.status.error" class="form-help warn">{{ bemfa.status.error }}</div>
        </el-form-item>
        <el-form-item :label="t('wol.bemfaEnable')"><el-switch v-model="bemfaForm.enable" /></el-form-item>
        <el-form-item :label="t('wol.bemfaUid')">
          <SecretInput v-model="bemfaForm.uid" class="mono" autocomplete="off" :placeholder="t('wol.bemfaUidPlaceholder')" :reveal="secretOf('/api/wol/bemfa/secrets', 'uid')" />
          <div class="form-help">{{ t('wol.bemfaUidHelp') }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bemfaVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="bemfaSaving" @click="saveBemfa">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { CopyPlus, Mic, Monitor, Pencil, Plus, Power, PowerOff, Trash2, Zap } from 'lucide-vue-next'
import { api } from '../api'
import { moduleBuilt } from '../nav'
import SecretInput from '../components/SecretInput.vue'
import { secretOf } from '../secrets'
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
import CopyHint from '../components/CopyHint.vue'
import { createItem, uniqueName, type CopySource } from '../copyDraft'
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
  // shutdownConn 为远程关机使用的 Web 终端 SSH 连接 key，空表示不支持关机
  shutdownConn?: string
  shutdownCommand?: string
  // bemfaTopic 为巴法云主题，空表示不接入语音控制
  bemfaTopic?: string
  remark: string
  enable: boolean
}

interface SSHTarget {
  key: string
  name: string
  host: string
  // trusted 表示已记录主机密钥，可以无人值守地执行命令
  trusted: boolean
}

interface BemfaView {
  settings: { enable: boolean; uid: string }
  status: { connected: boolean; topics: number; error?: string }
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
  // 唤醒设备没有启用开关
  batch: { enable: false },
  reload: load,
  matches: (d, q) => [d.name, d.group ?? '', d.remark, ...d.macs, d.checkAddr, d.bemfaTopic ?? ''].join('\n').toLowerCase().includes(q),
})
const status = ref<Record<string, string>>({})
const waking = reactive<Record<string, boolean>>({})
const stopping = reactive<Record<string, boolean>>({})
const sshTargets = ref<SSHTarget[]>([])
const bemfa = ref<BemfaView>({ settings: { enable: false, uid: '' }, status: { connected: false, topics: 0 } })
const bemfaVisible = ref(false)
const bemfaSaving = ref(false)
const bemfaForm = ref({ enable: false, uid: '' })
// 常用关机命令：Linux 需以 root 登录（非 root 用户可改为 sudo -n shutdown -h now 并配置免密 sudo）
const presets = [
  { label: 'Linux', cmd: 'shutdown -h now' },
  { label: 'Windows', cmd: 'shutdown /s /t 0' },
  { label: 'macOS', cmd: 'sudo -n shutdown -h now' },
]
const editorVisible = ref(false)
const editing = ref<Device>(empty())
// 复制条目时的模板，普通新增与编辑时为 null
const copyFrom = ref<CopySource | null>(null)
// 定时刷新，标签页在后台时暂停
const polling = usePolling(load, 10000)

// 输入框保留原始文本，回车、逗号可正常输入（见 useListText）
const macsText = useListText(() => editing.value.macs, (v) => (editing.value.macs = v), { split: splitLinesOrCommas })
const bcText = useListText(() => editing.value.broadcastIps, (v) => (editing.value.broadcastIps = v), {
  split: (v) => v.split(',').map((s) => s.trim()).filter(Boolean),
  separator: ', ',
})

function empty(): Device {
  return { name: '', macs: [], broadcastIps: [], port: 9, repeat: 3, checkAddr: '', shutdownConn: '', shutdownCommand: '', bemfaTopic: '', remark: '', enable: true }
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

// bemfaStatus 计算巴法云连接状态：未启用、未设置主题、连接中、已连接或连接失败
function bemfaStatus(): { type: StatusType; text: string } {
  const v = bemfa.value
  if (!v.settings.enable) {
    return { type: 'off', text: t('wol.bemfaOff') }
  }
  if (v.status.connected) {
    return { type: 'running', text: t('wol.bemfaConnected', { count: v.status.topics }) }
  }
  if (v.status.error) {
    return { type: 'error', text: t('wol.bemfaFailed') }
  }
  if (!devices.value.some((d) => d.bemfaTopic)) {
    return { type: 'warn', text: t('wol.bemfaNoTopics') }
  }
  return { type: 'pending', text: t('wol.bemfaConnecting') }
}

const bemfaDot = computed(() => {
  const s = bemfaStatus().type
  return s === 'running' ? 'ok' : s === 'error' || s === 'warn' ? 'bad' : ''
})

// connName 返回关机连接的名称；连接已被删除时显示 key 并注明
function connName(key: string) {
  return sshTargets.value.find((o) => o.key === key)?.name ?? t('wol.connMissing', { key })
}

async function load() {
  const [d, s, b] = await Promise.all([
    api.get<Device[]>('/api/wol/devices'),
    api.get<Record<string, string>>('/api/wol/status'),
    api.get<BemfaView>('/api/wol/bemfa'),
  ])
  devices.value = d
  status.value = s
  bemfa.value = b
}

async function loadTargets() {
  try {
    sshTargets.value = await api.get<SSHTarget[]>('/api/wol/ssh-targets')
  } catch {
    sshTargets.value = []
  }
}

// onConnChange 选择关机连接后若还没有命令，默认填 Linux 命令；清除连接时一并清除命令
function onConnChange(v: string | undefined) {
  if (!v) {
    editing.value.shutdownConn = ''
    editing.value.shutdownCommand = ''
    return
  }
  if (!editing.value.shutdownCommand) {
    editing.value.shutdownCommand = presets[0].cmd
  }
}

// openEditor 打开编辑框：copy 为 true 时以 d 为模板新建副本
function openEditor(d?: Device, copy = false) {
  editing.value = d ? JSON.parse(JSON.stringify(d)) : empty()
  copyFrom.value = null
  if (d && copy) {
    copyFrom.value = { key: d.key ?? '', name: d.name }
    editing.value.key = ''
    editing.value.name = uniqueName(d.name, devices.value.map((x) => x.name))
    // 巴法云主题不能被两台设备同时使用，副本不沿用
    editing.value.bemfaTopic = ''
  }
  loadTargets()
  editorVisible.value = true
}

async function save() {
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/wol/devices/${key}`, body)
    } else {
      await createItem('/api/wol/devices', body, copyFrom.value)
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

async function shutdown(d: Device) {
  const k = d.key ?? ''
  stopping[k] = true
  try {
    await api.post(`/api/wol/devices/${k}/shutdown`)
    ElMessage.success(t('wol.shutdownDone'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    stopping[k] = false
  }
}

function openBemfa() {
  bemfaForm.value = { ...bemfa.value.settings }
  bemfaVisible.value = true
}

async function saveBemfa() {
  bemfaSaving.value = true
  try {
    bemfa.value = await api.put<BemfaView>('/api/wol/bemfa', bemfaForm.value)
    bemfaVisible.value = false
    ElMessage.success(t('common.saved'))
    // 连接在后台建立，稍后刷新状态
    setTimeout(load, 1500)
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    bemfaSaving.value = false
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
  loadTargets()
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
/* 按钮较多时窄屏会换行，换行后的“关机”“唤醒”与其他卡片一样靠右 */
.wol-card :deep(.entity-actions) {
  justify-content: flex-end;
}
.full {
  width: 100%;
}
.opt-meta {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.opt-warn {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-color-warning);
}
.form-help.warn {
  color: var(--el-color-warning);
}
.presets {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px 4px;
  width: 100%;
}
.presets .el-button {
  margin-left: 0;
}
.dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  margin-left: 6px;
  border-radius: 50%;
  background: var(--el-color-info);
}
.dot.ok {
  background: var(--el-color-success);
}
.dot.bad {
  background: var(--el-color-danger);
}
.bemfa-steps {
  margin-bottom: 16px;
}
.bemfa-steps ol {
  margin: 0;
  padding-left: 18px;
  line-height: 1.7;
}
</style>
