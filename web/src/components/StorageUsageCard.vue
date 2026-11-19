<template>
  <div class="g-card" data-testid="storage-usage">
    <div class="g-card-head">
      <div class="g-card-title"><HardDrive :size="16" />{{ t('settings.storageUsage.title') }}</div>
      <el-button size="small" :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
    </div>
    <div class="g-card-body">
      <el-alert v-if="error" type="error" :title="error" :closable="false" show-icon />
      <div v-else-if="!usage" v-loading="true" class="su-placeholder" />
      <template v-else>
        <div class="su-summary">
          <div class="su-row">
            <span class="su-label">{{ t('settings.storageUsage.configDir') }}</span>
            <span class="mono su-path">{{ usage.configDir }}</span>
          </div>
          <div v-if="usage.disk" class="su-row">
            <span class="su-label">{{ t('settings.storageUsage.disk') }}</span>
            <div class="su-disk">
              <el-progress :percentage="diskPct" :show-text="false" :stroke-width="8" :status="diskPct >= 90 ? 'exception' : diskPct >= 75 ? 'warning' : undefined" />
              <div class="muted su-small">
                {{ t('settings.storageUsage.diskText', { used: formatBytes(usage.disk.used), total: formatBytes(usage.disk.total), free: formatBytes(usage.disk.free) }) }}
              </div>
            </div>
          </div>
          <div class="su-row">
            <span class="su-label">{{ t('settings.storageUsage.configTotal') }}</span>
            <span>
              <b>{{ formatBytes(usage.total) }}</b>
              <span class="muted su-small">{{ t('settings.storageUsage.files', { n: usage.files }) }}</span>
            </span>
          </div>
        </div>
        <div class="su-items">
          <div v-for="it in usage.items" :key="it.key" class="su-item" :data-key="it.key">
            <div class="su-name">{{ categoryName(it.key) }}</div>
            <div class="su-paths mono muted" :title="it.paths.join('  ')">{{ it.paths.join(' / ') }}</div>
            <div class="su-bar"><span :style="{ width: barWidth(it.bytes) }" /></div>
            <div class="su-files muted">{{ t('settings.storageUsage.files', { n: it.files }) }}</div>
            <div class="su-size">{{ formatBytes(it.bytes) }}</div>
          </div>
        </div>
        <div v-if="usage.truncated" class="form-help">{{ t('settings.storageUsage.truncated') }}</div>
        <div class="form-help">{{ t('settings.storageUsage.help') }}</div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { HardDrive, RefreshCw } from 'lucide-vue-next'
import { api, formatBytes } from '../api'

// 存储占用：配置目录所在分区的容量与按用途分类的占用，见 GET /api/system/storage。
// active 为 true（所在页签可见）时才第一次读取，统计要遍历配置目录。

interface StorageItem {
  key: string
  paths: string[]
  files: number
  bytes: number
}

interface StorageUsage {
  configDir: string
  disk: { path: string; total: number; used: number; free: number } | null
  total: number
  files: number
  items: StorageItem[]
  truncated: boolean
}

const props = defineProps<{ active: boolean }>()
const { t, te } = useI18n()
const usage = ref<StorageUsage>()
const loading = ref(false)
const error = ref('')

const diskPct = computed(() => {
  const d = usage.value?.disk
  return d && d.total > 0 ? Math.min(100, Math.round((d.used / d.total) * 100)) : 0
})

// 各项的条形长度以最大一项为满格
const maxBytes = computed(() => Math.max(1, ...(usage.value?.items ?? []).map((it) => it.bytes)))

function barWidth(bytes: number) {
  return `${Math.max(bytes > 0 ? 1 : 0, Math.round((bytes / maxBytes.value) * 100))}%`
}

function categoryName(key: string) {
  const k = `settings.storageUsage.cat.${key}`
  return te(k) ? t(k) : key
}

async function load() {
  loading.value = true
  try {
    usage.value = await api.get<StorageUsage>('/api/system/storage')
    error.value = ''
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

watch(
  () => props.active,
  (v) => {
    if (v && !usage.value && !loading.value) {
      load()
    }
  },
  { immediate: true },
)
</script>

<style scoped>
.su-placeholder {
  height: 80px;
}
.su-summary {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 14px;
}
.su-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}
.su-label {
  flex: 0 0 120px;
  color: var(--el-text-color-regular);
}
.su-path {
  word-break: break-all;
}
.su-disk {
  flex: 1;
  max-width: 480px;
}
.su-small {
  font-size: 12px;
  margin-left: 6px;
}
.su-disk .su-small {
  margin-left: 0;
  margin-top: 4px;
}
.su-items {
  border-top: 1px solid var(--el-border-color-lighter);
}
.su-item {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr) 160px 80px 80px;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.su-paths {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.su-bar {
  height: 6px;
  border-radius: 3px;
  background: var(--el-fill-color);
  overflow: hidden;
}
.su-bar span {
  display: block;
  height: 100%;
  background: var(--el-color-primary);
}
.su-files {
  font-size: 12px;
  text-align: right;
}
.su-size {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
@media (max-width: 768px) {
  .su-label {
    flex-basis: 90px;
  }
  .su-item {
    grid-template-columns: minmax(0, 1fr) auto;
    grid-template-areas:
      'name size'
      'paths files'
      'bar bar';
    row-gap: 4px;
  }
  .su-name {
    grid-area: name;
  }
  .su-size {
    grid-area: size;
  }
  .su-paths {
    grid-area: paths;
  }
  .su-files {
    grid-area: files;
  }
  .su-bar {
    grid-area: bar;
  }
}
</style>
