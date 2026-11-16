// 总览服务状态的纯函数（不依赖界面，便于单元测试）。数据来自 /api/status 中各模块的摘要。

export interface SummaryItem {
  label: string
  value: string
  level?: string
}

// SvcLevel 为一个模块在总览上的状态：idle 表示还没有配置任何条目或功能未启用（灰点），不算正常也不算异常
export type SvcLevel = 'ok' | 'warn' | 'error' | 'idle'

const rank: Record<string, number> = { ok: 1, warn: 2, error: 3 }

// 计数类的摘要值：0、0 / 0
const zeroValue = /^0(\s*\/\s*0)?$/

// serviceLevel 由模块摘要得出状态：有警告或错误时取最严重的；否则摘要全是 0（如“运行中 0 / 0”）
// 或全是说明性的 info 项（如“状态：未启用”）时为 idle；其余为 ok
export function serviceLevel(items: SummaryItem[]): SvcLevel {
  let level = 'ok'
  for (const it of items) {
    if (it.level && (rank[it.level] ?? 0) > (rank[level] ?? 0)) {
      level = it.level
    }
  }
  if (level === 'warn' || level === 'error') {
    return level
  }
  if (items.length && items.every((it) => it.level === 'info' || zeroValue.test(it.value.trim()))) {
    return 'idle'
  }
  return 'ok'
}

// firstAttention 返回最需要关注的模块序号：先找错误，再找警告；都没有时为 -1
export function firstAttention(levels: SvcLevel[]): number {
  const e = levels.indexOf('error')
  return e >= 0 ? e : levels.indexOf('warn')
}
