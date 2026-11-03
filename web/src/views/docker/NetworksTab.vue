<template>
  <div>
    <div class="toolbar g-sticky-bar g-sticky-desktop">
      <el-input v-model="keyword" :placeholder="t('docker.network.search')" clearable :prefix-icon="Search" class="t-search" />
      <CompactToggle v-model="compact" />
      <div class="t-ops">
        <el-button type="primary" :icon="Plus" @click="openCreate">{{ t('docker.network.create') }}</el-button>
        <el-button :icon="Eraser" @click="prune">{{ t('docker.network.pruneUnused') }}</el-button>
        <el-button :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <div v-loading="loading && !networks.length" class="card-list" :class="{ 'is-compact': compact }">
      <el-empty v-if="!loading && !filtered.length" :description="t('docker.network.empty')" />
      <EntityCard :compact="compact" v-for="n in filtered" :key="n.id">
        <template #lead>
          <div class="ico"><Network :size="18" /></div>
        </template>
        <template #title>
          <span>{{ n.name }}</span>
          <el-tag v-if="n.builtin" size="small" type="info">{{ t('docker.network.builtin') }}</el-tag>
          <el-tag v-if="n.internal" size="small" type="warning">{{ t('docker.network.internal') }}</el-tag>
          <el-tag v-if="n.ipv6" size="small">IPv6</el-tag>
        </template>
        <template #sub><span class="mono">{{ n.id.slice(0, 12) }}</span></template>
        <template #status><StatusBadge type="info" :text="n.driver" /></template>
        <Field :label="t('docker.network.scope')">{{ n.scope }}</Field>
        <Field :label="t('docker.network.subnet')" mono>{{ n.subnet || '-' }}</Field>
        <Field :label="t('docker.network.gateway')" mono>{{ n.gateway || '-' }}</Field>
        <Field :label="t('docker.network.connected')" wide>
          <div v-for="c in n.containers ?? []" :key="c.id" class="conn">
            <span class="c-name">{{ c.name }}</span>
            <span class="mono muted">{{ c.ip }}</span>
            <el-button text size="small" type="danger" :icon="Unplug" class="c-btn" :aria-label="t('docker.network.disconnect')" @click="disconnect(n, c)">{{ t('docker.network.disconnect') }}</el-button>
          </div>
          <span v-if="!(n.containers ?? []).length" class="muted">{{ t('common.none') }}</span>
        </Field>
        <template #actions>
          <el-button text :icon="Plug" class="btn" @click="openConnect(n)">{{ t('docker.network.connectContainer') }}</el-button>
          <el-button text type="danger" :icon="Trash2" class="btn" :disabled="n.builtin" :title="n.builtin ? t('docker.network.builtinNoDelete') : ''" @click="remove(n)">{{ t('common.delete') }}</el-button>
        </template>
      </EntityCard>
    </div>

    <el-dialog v-model="createVisible" :title="t('docker.network.create')" width="500px" :fullscreen="isMobile" append-to-body>
      <el-form :label-position="labelPosition" label-width="80px" @submit.prevent>
        <el-form-item :label="t('common.name')" required><el-input v-model="cf.name" /></el-form-item>
        <el-form-item :label="t('docker.network.driver')">
          <el-select v-model="cf.driver" class="full">
            <el-option label="bridge" value="bridge" />
            <el-option label="macvlan" value="macvlan" />
            <el-option label="ipvlan" value="ipvlan" />
            <el-option label="overlay" value="overlay" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('docker.network.subnet')"><el-input v-model="cf.subnet" placeholder="172.30.0.0/16" class="mono" /></el-form-item>
        <el-form-item :label="t('docker.network.gateway')"><el-input v-model="cf.gateway" placeholder="172.30.0.1" class="mono" /></el-form-item>
        <el-form-item :label="t('docker.network.options')">
          <div class="opts">
            <el-checkbox v-model="cf.internal">{{ t('docker.network.internalNet') }}</el-checkbox>
            <el-checkbox v-model="cf.attachable">{{ t('docker.network.attachable') }}</el-checkbox>
            <el-checkbox v-model="cf.ipv6">{{ t('docker.network.enableIpv6') }}</el-checkbox>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="busy" @click="create">{{ t('docker.network.createBtn') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="connVisible" :title="t('docker.network.connectTitle', { name: connTarget?.name })" width="480px" :fullscreen="isMobile" append-to-body>
      <el-form :label-position="labelPosition" label-width="80px" @submit.prevent>
        <el-form-item :label="t('docker.network.container')" required>
          <el-select v-model="connForm.container" filterable class="full" :placeholder="t('docker.network.selectContainer')">
            <el-option v-for="c in connChoices" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('docker.network.fixedIp')"><el-input v-model="connForm.ip" :placeholder="t('docker.network.optional')" class="mono" /></el-form-item>
        <el-form-item :label="t('docker.network.aliases')">
          <el-select v-model="connForm.aliases" multiple filterable allow-create default-first-option :reserve-keyword="false" class="full" :placeholder="t('docker.network.aliasPlaceholder')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="connVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="busy" @click="connect">{{ t('docker.network.connect') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Eraser, Network, Plug, Plus, RefreshCw, Search, Trash2, Unplug } from 'lucide-vue-next'
import { api } from '../../api'
import CompactToggle from '../../components/CompactToggle.vue'
import EntityCard from '../../components/EntityCard.vue'
import { useCompactView } from '../../composables/useCompactView'
import Field from '../../components/Field.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { useFormLayout } from '../../composables/useIsMobile'
import { formatBytes } from './format'
import { reportError } from './state'
import type { ContainerView, NetworkView } from './types'

const { t } = useI18n()
const compact = useCompactView('docker.networks')

const { isMobile, labelPosition } = useFormLayout()
const networks = ref<NetworkView[]>([])
const loading = ref(false)
const keyword = ref('')
const busy = ref(false)
const createVisible = ref(false)
const cf = reactive({ name: '', driver: 'bridge', subnet: '', gateway: '', internal: false, attachable: false, ipv6: false })
const connVisible = ref(false)
const connTarget = ref<NetworkView>()
const connChoices = ref<ContainerView[]>([])
const connForm = reactive({ container: '', ip: '', aliases: [] as string[] })

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  return k ? networks.value.filter((n) => n.name.toLowerCase().includes(k) || n.subnet.includes(k)) : networks.value
})

async function load() {
  loading.value = true
  try {
    networks.value = (await api.get<NetworkView[]>('/api/docker/networks')) ?? []
    reportError('')
  } catch (e) {
    reportError((e as Error).message)
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(cf, { name: '', driver: 'bridge', subnet: '', gateway: '', internal: false, attachable: false, ipv6: false })
  createVisible.value = true
}

async function create() {
  if (!cf.name.trim()) {
    ElMessage.warning(t('docker.network.nameRequired'))
    return
  }
  busy.value = true
  try {
    await api.post('/api/docker/networks', { ...cf, name: cf.name.trim(), subnet: cf.subnet.trim(), gateway: cf.gateway.trim() })
    createVisible.value = false
    ElMessage.success(t('docker.network.created'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

async function remove(n: NetworkView) {
  if (n.builtin) {
    return
  }
  try {
    await ElMessageBox.confirm(t('docker.network.deleteConfirm', { name: n.name }), t('docker.network.deleteTitle'), { type: 'warning' })
    await api.del(`/api/docker/networks/${n.id}`)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error((e as Error).message)
    }
  }
}

async function openConnect(n: NetworkView) {
  connTarget.value = n
  connForm.container = ''
  connForm.ip = ''
  connForm.aliases = []
  connVisible.value = true
  try {
    const all = (await api.get<ContainerView[]>('/api/docker/containers')) ?? []
    const joined = new Set((n.containers ?? []).map((c) => c.id))
    connChoices.value = all.filter((c) => !joined.has(c.id))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function connect() {
  if (!connTarget.value || !connForm.container) {
    ElMessage.warning(t('docker.network.containerRequired'))
    return
  }
  busy.value = true
  try {
    await api.post(`/api/docker/networks/${connTarget.value.id}/connect`, { container: connForm.container, aliases: connForm.aliases, ip: connForm.ip.trim() })
    connVisible.value = false
    ElMessage.success(t('docker.network.connected2'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

async function disconnect(n: NetworkView, c: { id: string; name: string }) {
  try {
    await ElMessageBox.confirm(t('docker.network.disconnectConfirm', { container: c.name, network: n.name }), t('docker.network.disconnectTitle'), { type: 'warning' })
    await api.post(`/api/docker/networks/${n.id}/disconnect`, { container: c.id, force: true })
    await load()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error((e as Error).message)
    }
  }
}

async function prune() {
  try {
    await ElMessageBox.confirm(t('docker.network.pruneConfirm'), t('docker.network.pruneTitle'), { type: 'warning' })
    const r = await api.post<{ deleted: number; spaceReclaimed: number }>('/api/docker/prune', { target: 'networks' })
    ElMessage.success(r.spaceReclaimed ? t('docker.network.prunedReclaimed', { count: r.deleted, size: formatBytes(r.spaceReclaimed) }) : t('docker.network.pruned', { count: r.deleted }))
    await load()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error((e as Error).message)
    }
  }
}

defineExpose({ reload: load })
onMounted(load)
</script>

<style scoped>
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  margin-bottom: 12px;
}
.t-search {
  width: 240px;
}
.t-ops {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-left: auto;
}
.t-ops .el-button + .el-button {
  margin-left: 0;
}
.ico {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}
.conn {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px 10px;
  padding: 2px 0;
}
.c-name {
  font-weight: 500;
}
.full {
  width: 100%;
}
.opts {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
}
@media (max-width: 767.98px) {
  .t-search {
    width: 100%;
  }
  .t-ops {
    width: 100%;
    margin-left: 0;
  }
  .t-ops .el-button {
    flex: 1 1 28%;
    min-height: 40px;
  }
  .btn,
  .c-btn {
    min-height: 40px;
  }
  .toolbar :deep(input),
  :deep(.el-dialog input) {
    font-size: 16px;
  }
}
</style>
