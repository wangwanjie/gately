<template>
  <!-- 快速建站向导：域名 → 后端地址 → HTTP / HTTPS，一步建好站点，按需复用或新建监听、选择已有证书或就地申请 -->
  <el-dialog
    v-model="visible"
    :title="t('quickSite.title')"
    width="min(720px, 96vw)"
    top="6vh"
    append-to-body
    :fullscreen="isMobile"
    :close-on-click-modal="false"
    class="quick-site"
  >
    <el-steps v-if="step < 3" :active="step" finish-status="success" simple class="qs-steps">
      <el-step :title="t('quickSite.stepDomain')" />
      <el-step :title="t('quickSite.stepBackend')" />
      <el-step :title="t('quickSite.stepProtocol')" />
    </el-steps>

    <!-- 第一步：域名 -->
    <div v-show="step === 0" class="qs-body">
      <label class="qs-label" for="qs-domain">{{ t('quickSite.domainLabel') }}</label>
      <el-input id="qs-domain" v-model="domainInput" class="mono" :placeholder="t('quickSite.domainPlaceholder')" autofocus @keyup.enter="next" />
      <div v-if="domainErr" class="qs-error">{{ domainErr }}</div>
      <div class="qs-help">{{ t('quickSite.domainHelp') }}</div>
    </div>

    <!-- 第二步：后端地址，可选常见应用模板 -->
    <div v-show="step === 1" class="qs-body">
      <div class="qs-label">{{ t('quickSite.templates') }}</div>
      <div class="tpl-grid">
        <button
          v-for="tp in templates"
          :key="tp.id"
          type="button"
          class="tpl"
          :class="{ on: tpl?.id === tp.id }"
          :aria-pressed="tpl?.id === tp.id"
          :data-tpl="tp.id"
          @click="pickTemplate(tp)"
        >
          <PanelIcon :icon="`builtin:${tp.icon}`" :name="tp.name" :color="tp.color" :size="26" />
          <span class="tpl-name">{{ tp.id === 'fnos' ? t('quickSite.fnos') : tp.name }}</span>
        </button>
      </div>
      <el-alert v-if="tpl" type="info" :closable="false" show-icon class="qs-note" :title="t(`quickSite.tpl.${tpl.id}`)" />

      <label class="qs-label" for="qs-target">{{ t('quickSite.targetLabel') }}</label>
      <div class="qs-row">
        <el-input id="qs-target" v-model="targetInput" class="mono" :placeholder="t('quickSite.targetPlaceholder')" @keyup.enter="next" @change="probeResult = null" />
        <el-button :icon="Activity" :loading="probing" :disabled="!targetInput.trim()" @click="probe">{{ t('quickSite.test') }}</el-button>
      </div>
      <div v-if="targetErr" class="qs-error">{{ targetErr }}</div>
      <div v-else-if="probeResult" class="qs-probe" :class="probeResult.reachable ? 'ok' : 'bad'" data-testid="probe-result">
        <CircleCheck v-if="probeResult.reachable" :size="15" />
        <CircleX v-else :size="15" />
        <span>{{ probeText }}</span>
      </div>
      <div v-else class="qs-help">{{ t('quickSite.targetHelp') }}</div>
      <el-checkbox v-if="targetIsHttps" v-model="skipVerify" class="qs-check">{{ t('quickSite.skipVerify') }}</el-checkbox>
      <div class="qs-help">{{ t('quickSite.wsHint') }}</div>
    </div>

    <!-- 第三步：HTTP 或 HTTPS、监听与证书 -->
    <div v-show="step === 2" class="qs-body">
      <div class="qs-label">{{ t('quickSite.protocol') }}</div>
      <el-radio-group v-model="https" class="qs-proto">
        <el-radio-button :value="true" :disabled="!sslOn" data-proto="https">HTTPS</el-radio-button>
        <el-radio-button :value="false" data-proto="http">HTTP</el-radio-button>
      </el-radio-group>
      <div class="qs-help">{{ sslOn ? (https ? t('quickSite.httpsHelp') : t('quickSite.httpHelp')) : t('quickSite.sslOff') }}</div>

      <div class="qs-label">{{ t('quickSite.listener') }}</div>
      <!-- 空字符串表示新建监听，需作为有效值显示对应选项 -->
      <el-select v-model="listenerKey" class="qs-full" data-testid="qs-listener" :empty-values="[null, undefined]">
        <el-option v-for="l in choices" :key="l.key" :value="l.key" :label="listenerText(l)" />
        <el-option value="" :label="t('quickSite.newListener')" />
      </el-select>
      <template v-if="!listenerKey">
        <div class="qs-row qs-port">
          <span>{{ t('quickSite.port') }}</span>
          <el-input-number v-model="newPort" :min="1" :max="65535" controls-position="right" data-testid="qs-port" />
        </div>
        <div class="qs-help">{{ planHint }}</div>
      </template>

      <template v-if="https">
        <div class="qs-label">{{ t('quickSite.cert') }}</div>
        <el-alert v-if="cert" type="success" :closable="false" show-icon :title="t('quickSite.certFound', { name: certName(cert) })" />
        <template v-else>
          <div class="qs-help qs-cert-hint">{{ t('quickSite.certNone', { domain }) }}</div>
          <el-radio-group v-model="challenge" class="qs-proto">
            <el-radio-button value="dns" data-challenge="dns">{{ t('ssl.challengeDns') }}</el-radio-button>
            <el-radio-button value="http" data-challenge="http">{{ t('ssl.challengeHttp') }}</el-radio-button>
          </el-radio-group>
          <el-form v-if="challenge === 'dns'" label-width="150px" :label-position="isMobile ? 'top' : 'right'" class="qs-form" data-testid="qs-cert-form">
            <el-form-item :label="t('ssl.dnsProvider')">
              <el-select v-model="provider" filterable class="qs-provider" @change="extra = {}">
                <el-option v-for="p in providers" :key="p.name" :label="p.title" :value="p.name" />
              </el-select>
              <el-link v-if="currentProvider?.docUrl" :href="currentProvider.docUrl" target="_blank" type="primary" class="cred-link">{{ t('ssl.getCredentials') }}</el-link>
            </el-form-item>
            <el-form-item v-if="currentProvider?.idLabel" :label="currentProvider.idLabel">
              <el-input v-model="credId" autocomplete="off" />
            </el-form-item>
            <el-form-item v-if="currentProvider?.secretLabel" :label="currentProvider.secretLabel">
              <el-input v-model="credSecret" type="password" show-password autocomplete="new-password" />
            </el-form-item>
            <el-form-item v-for="f in currentProvider?.extra ?? []" :key="f.key" :label="f.label">
              <el-input v-model="extra[f.key]" :placeholder="f.placeholder" />
            </el-form-item>
          </el-form>
          <el-alert v-else type="info" :closable="false" show-icon class="qs-note" :title="t('ssl.httpTip')" data-testid="qs-cert-form" />
          <!-- ACME 账户邮箱：证书模块的续签设置里有默认邮箱时预填，没有时必填 -->
          <div class="qs-row qs-email">
            <span>{{ t('ssl.email') }}</span>
            <el-input v-model="email" type="email" autocomplete="email" data-testid="qs-email" />
          </div>
          <div v-if="emailErr" class="qs-error">{{ emailErr }}</div>
          <div v-else class="qs-help">{{ t('quickSite.emailHelp') }}</div>
          <div v-if="domain.startsWith('*.') && challenge === 'http'" class="qs-error">{{ t('quickSite.wildcardHttp') }}</div>
        </template>
      </template>
    </div>

    <!-- 完成 -->
    <div v-if="step === 3" class="qs-body qs-done" data-testid="qs-done">
      <CircleCheck :size="44" class="done-icon" />
      <div class="done-title">{{ t('quickSite.doneTitle') }}</div>
      <div class="done-url"><UrlLink :href="resultURL" /></div>
      <el-alert v-if="certRequested" type="warning" :closable="false" show-icon :title="t('quickSite.certPending')">
        <el-button size="small" type="primary" link @click="goCert">{{ t('quickSite.viewCert') }}</el-button>
      </el-alert>
      <div class="dns-tip">
        <div class="dns-title"><HelpCircle :size="15" />{{ t('quickSite.dnsTitle') }}</div>
        <ul>
          <li>{{ t('quickSite.dnsRecord') }}</li>
          <li>
            {{ t('quickSite.dnsDynamic') }}
            <el-link v-if="ddnsOn" type="primary" @click="go('/ddns')">{{ t('quickSite.openDdns') }}</el-link>
          </li>
          <li>{{ t('quickSite.dnsHosts', { domain }) }}</li>
        </ul>
        <el-link :href="docsUrl(locale, '/docs/guide/quick-start')" target="_blank" type="primary">{{ t('quickSite.tutorial') }}</el-link>
      </div>
    </div>

    <template #footer>
      <template v-if="step < 3">
        <el-button v-if="step > 0" :disabled="saving" @click="step--">{{ t('quickSite.prev') }}</el-button>
        <el-button v-else @click="visible = false">{{ t('common.cancel') }}</el-button>
        <el-button v-if="step < 2" type="primary" @click="next">{{ t('quickSite.next') }}</el-button>
        <el-button v-else type="primary" :loading="saving" data-testid="qs-submit" @click="submit">{{ t('quickSite.publish') }}</el-button>
      </template>
      <template v-else>
        <el-button @click="restart">{{ t('quickSite.another') }}</el-button>
        <el-button type="primary" @click="visible = false">{{ t('common.close') }}</el-button>
      </template>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Activity, CircleCheck, CircleX, HelpCircle } from 'lucide-vue-next'
import { api } from '../api'
import PanelIcon from './PanelIcon.vue'
import UrlLink from './UrlLink.vue'
import { useIsMobile } from '../composables/useIsMobile'
import { moduleEnabled } from '../nav'
import { docsUrl } from '../docsUrl'
import templateData from '../siteTemplates.json'
import {
  domainError,
  findCert,
  hostOfTarget,
  listenerChoices,
  listenerPayload,
  normalizeDomain,
  normalizeTarget,
  planListener,
  sitePayload,
  siteURL,
  targetError,
  templateTarget,
  type CertLike,
  type ListenerBrief,
  type SiteTemplate,
} from '../quickSite'

const emit = defineEmits<{ done: [] }>()

const { t, locale } = useI18n()
const router = useRouter()
const isMobile = useIsMobile()
const templates = templateData as SiteTemplate[]

interface ProviderMeta {
  name: string
  title: string
  idLabel?: string
  secretLabel?: string
  extra?: { key: string; label: string; placeholder?: string }[]
  docUrl?: string
}

// 监听列表接口里只用于展示的运行态字段，保存时去掉
type ListenerView = ListenerBrief & Record<string, unknown> & { sites?: object[] | null }
const VIEW_FIELDS = ['running', 'error', 'stats', 'upstreams', 'firewall', 'firewallFailed', 'siteCerts']

interface ProbeResult {
  reachable: boolean
  status?: number
  insecure?: boolean
  reason?: string
  elapsedMs: number
}

const visible = ref(false)
const step = ref(0)
const saving = ref(false)
const domainInput = ref('')
const domainErr = ref('')
const targetInput = ref('')
const targetErr = ref('')
const tpl = ref<SiteTemplate | null>(null)
const skipVerify = ref(false)
const probing = ref(false)
const probeResult = ref<ProbeResult | null>(null)
const https = ref(true)
const listeners = ref<ListenerView[]>([])
const listenerKey = ref('')
const newPort = ref(443)
const certs = ref<CertLike[]>([])
const providers = ref<ProviderMeta[]>([])
const challenge = ref<'dns' | 'http'>('dns')
const provider = ref('cloudflare')
const credId = ref('')
const credSecret = ref('')
const extra = ref<Record<string, string>>({})
const email = ref('')
// defaultEmail 为证书模块续签设置中的默认邮箱
const defaultEmail = ref('')
const emailErr = ref('')
const resultURL = ref('')
const certRequested = ref(false)

const sslOn = computed(() => moduleEnabled('ssl'))
const ddnsOn = computed(() => moduleEnabled('ddns'))
const domain = computed(() => normalizeDomain(domainInput.value))
const target = computed(() => normalizeTarget(targetInput.value))
const targetIsHttps = computed(() => target.value.startsWith('https://'))
const choices = computed(() => listenerChoices(listeners.value, https.value))
const cert = computed(() => (https.value ? findCert(certs.value, domain.value) : null))
const currentProvider = computed(() => providers.value.find((p) => p.name === provider.value))
const plan = computed(() => planListener(listeners.value, https.value, newPort.value))
const planHint = computed(() => (plan.value.httpPort ? t('quickSite.planRedirect', { port: plan.value.port }) : t('quickSite.planPlain', { port: plan.value.port })))

// 切换协议时默认选中标准端口的已有监听，没有时新建
watch(https, () => resetListener())

function resetListener() {
  const c = choices.value
  const std = https.value ? 443 : 80
  listenerKey.value = c.find((l) => l.enable && l.port === std)?.key ?? c.find((l) => l.enable)?.key ?? ''
  newPort.value = std
}

function listenerText(l: ListenerBrief) {
  const st = l.enable ? '' : ` (${t('quickSite.disabled')})`
  return `${l.name || '-'} · ${l.tls ? 'https' : 'http'}://${l.listenIp || '*'}:${l.port}${st}`
}

function certName(c: CertLike) {
  return c.remark || (c.info?.domains ?? c.acme?.domains ?? []).join(', ')
}

const probeText = computed(() => {
  const r = probeResult.value
  if (!r) {
    return ''
  }
  if (r.reachable) {
    const base = r.status ? t('quickSite.probeOkHttp', { status: r.status, ms: r.elapsedMs }) : t('quickSite.probeOkTcp', { ms: r.elapsedMs })
    return r.insecure ? `${base} ${t('quickSite.probeInsecure')}` : base
  }
  return t(`quickSite.reason.${r.reason ?? 'error'}`)
})

function pickTemplate(tp: SiteTemplate) {
  if (tpl.value?.id === tp.id) {
    tpl.value = null
    return
  }
  tpl.value = tp
  targetInput.value = templateTarget(tp, hostOfTarget(targetInput.value))
  skipVerify.value = !!tp.site.insecureSkipVerify
  https.value = tp.https && sslOn.value
  probeResult.value = null
  targetErr.value = ''
}

async function probe() {
  targetErr.value = targetError(target.value) ? t('quickSite.targetInvalid') : ''
  if (targetErr.value) {
    return
  }
  probing.value = true
  try {
    probeResult.value = await api.post<ProbeResult>('/api/system/probe', { target: target.value })
    if (probeResult.value.insecure) {
      skipVerify.value = true
    }
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    probing.value = false
  }
}

function next() {
  if (step.value === 0) {
    const e = domainError(domain.value)
    domainErr.value = e ? t(`quickSite.domainErr.${e}`) : ''
    if (e) {
      return
    }
    // 域名已在某个站点里：提示但允许继续（换协议时可能需要再建一个）
    step.value = 1
    return
  }
  if (step.value === 1) {
    const e = targetError(target.value)
    targetErr.value = e === 'empty' ? t('quickSite.targetEmpty') : e ? t('quickSite.targetInvalid') : ''
    if (!e) {
      step.value = 2
    }
  }
}

// 证书申请的请求体：只填向导里的字段，其余取后端默认值
function certPayload() {
  const extraVals = Object.fromEntries(Object.entries(extra.value).filter(([, v]) => v.trim()))
  return {
    remark: '',
    enable: true,
    source: 'acme',
    acme: {
      domains: [domain.value],
      // 与默认邮箱相同时留空，以后修改默认邮箱时随之变化
      email: email.value.trim() === defaultEmail.value ? '' : email.value.trim(),
      ca: 'letsencrypt',
      keyType: 'P256',
      challenge: challenge.value,
      httpPort: 0,
      provider: challenge.value === 'dns' ? provider.value : '',
      credentials: challenge.value === 'dns' ? { id: credId.value.trim(), secret: credSecret.value.trim(), extra: extraVals } : { id: '', secret: '' },
      propagationTimeout: 300,
    },
    mapping: { enable: false, dir: '', script: '' },
    deploys: [],
  }
}

function plainListener(l: ListenerView): Record<string, unknown> {
  const out: Record<string, unknown> = JSON.parse(JSON.stringify(l))
  for (const k of VIEW_FIELDS) {
    delete out[k]
  }
  return out
}

async function submit() {
  if (https.value && !cert.value && challenge.value === 'http' && domain.value.startsWith('*.')) {
    return
  }
  emailErr.value = https.value && !cert.value && !/^[^\s]+@[^\s]+$/.test(email.value.trim()) ? t('quickSite.emailRequired') : ''
  if (emailErr.value) {
    return
  }
  saving.value = true
  try {
    certRequested.value = false
    // 先申请证书：凭据填错时在建站之前就报错；签发在后台进行
    if (https.value && !cert.value) {
      await api.post('/api/ssl/certs', certPayload())
      certRequested.value = true
    }
    const site = sitePayload(domain.value, target.value, tpl.value, skipVerify.value)
    let port = newPort.value
    if (listenerKey.value) {
      // 复用已有监听：重新读取最新内容再追加站点，避免覆盖别处的修改
      const fresh = await api.get<ListenerView[]>('/api/webservice/listeners')
      const l = fresh.find((x) => x.key === listenerKey.value)
      if (!l) {
        throw new Error(t('quickSite.listenerGone'))
      }
      const body = plainListener(l)
      const { key, ...rest } = body
      rest.sites = [...((rest.sites as object[] | null) ?? []), site]
      if (!rest.enable) {
        rest.enable = true
      }
      await api.put(`/api/webservice/listeners/${key as string}`, rest)
      port = l.port
    } else {
      await api.post('/api/webservice/listeners', listenerPayload(plan.value, [site]))
    }
    resultURL.value = siteURL(domain.value, https.value, port)
    step.value = 3
    emit('done')
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

function go(path: string) {
  visible.value = false
  router.push(path)
}

function goCert() {
  go(`/ssl?q=${encodeURIComponent(domain.value)}`)
}

function reset() {
  step.value = 0
  domainInput.value = ''
  domainErr.value = ''
  targetInput.value = ''
  targetErr.value = ''
  tpl.value = null
  skipVerify.value = false
  probeResult.value = null
  credId.value = ''
  credSecret.value = ''
  extra.value = {}
  challenge.value = 'dns'
  emailErr.value = ''
  certRequested.value = false
}

async function loadRefs() {
  const jobs: Promise<unknown>[] = [
    api
      .get<ListenerView[]>('/api/webservice/listeners')
      .then((v) => (listeners.value = v ?? []))
      .catch(() => (listeners.value = [])),
  ]
  if (sslOn.value) {
    jobs.push(
      api
        .get<CertLike[]>('/api/ssl/certs')
        .then((v) => (certs.value = v ?? []))
        .catch(() => (certs.value = [])),
    )
    jobs.push(
      api
        .get<{ defaultEmail?: string }>('/api/ssl/settings')
        .then((v) => {
          defaultEmail.value = v.defaultEmail?.trim() ?? ''
          if (!email.value) {
            email.value = defaultEmail.value
          }
        })
        .catch(() => undefined),
    )
    if (!providers.value.length) {
      jobs.push(
        api
          .get<ProviderMeta[]>('/api/ssl/providers')
          .then((v) => {
            providers.value = v ?? []
            if (!providers.value.some((p) => p.name === provider.value) && providers.value.length) {
              provider.value = providers.value[0].name
            }
          })
          .catch(() => undefined),
      )
    }
  }
  await Promise.all(jobs)
  resetListener()
}

async function restart() {
  reset()
  await loadRefs()
}

// open 打开向导；preferHttps 为 false 时默认 HTTP（证书模块关闭时总是 HTTP）
async function open(opts: { https?: boolean } = {}) {
  reset()
  https.value = sslOn.value && opts.https !== false
  visible.value = true
  await loadRefs()
}

defineExpose({ open })
</script>

<style scoped>
.qs-steps {
  margin-bottom: 18px;
}
.qs-body {
  min-height: 220px;
}
.qs-label {
  display: block;
  margin: 14px 0 6px;
  font-size: 13px;
  font-weight: 600;
}
.qs-label:first-child {
  margin-top: 0;
}
.qs-help {
  margin-top: 6px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
}
.qs-error {
  margin-top: 6px;
  font-size: 12px;
  color: var(--el-color-danger);
}
.qs-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.qs-row .el-input {
  flex: 1;
}
.qs-full {
  width: 100%;
}
.qs-port {
  margin-top: 10px;
  font-size: 13px;
}
.qs-note {
  margin: 10px 0 0;
}
.qs-check {
  margin-top: 8px;
}
.qs-cert-hint {
  margin: 0 0 8px;
}
.qs-email {
  margin-top: 12px;
  font-size: 13px;
}
.qs-email > span {
  flex-shrink: 0;
  min-width: 70px;
}
.qs-form {
  margin-top: 12px;
}
.qs-provider {
  width: 240px;
}
.cred-link {
  margin-left: 12px;
}
.qs-probe {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  font-size: 13px;
}
.qs-probe.ok {
  color: var(--el-color-success);
}
.qs-probe.bad {
  color: var(--el-color-danger);
}
.tpl-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
  gap: 8px;
}
.tpl {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 42px;
  padding: 6px 10px;
  font: inherit;
  font-size: 13px;
  color: inherit;
  text-align: left;
  cursor: pointer;
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  background: var(--el-bg-color);
}
.tpl:hover {
  border-color: var(--el-color-primary-light-5);
}
.tpl.on {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.tpl:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 2px;
}
.tpl-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.qs-done {
  text-align: center;
}
.done-icon {
  color: var(--el-color-success);
}
.done-title {
  margin-top: 8px;
  font-size: 17px;
  font-weight: 600;
}
.done-url {
  margin: 10px 0 14px;
  font-size: 15px;
  word-break: break-all;
}
.dns-tip {
  margin-top: 14px;
  padding: 12px 14px;
  text-align: left;
  font-size: 13px;
  line-height: 1.7;
  border-radius: 8px;
  background: var(--el-fill-color-light);
}
.dns-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
}
.dns-tip ul {
  margin: 6px 0 8px;
  padding-left: 20px;
}
@media (max-width: 640px) {
  /* 窄屏：步骤条只显示标题，不换行 */
  .qs-steps {
    padding: 10px 8px;
  }
  .qs-steps :deep(.el-step__head) {
    display: none;
  }
  .qs-steps :deep(.el-step__title) {
    font-size: 13px;
    white-space: nowrap;
    word-break: keep-all;
  }
  .tpl-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .qs-provider {
    width: 100%;
  }
  .cred-link {
    margin-left: 0;
    margin-top: 6px;
  }
  .qs-row {
    flex-wrap: wrap;
  }
}
</style>
