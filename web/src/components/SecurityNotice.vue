<template>
  <!-- 当前版本受已公开安全问题影响时的说明：安全公告与带安全修复的新版本，关于页与新版本弹窗共用 -->
  <div v-if="info.security" class="security-notice" role="alert">
    <div class="sn-title">
      <ShieldAlert :size="16" />
      <span>{{ t('about.security.title', { current: info.current }) }}</span>
    </div>
    <ul class="sn-list">
      <li v-for="a in info.advisories" :key="a.id">
        <a :href="a.url" target="_blank" rel="noopener noreferrer" class="mono">{{ a.id }}</a>
        <el-tag v-if="a.severity" size="small" :type="severityType(a.severity)" effect="light" class="sn-sev">{{ t(`about.security.severity.${a.severity}`) }}</el-tag>
        <span class="sn-summary">{{ a.summary }}</span>
        <span v-if="fixedIn(a)" class="sn-meta">{{ t('about.security.fixedIn', { version: fixedIn(a) }) }}</span>
      </li>
      <li v-if="info.securityReleases.length">
        <i18n-t keypath="about.security.releases" tag="span">
          <template #versions>
            <template v-for="(r, i) in info.securityReleases" :key="r.version">
              <span v-if="i > 0">, </span>
              <a :href="r.url" target="_blank" rel="noopener noreferrer" class="mono">{{ r.version }}</a>
            </template>
          </template>
        </i18n-t>
      </li>
    </ul>
    <div class="sn-advice">
      {{ info.hasUpdate ? t('about.security.advice', { latest: info.latest }) : t('about.security.noFix') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ShieldAlert } from 'lucide-vue-next'
import type { SecurityAdvisory, UpdateInfo } from '../api'

defineProps<{ info: UpdateInfo }>()
const { t } = useI18n()

function severityType(sev: string) {
  return sev === 'critical' || sev === 'high' ? 'danger' : sev === 'medium' ? 'warning' : 'info'
}

// fixedIn 返回公告中第一个给出的修复版本
function fixedIn(a: SecurityAdvisory) {
  return a.ranges.find((r) => r.patched)?.patched ?? ''
}
</script>

<style scoped>
.security-notice {
  padding: 12px 14px;
  border-radius: var(--g-radius);
  border: 1px solid var(--el-color-danger-light-5);
  background: var(--el-color-danger-light-9);
  font-size: 13px;
  line-height: 1.6;
  text-align: left;
}
.sn-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  color: var(--el-color-danger);
}
.sn-title svg {
  flex-shrink: 0;
}
.sn-list {
  margin: 6px 0;
  padding-left: 20px;
}
.sn-list li {
  margin: 4px 0;
  overflow-wrap: anywhere;
}
.sn-sev {
  margin: 0 6px;
}
.sn-summary {
  color: var(--el-text-color-primary);
}
.sn-meta {
  margin-left: 6px;
  color: var(--el-text-color-secondary);
}
.sn-advice {
  color: var(--el-text-color-regular);
}
</style>
