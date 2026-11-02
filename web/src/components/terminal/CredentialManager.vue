<template>
  <el-dialog
    :model-value="modelValue"
    :title="t('termConn.credential.title')"
    width="640px"
    :fullscreen="isMobile"
    class="term-dialog"
    append-to-body
    @update:model-value="(v: boolean) => $emit('update:modelValue', v)"
    @open="load"
  >
    <div class="tip">{{ t('termConn.credential.tip') }}</div>
    <div v-loading="loading" class="list">
      <div v-for="(c, i) in list" :key="c.key" class="row">
        <div class="info">
          <div class="name">
            {{ c.name }}
            <el-tag size="small" effect="plain">{{ c.authType === 'key' ? t('termConn.editor.authKey') : t('termConn.editor.authPassword') }}</el-tag>
          </div>
          <div class="meta">
            <span v-if="c.username">{{ t('termConn.credential.usernameMeta', { name: c.username }) }}</span>
            <span v-if="c.authType === 'key' && !c.privateKey && c.keyFile" class="mono">{{ c.keyFile }}</span>
            <span v-if="c.remark">{{ c.remark }}</span>
          </div>
        </div>
        <div class="ops">
          <el-button text :icon="ChevronUp" :disabled="i === 0" :aria-label="t('termConn.list.moveUp')" @click="move(i, -1)" />
          <el-button text :icon="ChevronDown" :disabled="i === list.length - 1" :aria-label="t('termConn.list.moveDown')" @click="move(i, 1)" />
          <el-button text :icon="Pencil" :aria-label="t('termConn.list.edit')" @click="edit(c)" />
          <el-button text :icon="CopyPlus" :aria-label="t('common.duplicate')" :title="t('common.duplicate')" @click="edit(c, true)" />
          <el-button text type="danger" :icon="Trash2" :aria-label="t('common.delete')" @click="remove(c)" />
        </div>
      </div>
      <el-empty v-if="!loading && !list.length" :description="t('termConn.credential.empty')" :image-size="60" />
    </div>
    <el-button :icon="Plus" @click="edit(null)">{{ t('termConn.credential.add') }}</el-button>
    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">{{ t('common.close') }}</el-button>
    </template>

    <!-- 新增或编辑单个预设 -->
    <el-dialog
      v-model="editorVisible"
      :title="form.key ? t('termConn.credential.titleEdit') : t('termConn.credential.titleAdd')"
      width="560px"
      :fullscreen="isMobile"
      class="term-dialog"
      append-to-body
      :close-on-click-modal="false"
    >
      <CopyHint :from="copyFrom" secrets />
      <el-form :model="form" label-width="100px" :label-position="labelPosition">
        <el-form-item :label="t('termConn.editor.name')" required>
          <el-input v-model="form.name" :placeholder="t('termConn.credential.namePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('termConn.editor.authType')">
          <el-radio-group v-model="form.authType">
            <el-radio-button value="password">{{ t('termConn.editor.authPassword') }}</el-radio-button>
            <el-radio-button value="key">{{ t('termConn.editor.authKey') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="t('termConn.editor.username')">
          <el-input v-model="form.username" :placeholder="t('termConn.credential.usernamePlaceholder')" />
        </el-form-item>
        <el-form-item v-if="form.authType === 'password'" :label="t('termConn.editor.password')">
          <SecretInput v-model="form.password" :placeholder="t('termConn.editor.passwordPlaceholder')" :reveal="secretOf(secretKey && `/api/webterminal/credentials/${secretKey}/secrets`, 'password')" />
        </el-form-item>
        <template v-else>
          <el-form-item :label="t('termConn.editor.privateKey')">
            <el-input v-model="form.privateKey" type="textarea" :rows="4" class="mono" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" />
            <span class="form-help">{{ t('termConn.editor.privateKeyHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('termConn.editor.keyFile')">
            <el-input v-model="form.keyFile" class="mono" placeholder="/root/.ssh/id_ed25519" />
            <span class="form-help">{{ t('termConn.editor.keyFileHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('termConn.editor.passphrase')">
            <SecretInput v-model="form.passphrase" :placeholder="t('termConn.editor.passphrasePlaceholder')" :reveal="secretOf(secretKey && `/api/webterminal/credentials/${secretKey}/secrets`, 'passphrase')" />
          </el-form-item>
        </template>
        <el-form-item :label="t('termConn.editor.remark')"><el-input v-model="form.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ChevronDown, ChevronUp, CopyPlus, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api } from '../../api'
import { useFormLayout } from '../../composables/useIsMobile'
import SecretInput from '../SecretInput.vue'
import { secretOf } from '../../secrets'
import { emptyCredential, type Credential } from './types'
import CopyHint from '../CopyHint.vue'
import { createItem, uniqueName, type CopySource } from '../../copyDraft'
import { isCancel, withAccess } from './security'

defineProps<{ modelValue: boolean }>()
// changed 在预设列表变化后发出，供连接编辑器刷新下拉选项
const emit = defineEmits<{ 'update:modelValue': [v: boolean]; changed: [list: Credential[]] }>()

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
const list = ref<Credential[]>([])
const loading = ref(false)
const editorVisible = ref(false)
const saving = ref(false)
const form = reactive<Credential>(emptyCredential())
// 复制预设时的模板，普通新增与编辑时为 null；查看密钥明文时复制取自模板
const copyFrom = ref<CopySource | null>(null)
const secretKey = computed(() => form.key || copyFrom.value?.key)

async function load() {
  loading.value = true
  try {
    list.value = (await api.get<Credential[] | null>('/api/webterminal/credentials')) ?? []
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

// edit 打开预设编辑框：copy 为 true 时以 c 为模板新建副本，密钥沿用模板
function edit(c: Credential | null, copy = false) {
  Object.assign(form, emptyCredential(), c ?? {})
  copyFrom.value = null
  if (c && copy) {
    copyFrom.value = { key: c.key ?? '', name: c.name }
    form.key = ''
    form.name = uniqueName(c.name, list.value.map((x) => x.name))
  }
  editorVisible.value = true
}

async function save() {
  if (!form.name.trim()) {
    ElMessage.warning(t('termConn.editor.needName'))
    return
  }
  saving.value = true
  try {
    const { key, ...body } = form
    if (key) {
      await api.put(`/api/webterminal/credentials/${encodeURIComponent(key)}`, body)
    } else {
      // 复制会沿用模板的密码与私钥，后端要求先通过 Web 终端的门禁（协议与二次验证）
      const from = copyFrom.value
      await (from ? withAccess(() => createItem('/api/webterminal/credentials', body, from)) : api.post('/api/webterminal/credentials', body))
    }
    editorVisible.value = false
    ElMessage.success(t('common.saved'))
    await load()
    emit('changed', list.value)
  } catch (e) {
    if (!isCancel(e)) {
      ElMessage.error((e as Error).message)
    }
  } finally {
    saving.value = false
  }
}

async function remove(c: Credential) {
  try {
    await ElMessageBox.confirm(t('termConn.credential.deleteConfirm', { name: c.name }), t('termConn.list.confirmTitle'), { type: 'warning' })
  } catch {
    return
  }
  try {
    // 仍被连接引用时后端会拒绝并列出相关连接
    await api.del(`/api/webterminal/credentials/${encodeURIComponent(c.key)}`)
    await load()
    emit('changed', list.value)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// move 上移或下移一项并立即保存顺序，失败时重新加载
async function move(i: number, d: number) {
  const j = i + d
  const next = [...list.value]
  ;[next[i], next[j]] = [next[j], next[i]]
  list.value = next
  try {
    await api.put('/api/webterminal/credentials-order', { keys: next.map((c) => c.key) })
    emit('changed', list.value)
  } catch (e) {
    ElMessage.error((e as Error).message)
    await load()
  }
}
</script>

<style scoped>
.tip {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-bottom: 12px;
}
.list {
  min-height: 60px;
  margin-bottom: 12px;
}
.row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.info {
  flex: 1;
  min-width: 0;
}
.name {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 500;
  word-break: break-all;
}
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-all;
}
.ops {
  display: flex;
  flex-shrink: 0;
}
.ops .el-button + .el-button {
  margin-left: 0;
}
</style>
