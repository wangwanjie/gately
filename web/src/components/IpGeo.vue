<template>
  <span v-if="parts.length" class="ip-geo" :class="{ lan: isLan }" :title="title" data-test="ip-geo">{{ text }}</span>
</template>

<script setup lang="ts">
// 显示 IP 的归属地（国家 · 省 · 城市），放在显示地址的文字旁边。没有导入 GeoIP 数据库、
// 关闭了显示或库中未收录时不占位；内网与本机地址不需要数据库，显示为「内网」。ip 可以是地址、地址:端口或 CIDR。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { geoOf } from '../composables/useIpGeo'
import { geoParts, lookupKey } from '../ipGeo'

const props = defineProps<{
  ip: string
  // short 只显示最后一级（城市、省或国家），完整内容在悬停提示里，用于空间紧张的地方
  short?: boolean
  // hideLan 不标出内网与本机地址，用于内网地址很多、标出来只是噪声的地方（如日志）
  hideLan?: boolean
}>()

const { t, locale } = useI18n()

const loc = computed(() => geoOf(lookupKey(props.ip)))
const isLan = computed(() => loc.value?.country === 'LAN')
const parts = computed(() => (isLan.value && props.hideLan ? [] : geoParts(loc.value, String(locale.value), t('ipGeo.lan'))))
const text = computed(() => (props.short ? parts.value[parts.value.length - 1] : parts.value.join(' · ')))
const title = computed(() => (isLan.value ? t('ipGeo.lanHelp') : parts.value.join(' · ')))
</script>

<style scoped>
.ip-geo {
  display: inline-block;
  flex: 0 1 auto;
  min-width: 0;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
  font-size: 11px;
  line-height: 16px;
  padding: 0 5px;
  border-radius: 4px;
  color: var(--el-text-color-regular);
  background: var(--el-fill-color);
  font-family: var(--el-font-family);
}
.ip-geo.lan {
  color: var(--el-text-color-secondary);
}
</style>
