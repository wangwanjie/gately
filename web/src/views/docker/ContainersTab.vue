<template>
  <div>
    <div class="toolbar">
      <el-input v-model="keyword" :placeholder="t('docker.container.list.search')" clearable :prefix-icon="Search" class="t-search" />
      <el-radio-group v-model="stateFilter" class="t-filter">
        <el-radio-button value="all">{{ t('docker.container.list.all') }}</el-radio-button>
        <el-radio-button value="running">{{ t('common.running') }}</el-radio-button>
        <el-radio-button value="stopped">{{ t('common.stopped') }}</el-radio-button>
      </el-radio-group>
      <el-checkbox v-model="grouped">{{ t('docker.container.list.groupByProject') }}</el-checkbox>
      <CompactToggle v-model="compact" />
      <div class="t-ops">
        <el-button type="primary" :icon="Plus" @click="openForm()">{{ t('docker.container.list.newContainer') }}</el-button>
        <el-button :icon="SearchCheck" :loading="checkingAll" @click="checkAll">{{ t('docker.container.list.checkAll') }}</el-button>
        <el-button :icon="ArrowUpCircle" :disabled="!selected.length" @click="upgradeSelected">{{ selected.length ? t('docker.container.list.upgradeSelectedCount', { count: selected.length }) : t('docker.container.list.upgradeSelected') }}</el-button>
        <el-button :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <div v-loading="loading && !containers.length" class="groups">
      <el-empty v-if="!loading && !filtered.length" :description="t('docker.container.list.empty')" />
      <div v-for="g in groups" :key="g.key" class="group">
        <button v-if="grouped" type="button" class="g-head" @click="toggle(g.key)">
          <ChevronRight :size="16" class="chev" :class="{ open: !collapsed[g.key] }" />
          <span class="g-name">{{ g.name }}</span>
          <span class="g-count">{{ t('docker.container.list.groupCount', { total: g.items.length, running: g.items.filter((c) => c.state === 'running').length }) }}</span>
        </button>
        <div v-show="!grouped || !collapsed[g.key]" class="card-list" :class="{ 'is-compact': compact }">
          <ContainerCard v-for="c in g.items" :key="c.id" :c="c" :selected="selectedIds.has(c.id)" :hide-project="grouped" :compact="compact" @command="onCommand" @select="onSelect" />
        </div>
      </div>
    </div>

    <ContainerDetail v-model="detailVisible" :container="detailTarget" :initial-tab="detailTab" />
    <ContainerForm v-model="formVisible" :edit-id="formEditId" :edit-name="formEditName" @saved="load" />

    <el-dialog v-model="delVisible" :title="t('docker.container.list.deleteTitle')" width="440px" :fullscreen="false" append-to-body>
      <i18n-t keypath="docker.container.list.deleteText" tag="p" class="dlg-text">
        <template #name><b>{{ delTarget?.name }}</b></template>
      </i18n-t>
      <el-checkbox v-model="delVolumes">{{ t('docker.container.list.deleteVolumes') }}</el-checkbox>
      <template #footer>
        <el-button @click="delVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :loading="delBusy" @click="doDelete">{{ t('common.delete') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="commitVisible" :title="t('docker.container.list.commitTitle')" width="480px" :fullscreen="isMobile" append-to-body>
      <el-form :label-position="labelPosition" label-width="80px">
        <el-form-item :label="t('docker.container.list.repo')" required><el-input v-model="commitForm.repo" :placeholder="t('docker.container.list.repoPlaceholder')" class="mono" /></el-form-item>
        <el-form-item :label="t('docker.container.list.tag')"><el-input v-model="commitForm.tag" placeholder="latest" class="mono" /></el-form-item>
        <el-form-item :label="t('common.remark')"><el-input v-model="commitForm.comment" /></el-form-item>
        <el-form-item :label="t('docker.container.list.pauseContainer')"><el-switch v-model="commitForm.pause" /><span class="help">{{ t('docker.container.list.pauseHelp') }}</span></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="commitVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="commitBusy" @click="doCommit">{{ t('docker.container.list.commit') }}</el-button>
      </template>
    </el-dialog>
    <RestartOverlay ref="overlay" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowUpCircle, ChevronRight, Plus, RefreshCw, Search, SearchCheck } from 'lucide-vue-next'
import { api } from '../../api'
import { useFormLayout } from '../../composables/useIsMobile'
import CompactToggle from '../../components/CompactToggle.vue'
import { useCompactView } from '../../composables/useCompactView'
import RestartOverlay from '../../components/RestartOverlay.vue'
import ContainerCard from './ContainerCard.vue'
import ContainerDetail from './ContainerDetail.vue'
import ContainerForm from './ContainerForm.vue'
import { reportError } from './state'
import { trackTask } from './tasks'
import type { ContainerView } from './types'

const { t } = useI18n()

const { isMobile, labelPosition } = useFormLayout()
const containers = ref<ContainerView[]>([])
const loading = ref(false)
const keyword = ref('')
const stateFilter = ref('all')
const grouped = ref(true)
const compact = useCompactView('docker.containers')
const collapsed = reactive<Record<string, boolean>>({})
const selectedIds = ref(new Set<string>())
const checkingAll = ref(false)

const detailVisible = ref(false)
const detailTarget = ref<ContainerView>()
const detailTab = ref('overview')
const formVisible = ref(false)
const formEditId = ref<string>()
const formEditName = ref('')
const delVisible = ref(false)
const delTarget = ref<ContainerView>()
const delVolumes = ref(false)
const delBusy = ref(false)
const commitVisible = ref(false)
const commitTarget = ref<ContainerView>()
const commitBusy = ref(false)
const commitForm = reactive({ repo: '', tag: 'latest', comment: '', pause: true })

const selected = computed(() => Array.from(selectedIds.value))

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  return containers.value.filter((c) => {
    if (stateFilter.value === 'running' && c.state !== 'running') {
      return false
    }
    if (stateFilter.value === 'stopped' && c.state === 'running') {
      return false
    }
    return !k || c.name.toLowerCase().includes(k) || c.image.toLowerCase().includes(k) || (c.project ?? '').toLowerCase().includes(k)
  })
})

const groups = computed(() => {
  if (!grouped.value) {
    return filtered.value.length ? [{ key: '*', name: '', items: filtered.value }] : []
  }
  const map = new Map<string, ContainerView[]>()
  for (const c of filtered.value) {
    const key = c.project || ''
    if (!map.has(key)) {
      map.set(key, [])
    }
    map.get(key)!.push(c)
  }
  // 项目在前按名称排序，独立容器放最后
  const keys = Array.from(map.keys()).sort((a, b) => {
    if (a === '' || b === '') {
      return a === '' ? 1 : -1
    }
    return a.localeCompare(b)
  })
  return keys.map((key) => ({ key: key || '__none', name: key || t('docker.container.list.standalone'), items: map.get(key)! }))
})

function toggle(key: string) {
  collapsed[key] = !collapsed[key]
}

function onSelect(id: string, v: boolean) {
  const s = new Set(selectedIds.value)
  if (v) {
    s.add(id)
  } else {
    s.delete(id)
  }
  selectedIds.value = s
}

async function load() {
  loading.value = true
  try {
    containers.value = (await api.get<ContainerView[]>('/api/docker/containers')) ?? []
    reportError('')
    const alive = new Set(containers.value.map((c) => c.id))
    selectedIds.value = new Set(Array.from(selectedIds.value).filter((id) => alive.has(id)))
  } catch (e) {
    reportError((e as Error).message)
  } finally {
    loading.value = false
  }
}

function openForm(c?: ContainerView) {
  formEditId.value = c?.id
  formEditName.value = c?.name ?? ''
  formVisible.value = true
}

function openDetail(c: ContainerView, tab: string) {
  detailTarget.value = c
  detailTab.value = tab
  detailVisible.value = true
}

async function simpleAction(c: ContainerView, action: string) {
  try {
    await api.post(`/api/docker/containers/${c.id}/${action}`)
    ElMessage.success(t('docker.container.list.executed'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function confirmThen(msg: string, title: string, fn: () => Promise<void>) {
  try {
    await ElMessageBox.confirm(msg, title, { type: 'warning' })
  } catch {
    return
  }
  await fn()
}

async function onCommand(c: ContainerView, cmd: string) {
  switch (cmd) {
    case 'start':
    case 'restart':
    case 'pause':
    case 'unpause':
      await simpleAction(c, cmd)
      break
    case 'stop':
      await confirmThen(t('docker.container.list.stopConfirm', { name: c.name }), t('docker.container.list.stopTitle'), () => simpleAction(c, 'stop'))
      break
    case 'kill':
      await confirmThen(t('docker.container.list.killConfirm', { name: c.name }), t('docker.container.list.killTitle'), () => simpleAction(c, 'kill'))
      break
    case 'detail':
      openDetail(c, 'overview')
      break
    case 'logs':
    case 'term':
    case 'files':
      openDetail(c, cmd)
      break
    case 'edit':
      openForm(c)
      break
    case 'rename':
      await rename(c)
      break
    case 'commit':
      commitTarget.value = c
      commitForm.repo = c.image.split(':')[0].replace(/^sha256$/, '')
      commitForm.tag = 'latest'
      commitForm.comment = ''
      commitForm.pause = true
      commitVisible.value = true
      break
    case 'check':
      await check(c)
      break
    case 'upgrade':
      await upgrade(c)
      break
    case 'delete':
      delTarget.value = c
      delVolumes.value = false
      delVisible.value = true
      break
  }
}

async function rename(c: ContainerView) {
  try {
    const { value } = await ElMessageBox.prompt(t('docker.container.list.renamePrompt'), t('docker.container.list.renameTitle'), { inputValue: c.name, inputPattern: /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/, inputErrorMessage: t('docker.container.list.renameInvalid') })
    await api.post(`/api/docker/containers/${c.id}/rename`, { name: value })
    await load()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error((e as Error).message)
    }
  }
}

async function doDelete() {
  if (!delTarget.value) {
    return
  }
  delBusy.value = true
  try {
    await api.del(`/api/docker/containers/${delTarget.value.id}${delVolumes.value ? '?volumes=1' : ''}`)
    delVisible.value = false
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    delBusy.value = false
  }
}

async function doCommit() {
  if (!commitTarget.value || !commitForm.repo.trim()) {
    ElMessage.warning(t('docker.container.list.repoRequired'))
    return
  }
  commitBusy.value = true
  try {
    await api.post(`/api/docker/containers/${commitTarget.value.id}/commit`, { repo: commitForm.repo.trim(), tag: commitForm.tag.trim() || 'latest', comment: commitForm.comment, pause: commitForm.pause })
    ElMessage.success(t('docker.container.list.committed'))
    commitVisible.value = false
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    commitBusy.value = false
  }
}

async function check(c: ContainerView) {
  c.busy = true
  try {
    await api.post(`/api/docker/containers/${c.id}/check`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    c.busy = false
  }
}

// overlay 是升级 Gately 自身后的重启等待层
const overlay = ref<InstanceType<typeof RestartOverlay>>()

async function upgrade(c: ContainerView) {
  const msg = c.self ? t('docker.container.list.selfUpgradeConfirm', { name: c.name }) : t('docker.container.list.upgradeConfirm', { name: c.name })
  await confirmThen(msg, t('docker.container.list.upgradeTitle'), async () => {
    c.busy = true
    try {
      const r = await api.post<{ self?: boolean; upToDate?: boolean }>(`/api/docker/containers/${c.id}/upgrade`)
      if (r?.self) {
        // 自身由辅助容器重建：已是最新时不会重启，否则等待 Gately 重启后刷新页面
        if (r.upToDate) {
          ElMessage.info(t('docker.container.list.selfUpToDate'))
          await load()
        } else {
          overlay.value?.start(t('docker.container.list.selfUpgradeRestarting'))
        }
        return
      }
      ElMessage.success(t('docker.container.list.upgradeDone'))
      await load()
    } catch (e) {
      ElMessage.error((e as Error).message)
    } finally {
      c.busy = false
    }
  })
}

async function checkAll() {
  checkingAll.value = true
  try {
    const r = await api.post<{ task: string }>('/api/docker/containers/check-all')
    trackTask(r.task, t('docker.container.list.checkAllTask'), () => {
      checkingAll.value = false
      load()
    })
  } catch (e) {
    checkingAll.value = false
    ElMessage.error((e as Error).message)
  }
}

async function upgradeSelected() {
  const withSelf = containers.value.some((c) => c.self && selectedIds.value.has(c.id))
  let msg = t('docker.container.list.batchConfirm', { count: selected.value.length })
  if (withSelf) {
    msg += ' ' + t('docker.container.list.batchSelfNote')
  }
  await confirmThen(msg, t('docker.container.list.batchTitle'), async () => {
    try {
      const r = await api.post<{ task: string }>('/api/docker/containers/upgrade-batch', { ids: selected.value })
      trackTask(r.task, t('docker.container.list.batchTask'), (task) => {
        // 包含自身且任务成功时 Gately 会随后重启（自身已是最新时等待层几秒后直接刷新）
        if (withSelf && task.status === 'done') {
          overlay.value?.start(t('docker.container.list.selfUpgradeRestarting'))
          return
        }
        load()
      })
    } catch (e) {
      ElMessage.error((e as Error).message)
    }
  })
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
.group + .group {
  margin-top: 16px;
}
.g-head {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 40px;
  padding: 4px 4px;
  margin-bottom: 8px;
  border: none;
  background: transparent;
  color: var(--el-text-color-primary);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.chev {
  transition: transform 0.15s;
  color: var(--el-text-color-secondary);
}
.chev.open {
  transform: rotate(90deg);
}
.g-name {
  font-weight: 600;
  word-break: break-all;
}
.g-count {
  font-size: 12px;
  color: var(--el-text-color-secondary);
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
  .t-filter {
    width: 100%;
  }
  .t-filter :deep(.el-radio-button) {
    flex: 1;
  }
  .t-filter :deep(.el-radio-button__inner) {
    width: 100%;
    min-height: 40px;
    line-height: 26px;
  }
  .t-ops {
    width: 100%;
    margin-left: 0;
  }
  .t-ops .el-button {
    flex: 1 1 40%;
    min-height: 40px;
  }
  .toolbar :deep(input) {
    font-size: 16px;
  }
}
</style>
