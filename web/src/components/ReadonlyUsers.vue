<template>
  <div class="g-card">
    <div class="g-card-head">
      <div class="g-card-title"><Eye :size="16" />{{ t('users.title') }}</div>
      <span class="g-card-desc">{{ t('users.count', { count: list.length }) }}</span>
    </div>
    <div class="g-card-body">
      <p class="desc">{{ t('users.desc') }}</p>
      <el-table v-if="list.length" :data="list" size="small" class="gap">
        <el-table-column :label="t('users.username')" prop="username" min-width="160" />
        <el-table-column :label="t('users.role')" width="120">
          <template #default><el-tag size="small" effect="light">{{ t('users.readonly') }}</el-tag></template>
        </el-table-column>
        <el-table-column :label="t('common.actions')" width="130" align="right">
          <template #default="{ row }">
            <el-tooltip :content="t('users.resetPassword')" placement="top">
              <el-button text :icon="KeyRound" :aria-label="t('users.resetPassword')" @click="resetPassword(row)" />
            </el-tooltip>
            <el-tooltip :content="t('common.delete')" placement="top">
              <el-button text type="danger" :icon="Trash2" :aria-label="t('common.delete')" @click="remove(row)" />
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>
      <el-button type="primary" plain :icon="Plus" @click="openAdd">{{ t('users.add') }}</el-button>
    </div>

    <el-dialog v-model="dialog" :title="t('users.add')" width="420px" append-to-body>
      <el-form label-position="top" @submit.prevent="create">
        <el-form-item :label="t('users.username')" required><el-input v-model="form.username" autocomplete="off" /></el-form-item>
        <el-form-item :label="t('users.password')" required>
          <el-input v-model="form.password" type="password" show-password autocomplete="new-password" />
          <PasswordStrength :password="form.password" :username="form.username" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="busy" @click="create">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Eye, KeyRound, Plus, Trash2 } from 'lucide-vue-next'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '../api'
import PasswordStrength from './PasswordStrength.vue'

interface UserItem {
  username: string
  role: string
}

const { t } = useI18n()
const list = ref<UserItem[]>([])
const dialog = ref(false)
const busy = ref(false)
const form = reactive({ username: '', password: '' })

async function load() {
  try {
    list.value = await api.get<UserItem[]>('/api/account/users')
  } catch {
    // 只读账号无权访问，保持空列表
  }
}

function openAdd() {
  form.username = ''
  form.password = ''
  dialog.value = true
}

async function create() {
  busy.value = true
  try {
    list.value = await api.post<UserItem[]>('/api/account/users', { username: form.username.trim(), password: form.password })
    dialog.value = false
    ElMessage.success(t('common.saved'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

async function resetPassword(row: UserItem) {
  let pwd = ''
  try {
    const r = await ElMessageBox.prompt(t('users.resetPrompt', { name: row.username }), t('users.resetPassword'), {
      inputType: 'password',
      inputPlaceholder: t('users.password'),
      confirmButtonText: t('common.save'),
      cancelButtonText: t('common.cancel'),
    })
    pwd = r.value
  } catch {
    return
  }
  try {
    await api.put(`/api/account/users/${encodeURIComponent(row.username)}`, { password: pwd })
    ElMessage.success(t('common.saved'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function remove(row: UserItem) {
  try {
    await ElMessageBox.confirm(t('users.deleteConfirm', { name: row.username }), t('common.delete'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
      confirmButtonClass: 'el-button--danger',
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  try {
    list.value = await api.del<UserItem[]>(`/api/account/users/${encodeURIComponent(row.username)}`)
    ElMessage.success(t('common.deleted'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(load)
</script>

<style scoped>
.desc {
  margin: 0 0 12px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.6;
}
.gap {
  margin-bottom: 12px;
}
</style>
