<template>
  <!-- 在线演示实例的提示条：说明数据会定时重置，并指向安装指南；非演示实例不显示 -->
  <div v-if="enabled" class="demo-banner" role="status">
    <FlaskConical :size="16" class="db-icon" />
    <span class="db-text">{{ t('demo.banner') }}</span>
    <a class="db-link" :href="docsUrl(locale, '/docs/guide/install')" target="_blank" rel="noopener noreferrer">
      {{ t('demo.install') }}<ArrowUpRight :size="14" />
    </a>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowUpRight, FlaskConical } from 'lucide-vue-next'
import { api } from '../api'
import { docsUrl } from '../docsUrl'

const { t, locale } = useI18n()
const enabled = ref(false)

onMounted(async () => {
  try {
    enabled.value = (await api.get<{ enabled: boolean }>('/api/demo')).enabled
  } catch {
    // 读取失败时不显示横幅
  }
})
</script>

<style scoped>
.demo-banner {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-wrap: wrap;
  gap: 4px 10px;
  padding: 8px 16px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--el-color-warning-dark-2);
  background: var(--el-color-warning-light-9);
  border-bottom: 1px solid var(--el-color-warning-light-5);
}
.db-icon {
  flex-shrink: 0;
}
.db-text {
  text-align: center;
}
.db-link {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-weight: 600;
  color: var(--el-color-primary);
  text-decoration: none;
  white-space: nowrap;
}
.db-link:hover {
  text-decoration: underline;
}
</style>
