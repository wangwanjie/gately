// 首次启动引导的共享状态：场景选择对话框（Onboarding.vue）与总览顶部的「开始使用」清单（GettingStarted.vue）共用
import { ref } from 'vue'
import { api } from '../api'

// OnboardingState 对应 GET /api/system/onboarding
export interface OnboardingState {
  // pending 为是否显示「开始使用」清单
  pending: boolean
  // scenePending 为是否需要先选择使用场景（只在全新安装时出现）
  scenePending: boolean
  scenes: string[]
  done: string[]
}

// onboardingState 为最近一次读取的状态；null 表示尚未读取或无权读取（只读账号）
export const onboardingState = ref<OnboardingState | null>(null)

let inflight: Promise<void> | null = null

// refreshOnboarding 重新读取引导状态；同时发起的多次调用共用一个请求
export function refreshOnboarding(): Promise<void> {
  if (!inflight) {
    inflight = api
      .get<OnboardingState>('/api/system/onboarding')
      .then((s) => {
        onboardingState.value = s
      })
      .catch(() => {
        // 只读账号无权访问该接口，不展示引导
        onboardingState.value = null
      })
      .finally(() => {
        inflight = null
      })
  }
  return inflight
}

// markOnboardingDone 把无法从配置判断的清单项标记为完成（如已运行网络自检）
export async function markOnboardingDone(item: string): Promise<void> {
  if (onboardingState.value?.done.includes(item)) {
    return
  }
  try {
    onboardingState.value = await api.post<OnboardingState>('/api/system/onboarding/done', { item })
  } catch {
    // 标记失败只会让该项保持未完成，不影响使用
  }
}

// dismissOnboarding 关闭「开始使用」清单，之后不再显示
export async function dismissOnboarding(): Promise<void> {
  await api.post('/api/system/onboarding/dismiss', {})
  if (onboardingState.value) {
    onboardingState.value = { ...onboardingState.value, pending: false, scenePending: false }
  }
}
