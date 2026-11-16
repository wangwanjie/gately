<template>
  <!-- 连接后端的选项：反向代理站点作用于全部后端；其他类型站点配置了按用户分流时作用于分流到的后端（#79） -->
  <el-form-item data-test="proxy-options">
    <template #label>
      <!-- 表单标签是顶端对齐的 flex 容器，文字与问号需包一层才能垂直居中 -->
      <span class="label-with-help">{{ t('web.options') }}<ProxyOptionsHelp /></span>
    </template>
    <!-- 多数应用只需要「把访客信息告诉后端」；其余选项是特定后端才用到的，收在「高级」里，有勾选时默认展开（issue: 选项术语墙） -->
    <div class="opt-main">
      <el-checkbox v-model="site.forwardedHeaders">{{ t('web.optForwarded') }}</el-checkbox>
    </div>
    <button type="button" class="opt-adv-btn" :aria-expanded="advOpen" @click="advOpen = !advOpen">
      <ChevronRight :size="14" class="opt-adv-icon" :class="{ 'is-open': advOpen }" aria-hidden="true" />
      {{ t('web.optAdvanced') }}
      <span v-if="!advOpen && advCount" class="opt-adv-count">{{ t('common.advancedChanged', { count: advCount }) }}</span>
    </button>
    <div v-show="advOpen" class="opt-adv">
      <el-checkbox v-model="site.insecureSkipVerify">{{ t('web.optSkipVerify') }}</el-checkbox>
      <el-checkbox v-model="site.useTargetHost">{{ t('web.optTargetHost') }}</el-checkbox>
      <el-checkbox v-model="site.disableKeepAlives">{{ t('web.optNoKeepAlive') }}</el-checkbox>
      <el-checkbox v-model="site.sendProxyProtocol">{{ t('web.optSendProxy') }}</el-checkbox>
      <el-checkbox v-model="site.upstreamSniFromHost">{{ t('web.optSniFromHost') }}</el-checkbox>
      <div class="form-help opt-help">{{ t('web.optSniFromHostHelp') }}</div>
      <el-checkbox v-model="site.upstreamH2c" data-test="opt-h2c">{{ t('web.optH2C') }}</el-checkbox>
      <div class="form-help opt-help" data-test="opt-h2c-help">{{ t('web.optH2CHelp') }}</div>
      <!-- 认证交给后端 Gately 时才能转发登录 Cookie，本站点开了认证就不可勾选 -->
      <el-checkbox v-model="site.passAuthCookies" :disabled="authOn && !site.passAuthCookies">{{ t('web.optPassAuthCookies') }}</el-checkbox>
      <div v-if="site.passAuthCookies" class="form-help opt-help">{{ t('web.optPassAuthCookiesHelp') }}</div>
    </div>
    <div v-if="site.type !== 'proxy'" class="form-help">{{ t('web.optUserRoutesOnly') }}</div>
  </el-form-item>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronRight } from 'lucide-vue-next'
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
  upstreamH2c: boolean
  passAuthCookies: boolean
}

const { t } = useI18n()
const site = defineModel<ProxyFields>('site', { required: true })
// authOn 表示站点启用了认证，此时不能转发 Gately 登录 Cookie
defineProps<{ authOn: boolean }>()

// advCount 为已勾选的高级选项数；组件在打开编辑框时创建，已有勾选时默认展开
const advCount = computed(() => {
  const v = site.value
  return [v.insecureSkipVerify, v.useTargetHost, v.disableKeepAlives, v.sendProxyProtocol, v.upstreamSniFromHost, v.upstreamH2c, v.passAuthCookies].filter(Boolean).length
})
const advOpen = ref(advCount.value > 0)
// 编辑框不销毁，打开另一个站点时（站点对象被整体替换）按该站点重新决定是否展开
watch(site, () => {
  advOpen.value = advCount.value > 0
})
</script>

<style scoped>
.opt-main,
.opt-adv {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  width: 100%;
}
.opt-adv {
  padding-left: 18px;
}
/* 选项说明紧跟在对应选项下面，与复选框文字对齐 */
.opt-help {
  margin: -4px 0 4px 24px;
}
.opt-adv-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 28px;
  padding: 0;
  border: 0;
  background: none;
  font: inherit;
  font-size: 13px;
  color: var(--el-color-primary);
  cursor: pointer;
}
.opt-adv-btn:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 2px;
  border-radius: 4px;
}
.opt-adv-icon {
  transition: transform 0.15s ease;
}
.opt-adv-icon.is-open {
  transform: rotate(90deg);
}
.opt-adv-count {
  color: var(--el-text-color-secondary);
}
.label-with-help {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
/* 窄屏下长选项文字换行，避免“向后端发送 PROXY v2（…）”等被裁掉 */
@media (max-width: 768px) {
  .el-checkbox {
    height: auto;
    min-height: 32px;
    padding: 4px 0;
    line-height: 1.4;
    white-space: normal;
  }
}
</style>
