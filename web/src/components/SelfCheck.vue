<template>
  <el-dialog v-model="visible" :title="t('selfcheck.title')" width="min(720px, 94vw)" append-to-body @opened="run">
    <p class="intro">{{ t('selfcheck.intro') }}</p>
    <div v-loading="loading" class="list">
      <div v-for="(r, i) in sorted" :key="i" class="row" :class="'s-' + r.status">
        <span class="icon">
          <CircleCheck v-if="r.status === 'ok'" :size="17" />
          <Info v-else-if="r.status === 'info'" :size="17" />
          <TriangleAlert v-else-if="r.status === 'warn'" :size="17" />
          <CircleX v-else :size="17" />
        </span>
        <div class="text">
          <div class="msg">{{ message(r) }}</div>
          <div v-if="hint(r)" class="hint">{{ hint(r) }}</div>
        </div>
      </div>
      <div v-if="!loading && !results.length && !error" class="empty">{{ t('selfcheck.empty') }}</div>
      <el-alert v-if="error" type="error" :closable="false" show-icon :title="error" />
    </div>
    <template #footer>
      <span v-if="summary" class="summary">{{ summary }}</span>
      <el-button :icon="RefreshCw" :loading="loading" @click="run">{{ t('selfcheck.rerun') }}</el-button>
      <el-button type="primary" @click="visible = false">{{ t('common.close') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CircleCheck, CircleX, Info, RefreshCw, TriangleAlert } from 'lucide-vue-next'
import { api } from '../api'

interface CheckResult {
  id: string
  status: 'ok' | 'info' | 'warn' | 'fail'
  params?: Record<string, string>
}

const { t } = useI18n()
const visible = ref(false)
const loading = ref(false)
const results = ref<CheckResult[]>([])
const error = ref('')

// 结果编码到文案键：键名使用静态字符串，便于 check-i18n 校验
const MESSAGES: Record<string, string> = {
  'net.publicIPv4.ok': 'selfcheck.r.publicIPv4Ok',
  'net.publicIPv4.fail': 'selfcheck.r.publicIPv4Fail',
  'net.publicIPv6.ok': 'selfcheck.r.publicIPv6Ok',
  'net.publicIPv6.info': 'selfcheck.r.publicIPv6None',
  'net.direct.ok': 'selfcheck.r.direct',
  'net.direct6.ok': 'selfcheck.r.direct6',
  'net.behindNat.info': 'selfcheck.r.behindNat',
  'net.cgnat.warn': 'selfcheck.r.cgnat',
  'time.clock.ok': 'selfcheck.r.clockOk',
  'time.clock.warn': 'selfcheck.r.clockSkew',
  'time.clock.info': 'selfcheck.r.clockUnknown',
  'port.80.ok': 'selfcheck.r.port80Free',
  'port.80.info': 'selfcheck.r.port80InUse',
  'port.80.warn': 'selfcheck.r.port80Denied',
  'port.443.ok': 'selfcheck.r.port443Free',
  'port.443.info': 'selfcheck.r.port443InUse',
  'port.443.warn': 'selfcheck.r.port443Denied',
  'ddns.match.ok': 'selfcheck.r.ddnsMatch',
  'ddns.mismatch.warn': 'selfcheck.r.ddnsMismatch',
  'ddns.unresolved.warn': 'selfcheck.r.ddnsUnresolved',
  'ddns.lookupFailed.warn': 'selfcheck.r.ddnsLookupFailed',
  'cert.ok.ok': 'selfcheck.r.certOk',
  'cert.expiring.warn': 'selfcheck.r.certExpiring',
  'cert.expired.fail': 'selfcheck.r.certExpired',
  'web.certOk.ok': 'selfcheck.r.webCertOk',
  'web.certMissing.warn': 'selfcheck.r.webCertMissing',
  'frp.ok.ok': 'selfcheck.r.frpOk',
  'frp.duplicate.warn': 'selfcheck.r.frpDuplicate',
  'frp.conflict.warn': 'selfcheck.r.frpConflict',
  'guard.firewall.ok': 'selfcheck.r.guardOk',
  'guard.os.info': 'selfcheck.r.guardOS',
  'guard.unsupported.info': 'selfcheck.r.guardUnsupported',
  'guard.missing.warn': 'selfcheck.r.guardMissing',
  'guard.permission.warn': 'selfcheck.r.guardPermission',
  'guard.error.warn': 'selfcheck.r.guardError',
  'guard.trapSkipped.info': 'selfcheck.r.guardTrapSkipped',
  'guard.trapUserspace.info': 'selfcheck.r.guardTrapUserspace',
  'guard.scanUnavailable.warn': 'selfcheck.r.guardScanUnavailable',
  'guard.jailError.warn': 'selfcheck.r.guardJailError',
  'module.summary.warn': 'selfcheck.r.moduleIssue',
  'module.summary.fail': 'selfcheck.r.moduleIssue',
}

const HINTS: Record<string, string> = {
  'net.direct6.ok': 'selfcheck.h.direct6',
  'net.publicIPv4.fail': 'selfcheck.h.publicIPv4Fail',
  'net.behindNat.info': 'selfcheck.h.behindNat',
  'net.cgnat.warn': 'selfcheck.h.cgnat',
  'time.clock.warn': 'selfcheck.h.clockSkew',
  'port.80.info': 'selfcheck.h.portInUse',
  'port.443.info': 'selfcheck.h.portInUse',
  'port.80.warn': 'selfcheck.h.portDenied',
  'port.443.warn': 'selfcheck.h.portDenied',
  'ddns.mismatch.warn': 'selfcheck.h.ddnsMismatch',
  'ddns.unresolved.warn': 'selfcheck.h.ddnsUnresolved',
  'ddns.lookupFailed.warn': 'selfcheck.h.ddnsLookupFailed',
  'cert.expiring.warn': 'selfcheck.h.certExpiring',
  'cert.expired.fail': 'selfcheck.h.certExpired',
  'web.certMissing.warn': 'selfcheck.h.webCertMissing',
  'frp.duplicate.warn': 'selfcheck.h.frpDuplicate',
  'frp.conflict.warn': 'selfcheck.h.frpConflict',
  'guard.unsupported.info': 'selfcheck.h.guardUnsupported',
  'guard.missing.warn': 'selfcheck.h.guardMissing',
  'guard.permission.warn': 'selfcheck.h.guardPermission',
  'guard.error.warn': 'selfcheck.h.guardError',
  'guard.trapSkipped.info': 'selfcheck.h.guardTrapSkipped',
  'guard.scanUnavailable.warn': 'selfcheck.h.guardScanUnavailable',
  'guard.jailError.warn': 'selfcheck.h.guardJailError',
}

function message(r: CheckResult): string {
  const key = MESSAGES[`${r.id}.${r.status}`]
  return key ? t(key, r.params ?? {}) : `${r.id}: ${JSON.stringify(r.params ?? {})}`
}

function hint(r: CheckResult): string {
  const key = HINTS[`${r.id}.${r.status}`]
  return key ? t(key, r.params ?? {}) : ''
}

// 问题排在前面
const order: Record<string, number> = { fail: 0, warn: 1, info: 2, ok: 3 }
const sorted = computed(() => [...results.value].sort((a, b) => order[a.status] - order[b.status]))
const summary = computed(() => {
  if (!results.value.length) {
    return ''
  }
  const bad = results.value.filter((r) => r.status === 'fail' || r.status === 'warn').length
  return bad ? t('selfcheck.issues', { count: bad }) : t('selfcheck.allGood')
})

async function run() {
  loading.value = true
  error.value = ''
  try {
    results.value = (await api.post<{ results: CheckResult[] }>('/api/system/selfcheck', {})).results
  } catch (e) {
    error.value = (e as Error).message
    results.value = []
  } finally {
    loading.value = false
  }
}

function open() {
  results.value = []
  visible.value = true
}

defineExpose({ open })
</script>

<style scoped>
.intro {
  margin: 0 0 12px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.list {
  min-height: 120px;
  max-height: 55vh;
  overflow: auto;
}
.row {
  display: flex;
  gap: 10px;
  padding: 9px 4px;
  border-bottom: 1px solid var(--el-border-color-extra-light);
}
.icon {
  flex-shrink: 0;
  margin-top: 1px;
}
.s-ok .icon {
  color: var(--el-color-success);
}
.s-info .icon {
  color: var(--el-color-info);
}
.s-warn .icon {
  color: var(--el-color-warning);
}
.s-fail .icon {
  color: var(--el-color-danger);
}
.msg {
  font-size: 14px;
  line-height: 1.5;
  word-break: break-all;
}
.hint {
  margin-top: 3px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
}
.empty {
  padding: 24px;
  text-align: center;
  color: var(--el-text-color-placeholder);
}
.summary {
  margin-right: 12px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
</style>
