<template>
  <el-dialog
    :model-value="modelValue"
    :title="form.key ? t('termConn.editor.titleEdit') : t('termConn.editor.titleAdd')"
    width="640px"
    :fullscreen="isMobile"
    class="term-dialog"
    :close-on-click-modal="false"
    destroy-on-close
    @update:model-value="(v: boolean) => $emit('update:modelValue', v)"
    @open="onOpen"
  >
    <el-form :model="form" label-width="100px" :label-position="labelPosition">
      <el-form-item :label="t('termConn.editor.name')" required><el-input v-model="form.name" :placeholder="t('termConn.editor.namePlaceholder')" /></el-form-item>
      <GroupInput v-model="form.group" :groups="groups ?? []" />
      <el-form-item :label="t('termConn.editor.type')">
        <el-radio-group v-model="form.type" :disabled="!!form.key">
          <el-radio-button value="ssh">SSH</el-radio-button>
          <el-radio-button value="telnet">Telnet</el-radio-button>
          <el-radio-button value="local">{{ t('termConn.editor.localShell') }}</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item :label="t('termConn.editor.enable')"><el-switch v-model="form.enable" /></el-form-item>

      <!-- SSH -->
      <template v-if="form.type === 'ssh'">
        <el-divider content-position="left">{{ t('termConn.editor.sshSection') }}</el-divider>
        <el-form-item :label="t('termConn.editor.host')" required><el-input v-model="form.ssh.host" :placeholder="t('termConn.editor.hostPlaceholder')" /></el-form-item>
        <el-form-item :label="t('termConn.editor.port')"><el-input-number v-model="form.ssh.port" :min="1" :max="65535" /></el-form-item>
        <el-form-item :label="t('termConn.editor.username')">
          <el-input v-model="form.ssh.username" :placeholder="usernamePlaceholder" />
        </el-form-item>
        <el-form-item :label="t('termConn.editor.authType')">
          <el-radio-group v-model="form.ssh.authType">
            <el-radio-button value="password">{{ t('termConn.editor.authPassword') }}</el-radio-button>
            <el-radio-button value="key">{{ t('termConn.editor.authKey') }}</el-radio-button>
            <el-radio-button value="credential">{{ t('termConn.editor.authCredential') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <!-- 认证预设：凭据在设置中统一维护，这里只选择引用哪一组 -->
        <el-form-item v-if="form.ssh.authType === 'credential'" :label="t('termConn.editor.credential')" required>
          <div class="cred">
            <el-select v-model="form.ssh.credentialKey" filterable :placeholder="t('termConn.editor.credentialPlaceholder')" :no-data-text="t('termConn.editor.credentialEmpty')" class="cred-select">
              <el-option v-for="c in credentials" :key="c.key" :label="c.name" :value="c.key">
                <span>{{ c.name }}</span>
                <span class="opt-meta">{{ credentialDesc(c) }}</span>
              </el-option>
            </el-select>
            <el-button :icon="KeyRound" @click="credVisible = true">{{ t('termConn.editor.credentialManage') }}</el-button>
          </div>
          <span class="form-help">{{ t('termConn.editor.credentialHelp') }}</span>
        </el-form-item>
        <el-form-item v-else-if="form.ssh.authType === 'password'" :label="t('termConn.editor.password')">
          <SecretInput v-model="form.ssh.password" :placeholder="t('termConn.editor.passwordPlaceholder')" :reveal="revealOf('ssh.password')" />
        </el-form-item>
        <template v-else>
          <el-form-item :label="t('termConn.editor.privateKey')">
            <el-input v-model="form.ssh.privateKey" type="textarea" :rows="4" class="mono" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" />
            <span class="form-help">{{ t('termConn.editor.privateKeyHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('termConn.editor.keyFile')">
            <el-input v-model="form.ssh.keyFile" class="mono" placeholder="/root/.ssh/id_ed25519" />
            <span class="form-help">{{ t('termConn.editor.keyFileHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('termConn.editor.passphrase')"><SecretInput v-model="form.ssh.passphrase" :placeholder="t('termConn.editor.passphrasePlaceholder')" :reveal="revealOf('ssh.passphrase')" /></el-form-item>
        </template>
        <el-form-item :label="t('termConn.editor.workDir')">
          <el-input v-model="form.ssh.workDir" class="mono" :placeholder="t('termConn.editor.sshWorkDirPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('termConn.editor.timeout')"><el-input-number v-model="form.ssh.timeout" :min="1" :max="300" /></el-form-item>
        <el-form-item :label="t('termConn.editor.keepAlive')">
          <el-input-number v-model="form.ssh.keepAlive" :min="0" :max="600" />
          <span class="help">{{ t('termConn.editor.keepAliveHelp') }}</span>
        </el-form-item>
        <el-form-item v-if="form.ssh.hostKey" :label="t('termConn.editor.hostKey')">
          <span class="fp mono">{{ form.ssh.hostKey }}</span>
        </el-form-item>
        <ProxyForm :proxy="form.ssh.proxy" :reveal="revealOf('ssh.proxy.password')" />
      </template>

      <!-- Telnet -->
      <template v-else-if="form.type === 'telnet'">
        <el-divider content-position="left">{{ t('termConn.editor.telnetSection') }}</el-divider>
        <el-form-item :label="t('termConn.editor.host')" required><el-input v-model="form.telnet.host" :placeholder="t('termConn.editor.hostPlaceholder')" /></el-form-item>
        <el-form-item :label="t('termConn.editor.port')"><el-input-number v-model="form.telnet.port" :min="1" :max="65535" /></el-form-item>
        <el-form-item :label="t('termConn.editor.username')">
          <el-input v-model="form.telnet.username" :placeholder="t('termConn.editor.telnetUsernamePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('termConn.editor.password')"><SecretInput v-model="form.telnet.password" :placeholder="t('termConn.editor.telnetPasswordPlaceholder')" :reveal="revealOf('telnet.password')" /></el-form-item>
        <el-form-item :label="t('termConn.editor.timeout')"><el-input-number v-model="form.telnet.timeout" :min="1" :max="300" /></el-form-item>
        <ProxyForm :proxy="form.telnet.proxy" :reveal="revealOf('telnet.proxy.password')" />
      </template>

      <!-- 本机 -->
      <template v-else>
        <el-divider content-position="left">{{ t('termConn.editor.localShell') }}</el-divider>
        <el-alert v-if="!allowLocalShell" type="warning" :closable="false" class="gap" :title="t('termConn.editor.localShellDisabled')" />
        <el-form-item :label="t('termConn.editor.shell')">
          <el-select v-model="form.local.shell" filterable allow-create default-first-option clearable :placeholder="t('termConn.editor.shellPlaceholder')" class="full">
            <el-option v-for="s in shells" :key="s" :label="s" :value="s" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('termConn.editor.workDir')"><el-input v-model="form.local.workDir" class="mono" :placeholder="t('termConn.editor.localWorkDirPlaceholder')" /></el-form-item>
        <el-form-item :label="t('termConn.editor.env')">
          <div class="env">
            <div v-for="(r, i) in envRows" :key="i" class="env-row">
              <el-input v-model="r.k" :placeholder="t('termConn.editor.envName')" class="mono" />
              <el-input v-model="r.v" :placeholder="t('termConn.editor.envValue')" class="mono" />
              <el-button text type="danger" :icon="Trash2" :aria-label="t('termConn.editor.envRemove')" @click="envRows.splice(i, 1)" />
            </div>
            <el-button size="small" :icon="Plus" @click="envRows.push({ k: '', v: '' })">{{ t('termConn.editor.envAdd') }}</el-button>
          </div>
        </el-form-item>
      </template>

      <el-divider content-position="left">{{ t('termConn.editor.other') }}</el-divider>
      <el-form-item v-if="form.type !== 'telnet'" :label="t('termConn.editor.quickDirs')">
        <el-input v-model="quickText" type="textarea" :rows="3" class="mono" :placeholder="t('termConn.editor.quickDirsPlaceholder')" />
        <span class="form-help">{{ t('termConn.editor.quickDirsHelp') }}</span>
      </el-form-item>
      <el-form-item :label="t('termConn.editor.remark')"><el-input v-model="form.remark" /></el-form-item>
    </el-form>
    <template #footer>
      <div class="foot">
        <el-button v-if="form.type !== 'local'" :loading="testing" :icon="Plug" @click="test">{{ t('termConn.editor.testConnection') }}</el-button>
        <span class="grow" />
        <el-button @click="$emit('update:modelValue', false)">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </div>
    </template>
    <CredentialManager v-model="credVisible" @changed="(l: Credential[]) => (credentials = l)" />
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { KeyRound, Plug, Plus, Trash2 } from 'lucide-vue-next'
import { api } from '../../api'
import { useFormLayout } from '../../composables/useIsMobile'
import CredentialManager from './CredentialManager.vue'
import ProxyForm from './ProxyForm.vue'
import SecretInput from '../SecretInput.vue'
import GroupInput from '../GroupInput.vue'
import { secretOf } from '../../secrets'
import { emptyConn, normalizeConn, type Conn, type Credential } from './types'

// groups 为已有的连接分组，用于分组输入提示
const props = defineProps<{ modelValue: boolean; conn: Conn | null; allowLocalShell: boolean; groups?: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [v: boolean]; saved: [] }>()

const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
const form = reactive<Conn>(emptyConn())

// revealOf 返回取回已保存连接中密码明文的函数，field 为后端明文表的键（例如 ssh.proxy.password）
function revealOf(field: string) {
  return secretOf(form.key && `/api/webterminal/connections/${form.key}/secrets`, field)
}
const envRows = ref<{ k: string; v: string }[]>([])
const quickText = ref('')
const shells = ref<string[]>([])
const saving = ref(false)
const testing = ref(false)
const credentials = ref<Credential[]>([])
const credVisible = ref(false)

// 选中的预设带有默认用户名时，用户名可留空并在占位提示中显示
const selectedCredential = computed(() => credentials.value.find((c) => c.key === form.ssh.credentialKey))
const usernamePlaceholder = computed(() => {
  const name = form.ssh.authType === 'credential' ? selectedCredential.value?.username : ''
  return name ? t('termConn.editor.usernameFromCredential', { name }) : ''
})

function credentialDesc(c: Credential): string {
  const kind = c.authType === 'key' ? t('termConn.editor.authKey') : t('termConn.editor.authPassword')
  return c.username ? `${kind} · ${c.username}` : kind
}

async function loadCredentials() {
  try {
    credentials.value = (await api.get<Credential[] | null>('/api/webterminal/credentials')) ?? []
  } catch {
    credentials.value = []
  }
}

function onOpen() {
  Object.assign(form, normalizeConn(props.conn ?? emptyConn()))
  envRows.value = Object.entries(form.local.env).map(([k, v]) => ({ k, v }))
  quickText.value = form.quickAccessDirs.join('\n')
  loadShells()
  loadCredentials()
}

async function loadShells() {
  try {
    shells.value = (await api.get<string[] | null>('/api/webterminal/shells')) ?? []
  } catch {
    shells.value = []
  }
}

// 切换到本机类型时再确保已加载 Shell 列表
watch(
  () => form.type,
  (type) => {
    if (type === 'local' && !shells.value.length) {
      loadShells()
    }
  },
)

// buildBody 把表单整理为提交给后端的 Connection。
function buildBody(): Conn {
  const body = JSON.parse(JSON.stringify(form)) as Conn
  const env: Record<string, string> = {}
  for (const r of envRows.value) {
    if (r.k.trim()) {
      env[r.k.trim()] = r.v
    }
  }
  body.local.env = env
  body.quickAccessDirs = quickText.value
    .split('\n')
    .map((s) => s.trim())
    .filter((s) => s)
  return body
}

function validate(body: Conn): string {
  if (!body.name.trim()) {
    return t('termConn.editor.needName')
  }
  if (body.type === 'ssh' && !body.ssh.host.trim()) {
    return t('termConn.editor.needSshHost')
  }
  if (body.type === 'ssh' && body.ssh.authType === 'credential') {
    if (!body.ssh.credentialKey) {
      return t('termConn.editor.needCredential')
    }
    // 连接与预设都没有用户名时无法登录
    if (!body.ssh.username.trim() && !selectedCredential.value?.username) {
      return t('termConn.editor.needUsername')
    }
  }
  if (body.type === 'telnet' && !body.telnet.host.trim()) {
    return t('termConn.editor.needTelnetHost')
  }
  return ''
}

async function save() {
  const body = buildBody()
  const msg = validate(body)
  if (msg) {
    ElMessage.warning(msg)
    return
  }
  saving.value = true
  try {
    const { key, ...rest } = body
    if (key) {
      await api.put(`/api/webterminal/connections/${encodeURIComponent(key)}`, rest)
    } else {
      await api.post('/api/webterminal/connections', rest)
    }
    emit('update:modelValue', false)
    emit('saved')
    ElMessage.success(t('common.saved'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

// test 测试当前表单中尚未保存的配置；编辑已有连接时带上 key，让后端合并已保存的密文。
async function test() {
  const body = buildBody()
  const msg = validate(body)
  if (msg) {
    ElMessage.warning(msg)
    return
  }
  testing.value = true
  try {
    const r = await api.post<{ message?: string } | null>('/api/webterminal/connections/test', body)
    ElMessage.success(r?.message || t('termConn.editor.testOk'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    testing.value = false
  }
}
</script>

<style scoped>
.full {
  width: 100%;
}
.gap {
  margin-bottom: 14px;
}
.fp {
  font-size: 12px;
  word-break: break-all;
  color: var(--el-text-color-secondary);
}
.env {
  width: 100%;
}
.env-row {
  display: flex;
  gap: 6px;
  margin-bottom: 6px;
}
.env-row .el-input:first-child {
  flex: 0 0 36%;
}
.cred {
  display: flex;
  gap: 8px;
  width: 100%;
}
.cred-select {
  flex: 1;
  min-width: 0;
}
.opt-meta {
  float: right;
  margin-left: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.foot {
  display: flex;
  align-items: center;
  gap: 8px;
}
.grow {
  flex: 1;
}
@media (max-width: 767.98px) {
  .foot {
    flex-wrap: wrap;
  }
}
</style>
