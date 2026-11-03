<template>
  <div>
    <div class="toolbar g-sticky-bar g-sticky-desktop">
      <el-input v-model="keyword" :placeholder="t('docker.image.list.search')" clearable :prefix-icon="Search" class="t-search" />
      <CompactToggle v-model="compact" />
      <div class="t-ops">
        <el-button type="primary" :icon="CloudDownload" @click="pullVisible = true">{{ t('docker.image.list.pull') }}</el-button>
        <el-button :icon="Hammer" @click="buildVisible = true">{{ t('docker.image.list.build') }}</el-button>
        <el-button :icon="PackageOpen" :loading="importing" @click="picker?.click()">{{ t('common.import') }}</el-button>
        <el-button :icon="Eraser" @click="pruneDangling">{{ t('docker.image.list.pruneDangling') }}</el-button>
        <el-button :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <input ref="picker" type="file" accept=".tar,application/x-tar" class="hidden" @change="onImport" />
      </div>
    </div>
    <el-progress v-if="importing" :percentage="importPercent" :stroke-width="10" class="gap" />

    <div v-loading="loading && !images.length" class="card-list" :class="{ 'is-compact': compact }">
      <el-empty v-if="!loading && !filtered.length" :description="t('docker.image.list.empty')" />
      <EntityCard :compact="compact" v-for="img in filtered" :key="img.id" :off="img.containers === 0 && img.dangling">
        <template #lead>
          <div class="ico"><Disc3 :size="18" /></div>
        </template>
        <template #title>
          <span v-if="img.tags?.length" class="mono tag-main">{{ img.tags[0] }}</span>
          <span v-else class="muted">&lt;none&gt;</span>
          <el-tag v-if="img.tags?.length > 1" size="small" type="info">+{{ img.tags.length - 1 }}</el-tag>
          <el-tag v-if="img.dangling" size="small" type="warning">{{ t('docker.image.list.dangling') }}</el-tag>
        </template>
        <template #sub><span class="mono">{{ shortId(img.id) }}</span></template>
        <template #status>
          <StatusBadge v-if="img.containers > 0" type="ok" :text="t('docker.image.list.usedBy', { count: img.containers })" />
          <StatusBadge v-else type="off" :text="t('docker.image.list.unused')" />
        </template>
        <Field :label="t('docker.image.list.size')">{{ formatBytes(img.size) }}</Field>
        <Field :label="t('docker.image.list.created')">{{ formatTime(img.created) }}</Field>
        <Field v-if="img.tags?.length > 1" :label="t('docker.image.list.allTags')" wide mono>
          <div v-for="tg in img.tags" :key="tg">{{ tg }}</div>
        </Field>
        <template #actions>
          <el-button text :icon="Info" class="btn" @click="showDetail(img)">{{ t('common.details') }}</el-button>
          <el-button text :icon="Tag" class="btn" @click="openTag(img)">{{ t('docker.image.list.tags') }}</el-button>
          <el-dropdown trigger="click" @command="(c: string) => onCommand(img, c)">
            <el-button text :icon="Ellipsis" class="btn">{{ t('common.more') }}</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="push" :icon="CloudUpload" :disabled="!img.tags?.length">{{ t('docker.image.list.push') }}</el-dropdown-item>
                <el-dropdown-item command="export" :icon="Download">{{ t('docker.image.list.exportTar') }}</el-dropdown-item>
                <el-dropdown-item command="delete" :icon="Trash2" divided>{{ t('common.delete') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </EntityCard>
    </div>

    <ImagePullDialog v-model="pullVisible" @done="load" />
    <ImageBuildDialog v-model="buildVisible" @done="load" />
    <ImageDetailDialog v-model="detailVisible" :image-id="detailId" />

    <el-dialog v-model="tagVisible" :title="t('docker.image.list.addTagTitle')" width="460px" :fullscreen="isMobile" append-to-body>
      <el-form :label-position="labelPosition" label-width="70px" @submit.prevent>
        <el-form-item :label="t('docker.image.list.repo')" required><el-input v-model="tagForm.repo" :placeholder="t('docker.image.list.repoPlaceholder')" class="mono" /></el-form-item>
        <el-form-item :label="t('docker.image.list.tag')"><el-input v-model="tagForm.tag" placeholder="latest" class="mono" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="tagVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="doTag">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="pushVisible" :title="t('docker.image.list.pushTitle')" width="460px" :fullscreen="isMobile" append-to-body>
      <el-form :label-position="labelPosition" label-width="70px" @submit.prevent>
        <el-form-item :label="t('docker.image.list.image')">
          <el-select v-model="pushRef" class="full">
            <el-option v-for="tg in pushTags" :key="tg" :label="tg" :value="tg" />
          </el-select>
        </el-form-item>
      </el-form>
      <div class="help block">{{ t('docker.image.list.pushHint') }}</div>
      <template #footer>
        <el-button @click="pushVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="doPush">{{ t('docker.image.list.push') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="delVisible" :title="t('docker.image.list.deleteTitle')" width="460px" append-to-body>
      <i18n-t :keypath="(delTarget?.tags?.length ?? 0) > 1 ? 'docker.image.list.deleteTextMulti' : 'docker.image.list.deleteText'" tag="p" class="dlg-text">
        <template #name><b class="mono">{{ delTarget?.tags?.[0] || shortId(delTarget?.id ?? '') }}</b></template>
        <template #count>{{ delTarget?.tags?.length ?? 0 }}</template>
      </i18n-t>
      <el-checkbox v-model="delForce">{{ t('docker.image.list.deleteForce') }}</el-checkbox>
      <template #footer>
        <el-button @click="delVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :loading="delBusy" @click="doDelete">{{ t('common.delete') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CloudDownload, CloudUpload, Disc3, Download, Ellipsis, Eraser, Hammer, Info, PackageOpen, RefreshCw, Search, Tag, Trash2 } from 'lucide-vue-next'
import { api } from '../../api'
import CompactToggle from '../../components/CompactToggle.vue'
import EntityCard from '../../components/EntityCard.vue'
import { useCompactView } from '../../composables/useCompactView'
import Field from '../../components/Field.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { useFormLayout } from '../../composables/useIsMobile'
import ImageBuildDialog from './ImageBuildDialog.vue'
import ImageDetailDialog from './ImageDetailDialog.vue'
import ImagePullDialog from './ImagePullDialog.vue'
import { download, formatBytes, formatTime, shortId, uploadFile } from './format'
import { reportError } from './state'
import { trackTask } from './tasks'
import type { ImageView } from './types'

const { t } = useI18n()
const compact = useCompactView('docker.images')

const { isMobile, labelPosition } = useFormLayout()
const images = ref<ImageView[]>([])
const loading = ref(false)
const keyword = ref('')
const pullVisible = ref(false)
const buildVisible = ref(false)
const detailVisible = ref(false)
const detailId = ref<string>()
const picker = ref<HTMLInputElement>()
const importing = ref(false)
const importPercent = ref(0)
const tagVisible = ref(false)
const tagTarget = ref<ImageView>()
const tagForm = reactive({ repo: '', tag: 'latest' })
const pushVisible = ref(false)
const pushTags = ref<string[]>([])
const pushRef = ref('')
const delVisible = ref(false)
const delTarget = ref<ImageView>()
const delForce = ref(false)
const delBusy = ref(false)

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  return k ? images.value.filter((i) => i.id.toLowerCase().includes(k) || (i.tags ?? []).some((tg) => tg.toLowerCase().includes(k))) : images.value
})

async function load() {
  loading.value = true
  try {
    images.value = (await api.get<ImageView[]>('/api/docker/images')) ?? []
    reportError('')
  } catch (e) {
    reportError((e as Error).message)
  } finally {
    loading.value = false
  }
}

function showDetail(img: ImageView) {
  detailId.value = img.id
  detailVisible.value = true
}

function openTag(img: ImageView) {
  tagTarget.value = img
  tagForm.repo = ''
  tagForm.tag = 'latest'
  tagVisible.value = true
}

async function doTag() {
  if (!tagTarget.value || !tagForm.repo.trim()) {
    ElMessage.warning(t('docker.image.list.repoRequired'))
    return
  }
  try {
    await api.post(`/api/docker/images/${encodeURIComponent(tagTarget.value.id)}/tag`, { repo: tagForm.repo.trim(), tag: tagForm.tag.trim() || 'latest' })
    tagVisible.value = false
    ElMessage.success(t('docker.image.list.tagAdded'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function doPush() {
  if (!pushRef.value) {
    return
  }
  try {
    const res = await api.post<{ task: string }>('/api/docker/images/push', { ref: pushRef.value })
    pushVisible.value = false
    trackTask(res.task, t('docker.image.list.pushTask', { ref: pushRef.value }))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function doDelete() {
  if (!delTarget.value) {
    return
  }
  delBusy.value = true
  try {
    await api.del(`/api/docker/images/${encodeURIComponent(delTarget.value.id)}${delForce.value ? '?force=1' : ''}`)
    delVisible.value = false
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    delBusy.value = false
  }
}

function onCommand(img: ImageView, cmd: string) {
  if (cmd === 'push') {
    pushTags.value = img.tags ?? []
    pushRef.value = pushTags.value[0] ?? ''
    pushVisible.value = true
  } else if (cmd === 'export') {
    download(`/api/docker/images/${encodeURIComponent(img.id)}/export`)
  } else if (cmd === 'delete') {
    delTarget.value = img
    delForce.value = false
    delVisible.value = true
  }
}

async function pruneDangling() {
  try {
    await ElMessageBox.confirm(t('docker.image.list.pruneConfirm'), t('docker.image.list.pruneTitle'), { type: 'warning' })
    const r = await api.post<{ deleted: number; spaceReclaimed: number }>('/api/docker/prune', { target: 'images' })
    ElMessage.success(t('docker.image.list.pruned', { deleted: r.deleted, size: formatBytes(r.spaceReclaimed) }))
    await load()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error((e as Error).message)
    }
  }
}

async function onImport(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) {
    return
  }
  importing.value = true
  importPercent.value = 0
  try {
    await uploadFile('/api/docker/images/load', file, 'application/x-tar', (p) => (importPercent.value = p))
    ElMessage.success(t('docker.image.list.imported'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    importing.value = false
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
.hidden {
  display: none;
}
.gap {
  margin-bottom: 12px;
}
.full {
  width: 100%;
}
.ico {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-secondary);
}
.tag-main {
  font-size: 14px;
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
    flex: 1 1 28%;
    min-height: 40px;
  }
  .btn {
    min-height: 40px;
    padding: 0 8px;
  }
  .toolbar :deep(input),
  :deep(.el-dialog input) {
    font-size: 16px;
  }
}
</style>
