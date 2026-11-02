<template>
  <!-- 升级提示：读取上次检查更新的结果（不访问网络）。受安全问题影响时醒目提示，只能暂缓 24 小时；普通新版本可忽略该版本 -->
  <div v-if="kind" class="update-banner" :class="kind" role="status">
    <component :is="kind === 'security' ? ShieldAlert : Sparkles" :size="18" class="ub-icon" />
    <span class="ub-text">{{ text }}</span>
    <div class="ub-actions">
      <el-button v-if="info?.hasUpdate" size="small" :type="kind === 'security' ? 'danger' : 'primary'" @click="dialog?.showRelease(info!)">
        {{ t('layout.update.view') }}
      </el-button>
      <el-button v-else-if="firstAdvisory" size="small" type="danger" tag="a" :href="firstAdvisory" target="_blank" rel="noopener noreferrer">
        {{ t('layout.update.view') }}
      </el-button>
      <el-button size="small" text @click="dismiss">{{ kind === 'security' ? t('layout.update.snooze') : t('layout.update.skip') }}</el-button>
    </div>
    <UpdateDialog ref="dialog" :is-admin="isAdmin" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ShieldAlert, Sparkles } from 'lucide-vue-next'
import { api, type UpdateInfo } from '../api'
import UpdateDialog from './UpdateDialog.vue'
import { bannerKind, securityKey, SNOOZE_KEY, SKIP_KEY, SNOOZE_MS } from '../updateBanner'

defineProps<{ isAdmin: boolean }>()
const { t } = useI18n()
const info = ref<UpdateInfo>()
const dialog = ref<InstanceType<typeof UpdateDialog>>()
// 读取 localStorage 的值放进响应式变量，点击后立即隐藏
const snooze = ref(localStorage.getItem(SNOOZE_KEY) ?? '')
const skipped = ref(localStorage.getItem(SKIP_KEY) ?? '')
const now = ref(Date.now())

const kind = computed(() => bannerKind(info.value, { snooze: snooze.value, skipped: skipped.value, now: now.value }))

const text = computed(() => {
  const u = info.value
  if (!u) {
    return ''
  }
  if (kind.value === 'security') {
    if (u.advisories.length) {
      return u.hasUpdate
        ? t('layout.update.securityAdvisories', { current: u.current, count: u.advisories.length, latest: u.latest })
        : t('layout.update.securityNoFix', { current: u.current, count: u.advisories.length })
    }
    return t('layout.update.securityReleases', { current: u.current, versions: u.securityReleases.map((r) => r.version).join(', '), latest: u.latest })
  }
  return t('layout.update.available', { latest: u.latest, current: u.current })
})

const firstAdvisory = computed(() => info.value?.advisories[0]?.url ?? '')

// dismiss 安全提示暂缓 24 小时（出现新的安全问题时立即重新提示），普通新版本忽略到下一个版本发布
function dismiss() {
  const u = info.value
  if (!u) {
    return
  }
  // now 只在读取结果时更新，这里同步为当前时间，否则暂缓的截止时间会被当成超过 24 小时的无效记录
  now.value = Date.now()
  if (kind.value === 'security') {
    snooze.value = JSON.stringify({ key: securityKey(u), until: now.value + SNOOZE_MS })
    localStorage.setItem(SNOOZE_KEY, snooze.value)
  } else {
    skipped.value = u.latest
    localStorage.setItem(SKIP_KEY, u.latest)
  }
}

async function load() {
  try {
    info.value = await api.get<UpdateInfo>('/api/system/update/state')
    now.value = Date.now()
  } catch {
    // 读取失败时不显示提示
  }
}

// 页面长时间打开时定期读取自动检查的新结果；该接口只读本地缓存，不访问 GitHub
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  load()
  timer = setInterval(load, 30 * 60 * 1000)
})
onBeforeUnmount(() => {
  if (timer) {
    clearInterval(timer)
  }
})

defineExpose({ reload: load })
</script>

<style scoped>
.update-banner {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  padding: 10px 14px;
  margin-bottom: 16px;
  border-radius: var(--g-radius);
  border: 1px solid var(--el-color-primary-light-5);
  background: var(--el-color-primary-light-9);
  font-size: 13px;
  line-height: 1.5;
}
.update-banner.security {
  border-color: var(--el-color-danger-light-5);
  background: var(--el-color-danger-light-9);
}
.ub-icon {
  flex-shrink: 0;
  color: var(--el-color-primary);
}
.security .ub-icon {
  color: var(--el-color-danger);
}
.ub-text {
  flex: 1;
  min-width: 200px;
}
.ub-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}
</style>
