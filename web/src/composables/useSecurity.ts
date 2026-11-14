// 管理端安全评分的全局共享状态：顶部加固提示与设置页的安全评分卡片读取同一份数据，
// 修改安全相关设置后调用 loadSecurity() 刷新。
import { ref } from 'vue'
import { api } from '../api'
import type { SecStatus } from '../security'

export const securityStatus = ref<SecStatus>()
const loading = ref(false)

// loadSecurity 拉取最新评分；失败（例如只读账号无权限）时保留旧值，不打扰用户
export async function loadSecurity(): Promise<SecStatus | undefined> {
  if (loading.value) {
    return securityStatus.value
  }
  loading.value = true
  try {
    securityStatus.value = await api.get<SecStatus>('/api/system/security')
  } catch {
    // 评分只是提示信息，读取失败不影响使用
  } finally {
    loading.value = false
  }
  return securityStatus.value
}

export function useSecurity() {
  return { status: securityStatus, loading, load: loadSecurity }
}
