// 管理端安全评分与密码强度的前端规则（纯函数，便于单元测试）。评分由后端 GET /api/system/security 计算。

export type SecLevel = 'good' | 'warn' | 'risk'
export type SecState = 'ok' | 'warn' | 'risk'

export interface SecItem {
  id: string
  status: SecState
  points: number
  max: number
}

export interface SecStatus {
  score: number
  level: SecLevel
  // 管理端可能暴露在公网
  exposed: boolean
  passwordIssue: '' | 'default' | 'weak'
  items: SecItem[]
}

// 评分项的显示顺序：与后端无关，按对安全的影响从大到小
export const ITEM_ORDER = ['password', 'twoFactor', 'entry', 'https', 'lockout', 'globalLimit', 'username']

// 加固提示的暂缓记录（localStorage）：仅 warn 级别可以暂缓，risk 级别一直显示
export const SEC_SNOOZE_KEY = 'gately-security-snooze'
export const SEC_SNOOZE_MS = 7 * 24 * 60 * 60 * 1000

// failingItems 返回未满分的检查项，扣分多的在前（分数相同时按固定顺序）
export function failingItems(s: Pick<SecStatus, 'items'> | undefined): SecItem[] {
  if (!s) {
    return []
  }
  return s.items
    .filter((it) => it.status !== 'ok' && it.points > 0)
    .sort((a, b) => b.points - a.points || ITEM_ORDER.indexOf(a.id) - ITEM_ORDER.indexOf(b.id))
}

// snoozeKey 是当前待加固问题的标识：出现新的问题时变化，暂缓随之失效
export function snoozeKey(s: Pick<SecStatus, 'items'>): string {
  return failingItems(s)
    .map((it) => it.id)
    .sort()
    .join(',')
}

// bannerLevel 返回顶部加固提示的级别：risk 一直显示；warn 在暂缓期内且问题未变化时不显示；其余不显示
export function bannerLevel(s: SecStatus | undefined, st: { snooze: string; now: number }): '' | 'warn' | 'risk' {
  if (!s || s.level === 'good') {
    return ''
  }
  if (s.level === 'risk') {
    return 'risk'
  }
  try {
    const v = JSON.parse(st.snooze || '{}') as { key?: string; until?: number }
    if (v.key === snoozeKey(s) && typeof v.until === 'number' && v.until > st.now && v.until - st.now <= SEC_SNOOZE_MS) {
      return ''
    }
  } catch {
    // 记录损坏时照常提示
  }
  return 'warn'
}

// 修复入口：所在设置页签与页内锚点（元素 id）
export interface FixTarget {
  tab: 'account' | 'access'
  anchor: string
}

export function fixTarget(id: string): FixTarget {
  switch (id) {
    case 'entry':
      return { tab: 'access', anchor: 'sec-entry' }
    case 'twoFactor':
      return { tab: 'account', anchor: 'sec-twofactor' }
    case 'lockout':
    case 'globalLimit':
      return { tab: 'account', anchor: 'sec-loginprotect' }
    default:
      return { tab: 'account', anchor: 'sec-account' }
  }
}

// 后端 CheckPasswordStrength 返回的原因代码，与界面文案 security.weak.<code> 一一对应
export const WEAK_CODES = ['tooShort', 'tooLong', 'common', 'username', 'product', 'sequence', 'repeat', 'simple', 'control'] as const
export type WeakCode = (typeof WEAK_CODES)[number]

export function isWeakCode(c: unknown): c is WeakCode {
  return typeof c === 'string' && (WEAK_CODES as readonly string[]).includes(c)
}

// localWeakCode 在不请求后端的情况下先判断长度类问题，其余规则（常见口令、序列等）以后端为准。
// 密码最长 72 字节（bcrypt 的限制），按 UTF-8 字节数计算。
export function localWeakCode(pw: string): '' | 'tooShort' | 'tooLong' | 'control' {
  if (/[\u0000\r\n]/.test(pw)) {
    return 'control'
  }
  if ([...pw].length < 10) {
    return 'tooShort'
  }
  if (new TextEncoder().encode(pw).length > 72) {
    return 'tooLong'
  }
  return ''
}

// 一个只用于界面展示的强度条分值（0~4）：长度与字符种类，后端不合格时封顶为 1
export function strengthBars(pw: string, serverOk: boolean | undefined): number {
  if (!pw) {
    return 0
  }
  if (serverOk === false || localWeakCode(pw) !== '') {
    return 1
  }
  let kinds = 0
  for (const re of [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9]/]) {
    if (re.test(pw)) {
      kinds++
    }
  }
  const len = [...pw].length
  let bars = 2
  if (kinds >= 3 || len >= 16) {
    bars = 3
  }
  if (kinds >= 3 && len >= 14) {
    bars = 4
  }
  return bars
}
