<template>
  <div>
    <div class="toolbar g-sticky-bar g-sticky-desktop">
      <el-input v-model="keyword" :placeholder="t('docker.volume.search')" clearable :prefix-icon="Search" class="t-search" />
      <CompactToggle v-model="compact" />
      <div class="t-ops">
        <el-button type="primary" :icon="Plus" @click="openCreate">{{ t('docker.volume.create') }}</el-button>
        <el-button :icon="PackageOpen" @click="openImport()">{{ t('common.import') }}</el-button>
        <el-button :icon="Eraser" @click="prune">{{ t('docker.volume.pruneUnused') }}</el-button>
        <el-button :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <div v-loading="loading && !volumes.length" class="card-list" :class="{ 'is-compact': compact }">
      <el-empty v-if="!loading && !filtered.length" :description="t('docker.volume.empty')" />
      <EntityCard :compact="compact" v-for="v in filtered" :key="v.name">
        <template #lead>
          <div class="ico"><Database :size="18" /></div>
        </template>
        <template #title><span class="name">{{ v.name }}</span></template>
        <template #sub>{{ formatTime(v.created) }}</template>
        <template #status>
          <StatusBadge v-if="v.refCount > 0" type="ok" :text="t('docker.volume.usedBy', { count: v.refCount })" />
          <StatusBadge v-else type="off" :text="t('docker.volume.unused')" />
        </template>
        <Field :label="t('docker.volume.size')">{{ formatSize(v.size) }}</Field>
        <Field :label="t('docker.volume.driver')">{{ v.driver }}</Field>
        <Field :label="t('docker.volume.mountpoint')" wide mono>{{ v.mountpoint }}</Field>
        <Field :label="t('docker.volume.users')" wide>
          <el-tag v-for="u in v.usedBy ?? []" :key="u.id" size="small" type="info" class="used">{{ u.name }}</el-tag>
          <span v-if="!(v.usedBy ?? []).length" class="muted">{{ t('common.none') }}</span>
        </Field>
        <template #actions>
          <el-button text :icon="Download" class="btn" @click="download(`/api/docker/volumes/${encodeURIComponent(v.name)}/export`)">{{ t('docker.volume.export') }}</el-button>
          <el-button text :icon="Upload" class="btn" @click="openImport(v.name)">{{ t('common.import') }}</el-button>
          <el-button text type="danger" :icon="Trash2" class="btn" @click="openDelete(v)">{{ t('common.delete') }}</el-button>
        </template>
      </EntityCard>
    </div>

    <el-dialog v-model="createVisible" :title="t('docker.volume.create')" width="440px" :fullscreen="isMobile" append-to-body>
      <el-form :label-position="labelPosition" label-width="70px" @submit.prevent>
        <el-form-item :label="t('common.name')" required><el-input v-model="cf.name" class="mono" /></el-form-item>
        <el-form-item :label="t('docker.volume.driver')"><el-input v-model="cf.driver" placeholder="local" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="busy" @click="create">{{ t('docker.volume.createBtn') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importVisible" :title="t('docker.volume.importTitle')" width="480px" :fullscreen="isMobile" append-to-body>
      <el-form :label-position="labelPosition" label-width="70px" @submit.prevent>
        <el-form-item :label="t('docker.volume.volumeName')" required>
          <el-input v-model="importName" :placeholder="t('docker.volume.importNamePlaceholder')" class="mono" />
        </el-form-item>
        <el-form-item :label="t('docker.volume.file')" required>
          <div class="full">
            <el-button :icon="Upload" @click="picker?.click()">{{ importFile ? importFile.name : t('docker.volume.pickFile') }}</el-button>
            <input ref="picker" type="file" accept=".gz,.tgz,application/gzip" class="hidden" @change="(e) => (importFile = (e.target as HTMLInputElement).files?.[0])" />
            <el-progress v-if="busy" :percentage="importPercent" class="gap-t" />
          </div>
        </el-form-item>
      </el-form>
      <div class="help block">{{ t('docker.volume.importHint') }}</div>
      <template #footer>
        <el-button @click="importVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="busy" @click="doImport">{{ t('common.import') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="delVisible" :title="t('docker.volume.deleteTitle')" width="440px" append-to-body>
      <i18n-t :keypath="delTarget?.refCount ? 'docker.volume.deleteTextUsed' : 'docker.volume.deleteText'" tag="p" class="dlg-text">
        <template #name><b class="mono">{{ delTarget?.name }}</b></template>
        <template #count>{{ delTarget?.refCount ?? 0 }}</template>
      </i18n-t>
      <el-checkbox v-model="delForce">{{ t('docker.volume.forceDelete') }}</el-checkbox>
      <template #footer>
        <el-button @click="delVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :loading="busy" @click="doDelete">{{ t('common.delete') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Database, Download, Eraser, PackageOpen, Plus, RefreshCw, Search, Trash2, Upload } from 'lucide-vue-next'
import { api } from '../../api'
import CompactToggle from '../../components/CompactToggle.vue'
import EntityCard from '../../components/EntityCard.vue'
import { useCompactView } from '../../composables/useCompactView'
import Field from '../../components/Field.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { useFormLayout } from '../../composables/useIsMobile'
import { download, formatBytes, formatSize, formatTime, uploadFile } from './format'
import { reportError } from './state'
import type { VolumeView } from './types'

const { t } = useI18n()
const compact = useCompactView('docker.volumes')

const { isMobile, labelPosition } = useFormLayout()
const volumes = ref<VolumeView[]>([])
const loading = ref(false)
const keyword = ref('')
const busy = ref(false)
const createVisible = ref(false)
const cf = reactive({ name: '', driver: 'local' })
const importVisible = ref(false)
const importName = ref('')
const importFile = ref<File>()
const importPercent = ref(0)
const picker = ref<HTMLInputElement>()
const delVisible = ref(false)
const delTarget = ref<VolumeView>()
const delForce = ref(false)

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  return k ? volumes.value.filter((v) => v.name.toLowerCase().includes(k)) : volumes.value
})

async function load() {
  loading.value = true
  try {
    volumes.value = (await api.get<VolumeView[]>('/api/docker/volumes')) ?? []
    reportError('')
  } catch (e) {
    reportError((e as Error).message)
  } finally {
    loading.value = false
  }
}

function openCreate() {
  cf.name = ''
  cf.driver = 'local'
  createVisible.value = true
}

async function create() {
  if (!cf.name.trim()) {
    ElMessage.warning(t('docker.volume.nameRequired'))
    return
  }
  busy.value = true
  try {
    await api.post('/api/docker/volumes', { name: cf.name.trim(), driver: cf.driver.trim() || 'local' })
    createVisible.value = false
    ElMessage.success(t('docker.volume.created'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

function openImport(name = '') {
  importName.value = name
  importFile.value = undefined
  importPercent.value = 0
  importVisible.value = true
}

async function doImport() {
  if (!importName.value.trim() || !importFile.value) {
    ElMessage.warning(t('docker.volume.importRequired'))
    return
  }
  busy.value = true
  try {
    await uploadFile(`/api/docker/volumes/${encodeURIComponent(importName.value.trim())}/import`, importFile.value, 'application/gzip', (p) => (importPercent.value = p))
    importVisible.value = false
    ElMessage.success(t('docker.volume.imported'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

function openDelete(v: VolumeView) {
  delTarget.value = v
  delForce.value = false
  delVisible.value = true
}

async function doDelete() {
  if (!delTarget.value) {
    return
  }
  busy.value = true
  try {
    await api.del(`/api/docker/volumes/${encodeURIComponent(delTarget.value.name)}${delForce.value ? '?force=1' : ''}`)
    delVisible.value = false
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

async function prune() {
  try {
    await ElMessageBox.confirm(t('docker.volume.pruneConfirm'), t('docker.volume.pruneTitle'), { type: 'warning' })
    const r = await api.post<{ deleted: number; spaceReclaimed: number }>('/api/docker/prune', { target: 'volumes' })
    ElMessage.success(t('docker.volume.pruned', { count: r.deleted, size: formatBytes(r.spaceReclaimed) }))
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
.name {
  font-family: var(--g-mono);
  font-size: 14px;
}
.used {
  margin: 0 4px 4px 0;
}
.hidden {
  display: none;
}
.full {
  width: 100%;
}
.gap-t {
  margin-top: 8px;
}
.dlg-text {
  margin: 0 0 12px;
  line-height: 1.6;
  word-break: break-all;
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
    flex: 1 1 40%;
    min-height: 40px;
  }
  .btn {
    min-height: 40px;
  }
  .toolbar :deep(input),
  :deep(.el-dialog input) {
    font-size: 16px;
  }
}
</style>
