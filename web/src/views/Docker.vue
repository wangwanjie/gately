<template>
  <div>
    <PageHeader title="Docker" doc="modules/docker" :desc="t('docker.page.desc')" :icon="Container">
      <el-button :icon="RefreshCw" :loading="refreshing" @click="refresh">{{ t('common.refresh') }}</el-button>
    </PageHeader>

    <el-alert v-if="dockerError" type="error" :title="t('docker.page.connectFailed')" :closable="false" show-icon class="err">
      <template #default>
        <div class="err-detail mono">{{ dockerError }}</div>
        <div>{{ t('docker.page.connectHint') }}</div>
        <el-button v-if="tab !== 'system'" size="small" class="err-btn" :icon="SlidersHorizontal" @click="tab = 'system'">{{ t('docker.page.openSettings') }}</el-button>
      </template>
    </el-alert>

    <el-tabs v-model="tab" class="docker-tabs">
      <el-tab-pane :label="t('docker.page.tabContainers')" name="containers" lazy><ContainersTab :ref="(el: unknown) => setRef('containers', el)" /></el-tab-pane>
      <el-tab-pane label="Compose" name="compose" lazy><ComposeTab :ref="(el: unknown) => setRef('compose', el)" /></el-tab-pane>
      <el-tab-pane :label="t('docker.page.tabImages')" name="images" lazy><ImagesTab :ref="(el: unknown) => setRef('images', el)" /></el-tab-pane>
      <el-tab-pane :label="t('docker.page.tabNetworks')" name="networks" lazy><NetworksTab :ref="(el: unknown) => setRef('networks', el)" /></el-tab-pane>
      <el-tab-pane :label="t('docker.page.tabVolumes')" name="volumes" lazy><VolumesTab :ref="(el: unknown) => setRef('volumes', el)" /></el-tab-pane>
      <el-tab-pane :label="t('docker.page.tabSystem')" name="system" lazy><SystemTab :ref="(el: unknown) => setRef('system', el)" @saved="loadInfo" /></el-tab-pane>
    </el-tabs>

    <TaskPanel />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Container, RefreshCw, SlidersHorizontal } from 'lucide-vue-next'
import { api } from '../api'
import PageHeader from '../components/PageHeader.vue'
import ComposeTab from './docker/ComposeTab.vue'
import ContainersTab from './docker/ContainersTab.vue'
import ImagesTab from './docker/ImagesTab.vue'
import NetworksTab from './docker/NetworksTab.vue'
import SystemTab from './docker/SystemTab.vue'
import TaskPanel from './docker/TaskPanel.vue'
import VolumesTab from './docker/VolumesTab.vue'
import { dockerError, dockerInfo, reportError } from './docker/state'
import { resumeRunningTasks } from './docker/tasks'
import type { DockerInfo } from './docker/types'

interface Reloadable {
  reload?: () => unknown
}

const { t } = useI18n()
const tab = ref('containers')
const refreshing = ref(false)
// 各标签页实例，用于顶部“刷新”按钮触发当前标签重新加载
const refs: Record<string, Reloadable | null> = {}

function setRef(name: string, el: unknown) {
  refs[name] = el as Reloadable | null
}

async function loadInfo() {
  try {
    dockerInfo.value = await api.get<DockerInfo>('/api/docker/info')
    reportError('')
  } catch (e) {
    reportError((e as Error).message)
  }
}

async function refresh() {
  refreshing.value = true
  try {
    await loadInfo()
    await refs[tab.value]?.reload?.()
  } finally {
    refreshing.value = false
  }
}

onMounted(() => {
  loadInfo()
  resumeRunningTasks()
})
</script>

<style scoped>
.err {
  margin-bottom: 16px;
}
.err-detail {
  font-size: 12px;
  word-break: break-all;
  margin: 2px 0 4px;
  opacity: 0.85;
}
.err-btn {
  margin-top: 8px;
}
.docker-tabs :deep(.el-tabs__header) {
  margin-bottom: 14px;
}
.docker-tabs :deep(.el-tabs__nav-wrap) {
  overflow-x: auto;
}
.docker-tabs :deep(.el-tabs__nav-wrap::after) {
  background-color: var(--g-border);
}
@media (max-width: 767.98px) {
  .docker-tabs :deep(.el-tabs__item) {
    min-height: 44px;
    line-height: 44px;
    padding: 0 14px;
  }
}
</style>
