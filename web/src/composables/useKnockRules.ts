// 端口敲门规则目录，供端口转发、协议分流、STUN 的“需要敲门”选择与卡片显示规则名称。
// 同一页面中多个选择框共用一次请求；读取失败（敲门模块未编译、关闭或令牌没有权限）时为空列表。
import { ref, type Ref } from 'vue'
import { api } from '../api'
import { moduleEnabled } from '../nav'

export interface KnockRuleInfo {
  key: string
  name: string
  enable: boolean
}

const rules = ref<KnockRuleInfo[]>([])
let pending: Promise<void> | null = null
let loadedAt = 0

// reload 重新读取规则目录；10 秒内重复调用复用上一次结果
function reload(): Promise<void> {
  if (pending) {
    return pending
  }
  if (Date.now() - loadedAt < 10000) {
    return Promise.resolve()
  }
  pending = (async () => {
    try {
      if (!moduleEnabled('knock')) {
        rules.value = []
        return
      }
      const list = await api.get<KnockRuleInfo[]>('/api/knock/rules')
      rules.value = list.map((r) => ({ key: r.key, name: r.name, enable: r.enable }))
    } catch {
      rules.value = []
    } finally {
      loadedAt = Date.now()
      pending = null
    }
  })()
  return pending
}

// useKnockRules 返回规则目录并在需要时刷新
export function useKnockRules(): Ref<KnockRuleInfo[]> {
  void reload()
  return rules
}

// knockRuleName 返回规则名称，找不到时返回 key
export function knockRuleName(key: string): string {
  return rules.value.find((r) => r.key === key)?.name ?? key
}
