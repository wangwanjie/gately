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

      <!-- 连接后端的超时，0 为默认值；分路径与按用户分流的后端同样适用 -->
      <el-form-item :label="t('siteTraffic.backendTimeouts')">
        <div class="timeouts">
          <span class="timeout-item">
            <span class="timeout-label">{{ t('siteTraffic.timeoutConnect') }}</span>
            <el-input-number v-model="site.timeouts.connect" :min="0" :max="300" controls-position="right" class="w-110" />
            <span class="timeout-unit">{{ t('siteTraffic.seconds') }}</span>
          </span>
          <span class="timeout-item">
            <span class="timeout-label">{{ t('siteTraffic.timeoutTls') }}</span>
            <el-input-number v-model="site.timeouts.tlsHandshake" :min="0" :max="300" controls-position="right" class="w-110" />
            <span class="timeout-unit">{{ t('siteTraffic.seconds') }}</span>
          </span>
          <span class="timeout-item">
            <span class="timeout-label">{{ t('siteTraffic.timeoutResponseHeader') }}</span>
            <el-input-number v-model="site.timeouts.responseHeader" :min="0" :max="86400" controls-position="right" class="w-110" />
            <span class="timeout-unit">{{ t('siteTraffic.seconds') }}</span>
          </span>
        </div>
        <div class="form-help">{{ t('siteTraffic.timeoutsHelp') }}</div>
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
        <!-- 按路径的缓存规则：更具体的路径优先，未命中时使用上面的默认设置 -->
        <el-form-item :label="t('siteTraffic.cacheRules')">
          <div class="rows">
            <div v-for="(r, i) in site.cache.rules" :key="i" class="cache-rule" data-test="cache-rule">
              <el-input v-model="r.path" :placeholder="t('siteTraffic.cachePathPlaceholder')" class="mono cache-path" :class="{ 'is-error': !!cachePathError(r.path) }" />
              <!-- 缓存方式合并为一个下拉：遵循后端 / 强制缓存（忽略后端 Cache-Control）/ 不缓存；
                   不缓存时桌面端保留时长占位，使各行的列对齐，移动端直接隐藏 -->
              <span class="cache-opts">
                <el-select :model-value="ruleMode(r)" class="w-160" data-test="cache-rule-mode" @update:model-value="(m: CacheRuleMode) => setRuleMode(r, m)">
                  <el-option :label="t('siteTraffic.cacheModeCache')" value="cache" />
                  <el-option :label="t('siteTraffic.cacheModeForce')" value="force" />
                  <el-option :label="t('siteTraffic.cacheModeNoCache')" value="none" />
                </el-select>
                <span class="cache-ttl" :class="{ 'is-placeholder': r.noCache }" :aria-hidden="r.noCache">
                  <el-input-number v-model="r.ttl" :min="0" :max="2592000" :step="60" :disabled="r.noCache" controls-position="right" class="w-120" />
                  <span class="timeout-unit">{{ t('siteTraffic.seconds') }}</span>
                </span>
              </span>
              <span class="cache-actions">
                <el-tooltip v-if="canPurge" :content="t('siteTraffic.cachePurgePath')" placement="top">
                  <el-button text :icon="Eraser" :loading="purging === r.path" :disabled="!r.path.trim()" :aria-label="t('siteTraffic.cachePurgePath')" data-test="cache-purge-rule" @click="purge(r.path.trim())" />
                </el-tooltip>
                <el-button text type="danger" :icon="Trash2" :aria-label="t('siteTraffic.deleteRule')" @click="site.cache.rules.splice(i, 1)" />
              </span>
              <!-- 路径写法不合法时就地提示（与后端校验一致）；强制缓存的响应由所有访客共用，提醒只用于与身份无关的内容 -->
              <div v-if="cachePathError(r.path)" class="cache-note is-error" data-test="cache-path-error">{{ t(`siteTraffic.cachePathError.${cachePathError(r.path)}`) }}</div>
              <div v-if="r.override && !r.noCache" class="cache-note is-warn" data-test="cache-force-warn">{{ t('siteTraffic.cacheForceWarn') }}</div>
            </div>
            <el-button text :icon="Plus" data-test="cache-rule-add" @click="site.cache.rules.push({ path: '', noCache: false, ttl: 0, override: false })">{{ t('siteTraffic.addRule') }}</el-button>
            <div class="form-help">{{ t('siteTraffic.cacheRulesHelp') }}</div>
          </div>
        </el-form-item>
        <!-- 已保存且运行中的站点显示当前缓存占用，可清空全部 -->
        <el-form-item v-if="cacheStats?.enabled" :label="t('siteTraffic.cacheUsage')">
          <span class="cache-usage" data-test="cache-usage">{{ t('siteTraffic.cacheUsageValue', { entries: cacheStats.entries, size: formatBytes(cacheStats.bytes), max: formatBytes(cacheStats.maxBytes) }) }}</span>
          <el-button v-if="canPurge" size="small" :icon="Eraser" :loading="purging === ''" class="gap" data-test="cache-purge-all" @click="purge('')">{{ t('siteTraffic.cachePurgeAll') }}</el-button>
          <el-button size="small" text :icon="RefreshCw" :aria-label="t('siteTraffic.cacheRefresh')" @click="loadCacheStats" />
        </el-form-item>
      </template>
    </template>

    <!-- 对访客的读写空闲超时，0 为默认 60 秒；防止极慢的访客长期占住连接 -->
    <el-form-item :label="t('siteTraffic.clientTimeouts')">
      <div class="timeouts">
        <span class="timeout-item">
          <span class="timeout-label">{{ t('siteTraffic.timeoutReadBody') }}</span>
          <el-input-number v-model="site.clientTimeouts.readIdle" :min="0" :max="3600" controls-position="right" class="w-110" />
          <span class="timeout-unit">{{ t('siteTraffic.seconds') }}</span>
        </span>
        <span class="timeout-item">
          <span class="timeout-label">{{ t('siteTraffic.timeoutWrite') }}</span>
          <el-input-number v-model="site.clientTimeouts.writeIdle" :min="0" :max="3600" controls-position="right" class="w-110" />
          <span class="timeout-unit">{{ t('siteTraffic.seconds') }}</span>
        </span>
      </div>
      <div class="form-help">{{ t('siteTraffic.clientTimeoutsHelp') }}</div>
    </el-form-item>

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
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Eraser, Plus, RefreshCw, Trash2 } from 'lucide-vue-next'
import { api, formatBytes } from '../api'
import { isReadonly } from '../nav'
import { cachePathError } from '../cachePath'

export interface Rewrite {
  pattern: string
  to: string
  redirect: boolean
  code: number
}

export interface BackendTimeouts {
  connect: number
  tlsHandshake: number
  responseHeader: number
}

export interface ClientTimeouts {
  readIdle: number
  writeIdle: number
}

export interface TrafficFields {
  type: string
  lbPolicy: string
  health: { enable: boolean; mode: string; path: string; interval: number; timeout: number; fails: number; passes: number }
  // 连接后端的超时（秒），0 为默认值
  timeouts: BackendTimeouts
  // 对访客的读写空闲超时（秒），0 为默认值
  clientTimeouts: ClientTimeouts
  compress: boolean
  rateLimit: { enable: boolean; rate: number; burst: number; maxConns: number }
  cache: CacheSettings
  rewrites: Rewrite[]
}

// CacheRule 是某个路径的缓存规则：noCache 为不缓存；ttl 为 0 时沿用站点默认；override 忽略后端的 Cache-Control
export interface CacheRule {
  path: string
  noCache: boolean
  ttl: number
  override: boolean
}

export interface CacheSettings {
  enable: boolean
  ttl: number
  maxMb: number
  rules: CacheRule[]
}

// CacheRuleMode 是界面上的缓存方式：cache 遵循后端缓存头，force 忽略后端缓存头按时长缓存，none 不缓存
type CacheRuleMode = 'cache' | 'force' | 'none'

function ruleMode(r: CacheRule): CacheRuleMode {
  if (r.noCache) {
    return 'none'
  }
  return r.override ? 'force' : 'cache'
}

function setRuleMode(r: CacheRule, m: CacheRuleMode) {
  r.noCache = m === 'none'
  r.override = m === 'force'
}

// CacheTarget 指向已保存的站点（所属监听与站点标识），新建或复制站点时为 null
export interface CacheTarget {
  listener: string
  site: string
}

interface CacheStats {
  enabled: boolean
  entries: number
  bytes: number
  maxBytes: number
}

const { t } = useI18n()
const site = defineModel<TrafficFields>('site', { required: true })
const props = defineProps<{ cacheTarget?: CacheTarget | null }>()

// cacheStats 为站点运行中的缓存状态；站点未保存、未运行或运行中的配置未启用缓存时不显示占用与清空按钮
const cacheStats = ref<CacheStats | null>(null)
// purging 为正在清空的路径，'' 表示清空全部，null 表示空闲
const purging = ref<string | null>(null)
// 清空缓存属于写操作，只读账号不显示按钮
const canPurge = computed(() => !!cacheStats.value?.enabled && !isReadonly.value)

async function loadCacheStats() {
  const target = props.cacheTarget
  if (!target) {
    cacheStats.value = null
    return
  }
  try {
    const q = new URLSearchParams({ listener: target.listener, site: target.site })
    cacheStats.value = await api.get<CacheStats>(`/api/webservice/cache?${q}`)
  } catch {
    // 监听未运行等情况下拿不到状态，只隐藏占用信息，不打扰编辑
    cacheStats.value = null
  }
}

// purge 清空站点缓存：path 为空时清空全部，否则清空路径命中的条目（按已保存的规则写法匹配）
async function purge(path: string) {
  const target = props.cacheTarget
  if (!target || purging.value !== null) {
    return
  }
  purging.value = path
  try {
    const r = await api.post<CacheStats & { removed: number }>('/api/webservice/cache/purge', { listener: target.listener, site: target.site, path })
    cacheStats.value = { enabled: r.enabled, entries: r.entries, bytes: r.bytes, maxBytes: r.maxBytes }
    ElMessage.success(t('siteTraffic.cachePurged', { n: r.removed }))
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    purging.value = null
  }
}

watch(() => [props.cacheTarget?.listener, props.cacheTarget?.site], loadCacheStats, { immediate: true })
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
.w-120 {
  width: 120px;
}
.w-160 {
  width: 160px;
}
/* 缓存规则一行：路径占满剩余宽度，窄屏时路径独占一行，其余控件换到下一行 */
.cache-rule {
  display: flex;
  gap: 6px 8px;
  align-items: center;
  flex-wrap: wrap;
}
.cache-path {
  flex: 1;
  min-width: 140px;
}
.cache-opts {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.cache-ttl {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}
.cache-ttl.is-placeholder {
  visibility: hidden;
}
.cache-actions {
  display: inline-flex;
  align-items: center;
  margin-left: auto;
}
/* 规则下方的提示独占一行 */
.cache-note {
  flex-basis: 100%;
  font-size: 12px;
  line-height: 1.5;
}
.cache-note.is-error {
  color: var(--el-color-danger);
}
.cache-note.is-warn {
  color: var(--el-color-warning);
}
.cache-path.is-error :deep(.el-input__wrapper) {
  box-shadow: 0 0 0 1px var(--el-color-danger) inset;
}
.cache-usage {
  font-size: 13px;
  color: var(--el-text-color-regular);
}
/* 窄屏：第一行为路径与操作按钮，第二行为缓存方式与时长 */
@media (max-width: 767.98px) {
  .cache-rule {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    padding: 8px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 6px;
  }
  .cache-path {
    min-width: 0;
  }
  .cache-opts {
    grid-column: 1 / -1;
    grid-row: 2;
  }
  .cache-actions {
    grid-column: 2;
    grid-row: 1;
  }
  .cache-ttl.is-placeholder {
    display: none;
  }
  .cache-note {
    grid-column: 1 / -1;
  }
}
.w-80 {
  width: 80px;
}
.gap {
  margin-left: 12px;
}
/* 三项超时在窄屏上逐项换行，每项的标签、输入框与单位保持在同一行 */
.timeouts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
}
.timeout-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}
.timeout-label,
.timeout-unit {
  font-size: 13px;
  color: var(--el-text-color-regular);
}
.inline-help {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
