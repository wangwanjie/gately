<template>
  <EntityCard :off="c.state !== 'running'" :compact="compact">
    <template #lead>
      <el-checkbox :model-value="selected" class="sel" @update:model-value="(v: unknown) => emit('select', c.id, !!v)" />
    </template>
    <template #title>
      <span>{{ c.name }}</span>
      <el-tag v-if="c.project && !hideProject" size="small" type="info">{{ c.project }}</el-tag>
    </template>
    <template #sub>{{ c.status }}</template>
    <template #status>
      <StatusBadge v-bind="containerBadge(c.state)" />
    </template>
    <Field :label="t('docker.container.card.image')" mono>
      {{ c.image }}
      <div v-if="c.update" class="upd">
        <StatusBadge v-if="c.update.error" type="error" :text="t('docker.container.card.checkFailed')" :title="c.update.error" />
        <StatusBadge v-else-if="c.update.hasUpdate" type="warn" :text="t('docker.container.card.hasUpdate')" />
        <StatusBadge v-else type="ok" :text="t('docker.container.card.upToDate')" />
      </div>
    </Field>
    <Field :label="t('docker.container.card.ports')" mono>
      <div v-for="p in c.ports ?? []" :key="p">
        <a v-if="isTcpPort(p)" href="javascript:void(0)" class="port" @click.prevent="openPort(p)">{{ p }}</a>
        <span v-else>{{ p }}</span>
      </div>
      <span v-if="!(c.ports ?? []).length" class="muted">-</span>
    </Field>
    <template #actions>
      <el-button v-if="c.state !== 'running'" text type="primary" :icon="Play" class="btn" :loading="c.busy" @click="emit('command', c, 'start')">{{ t('common.start') }}</el-button>
      <el-button v-else text type="danger" :icon="Square" class="btn" :disabled="c.self" :title="c.self ? t('docker.container.card.selfHint') : ''" @click="emit('command', c, 'stop')">{{ t('common.stop') }}</el-button>
      <el-button v-if="c.state === 'running'" text :icon="RotateCw" class="btn" @click="emit('command', c, 'restart')">{{ t('common.restart') }}</el-button>
      <el-button text :icon="PanelRight" class="btn" @click="emit('command', c, 'detail')">{{ t('common.details') }}</el-button>
      <el-dropdown trigger="click" @command="(cmd: string) => emit('command', c, cmd)">
        <el-button text :icon="Ellipsis" class="btn">{{ t('common.more') }}</el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="logs" :icon="ScrollText">{{ t('docker.container.card.logs') }}</el-dropdown-item>
            <el-dropdown-item v-if="c.state === 'running'" command="term" :icon="SquareTerminal">{{ t('docker.container.card.terminal') }}</el-dropdown-item>
            <el-dropdown-item command="files" :icon="FolderOpen">{{ t('docker.container.card.files') }}</el-dropdown-item>
            <el-dropdown-item v-if="c.state === 'running' && !c.self" command="pause" :icon="Pause" divided>{{ t('docker.container.card.pause') }}</el-dropdown-item>
            <el-dropdown-item v-if="c.state === 'paused'" command="unpause" :icon="Play" divided>{{ t('docker.container.card.unpause') }}</el-dropdown-item>
            <el-dropdown-item v-if="(c.state === 'running' || c.state === 'paused') && !c.self" command="kill" :icon="Zap">{{ t('docker.container.card.kill') }}</el-dropdown-item>
            <el-dropdown-item command="edit" :icon="Pencil" divided :disabled="c.self" :title="c.self ? t('docker.container.card.selfRebuildHint') : ''">{{ t('common.edit') }}</el-dropdown-item>
            <el-dropdown-item command="rename" :icon="TextCursorInput">{{ t('docker.container.card.rename') }}</el-dropdown-item>
            <el-dropdown-item command="commit" :icon="GitCommitHorizontal">{{ t('docker.container.card.commit') }}</el-dropdown-item>
            <el-dropdown-item command="check" :icon="SearchCheck" divided>{{ t('docker.container.card.checkUpdate') }}</el-dropdown-item>
            <!-- 升级、编辑都会先停止旧容器，自身所在的容器只能在宿主机上更新 -->
            <el-dropdown-item v-if="c.update?.hasUpdate" command="upgrade" :icon="ArrowUpCircle">{{ t('docker.container.card.upgrade') }}</el-dropdown-item>
            <el-dropdown-item v-if="!c.self" command="delete" :icon="Trash2" divided>{{ t('common.delete') }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </template>
  </EntityCard>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ArrowUpCircle, Ellipsis, FolderOpen, GitCommitHorizontal, PanelRight, Pause, Pencil, Play, RotateCw, ScrollText, SearchCheck, Square, SquareTerminal, TextCursorInput, Trash2, Zap } from 'lucide-vue-next'
import EntityCard from '../../components/EntityCard.vue'
import Field from '../../components/Field.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { containerBadge, isTcpPort, openPort } from './format'
import type { ContainerView } from './types'

const { t } = useI18n()

defineProps<{ c: ContainerView; selected: boolean; hideProject?: boolean; compact?: boolean }>()
const emit = defineEmits<{ command: [c: ContainerView, cmd: string]; select: [id: string, v: boolean] }>()
</script>

<style scoped>
.sel {
  height: auto;
}
.upd {
  margin-top: 4px;
}
.port {
  color: var(--el-color-primary);
  text-decoration: none;
}
.port:hover {
  text-decoration: underline;
}
@media (max-width: 767.98px) {
  .btn {
    min-height: 40px;
    padding: 0 8px;
  }
  .sel {
    min-width: 28px;
    min-height: 40px;
  }
}
</style>
