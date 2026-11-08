<template>
  <div>
    <el-form-item v-if="site.webAuth || site.basicAuth" :label="t('siteAccess.authScope')">
      <el-checkbox v-model="site.authOptional">{{ t('siteAccess.authOptional') }}</el-checkbox>
    </el-form-item>

    <el-form-item :label="t('siteAccess.pathRules')">
      <div class="rows">
        <div v-for="(r, i) in site.rules" :key="i" class="row">
          <el-input v-model="r.prefix" placeholder="/admin" class="w-prefix mono" />
          <el-select v-model="r.auth" class="w-auth" :empty-values="[null, undefined]">
            <el-option :label="t('siteAccess.authInherit')" value="" />
            <el-option :label="t('siteAccess.authOff')" value="off" />
            <el-option :label="t('siteAccess.authOn')" value="on" />
          </el-select>
          <el-select v-model="r.ipMode" class="w-ip" :empty-values="[null, undefined]">
            <el-option :label="t('siteAccess.ipInherit')" value="" />
            <el-option :label="t('siteAccess.ipWhitelist')" value="whitelist" />
            <el-option :label="t('siteAccess.ipBlacklist')" value="blacklist" />
          </el-select>
          <ListInput v-if="r.ipMode" v-model="r.ipList" :split="splitCommasOrSpaces" separator=", " placeholder="192.168.1.0/24, 10.0.0.1" class="mono flex" />
          <el-select v-if="r.ipMode && ipLists.length" v-model="r.ipSets" multiple collapse-tags :placeholder="t('siteAccess.ipSetsPlaceholder')" class="w-sets">
            <el-option v-for="o in ipLists" :key="o.key" :label="o.name" :value="o.key" />
          </el-select>
          <el-button text type="danger" :icon="Trash2" @click="site.rules.splice(i, 1)" />
        </div>
        <el-button text :icon="Plus" @click="site.rules.push({ prefix: '/', auth: '', ipMode: '', ipList: [], ipSets: [] })">{{ t('siteAccess.addRule') }}</el-button>
        <div class="form-help">{{ t('siteAccess.rulesHelp') }}</div>
      </div>
    </el-form-item>

    <el-form-item :label="t('siteAccess.countryFilter')">
      <el-radio-group v-model="site.countryMode">
        <el-radio-button value="">{{ t('siteAccess.countryOff') }}</el-radio-button>
        <el-radio-button value="allow">{{ t('siteAccess.countryAllow') }}</el-radio-button>
        <el-radio-button value="deny">{{ t('siteAccess.countryDeny') }}</el-radio-button>
      </el-radio-group>
    </el-form-item>
    <el-form-item v-if="site.countryMode" :label="t('siteAccess.countryCodes')">
      <ListInput v-model="site.countries" :split="splitCommasOrSpaces" separator=", " class="mono" :placeholder="t('siteAccess.countryCodesPlaceholder')" />
      <div class="form-help">
        {{ t('siteAccess.lanAlwaysAllowed') }}
        <span v-if="geoLoaded">{{ t('siteAccess.geoLoaded') }}</span>
        <span v-else class="warn">{{ t('siteAccess.geoMissing') }}</span>
      </div>
    </el-form-item>

    <el-form-item v-if="site.webAuth || site.basicAuth" :label="t('siteAccess.shareLinks')">
      <div class="rows">
        <div v-for="(l, i) in site.shareLinks" :key="l.key ?? i" class="row">
          <el-input v-model="l.name" :placeholder="t('siteAccess.shareNamePlaceholder')" class="w-name" />
          <el-date-picker v-model="l.expiresAt" type="datetime" :placeholder="t('siteAccess.noExpiry')" value-format="YYYY-MM-DDTHH:mm:ssZ" class="w-date" />
          <el-input-number v-model="l.maxUses" :min="0" controls-position="right" class="w-uses" />
          <el-input v-model="l.pathPrefix" :placeholder="t('siteAccess.pathLimitPlaceholder')" class="mono w-path" />
          <el-button text :icon="Copy" :disabled="!l.token" :title="l.token ? t('siteAccess.copyLink') : t('siteAccess.copyAfterSave')" @click="copyLink(l)" />
          <el-button text type="danger" :icon="Trash2" @click="site.shareLinks.splice(i, 1)" />
        </div>
        <el-button text :icon="Plus" @click="addLink">{{ t('siteAccess.addShareLink') }}</el-button>
        <div class="form-help">{{ t('siteAccess.shareHelp') }}</div>
      </div>
    </el-form-item>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Copy, Plus, Trash2 } from 'lucide-vue-next'
import { api } from '../api'
import { copyText } from '../composables/useClipboard'
import { splitCommasOrSpaces } from '../composables/useListText'
import { firstHost } from '../siteUrl'
import ListInput from './ListInput.vue'

export interface PathRule {
  prefix: string
  auth: string
  ipMode: string
  ipList: string[]
  // ipSets 为引用的订阅 IP 列表 key
  ipSets?: string[]
}

export interface ShareLink {
  key?: string
  name: string
  token: string
  expiresAt?: string
  maxUses: number
  pathPrefix: string
}

export interface AccessFields {
  webAuth: boolean
  basicAuth: boolean
  domains: string[] | null
  authOptional: boolean
  countryMode: string
  countries: string[] | null
  rules: PathRule[]
  shareLinks: ShareLink[]
}

const { t } = useI18n()
const props = withDefaults(defineProps<{ tls: boolean; port: number; ipLists?: { key: string; name: string }[] }>(), { ipLists: () => [] })
const site = defineModel<AccessFields>('site', { required: true })
const geoLoaded = ref(true)

onMounted(async () => {
  try {
    geoLoaded.value = (await api.get<{ loaded: boolean }>('/api/webservice/geoip')).loaded
  } catch {
    geoLoaded.value = false
  }
})

function addLink() {
  site.value.shareLinks.push({ name: '', token: '', maxUses: 0, pathPrefix: '' })
}

// copyLink 用站点的首个具体域名与监听端口拼出分享地址；通配域名（含首段通配 api-*.example.com）不能直接访问，
// 全是通配时退回当前页面的主机名
function copyLink(l: ShareLink) {
  const host = firstHost(site.value.domains) ?? location.hostname
  const defaultPort = props.tls ? 443 : 80
  const port = props.port === defaultPort ? '' : `:${props.port}`
  copyText(`${props.tls ? 'https' : 'http'}://${host}${port}/?_gs=${l.token}`)
}
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
  min-width: 140px;
}
.w-prefix {
  width: 130px;
}
.w-auth,
.w-ip {
  width: 140px;
}
.w-sets {
  width: 180px;
}
.w-name {
  width: 130px;
}
.w-date {
  width: 180px;
}
.w-uses {
  width: 100px;
}
.w-path {
  width: 140px;
}
.warn {
  color: var(--el-color-warning);
}
</style>
