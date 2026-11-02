// 复制条目时生成副本名称与草稿的纯函数，与界面无关，便于单元测试（tests/copyName.test.mjs）。

// CopyNameFormat 生成第 n 个副本的名称（n 从 1 开始）
export type CopyNameFormat = (name: string, n: number) => string

// asciiCopyName 用于只适合 ASCII 的名称（例如 frp 代理名，会出现在服务端日志与面板中）：name-copy、name-copy2
export const asciiCopyName: CopyNameFormat = (name, n) => (n <= 1 ? `${name}-copy` : `${name}-copy${n}`)

function escapeRegExp(s: string) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// copyBase 去掉名称末尾已有的副本后缀，复制「A 副本」得到「A 副本 2」而不是「A 副本 副本」。
// 后缀的样式从 fmt 的输出反推，换语言或换格式都适用
export function copyBase(name: string, fmt: CopyNameFormat): string {
  const mark = '\u0001'
  const num = '987654321'
  const patterns = [fmt(mark, 1), fmt(mark, Number(num))]
  for (const p of patterns) {
    const i = p.indexOf(mark)
    if (i < 0) {
      continue
    }
    const prefix = escapeRegExp(p.slice(0, i))
    const suffix = escapeRegExp(p.slice(i + mark.length)).replace(num, '\\d+')
    const m = new RegExp(`^${prefix}(.+?)${suffix}$`, 's').exec(name)
    if (m && m[1].trim()) {
      return m[1]
    }
  }
  return name
}

// copyName 生成与已有名称不重复的副本名称；模板没有名称时返回空串，交给用户填写
export function copyName(name: string | undefined, taken: Iterable<string | undefined>, fmt: CopyNameFormat): string {
  const raw = (name ?? '').trim()
  if (!raw) {
    return ''
  }
  const base = copyBase(raw, fmt)
  const used = new Set<string>()
  for (const s of taken) {
    if (s) {
      used.add(s.trim())
    }
  }
  // 名称数量有限，循环必然结束；上限只是防御
  for (let n = 1; n < 10000; n++) {
    const cand = fmt(base, n)
    if (!used.has(cand)) {
      return cand
    }
  }
  return fmt(base, 1)
}

// cloneItem 深拷贝条目作为副本草稿并去掉 key；条目都是可 JSON 序列化的配置
export function cloneItem<T extends object>(item: T): T {
  const draft = JSON.parse(JSON.stringify(item)) as T & { key?: string }
  draft.key = ''
  return draft
}
