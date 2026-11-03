<template>
  <div>
    <PageHeader :title="t('audit.title')" :desc="t('audit.desc')" :icon="ClipboardList">
      <el-button :icon="RefreshCw" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
    </PageHeader>

    <div class="g-card">
      <div class="g-card-head filters g-sticky-bar g-sticky-desktop">
        <el-select v-model="category" clearable :placeholder="t('audit.category')" class="f-cat" @change="reload">
          <el-option v-for="c in categories" :key="c" :label="catLabel(c)" :value="c" />
        </el-select>
        <el-input v-model="keyword" clearable :placeholder="t('audit.search')" :prefix-icon="Search" class="f-kw" @keyup.enter="reload" @clear="reload" />
        <span class="g-card-desc">{{ t('audit.total', { count: total }) }}</span>
      </div>
      <div class="g-card-body table-body">
        <el-table v-loading="loading" :data="entries" :empty-text="t('audit.empty')">
          <el-table-column :label="t('audit.time')" width="180">
            <template #default="{ row }">{{ new Date(row.time).toLocaleString() }}</template>
          </el-table-column>
          <el-table-column :label="t('audit.actor')" min-width="150">
            <template #default="{ row }">
              <div>{{ row.actor || '-' }}</div>
              <div class="meta mono">{{ row.ip }}</div>
              <div v-if="row.via" class="meta mono">{{ t('audit.via', { ip: row.via }) }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="t('audit.category')" width="110">
            <template #default="{ row }">
              <el-tag size="small" effect="light" :type="tagType(row.category)">{{ catLabel(row.category) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('audit.action')" min-width="300">
            <template #default="{ row }">
              <span class="mono">{{ row.method }} {{ row.path }}</span>
              <div v-if="row.detail" class="meta">{{ row.detail }}</div>
            </template>
          </el-table-column>
          <el-table-column :label="t('audit.status')" width="90" align="center">
            <template #default="{ row }">
              <el-tag size="small" effect="plain" :type="row.status >= 400 ? 'danger' : 'success'">{{ row.status || '-' }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
        <div class="pager">
          <el-pagination v-model:current-page="page" :page-size="pageSize" :total="total" layout="prev, pager, next" hide-on-single-page @current-change="load" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ClipboardList, RefreshCw, Search } from 'lucide-vue-next'
import { ElMessage } from 'element-plus'
import PageHeader from '../components/PageHeader.vue'
import { api } from '../api'

interface AuditEntry {
  time: string
  actor: string
  ip: string
  via?: string
  category: string
  module?: string
  method?: string
  path?: string
  status?: number
  detail?: string
}

const { t } = useI18n()
const categories = ['login', 'change', 'access']
const category = ref('')
const keyword = ref('')
const entries = ref<AuditEntry[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 50
const loading = ref(false)

// 类别文案使用静态键，便于 check-i18n 识别
function catLabel(c: string): string {
  if (c === 'login') {
    return t('audit.categories.login')
  }
  if (c === 'access') {
    return t('audit.categories.access')
  }
  return t('audit.categories.change')
}

function tagType(c: string): 'info' | 'warning' | 'success' {
  return c === 'login' ? 'info' : c === 'access' ? 'warning' : 'success'
}

async function load() {
  loading.value = true
  try {
    const qs = new URLSearchParams({ limit: String(pageSize), offset: String((page.value - 1) * pageSize) })
    if (category.value) {
      qs.set('category', category.value)
    }
    if (keyword.value.trim()) {
      qs.set('q', keyword.value.trim())
    }
    const res = await api.get<{ entries: AuditEntry[]; total: number }>('/api/audit?' + qs.toString())
    entries.value = res.entries
    total.value = res.total
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

// 过滤条件变化后回到第一页
function reload() {
  page.value = 1
  load()
}

onMounted(load)
</script>

<style scoped>
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}
.f-cat {
  width: 150px;
}
.f-kw {
  width: 260px;
  max-width: 100%;
}
.meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
</style>
