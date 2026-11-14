<template>
  <el-dialog v-model="visible" width="min(560px, 94vw)" align-center append-to-body :close-on-click-modal="false">
    <template #header>
      <div class="head">
        <Rocket :size="20" class="head-icon" />
        <div>
          <div class="title">{{ t('onboarding.title') }}</div>
          <div class="sub">{{ t('onboarding.subtitle') }}</div>
        </div>
      </div>
    </template>
    <ul class="steps">
      <li v-for="s in steps" :key="s.key">
        <span class="s-icon"><component :is="s.icon" :size="18" /></span>
        <div class="s-text">
          <div class="s-title">{{ s.title }}</div>
          <div class="s-desc">{{ s.desc }}</div>
        </div>
        <el-button size="small" @click="s.run">{{ t('onboarding.go') }}</el-button>
      </li>
    </ul>
    <template #footer>
      <el-button @click="visible = false">{{ t('onboarding.later') }}</el-button>
      <el-button type="primary" @click="done">{{ t('onboarding.done') }}</el-button>
    </template>
  </el-dialog>
  <SelfCheck ref="selfCheck" />
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Bell, DatabaseBackup, Fingerprint, Rocket, Stethoscope } from 'lucide-vue-next'
import { api } from '../api'
import SelfCheck from './SelfCheck.vue'
import { moduleEnabled } from '../nav'

const { t } = useI18n()
const router = useRouter()
const visible = ref(false)
const selfCheck = ref<InstanceType<typeof SelfCheck>>()

// 在不关闭引导的情况下跳转，让用户完成后回来继续下一项
function go(path: string, query?: Record<string, string>) {
  visible.value = false
  router.push({ path, query })
}

// 通知中心关闭时不再引导配置通知
const steps = computed(() => [
  { key: 'security', icon: Fingerprint, title: t('onboarding.security.title'), desc: t('onboarding.security.desc'), run: () => go('/settings', { tab: 'account' }) },
  { key: 'check', icon: Stethoscope, title: t('onboarding.check.title'), desc: t('onboarding.check.desc'), run: () => selfCheck.value?.open() },
  { key: 'import', icon: DatabaseBackup, title: t('onboarding.import.title'), desc: t('onboarding.import.desc'), run: () => go('/settings', { tab: 'backup' }) },
  { key: 'notify', icon: Bell, title: t('onboarding.notify.title'), desc: t('onboarding.notify.desc'), run: () => go('/notify'), module: 'notify' },
].filter((s) => moduleEnabled(s.module)))

async function done() {
  visible.value = false
  try {
    await api.post('/api/system/onboarding/dismiss', {})
  } catch {
    // 标记失败只会让引导再次出现，不影响使用
  }
}

onMounted(async () => {
  try {
    visible.value = (await api.get<{ pending: boolean }>('/api/system/onboarding')).pending
  } catch {
    // 只读账号无权访问该接口，不展示引导
  }
})
</script>

<style scoped>
.head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.head-icon {
  color: var(--el-color-primary);
}
.title {
  font-size: 17px;
  font-weight: 600;
}
.sub {
  margin-top: 2px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.steps {
  margin: 0;
  padding: 0;
  list-style: none;
}
.steps li {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 11px 2px;
  border-bottom: 1px solid var(--el-border-color-extra-light);
}
.steps li:last-child {
  border-bottom: 0;
}
.s-icon {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  border-radius: 9px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.s-text {
  flex: 1;
  min-width: 0;
}
.s-title {
  font-size: 14px;
  font-weight: 500;
}
.s-desc {
  margin-top: 2px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}
</style>
