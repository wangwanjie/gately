<template>
  <!-- 配置变更确认：保存后经新连接确认不了、或打开页面时发现待确认的变更时弹出，倒计时结束服务端自动回滚 -->
  <el-dialog
    :model-value="visible"
    :title="title"
    width="460px"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
    append-to-body
    class="config-confirm-dialog"
    data-test="config-confirm-dialog"
  >
    <div class="body">
      <component :is="state.phase === 'rollingBack' ? Loader2 : TriangleAlert" :size="34" :class="state.phase === 'rollingBack' ? 'spin' : 'warn'" />
      <p class="desc">{{ desc }}</p>
      <p v-if="moduleText && state.phase !== 'rollingBack'" class="modules">{{ t('configConfirm.modules', { modules: moduleText }) }}</p>
      <div v-if="state.phase !== 'rollingBack' && state.deadline" class="countdown" data-test="config-confirm-countdown">
        <span class="num">{{ remaining }}</span>
        <span class="muted">{{ t('configConfirm.countdown') }}</span>
      </div>
      <p v-if="state.phase === 'failed'" class="muted retrying">{{ t('configConfirm.retrying') }}</p>
      <!-- 连不上服务端时“立即回滚”同样发不出去：说明只能等自动回滚，或在服务器上用命令行回滚 -->
      <p v-if="state.phase !== 'rollingBack' && !state.reachable" class="unreachable" data-test="config-confirm-unreachable">
        {{ t('configConfirm.unreachable') }}<code>gately rollback -pending</code>
      </p>
    </div>
    <template v-if="state.phase !== 'rollingBack'" #footer>
      <el-button v-if="state.reachable" :loading="state.busy" data-test="config-confirm-rollback" @click="onRollback">{{ t('configConfirm.rollback') }}</el-button>
      <el-button v-if="state.phase === 'found'" type="primary" :loading="state.busy" data-test="config-confirm-keep" @click="onConfirm">{{ t('configConfirm.keep') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Loader2, TriangleAlert } from 'lucide-vue-next'
import { confirmNow, confirmState as state, rollbackNow } from '../configConfirm'

const { t } = useI18n()

const visible = computed(() => state.phase === 'failed' || state.phase === 'found' || state.phase === 'rollingBack')

const title = computed(() => {
  switch (state.phase) {
    case 'found':
      return t('configConfirm.foundTitle')
    case 'rollingBack':
      return t('configConfirm.rollingBackTitle')
  }
  return t('configConfirm.failedTitle')
})

const desc = computed(() => {
  switch (state.phase) {
    case 'found':
      return t('configConfirm.foundDesc')
    case 'rollingBack':
      return t('configConfirm.rollingBackDesc')
  }
  return t('configConfirm.failedDesc')
})

// 配置名换成导航中的模块名称
const configKeys: Record<string, string> = {
  portforward: 'nav.items.portforward.title',
  protoroute: 'nav.items.protoroute.title',
  webservice: 'nav.items.web.title',
  gately: 'layout.systemSettings',
}

function namesOf(configs: string[]): string {
  return configs.map((c) => (configKeys[c] ? t(configKeys[c]) : c)).join(t('configConfirm.separator'))
}

const moduleText = computed(() => namesOf(state.configs))

// 倒计时每秒刷新
const now = ref(Date.now())
const ticker = window.setInterval(() => {
  now.value = Date.now()
}, 1000)
onBeforeUnmount(() => window.clearInterval(ticker))
const remaining = computed(() => Math.max(0, Math.ceil((state.deadline - now.value) / 1000)))

async function onConfirm() {
  if (!(await confirmNow())) {
    ElMessage.error(t('configConfirm.confirmFailed'))
  }
}

async function onRollback() {
  try {
    await rollbackNow()
  } catch (e) {
    // 连不上服务端时界面改为说明只能等待自动回滚，不再显示英文的 Failed to fetch
    if (!state.reachable) {
      ElMessage.warning(t('configConfirm.rollbackUnreachable'))
      return
    }
    ElMessage.error((e as Error).message)
  }
}

// 弹窗出现后才确认成功（访问恢复，或用户在新地址确认）：提示一次
watch(
  () => state.confirmedAfterPrompt,
  () => ElMessage.success(t('configConfirm.confirmed')),
)

// 本标签页等待过的变更已被回滚：重新打开页面后提示原因
watch(
  () => state.rolledBack,
  (rb) => {
    if (!rb) {
      return
    }
    const key = rb.reason === 'manual' ? 'configConfirm.rolledBackManual' : rb.reason === 'boots' ? 'configConfirm.rolledBackBoots' : 'configConfirm.rolledBackTimeout'
    ElMessageBox.alert(t(key, { modules: namesOf(rb.configs) }), t('configConfirm.rolledBackTitle'), { type: 'warning' }).catch(() => undefined)
    state.rolledBack = null
  },
  { immediate: true },
)
</script>

<style scoped>
.body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  text-align: center;
}
.desc {
  margin: 0;
  line-height: 1.7;
  color: var(--el-text-color-regular);
}
.modules {
  margin: 0;
  font-weight: 600;
}
.countdown {
  display: flex;
  align-items: baseline;
  gap: 6px;
}
.countdown .num {
  font-size: 30px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: var(--el-color-warning);
}
.unreachable {
  margin: 0;
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 13px;
  line-height: 1.7;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-regular);
}
.unreachable code {
  margin-left: 4px;
  font-family: var(--el-font-family-mono, monospace);
}
.retrying {
  margin: 0;
  font-size: 12px;
}
.warn {
  color: var(--el-color-warning);
}
.spin {
  color: var(--el-color-primary);
  animation: spin 1s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
