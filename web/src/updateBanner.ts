// 顶部升级提示的显示规则（纯函数，便于单元测试）。
import type { UpdateInfo } from './api'

// localStorage 键：安全提示的暂缓记录（JSON：{key, until}）与忽略的普通版本号
export const SNOOZE_KEY = 'gately-update-snooze'
export const SKIP_KEY = 'gately-update-skip'
// 安全提示每次最多暂缓 24 小时
export const SNOOZE_MS = 24 * 60 * 60 * 1000

export type BannerKind = '' | 'security' | 'update'

// securityKey 是当前安全问题的标识：出现新的安全公告或安全修复版本时变化，暂缓随之失效
export function securityKey(u: Pick<UpdateInfo, 'advisories' | 'securityReleases'>): string {
  return [...u.advisories.map((a) => a.id), ...u.securityReleases.map((r) => r.version)].sort().join(',')
}

// bannerKind 返回要显示的提示：受安全问题影响时为 security（暂缓未到期且安全问题未变化时不显示），
// 有新版本且没有忽略该版本时为 update，否则为空
export function bannerKind(u: UpdateInfo | undefined, st: { snooze: string; skipped: string; now: number }): BannerKind {
  if (!u || !u.latest) {
    return ''
  }
  if (u.security) {
    try {
      const s = JSON.parse(st.snooze || '{}') as { key?: string; until?: number }
      if (s.key === securityKey(u) && typeof s.until === 'number' && s.until > st.now && s.until - st.now <= SNOOZE_MS) {
        return ''
      }
    } catch {
      // 记录损坏时照常提示
    }
    return 'security'
  }
  if (u.hasUpdate && st.skipped !== u.latest) {
    return 'update'
  }
  return ''
}
