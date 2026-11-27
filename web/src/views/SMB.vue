<template>
  <div>
    <PageHeader :title="t('smb.title')" doc="modules/tools#smb" :desc="t('smb.desc')" :icon="FolderTree">
      <el-button type="primary" :icon="Save" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
    </PageHeader>

    <el-alert v-if="!supported" type="warning" :title="t('smb.unsupported')" :closable="false" show-icon class="block-gap" />
    <el-alert v-else-if="error && settings?.enable" type="error" :title="error" :closable="false" show-icon class="block-gap" />

    <div v-if="settings" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Server :size="16" />{{ t('smb.service') }}</div>
        <StatusBadge v-bind="svcStatus" />
      </div>
      <div class="g-card-body">
        <el-form label-width="110px" :label-position="labelPosition" class="svc-form">
          <el-form-item :label="t('common.enable')"><el-switch v-model="settings.enable" :disabled="!supported" /></el-form-item>
          <el-form-item :label="t('smb.listenIp')"><el-input v-model="settings.listenIp" :placeholder="t('smb.listenIpPlaceholder')" class="w-260" /></el-form-item>
          <el-form-item :label="t('smb.port')">
            <el-input-number v-model="settings.port" :min="1" :max="65535" />
            <span class="help">{{ t('smb.portHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('smb.encryption')">
            <el-switch v-model="settings.requireEncryption" />
            <span class="help">{{ t('smb.encryptionHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('smb.mdns')">
            <el-switch v-model="settings.mdns" />
            <span class="help">{{ t('smb.mdnsHelp') }}</span>
          </el-form-item>
        </el-form>
      </div>
    </div>

    <FileUsers
      v-if="settings"
      v-model="settings.users"
      :desc="t('smb.usersDesc')"
      mount-summary-key="smb.mountSummary"
      :mount-name-placeholder="t('smb.shareName')"
      secrets-url="/api/smb/settings/secrets"
    />

    <SharedFileUsers
      v-if="settings && shared.loaded.value"
      v-model:enabled="settings.sharedUsers"
      :users="shared.savedUsers.value"
      :local-names="settings.users.map((u) => u.name)"
      :issues="smbIssues"
      :load-error="shared.loadError.value"
      case-insensitive
    />

    <div v-if="settings && savedShares.length" class="g-card">
      <div class="g-card-head">
        <div class="g-card-title"><Link2 :size="16" />{{ t('smb.connect') }} <span class="g-card-desc">{{ t('smb.connectDesc') }}</span></div>
      </div>
      <div class="g-card-body">
        <div class="c-head"><span>{{ t('smb.user') }}</span><span>{{ t('smb.shareName') }}</span><span>Windows</span><span>macOS / Linux</span></div>
        <div v-for="(s, i) in savedShares" :key="i" class="c-row">
          <span>{{ s.user }}</span>
          <span>{{ s.name }}<el-tag v-if="s.readOnly" size="small" type="info" class="ro-tag">{{ t('fileUsers.readOnly') }}</el-tag></span>
          <code v-if="savedPort === 445" class="mono">{{ uncPath(s.name) }}</code>
          <span v-else class="help-inline">{{ t('smb.windowsPortOnly') }}</span>
          <code class="mono">{{ smbURL(s.name) }}</code>
        </div>
      </div>
    </div>

    <LogCard url="/api/smb/logs" clearable />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { FolderTree, Link2, Save, Server } from 'lucide-vue-next'
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
  requireEncryption: boolean
  mdns: boolean
  users: FileUser[]
  // sharedUsers 为 true 时文件服务共享用户也可登录本服务
  sharedUsers: boolean
}

interface ShareRow {
  user: string
  name: string
  readOnly: boolean
}

const { t } = useI18n()
// 文件服务共享用户：WebDAV、FTP、SMB 共用同一份列表，在共享用户页面维护，这里只显示摘要
const shared = useSharedFileUsers()
// smbIssues 为已保存的共享用户中无法用于 SMB 的用户
const smbIssues = computed(() => shared.service('smb')?.issues ?? [])
const { labelPosition } = useFormLayout()
const settings = ref<Settings>()
// savedEnable 为已保存的启用状态，状态徽标以它为准
const savedEnable = ref(false)
const savedPort = ref(445)
// savedListenIp 为已保存的监听地址，连接地址以它为准，不随表单中未保存的修改变化
const savedListenIp = ref('')
// savedLocalShares 为已保存配置中本地用户的共享；savedSharedUsers 为已保存的共享用户开关
const savedLocalShares = ref<ShareRow[]>([])
// savedLocalNames 为已保存的本地用户名（含停用的用户）
const savedLocalNames = ref<string[]>([])
const savedSharedUsers = ref(false)
// savedShares 为已保存配置中的全部共享，用于展示连接地址：开启共享用户时加入可用于 SMB 的共享用户
// （与后端 buildAccounts 一致，排除与本地用户同名或无法用于 SMB 的共享用户）
const savedShares = computed((): ShareRow[] => {
  if (!savedEnable.value) {
    return []
  }
  if (!savedSharedUsers.value) {
    return savedLocalShares.value
  }
  // 停用的本地用户仍占用用户名，同名的共享用户也不能登录
  const local = new Set(savedLocalNames.value.map((n) => n.toLowerCase()))
  const bad = new Set(smbIssues.value.map((i) => i.name))
  const usable = shared.savedUsers.value.filter((u) => !u.disabled && !local.has(u.name.toLowerCase()) && !bad.has(u.name))
  return [...savedLocalShares.value, ...sharesOf(usable)]
})
const running = ref(false)
const error = ref('')
const supported = ref(true)
const saving = ref(false)

const svcStatus = computed((): { type: StatusType; text: string; title?: string } => {
  if (!savedEnable.value) {
    return { type: 'off', text: t('smb.statusOff') }
  }
  if (error.value) {
    return { type: 'error', text: t('smb.statusError'), title: error.value }
  }
  if (running.value) {
    return { type: 'running', text: t('common.running') }
  }
  return { type: 'warn', text: t('smb.statusNotRunning') }
})

// hostName 为连接地址中的主机（IPv6 不带方括号）：监听具体地址时用已保存的该地址，
// 否则用当前访问管理界面的主机名。不读表单中未保存的监听地址，避免显示尚未生效的地址
function hostName(): string {
  const ip = savedListenIp.value
  const h = ip && ip !== '0.0.0.0' && ip !== '::' ? ip : window.location.hostname
  return h.replace(/^\[|\]$/g, '')
}

// uncPath 为 Windows 资源管理器使用的 UNC 路径。UNC 不接受 IPv6 字面量，
// 需改写为 ipv6-literal.net 形式：冒号换成连字符，区域标识前的 % 换成 s
function uncPath(share: string): string {
  let h = hostName()
  if (h.includes(':')) {
    h = `${h.replace(/:/g, '-').replace(/%/g, 's')}.ipv6-literal.net`
  }
  return `\\\\${h}\\${share}`
}

// smbURL 为 macOS 访达与 Linux 使用的 smb:// 地址，共享名中的 # ? % 等字符会被转义，不会被误解析
function smbURL(share: string): string {
  const port = savedPort.value === 445 ? '' : `:${savedPort.value}`
  const h = hostName()
  // URL 中的 IPv6 需要方括号，区域标识的 % 需要转义为 %25
  const host = h.includes(':') ? `[${h.replace(/%/g, '%25')}]` : h
  // 只转义会破坏 URL 的字符（%、#、?、空白等），中文等非 ASCII 字符保持原样便于阅读，访达与各客户端都能识别
  const path = share.replace(/[%#?&\s]/g, (c) => encodeURIComponent(c))
  return `smb://${host}${port}/${path}`
}

// sharesOf 与后端 User.Shares 一致：单目录用户的共享名为用户名，多目录用户每个目录一个共享
function sharesOf(users: FileUser[]): ShareRow[] {
  const out: ShareRow[] = []
  for (const u of users) {
    if (u.mounts?.length) {
      for (const mt of u.mounts) {
        out.push({ user: u.name, name: mt.name, readOnly: mt.readOnly })
      }
    } else {
      out.push({ user: u.name, name: u.name, readOnly: u.readOnly })
    }
  }
  return out
}

async function load() {
  const r = await api.get<{ settings: Settings & { users: FileUser[] | null }; running: boolean; error: string; supported: boolean }>('/api/smb/settings')
  settings.value = { ...r.settings, users: r.settings.users ?? [], sharedUsers: !!r.settings.sharedUsers }
  savedEnable.value = r.settings.enable
  savedPort.value = r.settings.port
  savedListenIp.value = r.settings.listenIp
  // 停用的用户不能登录，不列出连接地址
  savedLocalShares.value = sharesOf(settings.value.users.filter((u) => !u.disabled))
  savedLocalNames.value = settings.value.users.map((u) => u.name)
  savedSharedUsers.value = !!r.settings.sharedUsers
  running.value = r.running
  error.value = r.error
  supported.value = r.supported
}

async function save() {
  saving.value = true
  try {
    const r = await api.put<{ running: boolean; error: string }>('/api/smb/settings', settings.value)
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
  max-width: 640px;
}
.svc-form .el-form-item:last-child {
  margin-bottom: 0;
}
.w-260 {
  width: 260px;
}
.c-head,
.c-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) minmax(0, 2fr) minmax(0, 2fr);
  gap: 10px;
  align-items: center;
}
.c-head {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  padding-bottom: 6px;
}
.c-row {
  padding: 8px 0;
  border-top: 1px solid var(--g-border);
  font-size: 13px;
}
.c-row code {
  word-break: break-all;
  user-select: all;
}
.ro-tag {
  margin-left: 6px;
}
.help-inline {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

@media (max-width: 767.98px) {
  .w-260 {
    width: 100%;
  }
  .c-head {
    display: none;
  }
  .c-row {
    grid-template-columns: 1fr 1fr;
  }
  .c-row code,
  .c-row .help-inline {
    grid-column: 1 / -1;
  }
}
</style>
