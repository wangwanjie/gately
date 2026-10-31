<template>
  <div>
    <PageHeader :title="t('ssl.title')" :desc="t('ssl.desc')" :icon="ShieldCheck">
      <CompactToggle v-model="compact" />
      <ListTools :gl="gl" :placeholder="t('ssl.filterPlaceholder')" />
      <el-button :icon="SlidersHorizontal" @click="openSettings">{{ t('ssl.settingsTitle') }}</el-button>
      <el-button type="primary" :icon="Plus" @click="openEditor()">{{ t('ssl.addCert') }}</el-button>
    </PageHeader>

    <ModuleBanner
      v-if="renewSettings && !renewSettings.enable"
      :title="t('ssl.renewOffTitle')"
      :desc="t('ssl.renewOffDesc')"
      :action="t('ssl.renewOffAction')"
      :loading="enabling"
      @enable="enableRenew"
    />

    <GroupedList :gl="gl" :loading="loading" :compact="compact" :empty-text="t('ssl.empty')">
      <template #item="{ it, item: c, sorting, bind }">
        <EntityCard v-bind="bind" :off="!c.enable" :compact="compact || sorting">
          <template #lead>
            <SortCheck v-if="sorting" :gl="gl" :id="it.id" />
            <el-switch v-else :model-value="c.enable" @change="(v: string | number | boolean) => toggle(c, Boolean(v))" />
          </template>
          <template #title>
            <span>{{ c.remark || t('ssl.unnamed') }}</span>
            <el-tag size="small" effect="plain">{{ sourceText(c.source) }}</el-tag>
            <el-tag v-if="isShortLived(c)" size="small" effect="plain" type="warning">{{ t('ssl.shortLivedTag') }}</el-tag>
          </template>
          <template #status>
            <StatusBadge v-bind="certStatus(c)" />
          </template>
          <Field :label="t('ssl.domain')" mono wide>{{ domainsOf(c).join(', ') || '-' }}</Field>
          <Field :label="t('ssl.validUntil')">{{ validUntil(c) }}</Field>
          <Field :label="t('ssl.issuer')">{{ c.info?.issuer || '-' }}</Field>
          <Field :label="t('ssl.updatedAt')">{{ c.updatedAt ? new Date(c.updatedAt).toLocaleString() : '-' }}</Field>
          <template v-if="!sorting && (c.enable && (c.state.lastError || c.state.mappingNote || c.state.nextRetry || enabledDeploys(c).length))" #extra>
            <div v-if="c.state.lastError" class="err-line">{{ c.state.lastError }}</div>
            <div v-if="c.state.nextRetry" class="meta">{{ t('ssl.nextRetry', { time: new Date(c.state.nextRetry).toLocaleString() }) }}</div>
            <div v-if="c.state.mappingNote" class="meta">{{ c.state.mappingNote }}</div>
            <div v-for="d in enabledDeploys(c)" :key="d.index" class="deploy-line" :class="deployLine(c, d.index).cls">
              <span class="dot" />
              <span class="deploy-name">{{ deployTitle(d.deploy) }}</span>
              <span class="meta">{{ deployLine(c, d.index).text }}</span>
            </div>
          </template>
          <template v-if="sorting" #actions>
            <SortMoves :gl="gl" :it="it" />
          </template>
          <template v-else #actions>
            <el-button v-if="c.source === 'acme'" text size="small" :icon="RotateCw" :disabled="c.state.obtaining || !c.enable" @click="renew(c)">{{ t('ssl.renew') }}</el-button>
            <el-button v-if="c.info && enabledDeploys(c).length" text size="small" :icon="Send" :loading="deploying === c.key" :disabled="!c.enable" @click="deployNow(c)">{{ t('ssl.deployNow') }}</el-button>
            <el-dropdown v-if="c.info" trigger="click" @command="(cmd: string) => download(c, cmd)">
              <el-button text size="small" :icon="Download">{{ t('common.download') }}</el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="crt">{{ t('ssl.dlCrt') }}</el-dropdown-item>
                  <el-dropdown-item command="key">{{ t('ssl.dlKey') }}</el-dropdown-item>
                  <el-dropdown-item command="issuer" :disabled="!c.issuerPem">{{ t('ssl.dlIssuer') }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-button text size="small" :icon="Pencil" @click="openEditor(c)">{{ t('common.edit') }}</el-button>
            <el-popconfirm :title="t('ssl.confirmDelete')" width="240" @confirm="remove(c)">
              <template #reference>
                <el-button text size="small" type="danger" :icon="Trash2">{{ t('common.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </EntityCard>
      </template>
    </GroupedList>

    <LogCard url="/api/ssl/logs" clearable />

    <el-dialog v-model="editorVisible" :title="editing.key ? t('ssl.editCert') : t('ssl.addCert')" width="760px" top="5vh" :fullscreen="isMobile">
      <el-form :model="editing" label-width="130px" :label-position="labelPosition">
        <el-form-item :label="t('common.remark')">
          <el-input v-model="editing.remark" :placeholder="t('ssl.remarkPlaceholder')" />
        </el-form-item>
        <GroupInput v-model="editing.group" :groups="gl.groupNames.value" />
        <el-form-item :label="t('ssl.source')">
          <el-radio-group v-model="editing.source" :disabled="!!editing.key">
            <el-radio-button value="acme">{{ t('ssl.sourceAcme') }}</el-radio-button>
            <el-radio-button value="upload">{{ t('ssl.sourceUpload') }}</el-radio-button>
            <el-radio-button value="file">{{ t('ssl.sourceFile') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <template v-if="editing.source === 'acme'">
          <el-form-item :label="t('ssl.domain')" required>
            <el-input v-model="domainsText" type="textarea" :rows="2" :placeholder="t('ssl.domainsPlaceholder')" class="mono" />
          </el-form-item>
          <el-alert v-if="ipNeedsSetup" type="warning" :closable="false" show-icon class="http-tip" :title="t(editing.acme.ca === 'custom' ? 'ssl.ipCertTipCustom' : 'ssl.ipCertTip')">
            <el-button size="small" type="primary" link @click="applyIPCert">{{ t('ssl.ipCertApply') }}</el-button>
          </el-alert>
          <el-form-item :label="t('ssl.email')"><el-input v-model="editing.acme.email" :placeholder="t('ssl.emailPlaceholder')" /></el-form-item>
          <el-form-item :label="t('ssl.ca')">
            <el-select v-model="editing.acme.ca" class="w-260">
              <el-option label="Let's Encrypt" value="letsencrypt" />
              <el-option :label="t('ssl.caStaging')" value="letsencrypt-staging" />
              <el-option label="ZeroSSL" value="zerossl" />
              <el-option label="Google Trust Services" value="google" />
              <el-option :label="t('ssl.caCustom')" value="custom" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="editing.acme.ca === 'custom'" :label="t('ssl.directoryUrl')">
            <el-input v-model="editing.acme.directoryUrl" placeholder="https://.../directory" />
          </el-form-item>
          <template v-if="['zerossl', 'google', 'custom'].includes(editing.acme.ca)">
            <el-form-item label="EAB Key ID"><el-input v-model="editing.acme.eabKid" /></el-form-item>
            <el-form-item label="EAB HMAC Key"><SecretInput v-model="editing.acme.eabHmac" :reveal="secretOf(editing.key && `/api/ssl/certs/${editing.key}/secrets`, 'eabHmac')" /></el-form-item>
          </template>
          <el-form-item :label="t('ssl.keyType')">
            <el-select v-model="editing.acme.keyType" class="w-160">
              <el-option label="ECC P-256" value="P256" />
              <el-option label="ECC P-384" value="P384" />
              <el-option label="RSA 2048" value="2048" />
              <el-option label="RSA 4096" value="4096" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('ssl.profile')">
            <!-- 空字符串表示 CA 默认配置，需作为有效值显示对应选项；也可输入 CA 支持的其他配置名 -->
            <el-select v-model="editing.acme.profile" class="w-260" filterable allow-create default-first-option :empty-values="[null, undefined]">
              <el-option :label="t('ssl.profileDefault')" value="" />
              <el-option :label="t('ssl.profileShortLived')" value="shortlived" />
            </el-select>
            <span class="help">{{ editing.acme.profile === 'shortlived' ? t('ssl.profileShortLivedHelp', { hours: renewHours }) : t('ssl.profileHelp') }}</span>
          </el-form-item>
          <el-form-item :label="t('ssl.network')">
            <!-- 限定签发时的出站网络：有 IPv6 地址但 IPv6 不通的主机选择仅 IPv4 -->
            <el-select v-model="editing.acme.network" class="w-160" :empty-values="[null, undefined]">
              <el-option :label="t('ssl.networkAuto')" value="" />
              <el-option :label="t('ssl.networkIpv4')" value="tcp4" />
              <el-option :label="t('ssl.networkIpv6')" value="tcp6" />
            </el-select>
            <span class="help">{{ t('ssl.networkHelp') }}</span>
          </el-form-item>
          <el-divider content-position="left">{{ t('ssl.verifyMethod') }}</el-divider>
          <el-form-item :label="t('ssl.challenge')">
            <el-radio-group v-model="editing.acme.challenge">
              <el-radio-button value="dns">{{ t('ssl.challengeDns') }}</el-radio-button>
              <el-radio-button value="http">{{ t('ssl.challengeHttp') }}</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <template v-if="editing.acme.challenge === 'http'">
            <el-alert type="info" :closable="false" show-icon class="http-tip" :title="t('ssl.httpTip')" />
            <el-form-item :label="t('ssl.httpPort')">
              <el-input-number v-model="editing.acme.httpPort" :min="0" :max="65535" />
              <span class="help">{{ t('ssl.httpPortHelp') }}</span>
            </el-form-item>
          </template>
          <template v-else>
          <el-divider content-position="left">{{ t('ssl.dnsVerify') }}</el-divider>
          <el-form-item :label="t('ssl.dnsProvider')">
            <el-select v-model="editing.acme.provider" class="w-260" @change="extra = {}">
              <el-option v-for="p in providers" :key="p.name" :label="p.title" :value="p.name" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="currentProvider?.idLabel" :label="currentProvider.idLabel">
            <el-input v-model="editing.acme.credentials.id" />
          </el-form-item>
          <el-form-item v-if="currentProvider?.secretLabel" :label="currentProvider.secretLabel">
            <SecretInput v-model="editing.acme.credentials.secret" :placeholder="t('ssl.secretPlaceholder')" :reveal="secretOf(editing.key && `/api/ssl/certs/${editing.key}/secrets`, 'secret')" />
          </el-form-item>
          <el-form-item v-for="f in currentProvider?.extra ?? []" :key="f.key" :label="f.label">
            <el-input v-model="extra[f.key]" :placeholder="f.placeholder" />
          </el-form-item>
          <el-form-item :label="t('ssl.zone')"><el-input v-model="editing.acme.zone" :placeholder="t('ssl.zonePlaceholder')" /></el-form-item>
          <el-form-item :label="t('ssl.resolvers')"><el-input v-model="resolversText" :placeholder="t('ssl.resolversPlaceholder')" /></el-form-item>
          <el-form-item :label="t('ssl.propagationTimeout')"><el-input-number v-model="editing.acme.propagationTimeout" :min="30" :max="3600" /></el-form-item>
          <el-form-item :label="t('ssl.propagationCheck')">
            <el-checkbox v-model="editing.acme.disableAuthoritativeCheck">{{ t('ssl.disableAuthCheck') }}</el-checkbox>
            <el-checkbox v-model="editing.acme.skipPropagationCheck">{{ t('ssl.skipCheck') }}</el-checkbox>
          </el-form-item>
          </template>
        </template>

        <template v-if="editing.source === 'upload'">
          <el-form-item :label="t('ssl.certPem')" required>
            <el-input v-model="editing.certPem" type="textarea" :rows="5" class="mono" :placeholder="t('ssl.certPemPlaceholder')" />
            <label class="file-pick"><Upload :size="14" />{{ t('ssl.readFromFile') }}<input type="file" accept=".pem,.crt,.cer" @change="(e) => readFile(e, 'certPem')" /></label>
          </el-form-item>
          <el-form-item :label="t('ssl.keyPem')" required>
            <el-input v-model="editing.keyPem" type="textarea" :rows="4" class="mono" :placeholder="t('ssl.keyPemPlaceholder')" />
            <label class="file-pick"><Upload :size="14" />{{ t('ssl.readFromFile') }}<input type="file" accept=".pem,.key" @change="(e) => readFile(e, 'keyPem')" /></label>
          </el-form-item>
        </template>

        <template v-if="editing.source === 'file'">
          <el-form-item :label="t('ssl.certPath')" required><el-input v-model="editing.certFile" placeholder="/etc/ssl/example/fullchain.pem" /></el-form-item>
          <el-form-item :label="t('ssl.keyPath')" required><el-input v-model="editing.keyFile" placeholder="/etc/ssl/example/privkey.pem" /></el-form-item>
          <el-form-item label=" "><div class="form-help">{{ t('ssl.fileHelp') }}</div></el-form-item>
        </template>

        <el-divider content-position="left">{{ t('ssl.mapping') }}</el-divider>
        <el-form-item :label="t('ssl.writeDir')"><el-switch v-model="editing.mapping.enable" /></el-form-item>
        <template v-if="editing.mapping.enable">
          <el-form-item :label="t('ssl.dir')"><el-input v-model="editing.mapping.dir" :placeholder="t('ssl.dirPlaceholder')" /></el-form-item>
          <el-form-item :label="t('ssl.postScript')">
            <el-input v-model="editing.mapping.script" type="textarea" :rows="3" class="mono" :placeholder="t('ssl.postScriptPlaceholder')" />
            <div class="form-help">{{ t('ssl.envHelp') }}</div>
          </el-form-item>
        </template>
        <el-divider content-position="left">{{ t('ssl.deploy') }}</el-divider>
        <div class="form-help deploy-help">{{ t('ssl.deployHelp') }}</div>
        <div v-for="(d, i) in editing.deploys" :key="i" class="deploy-box">
          <div class="deploy-head">
            <el-switch v-model="d.enable" size="small" />
            <el-radio-group v-model="d.type" size="small">
              <el-radio-button value="local">{{ t('ssl.deployLocal') }}</el-radio-button>
              <el-radio-button value="ssh">{{ t('ssl.deploySSH') }}</el-radio-button>
              <el-radio-button value="docker">{{ t('ssl.deployDocker') }}</el-radio-button>
            </el-radio-group>
            <el-input v-model="d.remark" size="small" :placeholder="t('common.remark')" class="deploy-remark" />
            <el-button text type="danger" size="small" :icon="Trash2" :aria-label="t('ssl.deleteDeploy')" @click="editing.deploys.splice(i, 1)" />
          </div>
          <template v-if="d.type === 'ssh' || d.type === 'local'">
            <el-form-item v-if="d.type === 'ssh'" :label="t('ssl.deployConnection')">
              <el-select v-model="d.connection" filterable :placeholder="t('ssl.deployConnectionPlaceholder')" :no-data-text="t('ssl.deployNoConnections')" class="full">
                <el-option v-for="o in targets.ssh" :key="o.key" :label="o.name" :value="o.key">
                  <span>{{ o.name }}</span>
                  <span class="opt-meta mono">{{ o.host }}</span>
                  <span v-if="!o.trusted" class="opt-warn">{{ t('ssl.deployUntrusted') }}</span>
                </el-option>
              </el-select>
              <div v-if="d.connection && targets.ssh.find((o) => o.key === d.connection && !o.trusted)" class="form-help warn">{{ t('ssl.deployUntrustedHelp') }}</div>
            </el-form-item>
            <el-form-item :label="d.type === 'ssh' ? t('ssl.deployDir') : t('ssl.deployLocalDir')">
              <el-input v-model="d.dir" class="mono" placeholder="/etc/nginx/certs" />
            </el-form-item>
            <el-form-item :label="t('ssl.deployFileNames')">
              <div class="deploy-names">
                <el-input v-model="d.certName" class="mono" :placeholder="defaultFileName('.crt')" />
                <el-input v-model="d.keyName" class="mono" :placeholder="defaultFileName('.key')" />
              </div>
              <div class="form-help">{{ t('ssl.deployFileNamesHelp') }}</div>
            </el-form-item>
            <el-form-item :label="d.type === 'ssh' ? t('ssl.deployCommand') : t('ssl.deployLocalCommand')">
              <el-input v-model="d.command" type="textarea" :rows="2" class="mono" :placeholder="d.type === 'ssh' ? 'systemctl reload nginx' : 'docker exec nginx nginx -s reload'" />
              <div class="form-help">{{ d.type === 'ssh' ? t('ssl.deployCommandHelp') : t('ssl.deployLocalCommandHelp') }}</div>
            </el-form-item>
          </template>
          <el-form-item v-else :label="t('ssl.deployContainers')">
            <el-select v-model="d.containers" multiple filterable allow-create default-first-option :placeholder="t('ssl.deployContainersPlaceholder')" class="full">
              <el-option v-for="n in targets.containers" :key="n" :label="n" :value="n" />
            </el-select>
            <div class="form-help">{{ targets.containersError ? t('ssl.deployContainersError', { error: targets.containersError }) : t('ssl.deployContainersHelp') }}</div>
          </el-form-item>
        </div>
        <el-button size="small" :icon="Plus" @click="addDeploy">{{ t('ssl.addDeploy') }}</el-button>
        <el-form-item :label="t('common.enable')" class="enable-row"><el-switch v-model="editing.enable" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editorVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="settingsVisible" :title="t('ssl.settingsTitle')" width="520px" :fullscreen="isMobile">
      <el-form v-if="settings" :model="settings" label-width="170px" :label-position="labelPosition">
        <el-form-item :label="t('ssl.autoRenew')"><el-switch v-model="settings.enable" /></el-form-item>
        <el-form-item :label="t('ssl.renewBeforeDays')"><el-input-number v-model="settings.renewBeforeDays" :min="1" :max="60" /></el-form-item>
        <el-form-item :label="t('ssl.shortLivedHours')"><el-input-number v-model="settings.shortLivedRenewHours" :min="1" :max="80" /></el-form-item>
        <el-form-item :label="t('ssl.defaultEmail')"><el-input v-model="settings.defaultEmail" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settingsVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveSettings">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Download, Pencil, Plus, RotateCw, Send, ShieldCheck, SlidersHorizontal, Trash2, Upload } from 'lucide-vue-next'
import { api } from '../api'
import SecretInput from '../components/SecretInput.vue'
import { secretOf } from '../secrets'
import PageHeader from '../components/PageHeader.vue'
import ModuleBanner from '../components/ModuleBanner.vue'
import CompactToggle from '../components/CompactToggle.vue'
import EntityCard from '../components/EntityCard.vue'
import GroupedList from '../components/GroupedList.vue'
import GroupInput from '../components/GroupInput.vue'
import ListTools from '../components/ListTools.vue'
import SortCheck from '../components/SortCheck.vue'
import SortMoves from '../components/SortMoves.vue'
import { useGroupedList } from '../composables/useGroupSort'
import { useCompactView } from '../composables/useCompactView'
import Field from '../components/Field.vue'
import StatusBadge, { type StatusType } from '../components/StatusBadge.vue'
import LogCard from '../components/LogCard.vue'
import { useFormLayout } from '../composables/useIsMobile'
import { useListText, splitLinesOrCommas } from '../composables/useListText'
import { usePolling } from '../composables/usePolling'

interface ProviderMeta {
  name: string
  title: string
  idLabel?: string
  secretLabel?: string
  extra?: { key: string; label: string; placeholder?: string }[]
}

interface Cert {
  key?: string
  remark: string
  enable: boolean
  // group 为列表中的分组名称，只影响展示，留空为未分组
  group?: string
  source: string
  acme: {
    domains: string[] | null
    email: string
    ca: string
    directoryUrl: string
    eabKid: string
    eabHmac: string
    keyType: string
    // profile 为 ACME 证书配置，空为 CA 默认；shortlived 为短期证书（IP 证书需要）
    profile: string
    challenge: string
    httpPort: number
    provider: string
    credentials: { id: string; secret: string; extra?: Record<string, string> }
    zone: string
    resolvers: string[] | null
    propagationTimeout: number
    skipPropagationCheck: boolean
    disableAuthoritativeCheck: boolean
    // network 限定签发时的出站网络：空为自动，tcp4 仅 IPv4，tcp6 仅 IPv6
    network?: string
  }
  certFile: string
  keyFile: string
  certPem: string
  keyPem: string
  issuerPem: string
  mapping: { enable: boolean; dir: string; script: string }
  deploys: Deploy[]
  updatedAt?: string
  // issuedProfile 为当前证书签发时的证书配置，由后端维护
  issuedProfile?: string
}

// Deploy 是证书变化后自动执行的部署目标：复制到本机目录、经 SSH 上传到远程主机，或重启本机容器
interface Deploy {
  enable: boolean
  type: 'local' | 'ssh' | 'docker'
  remark: string
  connection: string
  dir: string
  // certName / keyName 为自定义的证书与私钥文件名，需同时填写；留空沿用证书映射的文件名
  certName: string
  keyName: string
  command: string
  containers: string[]
  deployedHash?: string
  deployedAt?: string
}

interface DeployState {
  running: boolean
  lastError?: string
  nextRetry?: string
}

interface CertView extends Cert {
  key: string
  info?: { domains: string[]; issuer: string; notBefore: string; notAfter: string }
  state: { obtaining: boolean; lastError?: string; nextRetry?: string; mappingNote?: string; deploys?: DeployState[] }
}

// DeployTargets 对应 GET /api/ssl/deploy-targets
interface DeployTargets {
  ssh: { key: string; name: string; host: string; trusted: boolean }[]
  containers: string[]
  containersError?: string
}

const certs = ref<CertView[]>([])
const providers = ref<ProviderMeta[]>([])
const loading = ref(true)
const compact = useCompactView('ssl')
// 列表的搜索、分组与排序；顺序与分组的修改不会重新申请或部署证书
const gl = useGroupedList<CertView>({
  storageKey: 'ssl.groups',
  items: () => certs.value,
  base: '/api/ssl/certs',
  reload: load,
  matches: (c, q) => [c.remark, c.group ?? '', sourceText(c.source), ...domainsOf(c)].join('\n').toLowerCase().includes(q),
})
const editorVisible = ref(false)
const saving = ref(false)
const editing = ref<Cert>(emptyCert())
const extra = ref<Record<string, string>>({})
const settingsVisible = ref(false)
interface RenewSettings {
  enable: boolean
  renewBeforeDays: number
  shortLivedRenewHours: number
  defaultEmail: string
}
const settings = ref<RenewSettings>()
// renewSettings 为当前生效的续签设置，用于显示横幅
const renewSettings = ref<RenewSettings>()
const enabling = ref(false)
const targets = ref<DeployTargets>({ ssh: [], containers: [] })
const deploying = ref('')
const { t } = useI18n()
const { isMobile, labelPosition } = useFormLayout()
// 定时刷新，标签页在后台时暂停
const polling = usePolling(load, 5000)

const currentProvider = computed(() => providers.value.find((p) => p.name === editing.value.acme.provider))
// 输入框保留原始文本，回车、逗号可正常输入（见 useListText）
// isIPAddress 判断是否为 IP 地址（IPv6 可带方括号），域名中不会出现冒号
function isIPAddress(s: string) {
  const v = s.trim().replace(/^\[|\]$/g, '')
  return /^\d{1,3}(\.\d{1,3}){3}$/.test(v) || v.includes(':')
}
// 短期证书续签提前小时数，用于提示与状态判断
const renewHours = computed(() => renewSettings.value?.shortLivedRenewHours ?? 24)
// ipNeedsSetup 在填写了 IP 地址、但验证方式或证书配置不满足 IP 证书要求时提示一键设置
// （自建 CA 不限制 CA 与证书配置，但与后端一致仍要求 HTTP-01 验证）
const ipNeedsSetup = computed(() => {
  const a = editing.value.acme
  if (!(a.domains ?? []).some(isIPAddress)) {
    return false
  }
  if (a.ca === 'custom') {
    return a.challenge !== 'http'
  }
  return a.challenge !== 'http' || a.profile !== 'shortlived' || ['zerossl', 'google'].includes(a.ca)
})

// applyIPCert 按 IP 证书要求设置：Let's Encrypt、短期证书、HTTP-01 验证；自建 CA 只切换验证方式
function applyIPCert() {
  const a = editing.value.acme
  if (a.ca === 'custom') {
    a.challenge = 'http'
    return
  }
  if (a.ca !== 'letsencrypt-staging') {
    a.ca = 'letsencrypt'
  }
  a.profile = 'shortlived'
  a.challenge = 'http'
}

const domainsText = useListText(() => editing.value.acme.domains, (v) => (editing.value.acme.domains = v), { split: splitLinesOrCommas })
const resolversText = useListText(() => editing.value.acme.resolvers, (v) => (editing.value.acme.resolvers = v), {
  split: splitLinesOrCommas,
  separator: ', ',
})

function emptyCert(): Cert {
  return {
    remark: '',
    enable: true,
    source: 'acme',
    acme: {
      domains: [],
      email: '',
      ca: 'letsencrypt',
      directoryUrl: '',
      eabKid: '',
      eabHmac: '',
      keyType: 'P256',
      profile: '',
      challenge: 'dns',
      httpPort: 0,
      provider: 'alidns',
      credentials: { id: '', secret: '' },
      zone: '',
      resolvers: [],
      propagationTimeout: 300,
      skipPropagationCheck: false,
      disableAuthoritativeCheck: false,
      network: '',
    },
    certFile: '',
    keyFile: '',
    certPem: '',
    keyPem: '',
    issuerPem: '',
    mapping: { enable: false, dir: '', script: '' },
    deploys: [],
  }
}

function addDeploy() {
  editing.value.deploys.push({ enable: true, type: 'local', remark: '', connection: '', dir: '', certName: '', keyName: '', command: '', containers: [] })
}

// defaultFileName 返回未自定义文件名时使用的默认文件名（备注中的 * 替换为 _，与后端一致）
function defaultFileName(ext: string) {
  const base = editing.value.remark.trim().replaceAll('*', '_') || t('common.remark')
  return base + ext
}

// enabledDeploys 返回启用的部署目标及其下标（运行状态按下标对应）
function enabledDeploys(c: CertView) {
  return (c.deploys ?? []).map((deploy, index) => ({ deploy, index })).filter((d) => d.deploy.enable)
}

// deployTitle 返回部署目标的显示名：备注优先，否则显示主机或容器
function deployTitle(d: Deploy) {
  if (d.remark) {
    return d.remark
  }
  if (d.type === 'docker') {
    return t('ssl.deployDockerTitle', { names: (d.containers ?? []).join(', ') })
  }
  if (d.type === 'local') {
    return t('ssl.deployLocalTitle', { dir: d.dir })
  }
  const name = targets.value.ssh.find((o) => o.key === d.connection)?.name ?? d.connection
  return `${name}:${d.dir}`
}

// deployLine 计算部署目标的状态文字与样式
function deployLine(c: CertView, i: number): { text: string; cls: string } {
  const st = c.state.deploys?.[i]
  const d = c.deploys[i]
  if (st?.running) {
    return { text: t('ssl.deployRunning'), cls: 'pending' }
  }
  if (st?.lastError) {
    const retry = st.nextRetry ? t('ssl.deployRetryAt', { time: new Date(st.nextRetry).toLocaleString() }) : ''
    return { text: t('ssl.deployFailed', { error: st.lastError }) + retry, cls: 'error' }
  }
  if (d.deployedAt) {
    return { text: t('ssl.deployedAt', { time: new Date(d.deployedAt).toLocaleString() }), cls: 'ok' }
  }
  return { text: t('ssl.deployPending'), cls: 'pending' }
}

async function loadTargets() {
  try {
    targets.value = await api.get<DeployTargets>('/api/ssl/deploy-targets')
  } catch {
    targets.value = { ssh: [], containers: [] }
  }
}

// deployNow 立即部署到全部启用的目标，逐个提示结果
async function deployNow(c: CertView) {
  deploying.value = c.key
  try {
    const res = await api.post<{ index: number; name: string; ok: boolean; error?: string }[]>(`/api/ssl/certs/${c.key}/deploy`)
    const failed = res.filter((r) => !r.ok)
    if (failed.length) {
      ElMessage.error(failed.map((r) => `${r.name}: ${r.error}`).join('\n'))
    } else {
      ElMessage.success(t('ssl.deployDone', { count: res.length }))
    }
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    deploying.value = ''
    await load()
  }
}

function sourceText(s: string) {
  return ({ acme: 'ACME', upload: t('ssl.sourceUploadShort'), file: t('ssl.sourceFileShort') } as Record<string, string>)[s] ?? s
}

// isShortLived 判断是否为短期证书：配置为短期证书，或已签发证书总有效期不足 10 天（与后端一致）
function isShortLived(c: CertView) {
  if (c.source === 'acme' && c.acme.profile === 'shortlived') {
    return true
  }
  return !!c.info && new Date(c.info.notAfter).getTime() - new Date(c.info.notBefore).getTime() < 10 * 86400000
}

// validUntil 显示到期时间，短期证书精确到时分
function validUntil(c: CertView) {
  if (!c.info) {
    return '-'
  }
  const d = new Date(c.info.notAfter)
  return isShortLived(c) ? d.toLocaleString() : d.toLocaleDateString()
}

function domainsOf(c: CertView) {
  return c.info?.domains ?? c.acme.domains ?? []
}

// certStatus 计算证书展示状态：停用的证书只显示“已停用”
function certStatus(c: CertView): { type: StatusType; text: string; title?: string } {
  if (!c.enable) {
    return { type: 'off', text: t('ssl.deactivated') }
  }
  if (c.state.obtaining) {
    return { type: 'pending', text: t('ssl.issuing') }
  }
  if (c.info) {
    const left = new Date(c.info.notAfter).getTime() - Date.now()
    const d = Math.floor(left / 86400000)
    if (left < 0) {
      return { type: 'error', text: t('ssl.expired') }
    }
    if (isShortLived(c)) {
      // 短期证书剩余不足 2 天时按小时显示；剩余不到续签提前量的一半说明续签未成功
      const h = Math.floor(left / 3600000)
      const text = d < 2 ? t('ssl.hoursLeft', { hours: h }) : t('ssl.daysLeft', { days: d })
      if (c.state.lastError) {
        return { type: 'warn', text: `${text} · ${t('ssl.renewFailedShort')}`, title: c.state.lastError }
      }
      return { type: h < renewHours.value / 2 ? 'warn' : 'ok', text }
    }
    if (c.state.lastError) {
      return { type: 'warn', text: t('ssl.daysLeftRenewFailed', { days: d }), title: c.state.lastError }
    }
    return { type: d < 15 ? 'warn' : 'ok', text: t('ssl.daysLeft', { days: d }) }
  }
  if (c.state.lastError) {
    return { type: 'error', text: t('ssl.issueFailed'), title: c.state.lastError }
  }
  return { type: 'pending', text: c.source === 'acme' ? t('ssl.pendingIssue') : t('ssl.notLoaded') }
}

async function load() {
  try {
    certs.value = await api.get<CertView[]>('/api/ssl/certs')
  } finally {
    loading.value = false
  }
}

function openEditor(row?: CertView) {
  if (row) {
    const { info: _i, state: _s, ...cert } = row
    editing.value = JSON.parse(JSON.stringify(cert))
    // 旧配置没有验证方式字段，视为 DNS 验证
    editing.value.acme.challenge ||= 'dns'
    editing.value.acme.profile ??= ''
    // 自动网络在后端省略该字段，补成空串让下拉框选中「自动」
    editing.value.acme.network ??= ''
    editing.value.deploys = (editing.value.deploys ?? []).map((d) => ({ ...d, certName: d.certName ?? '', keyName: d.keyName ?? '', containers: d.containers ?? [] }))
  } else {
    editing.value = emptyCert()
  }
  loadTargets()
  extra.value = { ...(editing.value.acme.credentials.extra ?? {}) }
  editorVisible.value = true
}

function readFile(e: Event, field: 'certPem' | 'keyPem') {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (f) {
    f.text().then((text) => (editing.value[field] = text))
  }
}

async function save() {
  editing.value.acme.credentials.extra = { ...extra.value }
  saving.value = true
  try {
    const { key, ...body } = editing.value
    if (key) {
      await api.put(`/api/ssl/certs/${key}`, body)
    } else {
      await api.post('/api/ssl/certs', body)
    }
    editorVisible.value = false
    ElMessage.success(body.source === 'acme' ? t('ssl.savedAcme') : t('common.saved'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function toggle(row: CertView, enable: boolean) {
  try {
    await api.put(`/api/ssl/certs/${row.key}/enable`, { enable })
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function renew(row: CertView) {
  try {
    await api.post(`/api/ssl/certs/${row.key}/renew`)
    ElMessage.success(t('ssl.renewStarted'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

function download(row: CertView, type: string) {
  window.open(`/api/ssl/certs/${row.key}/download?type=${type}`, '_blank')
}

async function remove(row: CertView) {
  try {
    await api.del(`/api/ssl/certs/${row.key}`)
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function loadRenew() {
  renewSettings.value = await api.get<RenewSettings>('/api/ssl/settings')
}

async function openSettings() {
  await loadRenew()
  settings.value = JSON.parse(JSON.stringify(renewSettings.value))
  settingsVisible.value = true
}

async function enableRenew() {
  if (!renewSettings.value) {
    return
  }
  enabling.value = true
  try {
    await api.put('/api/ssl/settings', { ...renewSettings.value, enable: true })
    ElMessage.success(t('ssl.renewEnabled'))
    await loadRenew()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    enabling.value = false
  }
}

async function saveSettings() {
  try {
    await api.put('/api/ssl/settings', settings.value)
    settingsVisible.value = false
    ElMessage.success(t('common.saved'))
    await loadRenew()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

onMounted(async () => {
  loadRenew()
  loadTargets()
  providers.value = await api.get<ProviderMeta[]>('/api/ssl/providers')
  await load()
  polling.start()
})
</script>

<style scoped>
.http-tip {
  margin-bottom: 14px;
}
.err-line {
  font-size: 12px;
  color: var(--el-color-danger);
  word-break: break-all;
}
.w-260 {
  width: 260px;
}
.w-160 {
  width: 160px;
}
.file-pick {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-top: 6px;
  font-size: 12px;
  color: var(--el-color-primary);
  cursor: pointer;
}
.file-pick input {
  display: none;
}
.deploy-help {
  margin: -6px 0 12px;
}
.deploy-box {
  border: 1px solid var(--g-border);
  border-radius: 8px;
  padding: 10px 10px 0;
  margin-bottom: 10px;
}
.deploy-head {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 10px;
  align-items: center;
  margin-bottom: 12px;
}
.deploy-names {
  display: flex;
  gap: 8px;
  width: 100%;
}
.deploy-remark {
  width: 200px;
}
.full {
  width: 100%;
}
.opt-meta {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.opt-warn {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-color-warning);
}
.form-help.warn {
  color: var(--el-color-warning);
}
.enable-row {
  margin-top: 16px;
}
.deploy-line {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 12px;
}
.deploy-line .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--el-text-color-placeholder);
}
.deploy-line.ok .dot {
  background: var(--el-color-success);
}
.deploy-line.error .dot {
  background: var(--el-color-danger);
}
.deploy-line.error .meta {
  color: var(--el-color-danger);
  word-break: break-all;
}
.deploy-line.pending .dot {
  background: var(--el-color-warning);
}
.el-checkbox {
  white-space: normal;
  height: auto;
  min-height: 32px;
}
@media (max-width: 767.98px) {
  .w-260,
  .w-160 {
    width: 100%;
  }
  .deploy-remark {
    width: auto;
    flex: 1;
    min-width: 120px;
  }
}
</style>
