<template>
  <div>
    <PageHeader :title="t('ftp.title')" doc="modules/tools#ftp" :desc="t('ftp.desc')" :icon="HardDrive">
      <el-button type="primary" :icon="Save" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
    </PageHeader>

    <el-alert v-if="error && settings?.enable" type="error" :title="error" :closable="false" show-icon class="block-gap" />

    <div v-if="settings" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Server :size="16" />{{ t('ftp.service') }}</div>
        <StatusBadge v-bind="svcStatus" />
      </div>
      <div class="g-card-body">
        <el-form label-width="100px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('common.enable')"><el-switch v-model="settings.enable" /></el-form-item>
          <el-form-item :label="t('ftp.listenIp')"><el-input v-model="settings.listenIp" :placeholder="t('ftp.listenIpPlaceholder')" class="w-260" /></el-form-item>
          <el-form-item :label="t('ftp.port')"><el-input-number v-model="settings.port" :min="1" :max="65535" /></el-form-item>
          <el-form-item :label="t('ftp.encryption')">
            <el-select v-model="settings.tlsMode" class="w-260">
              <el-option :value="0" :label="t('ftp.tlsOptional')" />
              <el-option :value="1" :label="t('ftp.tlsExplicit')" />
              <el-option :value="2" :label="t('ftp.tlsImplicit')" />
            </el-select>
            <span class="help">{{ t('ftp.tlsHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('ftp.passivePorts')">
            <el-input-number v-model="settings.passiveStart" :min="0" :max="65535" :controls="false" class="w-110" />
            <span class="range-sep">—</span>
            <el-input-number v-model="settings.passiveEnd" :min="0" :max="65535" :controls="false" class="w-110" />
            <span class="help">{{ t('ftp.passiveHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('ftp.passiveIp')">
            <el-input v-model="settings.publicHost" :placeholder="t('ftp.passiveIpPlaceholder')" class="w-260" />
          </el-form-item>
          <el-form-item :label="t('ftp.idleTimeout')"><el-input-number v-model="settings.idleTimeout" :min="1" :max="86400" /><span class="help">{{ t('ftp.seconds') }}</span></el-form-item>
          <el-form-item :label="t('ftp.disableActive')"><el-switch v-model="settings.disableActiveMode" /><span class="help">{{ t('ftp.disableActiveHelp') }}</span></el-form-item>
        </el-form>
      </div>
    </div>

    <FileUsers v-if="settings" v-model="settings.users" secrets-url="/api/ftp/settings/secrets" />
    <SharedFileUsers
      v-if="settings && shared.loaded.value"
      v-model:enabled="settings.sharedUsers"
      :users="shared.savedUsers.value"
      :local-names="settings.users.map((u) => u.name)"
      :load-error="shared.loadError.value"
    />

    <LogCard url="/api/ftp/logs" clearable />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { HardDrive, Save, Server } from 'lucide-vue-next'
import { api } from '../api'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import FileUsers, { type FileUser } from '../components/FileUsers.vue'
import SharedFileUsers from '../components/SharedFileUsers.vue'
import { useSharedFileUsers } from '../composables/useSharedFileUsers'
import { useFormLayout } from '../composables/useIsMobile'

interface Settings {
  enable: boolean
  listenIp: string
  port: number
  passiveStart: number
  passiveEnd: number
  publicHost: string
  tlsMode: number
  idleTimeout: number
  connectionTimeout: number
  disableActiveMode: boolean
  users: FileUser[]
  // sharedUsers 为 true 时文件服务共享用户也可登录本服务
  sharedUsers: boolean
}

const { t } = useI18n()
// 文件服务共享用户：WebDAV、FTP、SMB 共用同一份列表，在共享用户页面维护，这里只显示摘要
const shared = useSharedFileUsers()
const { labelPosition } = useFormLayout()
const settings = ref<Settings>()
// savedEnable 为已保存的启用状态，状态徽标以它为准
const savedEnable = ref(false)
const running = ref(false)
const error = ref('')
const saving = ref(false)

const svcStatus = computed((): { type: StatusType; text: string; title?: string } => {
  if (!savedEnable.value) {
    return { type: 'off', text: t('ftp.statusOff') }
  }
  if (error.value) {
    return { type: 'error', text: t('ftp.statusError'), title: error.value }
  }
  if (running.value) {
    return { type: 'running', text: t('common.running') }
  }
  return { type: 'warn', text: t('ftp.statusNotRunning') }
})

async function load() {
  const r = await api.get<{ settings: Settings & { users: FileUser[] | null }; running: boolean; error: string }>('/api/ftp/settings')
  settings.value = { ...r.settings, users: r.settings.users ?? [], sharedUsers: !!r.settings.sharedUsers }
  savedEnable.value = r.settings.enable
  running.value = r.running
  error.value = r.error
}

async function save() {
  saving.value = true
  try {
    const r = await api.put<{ running: boolean; error: string }>('/api/ftp/settings', settings.value)
    running.value = r.running
    error.value = r.error
    ElMessage.success(t('common.saved'))
    await Promise.all([load(), shared.load()])
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

onMounted(() => Promise.all([load(), shared.load()]))
</script>

<style scoped>
.block-gap {
  margin-bottom: 16px;
}
.svc-form {
  max-width: 560px;
}
.svc-form .el-form-item:last-child {
  margin-bottom: 0;
}
.w-260 {
  width: 260px;
}
.w-110 {
  width: 110px;
}
.range-sep {
  margin: 0 8px;
  color: var(--el-text-color-secondary);
}

@media (max-width: 767.98px) {
  .w-260 {
    width: 100%;
  }
}
</style>
