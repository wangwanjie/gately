<template>
  <!-- 连接后端的选项：反向代理站点作用于全部后端；其他类型站点配置了按用户分流时作用于分流到的后端（#79） -->
  <el-form-item data-test="proxy-options">
    <template #label>
      <!-- 表单标签是顶端对齐的 flex 容器，文字与问号需包一层才能垂直居中 -->
      <span class="label-with-help">{{ t('web.options') }}<ProxyOptionsHelp /></span>
    </template>
    <el-checkbox v-model="site.insecureSkipVerify">{{ t('web.optSkipVerify') }}</el-checkbox>
    <el-checkbox v-model="site.useTargetHost">{{ t('web.optTargetHost') }}</el-checkbox>
    <el-checkbox v-model="site.disableKeepAlives">{{ t('web.optNoKeepAlive') }}</el-checkbox>
    <el-checkbox v-model="site.forwardedHeaders">{{ t('web.optForwarded') }}</el-checkbox>
    <el-checkbox v-model="site.sendProxyProtocol">{{ t('web.optSendProxy') }}</el-checkbox>
    <el-checkbox v-model="site.upstreamSniFromHost">{{ t('web.optSniFromHost') }}</el-checkbox>
    <!-- 认证交给后端 Gately 时才能转发登录 Cookie，本站点开了认证就不可勾选 -->
    <el-checkbox v-model="site.passAuthCookies" :disabled="authOn && !site.passAuthCookies">{{ t('web.optPassAuthCookies') }}</el-checkbox>
    <div class="form-help">{{ t('web.optSniFromHostHelp') }}</div>
    <div v-if="site.passAuthCookies" class="form-help">{{ t('web.optPassAuthCookiesHelp') }}</div>
    <div v-if="site.type !== 'proxy'" class="form-help">{{ t('web.optUserRoutesOnly') }}</div>
  </el-form-item>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import ProxyOptionsHelp from './ProxyOptionsHelp.vue'

// ProxyFields 是站点中连接后端的选项字段
export interface ProxyFields {
  type: string
  insecureSkipVerify: boolean
  useTargetHost: boolean
  disableKeepAlives: boolean
  forwardedHeaders: boolean
  sendProxyProtocol: boolean
  upstreamSniFromHost: boolean
  passAuthCookies: boolean
}

const { t } = useI18n()
const site = defineModel<ProxyFields>('site', { required: true })
// authOn 表示站点启用了认证，此时不能转发 Gately 登录 Cookie
defineProps<{ authOn: boolean }>()
</script>

<style scoped>
.label-with-help {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
</style>
