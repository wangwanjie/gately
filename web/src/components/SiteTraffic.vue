<template>
  <div>
    <template v-if="site.type === 'proxy'">
      <el-form-item :label="t('siteTraffic.loadBalance')">
        <el-select v-model="site.lbPolicy" class="w-200" :empty-values="[null, undefined]">
          <el-option :label="t('siteTraffic.lbRoundRobin')" value="" />
          <el-option :label="t('siteTraffic.lbRandom')" value="random" />
          <el-option :label="t('siteTraffic.lbIpHash')" value="ip_hash" />
          <el-option :label="t('siteTraffic.lbLeastConn')" value="least_conn" />
        </el-select>
        <div class="form-help">
          <i18n-t keypath="siteTraffic.lbHelp" tag="span">
            <template #example><span class="mono">http://10.0.0.2:8080 weight=3</span></template>
            <template #backup><span class="mono">backup</span></template>
          </i18n-t>
        </div>
      </el-form-item>

      <el-form-item :label="t('siteTraffic.healthCheck')">
        <el-switch v-model="site.health.enable" />
        <span class="inline-help">{{ t('siteTraffic.healthHelp') }}</span>
      </el-form-item>
      <template v-if="site.health.enable">
        <el-form-item :label="t('siteTraffic.checkMode')">
          <el-radio-group v-model="site.health.mode">
            <el-radio-button value="http">HTTP</el-radio-button>
            <el-radio-button value="tcp">TCP</el-radio-button>
          </el-radio-group>
          <el-input v-if="site.health.mode !== 'tcp'" v-model="site.health.path" placeholder="/healthz" class="mono w-200 gap" />
        </el-form-item>
        <el-form-item :label="t('siteTraffic.intervalTimeout')">
          <el-input-number v-model="site.health.interval" :min="1" :max="3600" />
          <span class="inline-help">{{ t('siteTraffic.seconds') }}</span>
          <el-input-number v-model="site.health.timeout" :min="1" :max="60" class="gap" />
          <span class="inline-help">{{ t('siteTraffic.seconds') }}</span>
        </el-form-item>
        <el-form-item :label="t('siteTraffic.threshold')">
          <el-input-number v-model="site.health.fails" :min="1" :max="20" />
          <span class="inline-help">{{ t('siteTraffic.failsUnavailable') }}</span>
          <el-input-number v-model="site.health.passes" :min="1" :max="20" class="gap" />
          <span class="inline-help">{{ t('siteTraffic.passesRecover') }}</span>
        </el-form-item>
      </template>

      <el-form-item :label="t('siteTraffic.responseCache')">
        <el-switch v-model="site.cache.enable" />
        <span class="inline-help">{{ t('siteTraffic.cacheHelp') }}</span>
      </el-form-item>
      <template v-if="site.cache.enable">
        <el-form-item :label="t('siteTraffic.defaultTtl')">
          <el-input-number v-model="site.cache.ttl" :min="0" :max="2592000" :step="30" />
          <span class="inline-help">{{ t('siteTraffic.ttlHelp') }}</span>
        </el-form-item>
        <el-form-item :label="t('siteTraffic.cacheCapacity')">
          <el-input-number v-model="site.cache.maxMb" :min="1" :max="4096" />
          <span class="inline-help">{{ t('siteTraffic.memoryMb') }}</span>
        </el-form-item>
      </template>
    </template>

    <el-form-item v-if="site.type !== 'redirect'" :label="t('siteTraffic.compress')">
      <el-switch v-model="site.compress" />
      <span class="inline-help">{{ t('siteTraffic.compressHelp') }}</span>
    </el-form-item>

    <el-form-item :label="t('siteTraffic.rateLimit')">
      <el-switch v-model="site.rateLimit.enable" />
      <span class="inline-help">{{ t('siteTraffic.perClientIp') }}</span>
    </el-form-item>
    <template v-if="site.rateLimit.enable">
      <el-form-item :label="t('siteTraffic.requestRate')">
        <el-input-number v-model="site.rateLimit.rate" :min="0" :step="1" :precision="1" />
        <span class="inline-help">{{ t('siteTraffic.perSecond') }}</span>
        <el-input-number v-model="site.rateLimit.burst" :min="0" class="gap" />
        <span class="inline-help">{{ t('siteTraffic.burst') }}</span>
      </el-form-item>
      <el-form-item :label="t('siteTraffic.maxConns')">
        <el-input-number v-model="site.rateLimit.maxConns" :min="0" />
        <span class="inline-help">{{ t('siteTraffic.maxConnsHelp') }}</span>
      </el-form-item>
    </template>

    <el-form-item :label="t('siteTraffic.rewrite')">
      <div class="rows">
        <div v-for="(r, i) in site.rewrites" :key="i" class="row">
          <el-input v-model="r.pattern" :placeholder="t('siteTraffic.patternPlaceholder')" class="mono flex" />
          <el-input v-model="r.to" :placeholder="t('siteTraffic.targetPlaceholder')" class="mono flex" />
          <el-select v-model="r.redirect" class="w-110">
            <el-option :label="t('siteTraffic.internalRewrite')" :value="false" />
            <el-option :label="t('siteTraffic.redirect')" :value="true" />
          </el-select>
          <el-select v-if="r.redirect" v-model="r.code" class="w-80">
            <el-option v-for="c in [301, 302, 307, 308]" :key="c" :label="c" :value="c" />
          </el-select>
          <el-button text type="danger" :icon="Trash2" @click="site.rewrites.splice(i, 1)" />
        </div>
        <el-button text :icon="Plus" @click="site.rewrites.push({ pattern: '', to: '', redirect: false, code: 302 })">{{ t('siteTraffic.addRule') }}</el-button>
        <div class="form-help">{{ t('siteTraffic.rewriteHelp') }}</div>
      </div>
    </el-form-item>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Plus, Trash2 } from 'lucide-vue-next'

export interface Rewrite {
  pattern: string
  to: string
  redirect: boolean
  code: number
}

export interface TrafficFields {
  type: string
  lbPolicy: string
  health: { enable: boolean; mode: string; path: string; interval: number; timeout: number; fails: number; passes: number }
  compress: boolean
  rateLimit: { enable: boolean; rate: number; burst: number; maxConns: number }
  cache: { enable: boolean; ttl: number; maxMb: number }
  rewrites: Rewrite[]
}

const { t } = useI18n()
const site = defineModel<TrafficFields>('site', { required: true })
</script>

<style scoped>
.rows {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.row {
  display: flex;
  gap: 6px;
  align-items: center;
  flex-wrap: wrap;
}
.flex {
  flex: 1;
  min-width: 150px;
}
.w-200 {
  width: 200px;
}
.w-110 {
  width: 110px;
}
.w-80 {
  width: 80px;
}
.gap {
  margin-left: 12px;
}
.inline-help {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
