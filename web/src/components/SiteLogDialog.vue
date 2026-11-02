<template>
  <!-- 按站点（可附带访客 IP）查看 Web 服务访问日志，供拓扑页与 Web 服务页的站点入口使用 -->
  <el-dialog v-model="visible" :title="title" width="1000px" top="6vh" :fullscreen="isMobile" append-to-body destroy-on-close>
    <el-alert v-if="!accessLog" type="warning" :closable="false" show-icon :title="t('siteLogs.accessLogOff')" class="tip" />
    <el-alert v-else-if="noName" type="info" :closable="false" show-icon :title="t('siteLogs.noName')" class="tip" />
    <LogViewer :key="seq" url="/api/webservice/logs" :initial="initial" height="60vh" />
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import LogViewer from './LogViewer.vue'
import { useFormLayout } from '../composables/useIsMobile'

const { t } = useI18n()
const { isMobile } = useFormLayout()

export interface SiteLogTarget {
  // name 为站点名称（访问日志按它完整匹配），为空时改按 domain 过滤
  name: string
  // label 为对话框标题中显示的站点名称（可能经过打码）
  label: string
  domain?: string
  ip?: string
  // ipLabel 为标题中显示的 IP（可能经过打码）
  ipLabel?: string
  accessLog: boolean
}

const visible = ref(false)
const target = ref<SiteLogTarget>({ name: '', label: '', accessLog: true })
// 每次打开都重新挂载日志查看器，带入新的过滤条件
const seq = ref(0)

const accessLog = computed(() => target.value.accessLog)
// 站点没有名称时访问日志里也没有站点字段，只能按域名部分匹配
const noName = computed(() => !!target.value.label && !target.value.name)
const initial = computed(() => ({
  site: target.value.name,
  host: target.value.name ? '' : (target.value.domain ?? ''),
  ip: target.value.ip ?? '',
}))
// 没有站点（拓扑页未选中站点时按 IP 查看）时标题只显示地址
const title = computed(() => {
  const tg = target.value
  const ip = tg.ipLabel ?? tg.ip ?? ''
  if (!tg.label) {
    return t('siteLogs.titleAll', { ip })
  }
  return ip ? t('siteLogs.titleIp', { site: tg.label, ip }) : t('siteLogs.title', { site: tg.label })
})

function open(tg: SiteLogTarget) {
  target.value = tg
  seq.value++
  visible.value = true
}

defineExpose({ open })
</script>

<style scoped>
.tip {
  margin-bottom: 12px;
}
</style>
