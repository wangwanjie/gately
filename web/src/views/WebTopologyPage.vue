<template>
  <!-- Web 网络拓扑独立页面：拓扑图本身见 components/WebTopology.vue -->
  <div>
    <PageHeader :title="t('webTopology.title')" :desc="t('webTopology.pageDesc')" :icon="Workflow">
      <el-input v-model="filterText" class="site-filter" clearable :prefix-icon="Search" :placeholder="t('web.filterPlaceholder')" />
      <el-button :icon="Server" @click="router.push({ name: 'web' })">{{ t('webTopology.openWeb') }}</el-button>
    </PageHeader>

    <WebTopology
      ref="topo"
      :query="filterText"
      @stats="(lk, sk, name) => statsDialog?.open(lk, sk, name)"
      @conns="(lk, sk, name) => connsDialog?.open(lk, sk, name)"
      @edit="(lk, sk) => editor?.editSite(lk, sk)"
      @edit-listener="(lk) => editor?.editListener(lk)"
      @logs="(target) => logsDialog?.open(target)"
    />

    <SiteStats ref="statsDialog" />
    <SiteConns ref="connsDialog" @changed="topo?.reload()" />
    <SiteLogDialog ref="logsDialog" />
    <!-- 复用 Web 服务页的编辑弹窗，在本页直接编辑监听与站点，保存后刷新拓扑 -->
    <Web ref="editor" dialog-only @saved="topo?.reload()" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Search, Server, Workflow } from 'lucide-vue-next'
import PageHeader from '../components/PageHeader.vue'
import WebTopology from '../components/WebTopology.vue'
import SiteStats from '../components/SiteStats.vue'
import SiteConns from '../components/SiteConns.vue'
import SiteLogDialog from '../components/SiteLogDialog.vue'
import Web from './Web.vue'

const { t } = useI18n()
const router = useRouter()

// filterText 为页头的站点搜索词，拓扑图按它筛选站点
const filterText = ref('')
const topo = ref<InstanceType<typeof WebTopology>>()
const statsDialog = ref<InstanceType<typeof SiteStats>>()
const connsDialog = ref<InstanceType<typeof SiteConns>>()
const logsDialog = ref<InstanceType<typeof SiteLogDialog>>()
const editor = ref<InstanceType<typeof Web>>()
</script>

<style scoped>
.site-filter {
  width: 220px;
}
@media (max-width: 767.98px) {
  .site-filter {
    width: 160px;
  }
}
</style>
