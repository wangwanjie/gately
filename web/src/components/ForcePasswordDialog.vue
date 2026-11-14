<template>
  <!-- 默认密码或强度不足的密码：必须先修改才能继续使用。对话框不能通过遮罩、Esc 或关闭按钮关闭；
       服务端同时拒绝登录会话的写操作，直到密码改好 -->
  <el-dialog
    :model-value="true"
    width="min(480px, 94vw)"
    align-center
    append-to-body
    :show-close="false"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    class="force-pw-dialog"
    modal-class="force-pw-modal"
  >
    <template #header>
      <div class="head">
        <span class="head-icon"><ShieldAlert :size="22" /></span>
        <div>
          <div class="title">{{ issue === 'default' ? t('security.force.titleDefault') : t('security.force.titleWeak') }}</div>
          <div class="sub">{{ issue === 'default' ? t('security.force.descDefault') : t('security.force.descWeak') }}</div>
        </div>
      </div>
    </template>

    <el-form label-position="top" class="force-form" @submit.prevent="submit">
      <el-form-item :label="t('security.force.username')">
        <el-input v-model="form.username" autocomplete="username" />
      </el-form-item>
      <el-form-item :label="t('security.force.current')" required>
        <el-input v-model="form.oldPassword" type="password" show-password autocomplete="current-password" />
        <div v-if="issue === 'default'" class="default-hint">
          <span>{{ t('security.force.defaultHint', { password: DEFAULT_PASSWORD }) }}</span>
          <el-button text type="primary" size="small" @click="form.oldPassword = DEFAULT_PASSWORD">{{ t('security.force.useDefault') }}</el-button>
        </div>
      </el-form-item>
      <el-form-item :label="t('security.force.newPassword')" required>
        <el-input v-model="form.newPassword" type="password" show-password autocomplete="new-password" />
        <PasswordStrength :password="form.newPassword" :username="form.username" @status="strength = $event" />
      </el-form-item>
      <el-form-item :label="t('security.force.confirm')" required>
        <el-input v-model="form.confirm" type="password" show-password autocomplete="new-password" @keyup.enter="submit" />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="foot">
        <el-button text :disabled="busy" @click="logout">{{ t('layout.logout') }}</el-button>
        <el-button type="primary" :loading="busy" :disabled="strength === 'weak'" @click="submit">{{ t('security.force.submit') }}</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { ShieldAlert } from 'lucide-vue-next'
import { api } from '../api'
import PasswordStrength from './PasswordStrength.vue'

const props = defineProps<{ issue: 'default' | 'weak'; username: string }>()
const emit = defineEmits<{ (e: 'done'): void }>()
const { t } = useI18n()
const router = useRouter()

// 默认密码是公开信息（README 与登录说明里都有），与后端 auth.DefaultPassword 一致
const DEFAULT_PASSWORD = 'gately.cn'

const form = reactive({ username: props.username, oldPassword: '', newPassword: '', confirm: '' })
const strength = ref<'' | 'ok' | 'weak'>('')
const busy = ref(false)

async function submit() {
  if (busy.value) {
    return
  }
  if (!form.oldPassword || !form.newPassword) {
    ElMessage.warning(t('security.force.required'))
    return
  }
  if (form.newPassword !== form.confirm) {
    ElMessage.error(t('security.force.mismatch'))
    return
  }
  busy.value = true
  try {
    await api.put('/api/account', { oldPassword: form.oldPassword, username: form.username.trim(), newPassword: form.newPassword })
  } catch (e) {
    ElMessage.error((e as Error).message)
    busy.value = false
    return
  }
  // 修改密码会让全部旧会话失效：用新密码重新登录，之后无缝继续；需要验证码等情况回到登录页
  try {
    await api.post('/api/login', { username: form.username.trim(), password: form.newPassword })
    ElMessage.success(t('security.force.done'))
    form.oldPassword = ''
    form.newPassword = ''
    form.confirm = ''
    emit('done')
  } catch {
    // 开启了两步验证等无法自动登录的情况：提示后回到登录页，由用户用新密码手动登录
    ElMessage.success(t('security.force.doneRelogin'))
    router.replace({ name: 'login' })
  } finally {
    busy.value = false
  }
}

async function logout() {
  await api.post('/api/logout').catch(() => undefined)
  router.replace({ name: 'login' })
}
</script>

<style scoped>
.head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}
.head-icon {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  color: var(--el-color-danger);
  background: var(--el-color-danger-light-9);
}
.title {
  font-size: 17px;
  font-weight: 600;
}
.sub {
  margin-top: 4px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}
.force-form :deep(.el-form-item) {
  margin-bottom: 14px;
}
.default-hint {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
</style>

<style>
/* 遮罩加深，强调必须先处理；对话框高度与内部滚动沿用全局 .el-dialog 规则（style.css），不再另设 */
.force-pw-modal {
  background: rgba(0, 0, 0, 0.62);
}
</style>
