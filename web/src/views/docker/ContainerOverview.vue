<template>
  <div v-loading="loading" class="ov">
    <el-alert v-if="error" type="error" :closable="false" show-icon :title="error" />
    <template v-if="d">
      <div class="g-card">
        <div class="g-card-head"><div class="g-card-title"><Info :size="16" />{{ t('docker.container.overview.basicInfo') }}</div></div>
        <div class="grid">
          <Field :label="t('common.status')"><StatusBadge v-bind="containerBadge(d.state, d.status)" /> <span class="muted">{{ d.status }}</span></Field>
          <Field :label="t('docker.container.overview.health')">{{ d.healthStatus || '-' }}</Field>
          <Field :label="t('docker.container.overview.restartCount')">{{ d.restartCount }}</Field>
          <Field :label="t('docker.container.overview.exitCode')">{{ d.exitCode }}</Field>
          <Field :label="t('docker.container.overview.image')" mono wide>{{ d.image }}</Field>
          <Field :label="t('docker.container.overview.imageId')" mono wide>{{ d.imageId }}</Field>
          <Field :label="t('docker.container.overview.created')">{{ formatTime(d.created) }}</Field>
          <Field :label="t('docker.container.overview.started')">{{ formatTime(d.started) }}</Field>
          <Field :label="t('docker.container.overview.startCommand')" mono wide>{{ [d.path, ...(d.args ?? [])].filter(Boolean).join(' ') || '-' }}</Field>
          <Field label="Entrypoint" mono>{{ d.entrypoint || '-' }}</Field>
          <Field :label="t('docker.container.overview.workdir')" mono>{{ d.workdir || '-' }}</Field>
          <Field :label="t('docker.container.overview.user')" mono>{{ d.user || '-' }}</Field>
          <Field :label="t('docker.container.overview.hostname')" mono>{{ d.hostname || '-' }}</Field>
          <Field :label="t('docker.container.overview.restartPolicy')">{{ d.restart || 'no' }}</Field>
          <Field :label="t('docker.container.overview.privileged')">{{ d.privileged ? t('common.yes') : t('common.no') }}</Field>
          <Field v-if="d.composeProject" label="Compose">{{ d.composeProject }} / {{ d.composeService }}</Field>
          <Field :label="t('docker.container.overview.memoryLimit')">{{ d.memoryMB ? d.memoryMB + ' MB' : t('docker.container.overview.unlimited') }}</Field>
          <Field :label="t('docker.container.overview.cpuLimit')">{{ d.cpus ? t('docker.container.overview.cpuCores', { count: d.cpus }) : t('docker.container.overview.unlimited') }}</Field>
        </div>
      </div>

      <div class="g-card">
        <div class="g-card-head"><div class="g-card-title"><Network :size="16" />{{ t('docker.container.overview.portsNetwork') }}</div></div>
        <div class="g-card-body">
          <div v-if="d.ports?.length" class="chips">
            <el-tag v-for="(p, i) in d.ports" :key="i" type="info" class="mono">{{ p.hostIp ? p.hostIp + ':' : '' }}{{ p.host || t('docker.container.overview.random') }} → {{ p.container }}/{{ p.proto }}</el-tag>
          </div>
          <div v-else class="muted">{{ t('docker.container.overview.noPorts') }}</div>
          <div v-for="n in d.networkDetails ?? []" :key="n.name" class="net">
            <div class="net-name">{{ n.name }}</div>
            <div class="net-info mono">{{ t('docker.container.overview.netInfo', { ip: n.ip || '-', gateway: n.gateway || '-', mac: n.mac || '-' }) }}<span v-if="n.aliases?.length">{{ t('docker.container.overview.netAliases', { aliases: n.aliases.join(', ') }) }}</span></div>
          </div>
        </div>
      </div>

      <div class="g-card">
        <div class="g-card-head"><div class="g-card-title"><HardDrive :size="16" />{{ t('docker.container.overview.mountsTitle', { count: d.mounts?.length ?? 0 }) }}</div></div>
        <div class="g-card-body">
          <div v-for="(m, i) in d.mounts ?? []" :key="i" class="mount">
            <el-tag size="small" :type="m.type === 'volume' ? 'success' : 'info'">{{ m.type }}</el-tag>
            <span class="mono path">{{ m.name || m.source }} → {{ m.destination }}</span>
            <el-tag size="small" :type="m.rw ? 'warning' : 'info'">{{ m.rw ? t('docker.container.overview.readWrite') : t('docker.container.overview.readOnly') }}</el-tag>
          </div>
          <div v-if="!d.mounts?.length" class="muted">{{ t('docker.container.overview.noMounts') }}</div>
        </div>
      </div>

      <div class="g-card">
        <div class="g-card-head"><div class="g-card-title"><Variable :size="16" />{{ t('docker.container.overview.envTitle', { count: envList.length }) }}</div></div>
        <div class="g-card-body">
          <div v-for="(e, i) in envList" :key="i" class="kv mono"><b>{{ e.k }}</b><span>{{ e.v }}</span></div>
          <div v-if="!envList.length" class="muted">{{ t('docker.container.overview.noEnv') }}</div>
        </div>
      </div>

      <div class="g-card">
        <div class="g-card-head"><div class="g-card-title"><Tags :size="16" />{{ t('docker.container.overview.labelsTitle', { count: labelList.length }) }}</div></div>
        <div class="g-card-body">
          <div v-for="(e, i) in labelList" :key="i" class="kv mono"><b>{{ e.k }}</b><span>{{ e.v }}</span></div>
          <div v-if="!labelList.length" class="muted">{{ t('docker.container.overview.noLabels') }}</div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HardDrive, Info, Network, Tags, Variable } from 'lucide-vue-next'
import { api } from '../../api'
import Field from '../../components/Field.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { containerBadge, formatTime } from './format'
import type { ContainerDetail } from './types'

const { t } = useI18n()

type Detail = ContainerDetail & { env?: string[]; labels?: Record<string, string> }

const props = defineProps<{ containerId: string }>()
const d = ref<Detail>()
const loading = ref(false)
const error = ref('')

const envList = computed(() =>
  (d.value?.env ?? []).map((l) => {
    const i = l.indexOf('=')
    return i < 0 ? { k: l, v: '' } : { k: l.slice(0, i), v: l.slice(i + 1) }
  }),
)
const labelList = computed(() => Object.entries(d.value?.labels ?? {}).map(([k, v]) => ({ k, v })))

onMounted(async () => {
  loading.value = true
  try {
    d.value = await api.get<Detail>(`/api/docker/containers/${props.containerId}`)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.ov {
  min-height: 120px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  gap: 12px 20px;
  padding: 14px 16px;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.net {
  margin-top: 10px;
}
.net-name {
  font-weight: 600;
  font-size: 13px;
}
.net-info {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-all;
}
.mount {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 8px;
  padding: 4px 0;
}
.path {
  font-size: 12px;
  word-break: break-all;
  min-width: 0;
}
.kv {
  display: flex;
  gap: 8px;
  font-size: 12px;
  line-height: 1.7;
  word-break: break-all;
}
.kv b {
  flex: 0 0 auto;
  max-width: 45%;
  color: var(--el-text-color-secondary);
  font-weight: 500;
}
.kv span {
  min-width: 0;
}
@media (max-width: 767.98px) {
  .grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    padding: 12px;
    gap: 10px 12px;
  }
  .kv {
    flex-direction: column;
    gap: 0;
  }
  .kv b {
    max-width: none;
  }
}
</style>
