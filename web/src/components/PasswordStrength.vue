<template>
  <!-- 新密码强度提示：长度先在本地判断，其余规则（常见口令、连续字符、含用户名等）以后端为准，输入停顿后才请求 -->
  <div class="pw-strength" :class="state" role="status" aria-live="polite">
    <div class="bars" :aria-label="t('security.strength.bars')">
      <span v-for="i in 4" :key="i" class="bar" :class="{ on: i <= bars }" />
    </div>
    <div class="msg">
      <template v-if="!password">{{ t('security.strength.hint') }}</template>
      <template v-else-if="state === 'checking'">{{ t('security.strength.checking') }}</template>
      <template v-else-if="state === 'ok'">{{ t('security.strength.ok') }}</template>
      <template v-else>{{ t(`security.weak.${code}`) }}</template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { isWeakCode, localWeakCode, strengthBars, type WeakCode } from '../security'

const props = defineProps<{ password: string; username?: string }>()
// status：'' 为空或检查中，'ok' 为合格，'weak' 为不合格
const emit = defineEmits<{ (e: 'status', v: '' | 'ok' | 'weak'): void }>()
const { t } = useI18n()

const serverOk = ref<boolean | undefined>(undefined)
const serverCode = ref<WeakCode | ''>('')
const checking = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
// 请求序号：只采纳最近一次请求的结果，避免输入过程中旧响应覆盖新结果
let seq = 0

const local = computed(() => (props.password ? localWeakCode(props.password) : ''))
const code = computed<WeakCode | ''>(() => (local.value || serverCode.value) as WeakCode | '')
const state = computed(() => {
  if (!props.password) {
    return 'empty'
  }
  if (local.value) {
    return 'weak'
  }
  if (checking.value) {
    return 'checking'
  }
  return serverOk.value === false ? 'weak' : serverOk.value === true ? 'ok' : 'checking'
})
const bars = computed(() => strengthBars(props.password, state.value === 'weak' ? false : serverOk.value))

watch(
  [() => props.password, () => props.username],
  ([pw, user]) => {
    clearTimeout(timer)
    seq++
    serverOk.value = undefined
    serverCode.value = ''
    if (!pw || localWeakCode(pw)) {
      checking.value = false
      return
    }
    checking.value = true
    const mine = seq
    timer = setTimeout(async () => {
      try {
        const r = await api.post<{ ok: boolean; code?: string }>('/api/account/password-check', { username: user ?? '', password: pw })
        if (mine !== seq) {
          return
        }
        serverOk.value = r.ok
        serverCode.value = !r.ok && isWeakCode(r.code) ? r.code : r.ok ? '' : 'common'
      } catch {
        // 预检失败（网络或权限）时不阻止提交，最终以保存时的后端校验为准
        if (mine === seq) {
          serverOk.value = undefined
        }
      } finally {
        if (mine === seq) {
          checking.value = false
        }
      }
    }, 350)
  },
  { immediate: true },
)

watch(
  state,
  (s) => {
    emit('status', s === 'ok' ? 'ok' : s === 'weak' ? 'weak' : '')
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  clearTimeout(timer)
  seq++
})
</script>

<style scoped>
.pw-strength {
  width: 100%;
  margin-top: 6px;
  font-size: 12px;
  line-height: 1.5;
}
.bars {
  display: flex;
  gap: 4px;
}
.bar {
  flex: 1;
  height: 4px;
  border-radius: 2px;
  background: var(--el-fill-color-dark);
  transition: background-color 0.2s;
}
.msg {
  margin-top: 4px;
  color: var(--el-text-color-secondary);
  overflow-wrap: anywhere;
}
.weak .bar.on {
  background: var(--el-color-danger);
}
.weak .msg {
  color: var(--el-color-danger);
}
.ok .bar.on {
  background: var(--el-color-success);
}
.ok .msg {
  color: var(--el-color-success);
}
.checking .bar.on,
.empty .bar.on {
  background: var(--el-color-warning);
}
</style>
