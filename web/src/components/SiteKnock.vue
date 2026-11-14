<template>
  <!-- 敲门准入与放行入口：准入要求来源地址持有敲门规则的有效授权；放行入口在访客通过本站点认证后为其地址登记授权 -->
  <div class="site-knock" data-test="site-knock">
    <el-form-item :label="t('siteKnock.admission')" data-test="knock-admission">
      <KnockSelect v-model="site.knock" />
      <div class="form-help">{{ t('siteKnock.admissionHelp') }}</div>
      <el-alert v-if="site.knock && !site.denyClose" type="info" :closable="false" show-icon class="tip" data-test="knock-deny-tip" :title="t('siteKnock.denyCloseTip')" />
    </el-form-item>
    <el-form-item v-if="site.knock" :label="t('siteKnock.lan')" data-test="knock-lan">
      <el-checkbox v-model="site.knockLan">{{ t('siteKnock.lanLabel') }}</el-checkbox>
      <div class="form-help">{{ t('siteKnock.lanHelp') }}</div>
    </el-form-item>

    <template v-if="!isDefault && (site.webAuth || site.basicAuth)">
      <el-form-item :label="t('siteKnock.grant')" data-test="knock-grant">
        <KnockSelect v-model="site.knockGrant.rule" />
        <div class="form-help">{{ t('siteKnock.grantHelp') }}</div>
        <el-alert v-if="sameRule" type="error" :closable="false" show-icon class="tip" data-test="knock-same-rule" :title="t('siteKnock.sameRule')" />
      </el-form-item>
      <template v-if="site.knockGrant.rule">
        <el-form-item :label="t('siteKnock.minutes')" data-test="knock-minutes">
          <div class="inline">
            <el-input-number v-model="site.knockGrant.minutes" :min="5" :max="10080" :step="30" controls-position="right" class="w-num" />
            <span class="muted">{{ t('siteKnock.minutesUnit') }}</span>
            <span class="muted">≈ {{ durationText }}</span>
          </div>
        </el-form-item>
        <el-form-item :label="t('siteKnock.v6Prefix')" data-test="knock-v6prefix">
          <div class="inline">
            <span class="mono muted">/</span>
            <el-input-number v-model="site.knockGrant.v6Prefix" :min="48" :max="128" controls-position="right" class="w-num" />
          </div>
          <div class="form-help">{{ t('siteKnock.v6PrefixHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('siteKnock.probe')" data-test="knock-probe">
          <div class="probes">
            <el-select v-model="site.knockGrant.probeV4" filterable allow-create clearable default-first-option :placeholder="t('siteKnock.probeV4Ph')" class="mono" data-test="knock-probe-v4">
              <el-option v-for="d in hosts" :key="d" :label="d" :value="d" />
            </el-select>
            <el-select v-model="site.knockGrant.probeV6" filterable allow-create clearable default-first-option :placeholder="t('siteKnock.probeV6Ph')" class="mono" data-test="knock-probe-v6">
              <el-option v-for="d in hosts" :key="d" :label="d" :value="d" />
            </el-select>
          </div>
          <div class="form-help">{{ t('siteKnock.probeHelp') }}</div>
        </el-form-item>
        <el-form-item :label="t('siteKnock.entry')" data-test="knock-entry">
          <div v-if="entry" class="entry">
            <a :href="entry.href" target="_blank" rel="noopener noreferrer" class="mono entry-link">{{ entry.text }}</a>
            <el-button text size="small" :icon="Copy" :aria-label="t('siteKnock.copy')" @click="copyText(entry.text)" />
          </div>
          <div v-else class="muted">{{ t('siteKnock.entryNoHost') }}</div>
          <div class="form-help">{{ t('siteKnock.entryHelp') }}</div>
        </el-form-item>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Copy } from 'lucide-vue-next'
import KnockSelect from './KnockSelect.vue'
import { copyText } from '../composables/useClipboard'
import { firstHost, hostPart, siteLink, type AccessListener } from '../siteUrl'
import { knockPagePath, type KnockFields } from '../siteKnock'

const props = defineProps<{ listener: AccessListener; isDefault?: boolean }>()
const site = defineModel<KnockFields>('site', { required: true })
const { t } = useI18n()

// hosts 为可作为探测域名的站点域名（具体主机，不含通配）
const hosts = computed(() => (site.value.domains ?? []).filter((d) => hostPart(d) && !d.includes(':')))

const sameRule = computed(() => !!site.value.knock && site.value.knock === site.value.knockGrant.rule)

// entry 为放行入口页面的地址：站点的第一个具体域名 + 监听的访问协议与端口
const entry = computed(() => {
  const host = firstHost(site.value.domains)
  if (!host) {
    return null
  }
  const base = siteLink(host, props.listener)
  return base ? { href: base.href + knockPagePath, text: base.text + knockPagePath } : null
})

// durationText 把分钟换算成小时或天，便于确认
const durationText = computed(() => {
  const m = site.value.knockGrant.minutes || 0
  if (m >= 1440 && m % 1440 === 0) {
    return t('siteKnock.days', { n: m / 1440 })
  }
  if (m >= 60) {
    return t('siteKnock.hours', { n: Math.round((m / 60) * 10) / 10 })
  }
  return t('siteKnock.mins', { n: m })
})
</script>

<style scoped>
.site-knock {
  width: 100%;
}
.tip {
  margin-top: 8px;
}
.inline {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.w-num {
  width: 140px;
}
.muted {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.probes {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  width: 100%;
}
.entry {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  max-width: 100%;
}
.entry-link {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
@media (max-width: 640px) {
  .probes {
    grid-template-columns: 1fr;
  }
}
</style>
