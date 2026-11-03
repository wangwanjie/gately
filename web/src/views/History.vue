<template>
  <div>
    <PageHeader :title="t('history.title')" :desc="t('history.desc')" :icon="History">
      <el-button :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
    </PageHeader>

    <div class="g-card">
      <div class="g-card-head filters g-sticky-bar">
        <el-select v-model="current" :placeholder="t('history.pickConfig')" class="f-name" filterable>
          <el-option v-for="f in files" :key="f.name" :label="f.title ? `${f.title} (${f.name}.json)` : `${f.name}.json`" :value="f.name" />
        </el-select>
        <span class="g-card-desc">{{ t('history.keepHint') }}</span>
      </div>
      <div class="g-card-body table-body">
        <el-table v-loading="loading" :data="versions" :empty-text="t('history.empty')">
          <el-table-column :label="t('history.savedAt')" min-width="200">
            <template #default="{ row }">{{ new Date(row.time).toLocaleString() }}</template>
          </el-table-column>
          <el-table-column :label="t('history.size')" width="120">
            <template #default="{ row }">{{ formatSize(row.size) }}</template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="220" align="right">
            <template #default="{ row }">
              <el-button text :icon="FileDiff" @click="showDiff(row)">{{ t('history.diff') }}</el-button>
              <el-button text type="warning" :icon="Undo2" @click="restore(row)">{{ t('history.restore') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <el-dialog v-model="diffVisible" :title="t('history.diffTitle', { name: current })" width="min(900px, 94vw)">
      <div class="legend">
        <span class="lg-del">- {{ t('history.legendOld') }}</span>
        <span class="lg-add">+ {{ t('history.legendNew') }}</span>
      </div>
      <div v-loading="diffLoading" class="diff mono">
        <div v-if="!diffLoading && diffLines.length === 0" class="muted">{{ t('history.noDiff') }}</div>
        <div v-for="(l, i) in diffLines" :key="i" class="line" :class="'op-' + (l.op === '+' ? 'add' : l.op === '-' ? 'del' : l.op === '@' ? 'skip' : 'eq')">
          <template v-if="l.op === '@'">{{ t('history.skipped', { count: l.text }) }}</template>
          <template v-else>{{ l.op }} {{ l.text }}</template>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { FileDiff, History, RefreshCw, Undo2 } from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import PageHeader from '../components/PageHeader.vue'
import { api } from '../api'

interface Version {
  id: string
  time: string
  size: number
}
interface HistoryFile {
  name: string
  title?: string
  versions: Version[]
}
interface DiffLine {
  op: string
  text: string
}

const { t } = useI18n()
const files = ref<HistoryFile[]>([])
const current = ref('')
const loading = ref(false)
const diffVisible = ref(false)
const diffLoading = ref(false)
const diffLines = ref<DiffLine[]>([])
const selected = ref<Version | null>(null)

const versions = computed(() => files.value.find((f) => f.name === current.value)?.versions ?? [])

function formatSize(n: number): string {
  return n >= 1024 ? `${(n / 1024).toFixed(1)} KB` : `${n} B`
}

async function load() {
  loading.value = true
  try {
    files.value = await api.get<HistoryFile[]>('/api/history')
    if (!files.value.some((f) => f.name === current.value)) {
      current.value = files.value[0]?.name ?? ''
    }
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

function url(v: Version, tail: string): string {
  return `/api/history/${encodeURIComponent(current.value)}/${encodeURIComponent(v.id)}/${tail}`
}

async function showDiff(v: Version) {
  selected.value = v
  diffVisible.value = true
  diffLoading.value = true
  diffLines.value = []
  try {
    diffLines.value = (await api.get<{ lines: DiffLine[] }>(url(v, 'diff'))).lines
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    diffLoading.value = false
  }
}

async function restore(v: Version) {
  try {
    await ElMessageBox.confirm(t('history.restoreConfirm', { name: current.value, time: new Date(v.time).toLocaleString() }), t('history.restoreTitle'), {
      type: 'warning',
      confirmButtonText: t('history.restore'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  try {
    await api.post(url(v, 'restore'))
    ElMessage.success(t('history.restored'))
    // 服务端回滚后会重启，稍后刷新列表
    setTimeout(load, 3000)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

watch(current, () => {
  diffVisible.value = false
})
onMounted(load)
</script>

<style scoped>
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}
.f-name {
  width: 300px;
  max-width: 100%;
}
.legend {
  display: flex;
  gap: 16px;
  margin-bottom: 8px;
  font-size: 12px;
}
.lg-del {
  color: var(--el-color-danger);
}
.lg-add {
  color: var(--el-color-success);
}
.diff {
  max-height: 60vh;
  overflow: auto;
  font-size: 12px;
  line-height: 1.6;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
}
.line {
  padding: 0 10px;
  white-space: pre-wrap;
  word-break: break-all;
}
.op-add {
  background: var(--el-color-success-light-9);
  color: var(--el-color-success-dark-2);
}
.op-del {
  background: var(--el-color-danger-light-9);
  color: var(--el-color-danger-dark-2);
}
.op-skip {
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
  text-align: center;
}
</style>
