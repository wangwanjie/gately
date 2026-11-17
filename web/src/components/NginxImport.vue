<template>
  <el-dialog v-model="visible" :title="t('nginxImport.title')" width="860px" top="5vh" :fullscreen="isMobile" @closed="reset">
    <template v-if="!report">
      <div class="form-help intro">{{ t('nginxImport.intro') }}</div>
      <pre class="cmd mono">nginx -T</pre>
      <!-- Nginx Proxy Manager 运行在容器里，在容器内执行 nginx -T 取完整配置 -->
      <div class="form-help intro">{{ t('nginxImport.npmIntro') }}</div>
      <pre class="cmd mono">{{ t('nginxImport.npmCommand') }}</pre>
      <el-input v-model="config" type="textarea" :rows="14" class="mono" :placeholder="PLACEHOLDER" />
      <div class="row">
        <label class="file-pick"><Upload :size="14" />{{ t('nginxImport.readFile') }}<input type="file" accept=".conf,.txt,text/plain" @change="readFile" /></label>
      </div>
    </template>

    <template v-else>
      <div class="summary">
        <el-tag type="success">{{ t('nginxImport.sitesReady', { count: report.sites }) }}</el-tag>
        <el-tag v-if="report.skipped" type="info">{{ t('nginxImport.sitesSkipped', { count: report.skipped }) }}</el-tag>
        <el-button text size="small" @click="report = undefined">{{ t('nginxImport.back') }}</el-button>
      </div>
      <el-alert v-for="(w, i) in report.warnings ?? []" :key="i" :title="w" type="warning" :closable="false" show-icon class="global-warn" />
      <div v-for="l in report.listeners ?? []" :key="`${l.listenIp}|${l.port}|${l.tls}`" class="listener">
        <div class="listener-head">
          <el-tag size="small" effect="plain" class="mono">{{ l.tls ? 'https' : 'http' }}://{{ l.listenIp || '*' }}:{{ l.port }}</el-tag>
          <span v-if="l.existing" class="meta">{{ t('nginxImport.mergeInto', { name: l.existing }) }}</span>
          <span v-else class="meta">{{ t('nginxImport.newListener') }}</span>
          <el-tag v-if="l.proxyProtocol && !l.existing" size="small" type="info">{{ t('nginxImport.proxyProtocol') }}</el-tag>
          <el-tag v-if="l.http3 && l.tls && !l.existing" size="small" type="info">HTTP/3</el-tag>
        </div>
        <div v-for="(s, i) in l.sites ?? []" :key="i" class="site" :class="{ skipped: s.skipped }">
          <div class="site-head">
            <span class="name">{{ s.default ? t('nginxImport.defaultSite') : s.name }}</span>
            <el-tag v-if="s.type" size="small">{{ typeText(s.type) }}</el-tag>
            <span class="meta mono target">{{ s.summary }}</span>
            <span class="meta src">{{ s.source }}</span>
          </div>
          <div v-if="(s.domains ?? []).length > 1 || (s.default && (s.domains ?? []).length)" class="meta mono">{{ (s.domains ?? []).join(', ') }}</div>
          <div v-if="s.skipped" class="skip">{{ t('nginxImport.skipped', { reason: s.skipped }) }}</div>
          <div v-for="(w, j) in s.warnings ?? []" :key="j" class="warn">{{ w }}</div>
          <div v-if="s.certMissing?.length" class="warn">
            {{ t('nginxImport.certMissing', { domains: s.certMissing.join(', ') }) }}
            <span v-if="s.certFiles?.length" class="mono">{{ t('nginxImport.certFiles', { files: s.certFiles.join(', ') }) }}</span>
          </div>
          <div v-if="s.ignored?.length" class="meta">{{ t('nginxImport.ignored', { names: s.ignored.join(', ') }) }}</div>
        </div>
      </div>
      <div class="enable-row">
        <el-checkbox v-model="enable">{{ t('nginxImport.enableNow') }}</el-checkbox>
        <div class="form-help">{{ enable ? t('nginxImport.enableNowHelp') : t('nginxImport.disabledHelp') }}</div>
      </div>
    </template>

    <template #footer>
      <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
      <el-button v-if="!report" type="primary" :loading="busy" :disabled="!config.trim()" @click="preview">{{ t('nginxImport.preview') }}</el-button>
      <el-button v-else type="primary" :loading="busy" :disabled="!report.sites" @click="apply">{{ t('nginxImport.import', { count: report.sites }) }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Upload } from 'lucide-vue-next'
import { api } from '../api'
import { useFormLayout } from '../composables/useIsMobile'

// 对应 POST /api/webservice/import/nginx 的返回结构
interface SitePlan {
  name: string
  // 默认站点没有域名时为 null
  domains: string[] | null
  type?: string
  summary?: string
  source: string
  default?: boolean
  skipped?: string
  warnings?: string[]
  ignored?: string[]
  certMissing?: string[]
  certFiles?: string[]
}
interface ListenerPlan {
  port: number
  tls: boolean
  listenIp: string
  http3: boolean
  proxyProtocol: boolean
  existing?: string
  sites: SitePlan[]
}
interface Report {
  listeners: ListenerPlan[]
  warnings: string[]
  sites: number
  skipped: number
  applied: boolean
}

// 示例配置是 nginx 语法，不随界面语言翻译
const PLACEHOLDER = 'server {\n    listen 443 ssl;\n    server_name app.example.com;\n    location / { proxy_pass http://127.0.0.1:8080; }\n}'

const { t } = useI18n()
const { isMobile } = useFormLayout()
const emit = defineEmits<{ imported: [] }>()
const visible = ref(false)
const config = ref('')
const report = ref<Report>()
const enable = ref(false)
const busy = ref(false)

function open() {
  visible.value = true
}

function reset() {
  config.value = ''
  report.value = undefined
  enable.value = false
}

function typeText(type: string) {
  return ({ proxy: t('nginxImport.typeProxy'), static: t('nginxImport.typeStatic'), redirect: t('nginxImport.typeRedirect'), text: t('nginxImport.typeText'), close: t('nginxImport.typeClose') } as Record<string, string>)[type] ?? type
}

function readFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (f) {
    f.text().then((text) => (config.value = text))
  }
}

async function preview() {
  busy.value = true
  try {
    report.value = await api.post<Report>('/api/webservice/import/nginx', { config: config.value, apply: false })
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

async function apply() {
  busy.value = true
  try {
    const r = await api.post<Report>('/api/webservice/import/nginx', { config: config.value, apply: true, enable: enable.value })
    ElMessage.success(t('nginxImport.done', { count: r.sites }))
    visible.value = false
    emit('imported')
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    busy.value = false
  }
}

defineExpose({ open })
</script>

<style scoped>
.intro {
  margin: 0 0 8px;
}
.cmd {
  margin: 0 0 10px;
  padding: 6px 10px;
  border-radius: 6px;
  background: var(--el-fill-color-light);
  font-size: 12px;
}
.row {
  margin-top: 8px;
}
.file-pick {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--el-color-primary);
  cursor: pointer;
}
.file-pick input {
  display: none;
}
.summary {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.global-warn {
  margin-bottom: 8px;
}
.listener {
  border: 1px solid var(--g-border);
  border-radius: 8px;
  padding: 10px;
  margin-bottom: 12px;
}
.listener-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 6px;
}
.site {
  padding: 6px 0;
  border-top: 1px dashed var(--g-border);
}
.site.skipped {
  opacity: 0.6;
}
.site-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.name {
  font-weight: 600;
}
.target {
  word-break: break-all;
}
.src {
  margin-left: auto;
}
.meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.warn {
  font-size: 12px;
  color: var(--el-color-warning);
  word-break: break-all;
}
.skip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.enable-row {
  margin-top: 8px;
}
</style>
