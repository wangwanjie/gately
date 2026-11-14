<template>
  <div class="shared-users">
    <!-- 列表只显示摘要，完整字段在编辑弹窗里修改，用户多时设置对话框不会过长 -->
    <div v-if="!model.length" class="empty">{{ t('sharedUsers.empty') }}</div>
    <div v-for="(u, i) in model" :key="i" class="item">
      <div class="item-head">
        <span class="name" :class="{ unnamed: !u.name.trim() }">{{ u.name.trim() || t('sharedUsers.unnamed') }}</span>
        <el-tag v-if="u.totpSecret" size="small" type="success">{{ t('sharedUsers.totpOn') }}</el-tag>
        <el-tag v-if="u.passkeys?.length" size="small" type="success">{{ t('sharedUsers.passkeyCount', { count: u.passkeys.length }) }}</el-tag>
        <el-tag v-if="!u.password && !u.hasPassword" size="small" type="warning">{{ t('sharedUsers.noPassword') }}</el-tag>
        <el-tag v-for="g in u.groups ?? []" :key="g" size="small" type="info">{{ g }}</el-tag>
        <span class="actions">
          <el-button text size="small" :icon="Pencil" @click="openEditor(i)">{{ t('common.edit') }}</el-button>
          <el-button text size="small" type="danger" :icon="Trash2" @click="model.splice(i, 1)">{{ t('common.delete') }}</el-button>
        </span>
      </div>
      <div v-if="profileOf(u) || u.remark" class="meta">
        <span v-if="profileOf(u)">{{ profileOf(u) }}</span>
        <span v-if="profileOf(u) && u.remark"> · </span>
        <span v-if="u.remark">{{ u.remark }}</span>
      </div>
    </div>
    <el-button size="small" :icon="Plus" @click="openEditor(-1)">{{ t('sharedUsers.add') }}</el-button>

    <el-dialog
      v-model="editorVisible"
      :title="editingIndex < 0 ? t('sharedUsers.add') : t('sharedUsers.editTitle')"
      width="560px"
      append-to-body
      :fullscreen="isMobile"
      :close-on-click-modal="false"
      :close-on-press-escape="!totpLoading"
      :show-close="!totpLoading"
    >
      <el-form v-if="draft" :model="draft" label-width="100px" :label-position="labelPosition" @submit.prevent>
        <el-form-item :label="t('sharedUsers.nameLabel')" required>
          <el-input v-model="draft.name" :placeholder="t('sharedUsers.namePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('sharedUsers.passwordLabel')">
          <el-input
            v-model="draft.password"
            type="password"
            show-password
            autocomplete="new-password"
            :placeholder="draft.hasPassword ? t('sharedUsers.passwordKeep') : t('sharedUsers.passwordPlaceholder')"
          />
        </el-form-item>
        <el-form-item label="TOTP">
          <div class="totp">
            <el-tag v-if="draft.totpSecret" size="small" type="success">{{ t('sharedUsers.totpOn') }}</el-tag>
            <el-tag v-else size="small" type="info">{{ t('sharedUsers.totpOff') }}</el-tag>
            <el-button size="small" :loading="totpLoading" @click="setupTOTP">
              {{ draft.totpSecret ? t('sharedUsers.totpReset') : t('sharedUsers.totpEnable') }}
            </el-button>
            <el-button v-if="draft.totpSecret" size="small" :loading="totpShowing" @click="showTOTP">{{ t('sharedUsers.totpShow') }}</el-button>
            <el-button v-if="draft.totpSecret" size="small" text type="danger" @click="draft.totpSecret = ''">{{ t('sharedUsers.totpDisable') }}</el-button>
          </div>
        </el-form-item>
        <!-- Passkey 只能由用户在站点上自助注册，管理员只能删除（例如设备丢失）；删除立即生效，不随下方的保存设置 -->
        <el-form-item v-if="draft.id" :label="t('sharedUsers.passkeys')">
          <div class="passkeys">
            <div v-if="!draft.passkeys?.length" class="form-help pk-none">{{ t('sharedUsers.passkeyNone') }}</div>
            <div v-for="p in draft.passkeys ?? []" :key="p.id" class="pk-item">
              <div class="pk-main">
                <span class="pk-name">{{ p.name }}</span>
                <span class="pk-meta">{{ t('sharedUsers.passkeyCreated', { time: fmtTime(p.createdAt) }) }}<template v-if="p.lastUsed"> · {{ t('sharedUsers.passkeyLastUsed', { time: fmtTime(p.lastUsed) }) }}</template></span>
              </div>
              <el-button text size="small" type="danger" :icon="Trash2" :loading="pkDeleting === p.id" @click="removePasskey(p)">{{ t('common.delete') }}</el-button>
            </div>
            <div class="form-help">{{ t('sharedUsers.passkeyHelp') }}</div>
          </div>
        </el-form-item>
        <!-- 可选的身份信息：OIDC 身份提供方把它们作为 email、name、groups 声明提供给应用 -->
        <el-form-item :label="t('sharedUsers.emailLabel')">
          <el-input v-model="draft.email" :placeholder="t('sharedUsers.emailPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('sharedUsers.displayNameLabel')">
          <el-input v-model="draft.displayName" :placeholder="t('sharedUsers.displayNamePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('sharedUsers.groupsLabel')">
          <el-select
            v-model="draft.groups"
            multiple
            filterable
            allow-create
            default-first-option
            :reserve-keyword="false"
            :placeholder="t('sharedUsers.groupsPlaceholder')"
          >
            <el-option v-for="g in allGroups" :key="g" :label="g" :value="g" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.remark')">
          <el-input v-model="draft.remark" :placeholder="t('common.remark')" />
        </el-form-item>
      </el-form>
      <div class="form-help">{{ t('sharedUsers.applyHelp') }}</div>
      <template #footer>
        <!-- 生成 TOTP 密钥的请求返回前不能关闭，否则新密钥只写进被丢弃的副本，二维码却照常弹出 -->
        <el-button :disabled="totpLoading" @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :disabled="totpLoading" @click="applyEditor">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="qrVisible" :title="t('sharedUsers.qrTitle', { name: qrName })" width="360px" append-to-body>
      <div class="qr-wrap">
        <img v-if="qr" :src="qr" class="qr" :alt="t('sharedUsers.qrTitle', { name: qrName })" />
        <div class="mono secret">{{ qrSecret }}</div>
        <div class="form-help">{{ t('sharedUsers.qrHelp') }}</div>
      </div>
      <template #footer>
        <el-button type="primary" @click="qrVisible = false">{{ t('common.close') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, toRaw, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Pencil, Plus, Trash2 } from 'lucide-vue-next'
import QRCode from 'qrcode'
import { api } from '../api'
import { useFormLayout } from '../composables/useIsMobile'
import { SECRET_MASK, fetchSecret } from '../secrets'
import type { AuthUser, PasskeyInfo } from '../sharedUsers'

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
const model = defineModel<AuthUser[]>({ required: true })
const qrVisible = ref(false)
const qr = ref('')
const qrSecret = ref('')
const qrName = ref('')
const totpLoading = ref(false)
const totpShowing = ref(false)
// 编辑弹窗：draft 为正在编辑的副本，editingIndex 为原用户在列表中的位置，-1 表示新增
const editorVisible = ref(false)
const editingIndex = ref(-1)
const draft = ref<AuthUser>()
// savedNames 记录每个用户载入时的用户名：TOTP 密钥明文表按已保存的用户名索引，界面上改名后仍按原名取回
const savedNames = new WeakMap<AuthUser, string>()
watch(
  model,
  (list) => {
    for (const u of list) {
      if (!savedNames.has(toRaw(u))) {
        savedNames.set(toRaw(u), u.name)
      }
    }
  },
  { immediate: true },
)
// 已用过的分组，作为其他用户的候选项
const allGroups = computed(() => [...new Set(model.value.flatMap((u) => u.groups ?? []))].sort())

// profileOf 返回列表摘要里的显示名与邮箱
function profileOf(u: AuthUser): string {
  return [u.displayName?.trim(), u.email?.trim()].filter(Boolean).join(' · ')
}

// openEditor 打开编辑弹窗，编辑的是副本，点「确定」才写回列表，取消不影响原数据
function openEditor(i: number) {
  editingIndex.value = i
  if (i < 0) {
    draft.value = { name: '', password: '', totpSecret: '', remark: '', email: '', displayName: '', groups: [] }
  } else {
    draft.value = JSON.parse(JSON.stringify(toRaw(model.value[i])))
  }
  editorVisible.value = true
}

// applyEditor 把副本写回：已有用户原地合并字段，保持对象不变，savedNames 仍能按原用户名取回 TOTP 明文
function applyEditor() {
  const d = draft.value
  if (!d) {
    return
  }
  if (!d.name.trim()) {
    ElMessage.warning(t('sharedUsers.nameFirst'))
    return
  }
  d.name = d.name.trim()
  // 与后端一致按用户名精确比较，重名时当场提示，不必等到保存设置才报错
  if (model.value.some((u, i) => i !== editingIndex.value && u.name.trim() === d.name)) {
    ElMessage.warning(t('sharedUsers.nameDuplicate', { name: d.name }))
    return
  }
  if (editingIndex.value < 0) {
    model.value.push(d)
  } else {
    Object.assign(model.value[editingIndex.value], d)
  }
  editorVisible.value = false
}

const pkDeleting = ref('')

function fmtTime(s: string): string {
  const d = new Date(s)
  return isNaN(d.getTime()) ? '' : d.toLocaleString()
}

// removePasskey 删除用户的一个 Passkey，立即保存（不随设置的保存按钮）；成功后同步编辑副本与列表中的用户
async function removePasskey(p: PasskeyInfo) {
  const d = draft.value
  if (!d?.id) {
    return
  }
  try {
    await ElMessageBox.confirm(t('sharedUsers.passkeyDeleteConfirm', { name: p.name, user: d.name }), t('sharedUsers.passkeyDeleteTitle'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  pkDeleting.value = p.id
  try {
    await api.del(`/api/webservice/users/${encodeURIComponent(d.id)}/passkeys/${encodeURIComponent(p.id)}`)
    d.passkeys = (d.passkeys ?? []).filter((x) => x.id !== p.id)
    const orig = model.value.find((u) => u.id === d.id)
    if (orig) {
      orig.passkeys = (orig.passkeys ?? []).filter((x) => x.id !== p.id)
    }
    ElMessage.success(t('sharedUsers.passkeyDeleted'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    pkDeleting.value = ''
  }
}

// totpURL 生成验证器 App 使用的 otpauth 地址，格式与后端 auth.TOTPURL 一致
function totpURL(secret: string, account: string): string {
  const q = new URLSearchParams({ digits: '6', issuer: 'Gately', period: '30', secret })
  return `otpauth://totp/${encodeURIComponent('Gately:' + account)}?${q.toString()}`
}

// showTOTP 重新展示已有密钥的二维码，便于在新设备上添加；已保存的密钥是脱敏占位符，先按原用户的已保存用户名向后端取回明文
async function showTOTP() {
  const u = draft.value
  if (!u) {
    return
  }
  totpShowing.value = true
  try {
    let secret: string | undefined = u.totpSecret
    if (secret === SECRET_MASK) {
      const orig = editingIndex.value >= 0 ? model.value[editingIndex.value] : undefined
      const savedName = orig ? savedNames.get(toRaw(orig)) : undefined
      secret = await fetchSecret('/api/webservice/settings/secrets/totp', savedName ?? u.name)
    }
    if (!secret) {
      return
    }
    const name = u.name.trim()
    qrSecret.value = secret
    qrName.value = name
    qr.value = await QRCode.toDataURL(totpURL(secret, name), { width: 200, margin: 1 })
    qrVisible.value = true
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    totpShowing.value = false
  }
}

// setupTOTP 生成新密钥并展示二维码；保存设置后生效，旧密钥同时作废
async function setupTOTP() {
  const u = draft.value
  if (!u) {
    return
  }
  if (!u.name.trim()) {
    ElMessage.warning(t('sharedUsers.nameFirst'))
    return
  }
  totpLoading.value = true
  try {
    const r = await api.post<{ secret: string; url: string }>('/api/webservice/totp-secret', { name: u.name.trim() })
    // 请求期间弹窗已关闭或换成了别的用户时丢弃结果，避免展示一个不会被保存的密钥
    if (draft.value !== u || !editorVisible.value) {
      return
    }
    u.totpSecret = r.secret
    qrSecret.value = r.secret
    qrName.value = u.name.trim()
    qr.value = await QRCode.toDataURL(r.url, { width: 200, margin: 1 })
    qrVisible.value = true
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    totpLoading.value = false
  }
}
</script>

<style scoped>
.shared-users {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}
.empty {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.item {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid var(--g-border);
  border-radius: 8px;
  padding: 6px 10px;
}
.item-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.name {
  font-weight: 600;
  overflow-wrap: anywhere;
}
.name.unnamed {
  font-weight: normal;
  color: var(--el-text-color-placeholder);
}
.actions {
  margin-left: auto;
}
.meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
  overflow-wrap: anywhere;
}
.totp {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.passkeys {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.pk-item {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--g-border);
  border-radius: 8px;
  padding: 4px 8px;
}
.pk-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.pk-name {
  font-weight: 600;
  overflow-wrap: anywhere;
}
.pk-meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
}
.pk-none {
  margin-top: 0;
}
.qr-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.qr {
  width: 200px;
  height: 200px;
}
.secret {
  word-break: break-all;
  text-align: center;
}
</style>
