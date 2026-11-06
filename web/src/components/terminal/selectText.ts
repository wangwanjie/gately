// 选择模式下把选区换算成文本的纯函数，不依赖 DOM，便于单元测试。

// RowPoint 是选区端点在叠层里的位置：第 r 行的第 o 个字符前。
export interface RowPoint {
  r: number
  o: number
}

// edgePoint 返回首行开头或末行结尾，用于端点落在文字行之外（顶部占位、叠层外的页面元素）时收拢到行范围内。
export function edgePoint(rows: string[], side: 'start' | 'end'): RowPoint {
  if (side === 'start' || !rows.length) {
    return { r: 0, o: 0 }
  }
  const r = rows.length - 1
  return { r, o: rows[r].length }
}

// joinRows 按终端逻辑行拼出 s 到 e 之间的文本：wrapped[r] 为 true 表示第 r 行是上一行的软换行续行，直接相连，不插入换行符。
// 端点超出行范围时收拢到边界；s 不在 e 之前（例如整个选区都在行外）时返回空串。
export function joinRows(rows: string[], wrapped: boolean[], s: RowPoint, e: RowPoint): string {
  if (!rows.length) {
    return ''
  }
  const clamp = (p: RowPoint): RowPoint => {
    if (p.r < 0) {
      return edgePoint(rows, 'start')
    }
    if (p.r >= rows.length) {
      return edgePoint(rows, 'end')
    }
    return { r: p.r, o: Math.max(0, Math.min(p.o, rows[p.r].length)) }
  }
  const a = clamp(s)
  const b = clamp(e)
  if (a.r > b.r || (a.r === b.r && a.o >= b.o)) {
    return ''
  }
  let out = ''
  for (let r = a.r; r <= b.r; r++) {
    out += rows[r].slice(r === a.r ? a.o : 0, r === b.r ? b.o : undefined)
    if (r < b.r && !wrapped[r + 1]) {
      out += '\n'
    }
  }
  // 选到末尾的空行时去掉多余的换行
  return out.replace(/\n+$/, '')
}
