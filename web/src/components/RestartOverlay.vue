<template>
  <!-- 重启 / 恢复期间的全屏等待层：轮询 /api/session，服务恢复后自动刷新页面 -->
  <Teleport to="body">
    <div v-if="visible" class="restart-overlay">
      <div class="box">
        <template v-if="!timedOut">
          <Loader2 :size="36" class="spin" />
          <div class="title">{{ title }}</div>
          <div class="desc">{{ t('components.restartOverlay.resumeHint') }}</div>
          <div class="elapsed">{{ t('components.restartOverlay.waited', { seconds: elapsed }) }}</div>
        </template>
        <template v-else>
          <TriangleAlert :size="36" class="warn" />
          <div class="title">{{ t('components.restartOverlay.timeoutTitle') }}</div>
          <div class="desc">{{ t('components.restartOverlay.timeoutDesc') }}</div>
          <el-button type="primary" :icon="RefreshCw" @click="reload">{{ t('components.restartOverlay.reload') }}</el-button>
        </template>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Loader2, RefreshCw, TriangleAlert } from 'lucide-vue-next'
import { useScrollLock } from '../composables/useScrollLock'

const { t } = useI18n()

const visible = ref(false)
const title = ref('')
const elapsed = ref(0)
const timedOut = ref(false)
let timer: number | undefined
let ticker: number | undefined
// 等待层盖住整个页面，期间页面不随滚轮滚动；等待层只会随页面刷新消失，因此无需解锁
const lockScroll = useScrollLock()

function reload() {
  location.reload()
}

function stop() {
  window.clearTimeout(timer)
  window.clearInterval(ticker)
}

// start 显示等待层并开始轮询。为避免旧进程尚未退出时误判，须先观察到一次失败或等待满 6 秒才接受成功响应。
// waitMs 为最长等待时间：重启后的配置待确认时，确认不了要等服务端超时回滚、再次重启，需要更久
function start(text: string, waitMs = 120000) {
  stop()
  title.value = text
  visible.value = true
  lockScroll(true)
  timedOut.value = false
  elapsed.value = 0
  const begin = Date.now()
  let sawDown = false
  ticker = window.setInterval(() => {
    elapsed.value = Math.floor((Date.now() - begin) / 1000)
  }, 1000)
  const poll = async () => {
    if (Date.now() - begin > waitMs) {
      timedOut.value = true
      stop()
      return
    }
    try {
      const resp = await fetch('/api/session', { credentials: 'same-origin', cache: 'no-store' })
      const ready = sawDown || Date.now() - begin > 6000
      if (ready && resp.status === 200) {
        stop()
        location.reload()
        return
      }
      if (ready && resp.status === 401) {
        // 恢复备份后账号可能变化，回到登录页
        stop()
        location.hash = '#/login'
        location.reload()
        return
      }
      if (resp.status >= 500) {
        sawDown = true
      }
    } catch {
      sawDown = true
    }
    timer = window.setTimeout(poll, 1500)
  }
  timer = window.setTimeout(poll, 1500)
}

onBeforeUnmount(stop)
defineExpose({ start })
</script>

<style scoped>
.restart-overlay {
  position: fixed;
  inset: 0;
  z-index: 5000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: color-mix(in srgb, var(--el-bg-color-page) 80%, transparent);
  backdrop-filter: blur(6px);
  -webkit-backdrop-filter: blur(6px);
}
.box {
  width: 100%;
  max-width: 380px;
  padding: 32px 28px;
  border-radius: 14px;
  border: 1px solid var(--g-border);
  background: var(--el-bg-color);
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  box-shadow: 0 20px 50px -20px rgba(0, 0, 0, 0.3);
}
.spin {
  color: var(--el-color-primary);
  animation: spin 1s linear infinite;
}
.warn {
  color: var(--el-color-warning);
}
.title {
  font-size: 17px;
  font-weight: 600;
}
.desc {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
.elapsed {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
