<template>
  <el-drawer
    :model-value="modelValue"
    :size="isMobile ? '100%' : '860px'"
    direction="rtl"
    destroy-on-close
    append-to-body
    class="ct-drawer"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
  >
    <template #header>
      <div class="dh">
        <span class="dh-name">{{ container?.name }}</span>
        <StatusBadge v-if="container" v-bind="containerBadge(container.state, container.status)" />
      </div>
    </template>
    <el-tabs v-model="tab" class="tabs">
      <el-tab-pane :label="t('docker.container.detail.overview')" name="overview" />
      <el-tab-pane :label="t('docker.container.detail.logs')" name="logs" />
      <el-tab-pane :label="t('docker.container.detail.terminal')" name="term" :disabled="!running" />
      <el-tab-pane :label="t('docker.container.detail.stats')" name="stats" :disabled="!running" />
      <el-tab-pane :label="t('docker.container.detail.files')" name="files" />
    </el-tabs>
    <div v-if="container" class="pane">
      <ContainerOverview v-if="tab === 'overview'" :key="container.id + container.state" :container-id="container.id" />
      <ContainerLogs v-else-if="tab === 'logs'" :key="container.id" :container-id="container.id" />
      <ContainerTerm v-else-if="tab === 'term'" :key="container.id" :container-id="container.id" />
      <ContainerStats v-else-if="tab === 'stats'" :key="container.id" :container-id="container.id" />
      <ContainerFiles v-else-if="tab === 'files'" :key="container.id" :container-id="container.id" />
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import StatusBadge from '../../components/StatusBadge.vue'
import { useIsMobile } from '../../composables/useIsMobile'
import { containerBadge } from './format'
import ContainerOverview from './ContainerOverview.vue'
import ContainerLogs from './ContainerLogs.vue'
import ContainerTerm from './ContainerTerm.vue'
import ContainerStats from './ContainerStats.vue'
import ContainerFiles from './ContainerFiles.vue'
import type { ContainerView } from './types'

const { t } = useI18n()

const props = defineProps<{ modelValue: boolean; container?: ContainerView; initialTab?: string }>()
const emit = defineEmits<{ 'update:modelValue': [v: boolean] }>()

const isMobile = useIsMobile()
const tab = ref('overview')
const running = computed(() => props.container?.state === 'running')

// 每次打开时回到指定标签
watch(
  () => props.modelValue,
  (v) => {
    if (v) {
      tab.value = props.initialTab || 'overview'
    }
  },
)
</script>

<style scoped>
.dh {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.dh-name {
  font-size: 16px;
  font-weight: 600;
  word-break: break-all;
}
.tabs :deep(.el-tabs__header) {
  margin-bottom: 12px;
}
.tabs :deep(.el-tabs__nav-wrap) {
  overflow-x: auto;
}
.pane {
  min-width: 0;
}
</style>
<style>
.ct-drawer .el-drawer__body {
  padding-top: 0;
  padding-bottom: calc(16px + env(safe-area-inset-bottom));
}
@media (max-width: 767.98px) {
  .ct-drawer .el-drawer__body {
    padding-left: 12px;
    padding-right: 12px;
  }
}
</style>
